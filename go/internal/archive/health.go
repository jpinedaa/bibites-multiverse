package archive

// The public production-health projection.
//
// This is deliberately narrower than the files it reads. monitor.sh holds alert
// messages, billing reconciliation and operational paths; service-host.jsonl
// names process IDs and cgroup internals. The dashboard needs none of those.
// Every string that can leave this handler is defined below, and every numeric
// field is copied by name or reduced to a utilization percentage. Raw host
// capacity is private inventory, not a dashboard result. An operator file must
// never become public merely because the archive can read it.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	monitorCadenceSeconds  = int64(5 * 60)
	monitorStaleAfter      = 11 * time.Minute
	hostCadenceSeconds     = int64(60)
	hostStaleAfter         = 3 * time.Minute
	hostHistorySamples     = 121 // two hours plus both endpoints at one-minute cadence
	hostHistoryReadMaxByte = int64(1 << 20)
)

type productionHealthView struct {
	GeneratedAt string            `json:"generatedAt"`
	Monitor     monitorHealthView `json:"monitor"`
	ServiceHost hostHealthView    `json:"serviceHost"`
}

type monitorHealthView struct {
	Available      bool               `json:"available"`
	AsOf           string             `json:"asOf,omitempty"`
	AgeSeconds     *int64             `json:"ageSeconds"`
	CadenceSeconds int64              `json:"cadenceSeconds"`
	Freshness      string             `json:"freshness"`
	Overall        string             `json:"overall"`
	Checks         []monitorCheckView `json:"checks"`
}

type monitorCheckView struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Group    string `json:"group"`
	Severity string `json:"severity"`
}

type monitorCheckDefinition struct {
	label string
	group string
}

// This is both presentation order and the disclosure boundary. A new monitor
// file is private until somebody deliberately adds a public name and group here.
var publicMonitorChecks = map[string]monitorCheckDefinition{
	"units":           {"Required services", "Availability"},
	"stream-origin":   {"Stream origin", "Availability"},
	"relay-healthz":   {"Relay probe", "Availability"},
	"archive-healthz": {"Archive probe", "Availability"},
	"subscribed":      {"Archive subscription", "Availability"},
	"status-age":      {"Map status freshness", "Availability"},
	"peers":           {"World presence", "Worlds and routing"},
	"lane-bypass":     {"Lane bypasses", "Worlds and routing"},
	"hosts-pin":       {"Internal relay route", "Worlds and routing"},
	"disk":            {"Archive disk", "Capacity"},
	"replay":          {"Archive memory headroom", "Capacity"},
	"replay-cost":     {"Next restart cost", "Capacity"},
	"swap":            {"Swap pressure", "Capacity"},
	"cold-copy":       {"Off-host ledger copy", "Record integrity"},
	"rollup":          {"Archive roll-up", "Record integrity"},
	"duplicates":      {"Duplicate guard", "Record integrity"},
	"gaps":            {"Genome gaps", "Record integrity"},
	"backup":          {"Backup", "Record integrity"},
	"transfer":        {"Monthly transfer", "Cost and transfer"},
	"transfer-rate":   {"Transfer rate", "Cost and transfer"},
	"billing":         {"Billing reconciliation", "Cost and transfer"},
	"errors":          {"Recent service errors", "Maintenance"},
	"cert":            {"TLS certificate", "Maintenance"},
	"reboot":          {"Pending host reboot", "Maintenance"},
}

type hostHealthView struct {
	Available      bool               `json:"available"`
	AsOf           string             `json:"asOf,omitempty"`
	AgeSeconds     *int64             `json:"ageSeconds"`
	CadenceSeconds int64              `json:"cadenceSeconds"`
	Freshness      string             `json:"freshness"`
	SampleCount    int                `json:"sampleCount"`
	WindowSeconds  int64              `json:"windowSeconds"`
	Latest         *hostLatestView    `json:"latest,omitempty"`
	History        []hostHistoryPoint `json:"history"`
}

type hostLoad struct {
	One     *float64 `json:"one"`
	Five    *float64 `json:"five"`
	Fifteen *float64 `json:"fifteen"`
}

type hostMemory struct {
	TotalBytes     *int64 `json:"totalBytes"`
	AvailableBytes *int64 `json:"availableBytes"`
	SwapTotalBytes *int64 `json:"swapTotalBytes"`
	SwapFreeBytes  *int64 `json:"swapFreeBytes"`
}

type publicHostMemory struct {
	AvailablePct *float64 `json:"availablePct"`
	SwapUsedPct  *float64 `json:"swapUsedPct"`
}

type hostDisk struct {
	DataFreeBytes *int64   `json:"dataFreeBytes"`
	DataUsedPct   *float64 `json:"dataUsedPct"`
}

type publicHostDisk struct {
	DataUsedPct *float64 `json:"dataUsedPct"`
}

type hostTCP struct {
	CurrEstab       *int64 `json:"currEstab"`
	ActiveOpens     *int64 `json:"activeOpens"`
	PassiveOpens    *int64 `json:"passiveOpens"`
	AttemptFails    *int64 `json:"attemptFails"`
	EstabResets     *int64 `json:"estabResets"`
	RetransSegs     *int64 `json:"retransSegs"`
	OutResets       *int64 `json:"outResets"`
	ListenOverflows *int64 `json:"listenOverflows"`
	ListenDrops     *int64 `json:"listenDrops"`
	SynRetrans      *int64 `json:"synRetrans"`
}

type hostConntrack struct {
	Count *int64 `json:"count"`
	Max   *int64 `json:"max"`
}

type publicHostConntrack struct {
	UsedPct *float64 `json:"usedPct"`
}

type rawHostUnit struct {
	Unit             string  `json:"unit"`
	ActiveState      *string `json:"activeState"`
	SubState         *string `json:"subState"`
	MemoryBytes      *int64  `json:"memoryBytes"`
	AnonBytes        *int64  `json:"anonBytes"`
	FileBytes        *int64  `json:"fileBytes"`
	CPUNsec          *int64  `json:"cpuNsec"`
	Restarts         *int64  `json:"restarts"`
	MainPID          *int64  `json:"mainPid"`
	MainRssAnonBytes *int64  `json:"mainRssAnonBytes"`
	MainVmHwmBytes   *int64  `json:"mainVmHwmBytes"`
}

type rawHostSample struct {
	At         string        `json:"at"`
	CPUBusyPct *float64      `json:"cpuBusyPct"`
	Load       hostLoad      `json:"load"`
	Memory     hostMemory    `json:"memory"`
	Disk       hostDisk      `json:"disk"`
	TCP        hostTCP       `json:"tcp"`
	Conntrack  hostConntrack `json:"conntrack"`
	Units      []rawHostUnit `json:"units"`
	parsedAt   time.Time
}

type hostUnitView struct {
	Name             string  `json:"name"`
	ActiveState      *string `json:"activeState"`
	SubState         *string `json:"subState"`
	MemoryBytes      *int64  `json:"memoryBytes"`
	AnonBytes        *int64  `json:"anonBytes"`
	FileBytes        *int64  `json:"fileBytes"`
	Restarts         *int64  `json:"restarts"`
	MainRssAnonBytes *int64  `json:"mainRssAnonBytes"`
	MainVmHwmBytes   *int64  `json:"mainVmHwmBytes"`
}

type hostLatestView struct {
	At         string              `json:"at"`
	CPUBusyPct *float64            `json:"cpuBusyPct"`
	Load       hostLoad            `json:"load"`
	Memory     publicHostMemory    `json:"memory"`
	Disk       publicHostDisk      `json:"disk"`
	TCP        hostTCP             `json:"tcp"`
	Conntrack  publicHostConntrack `json:"conntrack"`
	Units      []hostUnitView      `json:"units"`
}

type hostHistoryPoint struct {
	At                 string   `json:"at"`
	CPUBusyPct         *float64 `json:"cpuBusyPct"`
	MemoryAvailablePct *float64 `json:"memoryAvailablePct"`
	DiskUsedPct        *float64 `json:"diskUsedPct"`
	ArchiveAnonBytes   *int64   `json:"archiveAnonBytes"`
	EstabResets        *int64   `json:"estabResets"`
	AttemptFails       *int64   `json:"attemptFails"`
	RetransSegs        *int64   `json:"retransSegs"`
	OutResets          *int64   `json:"outResets"`
	ListenOverflows    *int64   `json:"listenOverflows"`
	ListenDrops        *int64   `json:"listenDrops"`
	SynRetrans         *int64   `json:"synRetrans"`
}

func (a *Archive) productionHealthView(now time.Time) productionHealthView {
	now = now.UTC()
	a.healthMu.Lock()
	defer a.healthMu.Unlock()
	if !a.healthAt.IsZero() && now.Sub(a.healthAt) >= 0 && now.Sub(a.healthAt) < 15*time.Second {
		view := a.healthCached
		refreshHealthAges(&view, now)
		return view
	}
	a.healthCached = productionHealthView{
		GeneratedAt: now.Format(time.RFC3339),
		Monitor:     readMonitorHealth(a.cfg.MonitorStateDir, now),
		ServiceHost: readHostHealth(a.cfg.HostMetricsFile, now),
	}
	a.healthAt = now
	return a.healthCached
}

func refreshHealthAges(view *productionHealthView, now time.Time) {
	view.GeneratedAt = now.UTC().Format(time.RFC3339)
	refreshAge(view.Monitor.Available, view.Monitor.AsOf, monitorStaleAfter,
		&view.Monitor.AgeSeconds, &view.Monitor.Freshness, now)
	refreshAge(view.ServiceHost.Available, view.ServiceHost.AsOf, hostStaleAfter,
		&view.ServiceHost.AgeSeconds, &view.ServiceHost.Freshness, now)
}

func refreshAge(available bool, asOf string, staleAfter time.Duration, ageSeconds **int64,
	freshnessValue *string, now time.Time) {
	if !available {
		return
	}
	at, err := time.Parse(time.RFC3339, asOf)
	if err != nil {
		*ageSeconds = nil
		*freshnessValue = "unknown"
		return
	}
	age := now.Sub(at)
	seconds := nonnegativeSeconds(age)
	*ageSeconds = &seconds
	*freshnessValue = freshness(age, staleAfter)
}

func unknownMonitorHealth() monitorHealthView {
	return monitorHealthView{
		CadenceSeconds: monitorCadenceSeconds,
		Freshness:      "unknown",
		Overall:        "unknown",
		Checks:         []monitorCheckView{},
	}
}

func readMonitorHealth(dir string, now time.Time) monitorHealthView {
	view := unknownMonitorHealth()
	if strings.TrimSpace(dir) == "" {
		return view
	}
	completedRaw, err := os.ReadFile(filepath.Join(dir, "completed-at"))
	if err != nil {
		return view
	}
	completed, err := strconv.ParseInt(strings.TrimSpace(string(completedRaw)), 10, 64)
	if err != nil || completed <= 0 {
		return view
	}
	asOf := time.Unix(completed, 0).UTC()
	age := nonnegativeSeconds(now.Sub(asOf))
	view.Available = true
	view.AsOf = asOf.Format(time.RFC3339)
	view.AgeSeconds = &age
	view.Freshness = freshness(now.Sub(asOf), monitorStaleAfter)

	activeRaw, _ := os.ReadFile(filepath.Join(dir, "checks"))
	unknown := false
	overall := "pass"
	seen := make(map[string]bool)
	for _, id := range strings.Fields(string(activeRaw)) {
		def, public := publicMonitorChecks[id]
		if !public || seen[id] {
			continue
		}
		seen[id] = true
		sevRaw, _ := os.ReadFile(filepath.Join(dir, "sev."+id))
		sev := publicSeverity(strings.TrimSpace(string(sevRaw)))
		view.Checks = append(view.Checks, monitorCheckView{
			ID: id, Label: def.label, Group: def.group, Severity: sev,
		})
		switch sev {
		case "critical":
			overall = "critical"
		case "warning":
			if overall != "critical" {
				overall = "warning"
			}
		case "unknown":
			unknown = true
		}
	}
	if len(view.Checks) == 0 || (unknown && overall != "critical") {
		overall = "unknown"
	}
	view.Overall = overall
	return view
}

func publicSeverity(severity string) string {
	switch severity {
	case "OK":
		return "pass"
	case "WARN":
		return "warning"
	case "CRIT":
		return "critical"
	default:
		return "unknown"
	}
}

func unknownHostHealth() hostHealthView {
	return hostHealthView{
		CadenceSeconds: hostCadenceSeconds,
		Freshness:      "unknown",
		History:        []hostHistoryPoint{},
	}
}

func readHostHealth(path string, now time.Time) hostHealthView {
	view := unknownHostHealth()
	if strings.TrimSpace(path) == "" {
		return view
	}
	samples := tailHostSamples(path)
	if len(samples) == 0 {
		return view
	}
	latest := samples[len(samples)-1]
	age := nonnegativeSeconds(now.Sub(latest.parsedAt))
	view.Available = true
	view.AsOf = latest.parsedAt.UTC().Format(time.RFC3339)
	view.AgeSeconds = &age
	view.Freshness = freshness(now.Sub(latest.parsedAt), hostStaleAfter)
	view.SampleCount = len(samples)
	if len(samples) > 1 {
		view.WindowSeconds = nonnegativeSeconds(latest.parsedAt.Sub(samples[0].parsedAt))
	}
	view.Latest = publicHostLatest(latest)
	view.History = make([]hostHistoryPoint, 0, len(samples))
	for _, sample := range samples {
		view.History = append(view.History, publicHostHistory(sample))
	}
	return view
}

func freshness(age time.Duration, staleAfter time.Duration) string {
	if age < -30*time.Second {
		return "unknown"
	}
	if age <= staleAfter {
		return "fresh"
	}
	return "stale"
}

func nonnegativeSeconds(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return int64(d / time.Second)
}

func tailHostSamples(path string) []rawHostSample {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() <= 0 {
		return nil
	}
	start := info.Size() - hostHistoryReadMaxByte
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	buf, err := io.ReadAll(io.LimitReader(f, hostHistoryReadMaxByte))
	if err != nil {
		return nil
	}
	if start > 0 {
		cut := bytes.IndexByte(buf, '\n')
		if cut < 0 {
			return nil
		}
		buf = buf[cut+1:]
	}
	lines := bytes.Split(buf, []byte{'\n'})
	capacity := len(lines)
	if capacity > hostHistorySamples {
		capacity = hostHistorySamples
	}
	samples := make([]rawHostSample, 0, capacity)
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sample rawHostSample
		if err := json.Unmarshal(line, &sample); err != nil {
			continue
		}
		at, err := time.Parse(time.RFC3339, sample.At)
		if err != nil {
			continue
		}
		sample.parsedAt = at.UTC()
		samples = append(samples, sample)
	}
	if len(samples) > hostHistorySamples {
		samples = samples[len(samples)-hostHistorySamples:]
	}
	return samples
}

var publicHostUnits = map[string]string{
	"multiverse-relay":   "Relay",
	"multiverse-archive": "Archive",
	"nginx":              "Front door",
	"multiverse-stream":  "Stream origin",
}

func publicHostLatest(sample rawHostSample) *hostLatestView {
	units := make([]hostUnitView, 0, len(sample.Units))
	for _, unit := range sample.Units {
		name, public := publicHostUnits[unit.Unit]
		if !public {
			continue
		}
		units = append(units, hostUnitView{
			Name: name, ActiveState: publicUnitActiveState(unit.ActiveState),
			SubState:    publicUnitSubState(unit.SubState),
			MemoryBytes: unit.MemoryBytes, AnonBytes: unit.AnonBytes,
			FileBytes: unit.FileBytes, Restarts: unit.Restarts,
			MainRssAnonBytes: unit.MainRssAnonBytes, MainVmHwmBytes: unit.MainVmHwmBytes,
		})
	}
	return &hostLatestView{
		At: sample.parsedAt.UTC().Format(time.RFC3339), CPUBusyPct: sample.CPUBusyPct,
		Load: sample.Load,
		Memory: publicHostMemory{
			AvailablePct: percentage(sample.Memory.AvailableBytes, sample.Memory.TotalBytes),
			SwapUsedPct:  usedPercentage(sample.Memory.SwapFreeBytes, sample.Memory.SwapTotalBytes),
		},
		Disk: publicHostDisk{DataUsedPct: sample.Disk.DataUsedPct}, TCP: sample.TCP,
		Conntrack: publicHostConntrack{UsedPct: percentage(sample.Conntrack.Count, sample.Conntrack.Max)},
		Units:     units,
	}
}

func percentage(part, total *int64) *float64 {
	if part == nil || total == nil || *total <= 0 || *part < 0 {
		return nil
	}
	value := float64(*part) * 100 / float64(*total)
	return &value
}

func usedPercentage(free, total *int64) *float64 {
	if free == nil || total == nil || *total <= 0 || *free < 0 {
		return nil
	}
	used := *total - *free
	if used < 0 {
		return nil
	}
	return percentage(&used, total)
}

func publicUnitActiveState(value *string) *string {
	return publicNamedState(value, "active", "reloading", "inactive", "failed", "activating", "deactivating", "maintenance")
}

func publicUnitSubState(value *string) *string {
	return publicNamedState(value, "running", "exited", "dead", "failed", "start", "stop", "auto-restart", "waiting", "listening")
}

func publicNamedState(value *string, allowed ...string) *string {
	if value == nil {
		return nil
	}
	for _, state := range allowed {
		if *value == state {
			out := state
			return &out
		}
	}
	return nil
}

func publicHostHistory(sample rawHostSample) hostHistoryPoint {
	var archiveAnon *int64
	for _, unit := range sample.Units {
		if unit.Unit == "multiverse-archive" {
			archiveAnon = unit.AnonBytes
			break
		}
	}
	return hostHistoryPoint{
		At: sample.parsedAt.UTC().Format(time.RFC3339), CPUBusyPct: sample.CPUBusyPct,
		MemoryAvailablePct: percentage(sample.Memory.AvailableBytes, sample.Memory.TotalBytes),
		DiskUsedPct:        sample.Disk.DataUsedPct, ArchiveAnonBytes: archiveAnon,
		EstabResets: sample.TCP.EstabResets, AttemptFails: sample.TCP.AttemptFails,
		RetransSegs: sample.TCP.RetransSegs, OutResets: sample.TCP.OutResets,
		ListenOverflows: sample.TCP.ListenOverflows, ListenDrops: sample.TCP.ListenDrops,
		SynRetrans: sample.TCP.SynRetrans,
	}
}
