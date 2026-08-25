package sidecar

import (
	"testing"
	"time"
)

func TestFixedAdmissionValidationIsCaseInsensitive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = " FIXED "
	cfg.InboundPopulationLimit = 0
	if err := validateAdmissionConfig(cfg); err == nil {
		t.Fatal("uppercase fixed mode with no limit passed validation")
	}
}

func TestAdaptiveAdmissionLearnsRobustCapacityAndShadowDoesNotRefuse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	cfg.AdmissionMinSamples = 5
	cfg.AdmissionSampleInterval = time.Minute
	cfg.AdmissionSampleWindow = time.Hour
	cfg.AdmissionSafetyMargin = 0.9
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	// Four ordinary samples have a machine budget of 450. One save-stall
	// outlier has a budget of 45; the median must still produce floor(40.5).
	for i, achieved := range []float64{10, 10, 1, 10, 10} {
		a.observe(now.Add(time.Duration(i)*time.Minute), 45, achieved, 10, false)
	}
	s := a.snapshot()
	if !s.Ready || s.EstimatedLimit != 40 || s.EffectiveLimit != 40 {
		t.Fatalf("snapshot = %+v, want a ready limit of 40", s)
	}
	if !a.admit(100, true) {
		t.Fatal("adaptive-shadow refused an arrival")
	}
	if got := a.snapshot(); got.Enforcing || got.RejectedTotal != 0 || got.Reason != "shadow_only" {
		t.Fatalf("shadow snapshot = %+v", got)
	}
}

func TestFixedAdmissionCountsToLimitAndReopensWithHysteresis(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionFixed
	cfg.InboundPopulationLimit = 50
	cfg.InboundPopulationHysteresis = 5
	a := newAdmissionController(cfg)
	if !a.admit(49, true) {
		t.Fatal("load below the fixed limit was refused")
	}
	if a.admit(50, true) {
		t.Fatal("load at the fixed limit was admitted")
	}
	if a.admit(48, true) {
		t.Fatal("closed gate reopened inside its hysteresis band")
	}
	if !a.admit(45, true) {
		t.Fatal("gate did not reopen at limit-hysteresis")
	}
	if got := a.snapshot().RejectedTotal; got != 2 {
		t.Fatalf("rejectedTotal = %d, want 2", got)
	}
}

func TestAdaptiveAdmissionIgnoresWorldConfiguredBelowTarget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptive
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	a.observe(time.Now(), 80, 5, 5, false)
	if a.snapshot().Ready {
		t.Fatal("a world configured for x5 trained an x10 controller")
	}
}

func TestAdaptiveAdmissionStateSurvivesShadowToEnforcingRestart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	cfg.AdmissionMinSamples = 3
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if !a.observe(now.Add(time.Duration(i)*time.Minute), 50, 10, 10, false) {
			t.Fatalf("sample %d was not recorded", i)
		}
	}
	disk := a.diskState(now.Add(3 * time.Minute))

	cfg.InboundAdmissionMode = AdmissionAdaptive
	restarted := newAdmissionController(cfg)
	if !restarted.restore(disk, now.Add(4*time.Minute)) {
		t.Fatal("matching shadow state was not restored into adaptive mode")
	}
	got := restarted.snapshot()
	if !got.Ready || !got.Enforcing || got.EffectiveLimit != 45 {
		t.Fatalf("restored controller = %+v, want ready enforcing limit 45", got)
	}
}

// The shipped default is the ENFORCING adaptive mode. A world that has learned
// a limit below its own population must shed new arrivals, not merely report
// that it would; that is what the gate is for, and leaving the participant
// package in shadow made "WOULD CLOSE" the permanent end state of every
// install. This test is the only thing standing between that decision and a
// silent revert, so it pins both halves: the mode, and the fail-open window a
// cold install still gets before the estimator is ready.
func TestDefaultAdmissionEnforcesOnceReady(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.InboundAdmissionMode != AdmissionAdaptive {
		t.Fatalf("default inbound admission = %q, want %q",
			cfg.InboundAdmissionMode, AdmissionAdaptive)
	}
	cfg.AdmissionMinSamples = 3
	a := newAdmissionController(cfg)

	// Nothing learned yet: the gate fails open rather than refusing on an
	// estimate it does not have.
	if !a.admit(10_000, true) {
		t.Fatal("the default refused an arrival before the estimator was ready")
	}
	if got := a.snapshot(); got.Enforcing || got.Reason != "learning" {
		t.Fatalf("cold snapshot = %+v, want non-enforcing and learning", got)
	}

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		a.observe(now.Add(time.Duration(i)*time.Minute), 50, 10, 10, false)
	}
	got := a.snapshot()
	if !got.Ready || !got.Enforcing || got.EffectiveLimit != 45 {
		t.Fatalf("learned snapshot = %+v, want ready enforcing limit 45", got)
	}
	if a.admit(45, true) {
		t.Fatal("the default admitted an arrival at its learned limit")
	}
	if got := a.snapshot(); !got.Closed || got.Reason != "population_limit" {
		t.Fatalf("closed snapshot = %+v, want closed on population_limit", got)
	}
}
