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
		a.observe(now.Add(time.Duration(i)*time.Minute), 45, achieved, false)
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

func TestAdaptiveAdmissionLearnsBelowTheReferenceTarget(t *testing.T) {
	// A world running at ×5 never reaches the ×10 reference, and it does not
	// have to: its budget of 80×5=400 prices a ×10 limit of floor(400×0.9/10).
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptive
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	if !a.observe(time.Now(), 80, 5, false) {
		t.Fatal("a world achieving x5 did not train the x10 controller")
	}
	got := a.snapshot()
	if !got.Ready || got.TargetTimeScale != 10 || got.EstimatedLimit != 36 ||
		got.EffectiveLimit != 36 {
		t.Fatalf("snapshot = %+v, want a ready x10-priced limit of 36", got)
	}
}

func TestPausedHeartbeatAddsNoSample(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptive
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if !a.observe(now, 50, 10, false) {
		t.Fatal("initial sample was not recorded")
	}
	if a.observe(now.Add(10*time.Minute), 50, 8, true) {
		t.Fatal("a paused heartbeat contributed a capacity sample")
	}
	if got := a.snapshot(); got.SampleCount != 1 || got.EffectiveLimit != 45 {
		t.Fatalf("snapshot after paused heartbeat = %+v, want one sample and limit 45", got)
	}
}

func TestAdmissionRestoreRescalesBudgetsAcrossTargetChange(t *testing.T) {
	// The deployed regression this pins: worlds that learned while sized for
	// ×100 held 60 budget samples and a limit clamped to the minimum. The same
	// samples restored under the ×10 default must reprice, not start over.
	recordedCfg := DefaultConfig()
	recordedCfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	recordedCfg.InboundTargetTimeScale = 100
	recordedCfg.AdmissionMinSamples = 1
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	original := newAdmissionController(recordedCfg)
	if !original.observe(now, 50, 10, false) {
		t.Fatal("the x100-sized sample was not recorded")
	}
	if got := original.snapshot().EffectiveLimit; got != 10 {
		t.Fatalf("x100-sized limit = %d, want the clamped minimum 10", got)
	}
	disk := original.diskState(now)

	restoredCfg := DefaultConfig()
	restoredCfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	restoredCfg.AdmissionMinSamples = 1
	restored := newAdmissionController(restoredCfg)
	if !restored.restore(disk, now.Add(time.Minute)) {
		t.Fatal("budget samples recorded while sized for x100 were rejected by the x10 default")
	}
	if got := restored.snapshot(); got.TargetTimeScale != 10 || got.SampleCount != 1 ||
		got.EffectiveLimit != 45 {
		t.Fatalf("restored controller = %+v, want the same sample repriced to limit 45", got)
	}

	marginCfg := DefaultConfig()
	marginCfg.AdmissionSafetyMargin = 0.5
	changed := newAdmissionController(marginCfg)
	if changed.restore(disk, now.Add(time.Minute)) {
		t.Fatal("state recorded under a different safety margin was restored")
	}
}

func TestAdaptiveAdmissionStateSurvivesShadowToEnforcingRestart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	cfg.AdmissionMinSamples = 3
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if !a.observe(now.Add(time.Duration(i)*time.Minute), 50, 10, false) {
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
