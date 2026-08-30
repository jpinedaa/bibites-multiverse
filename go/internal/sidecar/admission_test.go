package sidecar

import (
	"flag"
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
	cfg.InboundTargetAuto = false
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	a.observe(time.Now(), 80, 5, 5, false)
	if a.snapshot().Ready {
		t.Fatal("a world configured for x5 trained an x10 controller")
	}
}

func TestAdaptiveAdmissionFollowsRequestedTargetByDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	if !a.observe(now, 80, 5, 5, false) {
		t.Fatal("a world requesting x5 did not train the default controller")
	}
	got := a.snapshot()
	if !got.Ready || got.TargetTimeScale != 5 || got.EstimatedLimit != 72 ||
		got.EffectiveLimit != 72 {
		t.Fatalf("snapshot = %+v, want a ready x5 limit of 72", got)
	}
}

func TestAdaptiveAdmissionRecalculatesWhenRequestedTargetChanges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptive
	cfg.AdmissionMinSamples = 1
	a := newAdmissionController(cfg)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	if !a.observe(now, 50, 10, 10, false) {
		t.Fatal("initial x10 sample was not recorded")
	}
	if got := a.snapshot().EffectiveLimit; got != 45 {
		t.Fatalf("initial effective limit = %d, want 45", got)
	}
	// A paused heartbeat cannot contribute a capacity sample, but its requested
	// speed still changes the target and must not leave an enforcing x10 limit
	// attached to an x5 world.
	if !a.observe(now.Add(10*time.Second), 50, 0, 5, true) {
		t.Fatal("the requested-target change was not reported")
	}
	got := a.snapshot()
	if got.TargetTimeScale != 5 || got.SampleCount != 1 || got.EffectiveLimit != 90 {
		t.Fatalf("snapshot after target change = %+v, want one sample and x5 limit 90", got)
	}
}

func TestAdmissionTargetOverrideDetection(t *testing.T) {
	newFlags := func(t *testing.T, args ...string) *flag.FlagSet {
		t.Helper()
		fs := flag.NewFlagSet("admission-target-test", flag.ContinueOnError)
		fs.Float64("inbound-target-time-scale", defaultAdmissionTarget, "")
		if err := fs.Parse(args); err != nil {
			t.Fatalf("parse flags: %v", err)
		}
		return fs
	}

	t.Run("omitted follows requested speed", func(t *testing.T) {
		t.Setenv("MULTIVERSE_INBOUND_TARGET_TIME_SCALE", "")
		if inboundTargetWasExplicit(newFlags(t)) {
			t.Fatal("an omitted target override was classified as fixed")
		}
	})
	t.Run("flag fixes target", func(t *testing.T) {
		t.Setenv("MULTIVERSE_INBOUND_TARGET_TIME_SCALE", "")
		if !inboundTargetWasExplicit(newFlags(t, "--inbound-target-time-scale", "6.5")) {
			t.Fatal("an explicit target flag was classified as requested-speed following")
		}
	})
	t.Run("environment fixes target", func(t *testing.T) {
		t.Setenv("MULTIVERSE_INBOUND_TARGET_TIME_SCALE", "5")
		if !inboundTargetWasExplicit(newFlags(t)) {
			t.Fatal("an explicit target environment value was classified as requested-speed following")
		}
	})
	t.Run("invalid environment is omitted", func(t *testing.T) {
		t.Setenv("MULTIVERSE_INBOUND_TARGET_TIME_SCALE", "not-a-speed")
		if inboundTargetWasExplicit(newFlags(t)) {
			t.Fatal("an ignored target environment value was classified as fixed")
		}
	})
}

func TestAdmissionTargetRestorePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InboundAdmissionMode = AdmissionAdaptiveShadow
	cfg.AdmissionMinSamples = 1
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	original := newAdmissionController(cfg)
	original.observe(now, 80, 5, 5, false)
	disk := original.diskState(now)

	following := newAdmissionController(cfg)
	if !following.restore(disk, now.Add(time.Minute)) {
		t.Fatal("requested-speed mode rejected valid state recorded at the selected target")
	}
	if got := following.snapshot(); got.TargetTimeScale != 5 || got.EffectiveLimit != 72 {
		t.Fatalf("restored requested-speed controller = %+v, want x5 limit 72", got)
	}

	fixedCfg := cfg
	fixedCfg.InboundTargetAuto = false
	fixedCfg.InboundTargetTimeScale = 10
	fixed := newAdmissionController(fixedCfg)
	if fixed.restore(disk, now.Add(time.Minute)) {
		t.Fatal("fixed x10 policy restored state recorded for x5")
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
