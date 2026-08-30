package sidecar

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"multiverse/internal/fsutil"
	"multiverse/internal/journal"
)

// Population admission is decided by the receiving sidecar before it writes
// a new inbound journal entry. That placement matters: OVERLOADED is only a
// custody-safe reroute proof while the receiver has durably taken nothing.
const (
	AdmissionOff            = "off"
	AdmissionFixed          = "fixed"
	AdmissionAdaptiveShadow = "adaptive-shadow"
	AdmissionAdaptive       = "adaptive"

	defaultAdmissionTarget       = 10.0
	defaultAdmissionMin          = 10
	defaultAdmissionMax          = 200
	defaultAdmissionHysteresis   = 5
	defaultAdmissionSafetyMargin = 0.90
	defaultAdmissionMinSamples   = 10
	defaultAdmissionSampleEvery  = time.Minute
	defaultAdmissionWindow       = time.Hour
	admissionStateName           = "admission-state.json"
	admissionStateSchema         = "multiverse-admission/1"
)

type admissionSample struct {
	at     time.Time
	budget float64 // population * achieved time scale
}

type admissionDiskSample struct {
	AtMs   int64   `json:"atMs"`
	Budget float64 `json:"budget"`
}

type admissionDiskState struct {
	Schema        string                `json:"schema"`
	Target        float64               `json:"targetTimeScale"`
	Minimum       int                   `json:"minimumLimit"`
	Maximum       int                   `json:"maximumLimit"`
	Margin        float64               `json:"safetyMargin"`
	Samples       []admissionDiskSample `json:"samples"`
	Estimated     int                   `json:"estimatedLimit,omitempty"`
	Effective     int                   `json:"effectiveLimit,omitempty"`
	Ready         bool                  `json:"ready"`
	RejectedTotal int                   `json:"rejectedTotal"`
}

type admissionController struct {
	mode            string
	fixedLimit      int
	target          float64
	followRequested bool
	minLimit        int
	maxLimit        int
	hysteresis      int
	margin          float64
	minSamples      int
	every           time.Duration
	window          time.Duration

	samples   []admissionSample
	next      time.Time
	estimated int
	effective int
	ready     bool
	closed    bool
	committed int
	known     bool
	rejected  int
	reason    string
}

type admissionSnapshot struct {
	Mode            string  `json:"mode"`
	TargetTimeScale float64 `json:"targetTimeScale"`
	FixedLimit      int     `json:"fixedLimit,omitempty"`
	EstimatedLimit  int     `json:"estimatedLimit,omitempty"`
	EffectiveLimit  int     `json:"effectiveLimit,omitempty"`
	MinimumLimit    int     `json:"minimumLimit,omitempty"`
	MaximumLimit    int     `json:"maximumLimit,omitempty"`
	Hysteresis      int     `json:"hysteresis,omitempty"`
	SampleCount     int     `json:"sampleCount"`
	Ready           bool    `json:"ready"`
	Enforcing       bool    `json:"enforcing"`
	Closed          bool    `json:"closed"`
	PopulationKnown bool    `json:"populationKnown"`
	Committed       int     `json:"committedPopulation"`
	RejectedTotal   int     `json:"rejectedTotal"`
	Reason          string  `json:"reason,omitempty"`
}

func validAdmissionMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AdmissionOff, AdmissionFixed, AdmissionAdaptiveShadow, AdmissionAdaptive:
		return true
	default:
		return false
	}
}

func validateAdmissionConfig(cfg Config) error {
	mode := strings.ToLower(strings.TrimSpace(cfg.InboundAdmissionMode))
	if !validAdmissionMode(mode) {
		return fmt.Errorf("sidecar: inbound admission mode %q is invalid; use off, fixed, adaptive-shadow, or adaptive",
			cfg.InboundAdmissionMode)
	}
	if mode == AdmissionFixed && cfg.InboundPopulationLimit <= 0 {
		return fmt.Errorf("sidecar: fixed inbound admission needs a positive population limit")
	}
	if cfg.InboundTargetTimeScale <= 0 || math.IsNaN(cfg.InboundTargetTimeScale) ||
		math.IsInf(cfg.InboundTargetTimeScale, 0) {
		return fmt.Errorf("sidecar: inbound target time scale must be positive and finite")
	}
	if cfg.InboundPopulationMin <= 0 || cfg.InboundPopulationMax < cfg.InboundPopulationMin {
		return fmt.Errorf("sidecar: inbound population range must be positive and max >= min")
	}
	if cfg.AdmissionSafetyMargin <= 0 || cfg.AdmissionSafetyMargin > 1 {
		return fmt.Errorf("sidecar: admission safety margin must be in (0,1]")
	}
	return nil
}

func newAdmissionController(cfg Config) admissionController {
	return admissionController{
		mode:            strings.ToLower(strings.TrimSpace(cfg.InboundAdmissionMode)),
		fixedLimit:      cfg.InboundPopulationLimit,
		target:          cfg.InboundTargetTimeScale,
		followRequested: cfg.InboundTargetAuto,
		minLimit:        cfg.InboundPopulationMin,
		maxLimit:        cfg.InboundPopulationMax,
		hysteresis:      cfg.InboundPopulationHysteresis,
		margin:          cfg.AdmissionSafetyMargin,
		minSamples:      cfg.AdmissionMinSamples,
		every:           cfg.AdmissionSampleInterval,
		window:          cfg.AdmissionSampleWindow,
		reason:          "waiting_for_population",
	}
}

// observe adds at most one independent capacity sample per interval. The
// estimator uses the median of population*achievedTimeScale: the product is an
// empirical CPU budget, and dividing it by the desired scale gives the
// population this machine has historically supported at that speed. A median
// rejects isolated save stalls; the safety margin keeps the result off the
// cliff where the game already failed to hold the target.
func (a *admissionController) observe(now time.Time, population int, achieved,
	requestedTarget float64, paused bool) bool {
	a.trim(now)
	changed := a.followRequestedTarget(requestedTarget)
	if a.mode == AdmissionOff || paused || population <= 0 || achieved <= 0 ||
		requestedTarget <= 0 || !wireFinite(requestedTarget) {
		return changed
	}
	if !a.followRequested && requestedTarget+0.01 < a.target {
		return changed
	}
	if !a.next.IsZero() && now.Before(a.next) {
		return changed
	}
	a.next = now.Add(a.every)
	a.samples = append(a.samples, admissionSample{at: now, budget: float64(population) * achieved})
	a.trim(now)
	a.recalculate(false)
	return true
}

// followRequestedTarget makes the ordinary participant default follow the
// speed selected in the game. An explicit operator target leaves this disabled
// and retains the fixed-target sampling gate above.
func (a *admissionController) followRequestedTarget(requested float64) bool {
	if !a.followRequested || requested <= 0 || !wireFinite(requested) ||
		math.Abs(requested-a.target) <= 0.01 {
		return false
	}
	a.target = requested
	// Existing samples are machine budgets, not target-specific limits. Reapply
	// them immediately at the new requested speed so an enforcing controller
	// never carries a stale limit across a slider change.
	a.recalculate(true)
	return true
}

func (a *admissionController) recalculate(resetEffective bool) {
	if len(a.samples) < a.minSamples {
		return
	}
	budgets := make([]float64, 0, len(a.samples))
	for _, sample := range a.samples {
		budgets = append(budgets, sample.budget)
	}
	sort.Float64s(budgets)
	median := budgets[len(budgets)/2]
	if len(budgets)%2 == 0 {
		median = (budgets[len(budgets)/2-1] + median) / 2
	}
	estimate := int(math.Floor(median * a.margin / a.target))
	estimate = clampInt(estimate, a.minLimit, a.maxLimit)
	a.estimated = estimate
	if !a.ready || resetEffective {
		a.ready = true
		a.effective = estimate
		return
	}
	// A slowly moving control value avoids a single changing hour of ecology
	// opening and closing the gate by dozens of organisms at once. Downward
	// movement is deliberately twice as fast as upward movement.
	if estimate < a.effective {
		step := maxInt(1, int(math.Ceil(float64(a.effective)*0.10)))
		a.effective = maxInt(estimate, a.effective-step)
	} else if estimate > a.effective {
		step := maxInt(1, int(math.Ceil(float64(a.effective)*0.05)))
		a.effective = minInt(estimate, a.effective+step)
	}
}

func (a *admissionController) trim(now time.Time) {
	cut := now.Add(-a.window)
	i := 0
	for i < len(a.samples) && a.samples[i].at.Before(cut) {
		i++
	}
	if i > 0 {
		a.samples = append(a.samples[:0], a.samples[i:]...)
	}
	if len(a.samples) < a.minSamples && a.mode != AdmissionFixed {
		a.ready = false
	}
}

func (a *admissionController) limit() int {
	if a.mode == AdmissionFixed {
		return a.fixedLimit
	}
	if a.ready {
		return a.effective
	}
	return 0
}

func (a *admissionController) enforcing() bool {
	return a.mode == AdmissionFixed || (a.mode == AdmissionAdaptive && a.ready)
}

// admit updates hysteresis from the current committed load and answers whether
// one new network arrival may take custody. Shadow mode runs the exact same
// estimator and publishes the exact same limit, but never refuses.
func (a *admissionController) admit(committed int, known bool) bool {
	allowed := a.refresh(committed, known)
	if !allowed {
		a.rejected++
	}
	return allowed
}

func (a *admissionController) refresh(committed int, known bool) bool {
	a.committed, a.known = committed, known
	if !known {
		a.closed = false
		a.reason = "population_unknown"
		return true
	}
	limit := a.limit()
	if limit <= 0 {
		a.closed = false
		if a.mode == AdmissionOff {
			a.reason = "disabled"
		} else {
			a.reason = "learning"
		}
		return true
	}
	if a.closed {
		if committed <= maxInt(0, limit-a.hysteresis) {
			a.closed = false
		}
	} else if committed >= limit {
		a.closed = true
	}
	if a.mode == AdmissionAdaptiveShadow {
		a.reason = "shadow_only"
		return true
	}
	if !a.enforcing() {
		a.reason = "learning"
		return true
	}
	if a.closed {
		a.reason = "population_limit"
		return false
	}
	a.reason = "open"
	return true
}

func (a *admissionController) snapshot() admissionSnapshot {
	return admissionSnapshot{
		Mode:            a.mode,
		TargetTimeScale: a.target,
		FixedLimit:      a.fixedLimit,
		EstimatedLimit:  a.estimated,
		EffectiveLimit:  a.limit(),
		MinimumLimit:    a.minLimit,
		MaximumLimit:    a.maxLimit,
		Hysteresis:      a.hysteresis,
		SampleCount:     len(a.samples),
		Ready:           a.ready || a.mode == AdmissionFixed,
		Enforcing:       a.enforcing(),
		Closed:          a.closed,
		PopulationKnown: a.known,
		Committed:       a.committed,
		RejectedTotal:   a.rejected,
		Reason:          a.reason,
	}
}

func (s *Sidecar) committedPopulationLocked() (int, bool) {
	if s.mod == nil || !s.mod.handshaked || !s.mod.havePopulation {
		return 0, false
	}
	return s.mod.population + s.jr.CountPending(journal.In) + s.admittedSinceHeartbeat, true
}

func (a *admissionController) diskState(now time.Time) admissionDiskState {
	a.trim(now)
	d := admissionDiskState{
		Schema: admissionStateSchema, Target: a.target, Minimum: a.minLimit,
		Maximum: a.maxLimit, Margin: a.margin, Estimated: a.estimated,
		Effective: a.effective, Ready: a.ready, RejectedTotal: a.rejected,
		Samples: make([]admissionDiskSample, 0, len(a.samples)),
	}
	for _, sample := range a.samples {
		d.Samples = append(d.Samples, admissionDiskSample{
			AtMs: sample.at.UnixMilli(), Budget: sample.budget})
	}
	return d
}

func (a *admissionController) restore(d admissionDiskState, now time.Time) bool {
	if d.Schema != admissionStateSchema || d.Minimum != a.minLimit ||
		d.Maximum != a.maxLimit || d.Margin != a.margin {
		return false
	}
	if a.followRequested {
		if d.Target <= 0 || !wireFinite(d.Target) {
			return false
		}
		a.target = d.Target
	} else if d.Target != a.target {
		return false
	}
	a.samples = nil
	for _, sample := range d.Samples {
		at := time.UnixMilli(sample.AtMs)
		if sample.Budget > 0 && wireFinite(sample.Budget) && !at.After(now) {
			a.samples = append(a.samples, admissionSample{at: at, budget: sample.Budget})
		}
	}
	sort.Slice(a.samples, func(i, j int) bool { return a.samples[i].at.Before(a.samples[j].at) })
	a.estimated, a.effective = d.Estimated, d.Effective
	a.ready, a.rejected = d.Ready, maxInt(0, d.RejectedTotal)
	a.trim(now)
	if len(a.samples) > 0 {
		a.next = a.samples[len(a.samples)-1].at.Add(a.every)
	}
	return true
}

func wireFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (s *Sidecar) loadAdmissionState() {
	path := filepath.Join(s.cfg.DataDir, admissionStateName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.log.Warn("sidecar: adaptive admission state could not be read; learning starts fresh",
			"file", path, "err", err)
		return
	}
	var d admissionDiskState
	if err := json.Unmarshal(b, &d); err != nil {
		s.log.Warn("sidecar: adaptive admission state is malformed; learning starts fresh",
			"file", path, "err", err)
		return
	}
	if s.admission.restore(d, time.Now()) {
		s.log.Info("sidecar: restored adaptive population admission state",
			"samples", len(s.admission.samples), "estimatedLimit", s.admission.estimated,
			"effectiveLimit", s.admission.effective, "ready", s.admission.ready)
	} else {
		s.log.Info("sidecar: adaptive admission settings changed; learning starts fresh")
	}
}

func (s *Sidecar) saveAdmissionState() {
	// Contract A heartbeats and Contract B refusals arrive on different
	// goroutines. Serialize their scratch-file transaction without holding the
	// main sidecar lock across disk I/O.
	s.admissionSaveMu.Lock()
	defer s.admissionSaveMu.Unlock()
	s.mu.Lock()
	d := s.admission.diskState(time.Now())
	s.mu.Unlock()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		s.log.Warn("sidecar: adaptive admission state could not be encoded", "err", err)
		return
	}
	path := filepath.Join(s.cfg.DataDir, admissionStateName)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	if f != nil {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err == nil {
		err = fsutil.SyncDir(s.cfg.DataDir)
	}
	if err != nil {
		_ = os.Remove(tmp)
		s.log.Warn("sidecar: adaptive admission state could not be persisted", "file", path, "err", err)
	}
}

func clampInt(v, low, high int) int { return minInt(maxInt(v, low), high) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
