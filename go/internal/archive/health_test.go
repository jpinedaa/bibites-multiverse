package archive

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeHealthFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

func ptr[T any](value T) *T { return &value }

func TestMonitorHealthPublishesOnlyNamedVerdicts(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.UTC)
	writeHealthFixture(t, filepath.Join(dir, "completed-at"), fmt.Sprint(now.Add(-2*time.Minute).Unix()))
	writeHealthFixture(t, filepath.Join(dir, "checks"), "units billing operator-private\n")
	writeHealthFixture(t, filepath.Join(dir, "sev.units"), "OK")
	writeHealthFixture(t, filepath.Join(dir, "sev.billing"), "WARN")
	writeHealthFixture(t, filepath.Join(dir, "sev.operator-private"), "CRIT")
	writeHealthFixture(t, filepath.Join(dir, "message.billing"), "private cost and a host path")

	view := readMonitorHealth(dir, now)
	if !view.Available || view.Freshness != "fresh" || view.Overall != "warning" {
		t.Fatalf("monitor summary = %+v, want available fresh warning", view)
	}
	if view.AgeSeconds == nil || *view.AgeSeconds != 120 {
		t.Fatalf("ageSeconds = %v, want 120", view.AgeSeconds)
	}
	if len(view.Checks) != 2 {
		t.Fatalf("published %d checks, want the two allow-listed checks", len(view.Checks))
	}
	if view.Checks[0].Label != "Required services" || view.Checks[0].Severity != "pass" {
		t.Fatalf("first check = %+v", view.Checks[0])
	}
	if view.Checks[1].Label != "Billing reconciliation" || view.Checks[1].Severity != "warning" {
		t.Fatalf("second check = %+v", view.Checks[1])
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"operator-private", "private cost", dir, "message.billing"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public monitor JSON disclosed %q: %s", forbidden, body)
		}
	}
}

func TestMonitorHealthTreatsMissingAndStaleAsUnknown(t *testing.T) {
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.UTC)
	if view := readMonitorHealth("", now); view.Available || view.Freshness != "unknown" || view.Overall != "unknown" {
		t.Fatalf("unconfigured monitor = %+v", view)
	}
	dir := t.TempDir()
	writeHealthFixture(t, filepath.Join(dir, "completed-at"), fmt.Sprint(now.Add(-12*time.Minute).Unix()))
	writeHealthFixture(t, filepath.Join(dir, "checks"), "units")
	writeHealthFixture(t, filepath.Join(dir, "sev.units"), "OK")
	view := readMonitorHealth(dir, now)
	if !view.Available || view.Freshness != "stale" || view.Overall != "pass" {
		t.Fatalf("stale monitor = %+v; the last result stays visible but freshness must be stale", view)
	}
	writeHealthFixture(t, filepath.Join(dir, "completed-at"), fmt.Sprint(now.Add(time.Minute).Unix()))
	if future := readMonitorHealth(dir, now); future.Freshness != "unknown" {
		t.Fatalf("a completion marker one minute in the future is %q, want unknown", future.Freshness)
	}
}

func hostFixture(at string, cpu, available, resets int, extraUnit string) string {
	return fmt.Sprintf(`{"at":%q,"cpuBusyPct":%d,"load":{"one":0.5,"five":0.4,"fifteen":0.3},"memory":{"totalBytes":1000,"availableBytes":%d,"swapTotalBytes":100,"swapFreeBytes":90},"disk":{"dataFreeBytes":5000,"dataUsedPct":42},"tcp":{"currEstab":7,"activeOpens":1,"passiveOpens":2,"attemptFails":3,"estabResets":%d,"retransSegs":20,"outResets":4,"listenOverflows":0,"listenDrops":0,"synRetrans":2},"conntrack":{"count":9,"max":100},"units":[{"unit":"multiverse-archive","activeState":"active","subState":"running","memoryBytes":800,"anonBytes":600,"fileBytes":100,"cpuNsec":123,"restarts":1,"mainPid":9876,"mainRssAnonBytes":590,"mainVmHwmBytes":700}%s]}`,
		at, cpu, available, resets, extraUnit)
}

func TestHostHealthReturnsARecentSanitizedWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service-host.jsonl")
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.UTC)
	first := hostFixture(now.Add(-time.Minute).Format(time.RFC3339), 20, 500, 10, `,{"unit":"secret-customer-daemon","activeState":"failed","mainPid":1234}`)
	last := hostFixture(now.Add(-10*time.Second).Format(time.RFC3339), 35, 400, 12,
		`,{"unit":"multiverse-relay","activeState":"private-state","subState":"secret-sub","mainPid":4321}`)
	writeHealthFixture(t, path, "not json\n"+first+"\n"+last+"\n")

	view := readHostHealth(path, now)
	if !view.Available || view.Freshness != "fresh" || view.SampleCount != 2 || view.WindowSeconds != 50 {
		t.Fatalf("host summary = %+v", view)
	}
	if view.Latest == nil || view.Latest.CPUBusyPct == nil || *view.Latest.CPUBusyPct != 35 {
		t.Fatalf("latest host sample = %+v", view.Latest)
	}
	if len(view.Latest.Units) != 2 || view.Latest.Units[0].Name != "Archive" ||
		view.Latest.Units[1].Name != "Relay" || view.Latest.Units[1].ActiveState != nil ||
		view.Latest.Units[1].SubState != nil {
		t.Fatalf("public units = %+v, want named units with unknown arbitrary states", view.Latest.Units)
	}
	if view.History[0].ArchiveAnonBytes == nil || *view.History[0].ArchiveAnonBytes != 600 {
		t.Fatalf("archive anonymous series = %+v", view.History)
	}
	if view.Latest.Memory.AvailablePct == nil || *view.Latest.Memory.AvailablePct != 40 ||
		view.Latest.Memory.SwapUsedPct == nil || *view.Latest.Memory.SwapUsedPct != 10 ||
		view.Latest.Conntrack.UsedPct == nil || *view.Latest.Conntrack.UsedPct != 9 ||
		view.History[0].MemoryAvailablePct == nil || *view.History[0].MemoryAvailablePct != 50 {
		t.Fatalf("public utilization percentages = latest %+v history %+v", view.Latest, view.History[0])
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-customer-daemon", "private-state", "secret-sub", "mainPid", "9876", "4321", path, `"totalBytes"`, `"dataFreeBytes"`, `"max"`} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public host JSON disclosed %q: %s", forbidden, body)
		}
	}
}

func TestHostHealthProjectsPressureTrafficAndArchiveRates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service-host.jsonl")
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	first := fmt.Sprintf(`{"at":%q,"pressure":{"cpu":{"someUsec":1000000,"fullUsec":0},"memory":{"someUsec":0,"fullUsec":2000000},"io":{"someUsec":0,"fullUsec":3000000}},"vm":{"swapInPages":10,"swapOutPages":20,"oomKills":1},"archive":{"ledgerRecords":1000}}`, now.Add(-70*time.Second).Format(time.RFC3339))
	lastAt := now.Add(-10 * time.Second)
	trafficAt := lastAt.Add(-5 * time.Second)
	last := fmt.Sprintf(`{"at":%q,"pressure":{"cpu":{"someUsec":7000000,"fullUsec":0},"memory":{"someUsec":0,"fullUsec":3500000},"io":{"someUsec":0,"fullUsec":6000000}},"vm":{"swapInPages":13,"swapOutPages":24,"oomKills":2},"traffic":{"asOf":%q,"available":true,"complete":true,"windowSeconds":60,"requests":120,"status1xx":0,"status2xx":100,"status3xx":0,"status4xx":14,"status5xx":6,"p50Ms":12.5,"p95Ms":48.25,"clientAddress":"198.51.100.4","routes":[{"id":"pages","requests":60,"status5xx":0,"p50Ms":8,"p95Ms":20},{"id":"api","requests":30,"status5xx":3,"p50Ms":20,"p95Ms":60},{"id":"private-customer-route","requests":30,"status5xx":3,"p50Ms":90,"p95Ms":120}]},"archive":{"ledgerRecords":1120}}`, lastAt.Format(time.RFC3339), trafficAt.Format(time.RFC3339))
	writeHealthFixture(t, path, first+"\n"+last+"\n")

	view := readHostHealth(path, now)
	if view.Latest == nil {
		t.Fatal("latest host view is nil")
	}
	pressure := view.Latest.Pressure
	if pressure.WindowSeconds == nil || *pressure.WindowSeconds != 60 ||
		pressure.CPUSomePct == nil || *pressure.CPUSomePct != 10 ||
		pressure.MemoryFullPct == nil || *pressure.MemoryFullPct != 2.5 ||
		pressure.IOFullPct == nil || *pressure.IOFullPct != 5 {
		t.Fatalf("pressure projection = %+v", pressure)
	}
	vm := view.Latest.VM
	if vm.SwapInPages == nil || *vm.SwapInPages != 3 || vm.SwapOutPages == nil || *vm.SwapOutPages != 4 || vm.OOMKills == nil || *vm.OOMKills != 1 {
		t.Fatalf("VM event projection = %+v", vm)
	}
	traffic := view.Latest.Traffic
	if !traffic.Available || !traffic.Complete || traffic.RequestsPerSec == nil || *traffic.RequestsPerSec != 2 ||
		traffic.P50Ms == nil || *traffic.P50Ms != 12.5 || traffic.P95Ms == nil || *traffic.P95Ms != 48.25 ||
		traffic.Status1xxPct == nil || *traffic.Status1xxPct != 0 ||
		traffic.Status2xxPct == nil || *traffic.Status2xxPct != 100.0*100/120 ||
		traffic.Status3xxPct == nil || *traffic.Status3xxPct != 0 ||
		traffic.Status4xxPct == nil || *traffic.Status4xxPct != 14.0*100/120 ||
		traffic.ServerErrorPct == nil || *traffic.ServerErrorPct != 5 || len(traffic.Routes) != len(publicTrafficRoutes) {
		t.Fatalf("traffic projection = %+v", traffic)
	}
	if traffic.Routes[0].ID != "pages" || traffic.Routes[0].RequestsPerSec == nil || *traffic.Routes[0].RequestsPerSec != 1 ||
		traffic.Routes[1].ID != "api" || traffic.Routes[1].ServerErrorPct == nil || *traffic.Routes[1].ServerErrorPct != 10 ||
		traffic.Routes[2].RequestsPerSec != nil {
		t.Fatalf("route projection = %+v", traffic.Routes)
	}
	history := view.History
	if len(history) != 2 || history[0].ArchiveRecordsPerSec != nil || history[1].ArchiveRecordsPerSec == nil || *history[1].ArchiveRecordsPerSec != 2 ||
		history[1].CPUPressureSomePct == nil || *history[1].CPUPressureSomePct != 10 || history[1].OOMKills == nil || *history[1].OOMKills != 1 {
		t.Fatalf("derived history = %+v", history)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-customer-route", "198.51.100.4", "clientAddress", "someUsec", "fullUsec", trafficAt.Format(time.RFC3339)} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public health JSON disclosed %q: %s", forbidden, body)
		}
	}
}

func TestTrafficFreshnessAppliesToEveryHistoryPoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service-host.jsonl")
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	start := now.Add(-4 * time.Minute)
	trafficSample := func(at time.Time, asOfField string) string {
		return fmt.Sprintf(`{"at":%q,"traffic":{%s"available":true,"complete":true,"windowSeconds":60,"requests":120,"status1xx":0,"status2xx":120,"status3xx":0,"status4xx":0,"status5xx":0,"p50Ms":12.5,"p95Ms":48.25,"routes":[{"id":"pages","requests":120,"status5xx":0,"p50Ms":12.5,"p95Ms":48.25}]}}`,
			at.Format(time.RFC3339), asOfField)
	}
	asOf := func(at time.Time) string {
		return fmt.Sprintf(`"asOf":%q,`, at.Format(time.RFC3339))
	}
	lines := []string{
		trafficSample(start, asOf(start)),
		trafficSample(start.Add(time.Minute), asOf(start)), // the producer clock froze
		trafficSample(start.Add(2*time.Minute), ""),
		trafficSample(start.Add(3*time.Minute), `"asOf":"not-a-time",`),
		trafficSample(now, asOf(now.Add(31*time.Second))),
	}
	writeHealthFixture(t, path, strings.Join(lines, "\n")+"\n")

	view := readHostHealth(path, now)
	if view.SampleCount != len(lines) || len(view.History) != len(lines) {
		t.Fatalf("traffic freshness fixture returned %d/%d points, want %d", view.SampleCount, len(view.History), len(lines))
	}
	if view.History[0].RequestsPerSec == nil || *view.History[0].RequestsPerSec != 2 ||
		view.History[0].HTTPP50Ms == nil || view.History[0].HTTPStatus2xxPct == nil {
		t.Fatalf("fresh traffic point lost its public metrics: %+v", view.History[0])
	}
	hasTrafficMetrics := func(point hostHistoryPoint) bool {
		return point.RequestsPerSec != nil || point.HTTPP50Ms != nil || point.HTTPP95Ms != nil ||
			point.HTTPStatus1xxPct != nil || point.HTTPStatus2xxPct != nil ||
			point.HTTPStatus3xxPct != nil || point.HTTPStatus4xxPct != nil ||
			point.HTTPServerErrorPct != nil
	}
	for i, reason := range []string{"frozen", "missing", "malformed", "future"} {
		if point := view.History[i+1]; hasTrafficMetrics(point) {
			t.Errorf("%s traffic timestamp produced history metrics: %+v", reason, point)
		}
	}
	if traffic := view.Latest.Traffic; !traffic.Available || traffic.Complete ||
		traffic.RequestsPerSec != nil || traffic.P50Ms != nil || traffic.P95Ms != nil ||
		traffic.Status1xxPct != nil || traffic.Status2xxPct != nil ||
		traffic.Status3xxPct != nil || traffic.Status4xxPct != nil ||
		traffic.ServerErrorPct != nil || len(traffic.Routes) != 0 {
		t.Fatalf("future traffic timestamp produced latest metrics: %+v", traffic)
	}
}

func TestTrafficFreshnessRequiresCanonicalTimestampAndThirtySecondTolerance(t *testing.T) {
	regularSampleAt := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	leapDaySampleAt := time.Date(2024, 2, 29, 16, 0, 0, 0, time.UTC)
	window := int64(60)
	requests := int64(60)
	p50 := 12.5
	for _, tc := range []struct {
		name     string
		sampleAt time.Time
		asOf     string
		complete bool
	}{
		{"past boundary", regularSampleAt, regularSampleAt.Add(-30 * time.Second).Format(time.RFC3339), true},
		{"future boundary", regularSampleAt, regularSampleAt.Add(30 * time.Second).Format(time.RFC3339), true},
		{"frozen outside", regularSampleAt, regularSampleAt.Add(-31 * time.Second).Format(time.RFC3339), false},
		{"future outside", regularSampleAt, regularSampleAt.Add(31 * time.Second).Format(time.RFC3339), false},
		{"missing", regularSampleAt, "", false},
		{"malformed", regularSampleAt, "not-a-time", false},
		{"leap second", regularSampleAt, "2026-08-24T15:59:60Z", false},
		{"February 30", time.Date(2026, 3, 2, 16, 0, 0, 0, time.UTC), "2026-02-30T16:00:00Z", false},
		{"non-leap February 29", time.Date(2026, 3, 1, 16, 0, 0, 0, time.UTC), "2026-02-29T16:00:00Z", false},
		{"short fields", regularSampleAt, "2026-8-24T16:00:00Z", false},
		{"offset", regularSampleAt, "2026-08-24T12:00:00-04:00", false},
		{"fraction", regularSampleAt, "2026-08-24T16:00:00.000Z", false},
		{"valid leap day", leapDaySampleAt, "2024-02-29T16:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := publicTraffic(rawTrafficView{
				AsOf: tc.asOf, Available: true, Complete: true,
				WindowSeconds: &window, Requests: &requests, Status2xx: &requests, P50Ms: &p50,
				Routes: []rawTrafficRoute{{ID: "pages", Requests: &requests, P50Ms: &p50}},
			}, tc.sampleAt)
			if view.Complete != tc.complete || (view.RequestsPerSec != nil) != tc.complete ||
				(view.P50Ms != nil) != tc.complete || (view.Status2xxPct != nil) != tc.complete ||
				(len(view.Routes) != 0) != tc.complete {
				t.Fatalf("producer time %q produced %+v, want complete=%t", tc.asOf, view, tc.complete)
			}
		})
	}
}

func TestHostHealthKeepsSampleWithNullMalformedTrafficIntegers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service-host.jsonl")
	sampleAt := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	sample := fmt.Sprintf(`{"at":%q,"traffic":{"asOf":%q,"available":true,"complete":true,"windowSeconds":null,"requests":null,"status1xx":null,"status2xx":null,"status3xx":9007199254740991,"status4xx":null,"status5xx":null,"p50Ms":null,"p95Ms":4.5,"routes":[{"id":"pages","requests":null,"status5xx":0,"p95Ms":2.5}]}}`,
		sampleAt.Format(time.RFC3339), sampleAt.Format(time.RFC3339))
	writeHealthFixture(t, path, sample+"\n")

	view := readHostHealth(path, sampleAt)
	if !view.Available || view.SampleCount != 1 || len(view.History) != 1 || view.Latest == nil {
		t.Fatalf("normalized traffic integers dropped the host sample: %+v", view)
	}
	traffic := view.Latest.Traffic
	if traffic.Complete || traffic.RequestsPerSec != nil || traffic.P50Ms != nil ||
		traffic.P95Ms != nil || traffic.Status3xxPct != nil || len(traffic.Routes) != 0 ||
		view.History[0].RequestsPerSec != nil || view.History[0].HTTPP95Ms != nil ||
		view.History[0].HTTPStatus3xxPct != nil {
		t.Fatalf("normalized malformed traffic produced public metrics: latest=%+v history=%+v", traffic, view.History[0])
	}
}

func TestDerivedHealthMetricsRejectResetsAndInvalidWindows(t *testing.T) {
	previous := rawHostSample{
		parsedAt: time.Unix(100, 0),
		Pressure: rawHostPressure{CPU: rawPressureResource{SomeUsec: ptr(int64(5_000_000))}},
		VM:       rawHostVM{OOMKills: ptr(int64(4))},
		Archive:  rawArchiveSample{LedgerRecords: ptr(int64(500))},
	}
	current := rawHostSample{
		parsedAt: time.Unix(160, 0),
		Pressure: rawHostPressure{CPU: rawPressureResource{SomeUsec: ptr(int64(2_000_000))}},
		VM:       rawHostVM{OOMKills: ptr(int64(3))},
		Archive:  rawArchiveSample{LedgerRecords: ptr(int64(400))},
	}
	history := publicHostHistory(current, &previous)
	if history.CPUPressureSomePct != nil || history.OOMKills != nil || history.ArchiveRecordsPerSec != nil {
		t.Fatalf("counter resets produced interval values: %+v", history)
	}

	badWindow := int64(3601)
	requests := int64(5)
	tooManyErrors := int64(6)
	traffic := publicTraffic(rawTrafficView{
		Available: true, Complete: true, WindowSeconds: &badWindow,
		Requests: &requests, Status5xx: &tooManyErrors,
	}, current.parsedAt)
	if traffic.Complete || traffic.RequestsPerSec != nil || traffic.ServerErrorPct != nil {
		t.Fatalf("invalid traffic window produced public metrics: %+v", traffic)
	}

	pressure := pressurePercentage(ptr(int64(130_000_000)), ptr(int64(1_000_000)), time.Minute)
	if pressure == nil || *pressure != 100 {
		t.Fatalf("impossible PSI interval = %v, want capped 100", pressure)
	}
}

func TestHostHealthBoundsHistoryAndRejectsOldData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service-host.jsonl")
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.UTC)
	var lines []string
	for i := 0; i < 130; i++ {
		lines = append(lines, hostFixture(now.Add(time.Duration(i-140)*time.Minute).Format(time.RFC3339), i, 500, i, ""))
	}
	writeHealthFixture(t, path, strings.Join(lines, "\n")+"\n")
	view := readHostHealth(path, now)
	if view.SampleCount != hostHistorySamples || len(view.History) != hostHistorySamples {
		t.Fatalf("history has %d/%d samples, want cap %d", view.SampleCount, len(view.History), hostHistorySamples)
	}
	if view.Freshness != "stale" {
		t.Fatalf("latest eleven-minute-old sample freshness = %q, want stale", view.Freshness)
	}
}

func TestProductionHealthCacheKeepsFreshnessClockCurrent(t *testing.T) {
	dir := t.TempDir()
	metrics := filepath.Join(dir, "service-host.jsonl")
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.UTC)
	writeHealthFixture(t, filepath.Join(dir, "completed-at"), fmt.Sprint(now.Add(-time.Minute).Unix()))
	writeHealthFixture(t, filepath.Join(dir, "checks"), "units")
	writeHealthFixture(t, filepath.Join(dir, "sev.units"), "OK")
	writeHealthFixture(t, metrics, hostFixture(now.Add(-10*time.Second).Format(time.RFC3339), 20, 500, 10, "")+"\n")
	a := &Archive{cfg: Config{MonitorStateDir: dir, HostMetricsFile: metrics}}

	first := a.productionHealthView(now)
	if first.Monitor.AgeSeconds == nil || *first.Monitor.AgeSeconds != 60 ||
		first.ServiceHost.AgeSeconds == nil || *first.ServiceHost.AgeSeconds != 10 {
		t.Fatalf("first ages = monitor %v host %v", first.Monitor.AgeSeconds, first.ServiceHost.AgeSeconds)
	}
	if err := os.Remove(filepath.Join(dir, "completed-at")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(metrics); err != nil {
		t.Fatal(err)
	}
	cached := a.productionHealthView(now.Add(10 * time.Second))
	if cached.Monitor.AgeSeconds == nil || *cached.Monitor.AgeSeconds != 70 ||
		cached.ServiceHost.AgeSeconds == nil || *cached.ServiceHost.AgeSeconds != 20 ||
		cached.GeneratedAt != now.Add(10*time.Second).Format(time.RFC3339) {
		t.Fatalf("cached view did not advance its clock: %+v", cached)
	}
	refreshed := a.productionHealthView(now.Add(16 * time.Second))
	if refreshed.Monitor.Available || refreshed.ServiceHost.Available {
		t.Fatalf("cache survived past its read bound after sources disappeared: %+v", refreshed)
	}
}

func TestProductionHealthPageAndAPIAreSelfContained(t *testing.T) {
	for _, want := range []string{
		"Production telemetry", "Live system map", "Compute", "Cloud &amp; services",
		"Application &amp; traffic", "Archive &amp; data", "Data coverage",
		"Automated service checks", "not collected", "not centralized", "not installed",
		`get("/api/health")`, `get("/api/status")`, `get("/api/viewers")`,
		`data-tooltip=`, `role="tooltip"`, "pointerover", "focusin",
		"formatChartTime", "chart-readout", "onpointermove", `event.key==="ArrowLeft"`,
	} {
		if !strings.Contains(healthPageHTML, want) {
			t.Fatalf("health page is missing %q", want)
		}
	}
	for _, forbidden := range []string{"<script src=", `<link rel="stylesheet"`, "@import", "//cdn"} {
		if strings.Contains(healthPageHTML, forbidden) {
			t.Fatalf("health page depends on an external rendering asset: %q", forbidden)
		}
	}

	a := rigShapedArchive(t)
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)
	for _, tc := range []struct {
		path, contentType, contains string
	}{
		{"/health", "text/html", "Production telemetry"},
		{"/api/health", "application/json", `"monitor"`},
	} {
		resp, err := http.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", tc.path, readErr)
		}
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), tc.contentType) || !strings.Contains(string(body), tc.contains) {
			t.Errorf("GET %s = status %d type %q", tc.path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}

func TestProductionHealthMapAndChartsStayInteractive(t *testing.T) {
	const eightTrackMap = "grid-template-columns:116px 150px minmax(80px,1fr) 150px minmax(80px,1fr) 150px minmax(80px,1fr) 150px"
	if !strings.Contains(healthPageHTML, eightTrackMap) {
		t.Fatal("system-map rows do not define one track for each of their eight items")
	}

	flowPattern := regexp.MustCompile(`<div class="flow(?: [^"]*)?"[^>]*>`)
	flows := flowPattern.FindAllString(healthPageHTML, -1)
	if len(flows) != 9 {
		t.Fatalf("health page has %d system-map connectors, want 9", len(flows))
	}
	for _, flow := range flows {
		if !strings.Contains(flow, `data-tooltip="`) {
			t.Errorf("system-map connector has no tooltip: %s", flow)
		}
		for _, field := range []string{"Measures:", "Unit:", "Source:", "Read:"} {
			if !strings.Contains(flow, field) {
				t.Errorf("system-map connector tooltip lacks %q: %s", field, flow)
			}
		}
	}

	tooltipPattern := regexp.MustCompile(`data-tooltip="([^"]+)"`)
	for _, match := range tooltipPattern.FindAllStringSubmatch(healthPageHTML, -1) {
		if strings.HasPrefix(match[1], "Measures:") {
			continue
		}
		if !strings.Contains(healthPageHTML, strconv.Quote(match[1])+`:[`) {
			t.Errorf("static tooltip has no metric-specific explanation: %q", match[1])
		}
	}
	for _, dynamicHelp := range []string{`/^Slot [0-9]+ is /`, `This color identifies the `} {
		if !strings.Contains(healthPageHTML, dynamicHelp) {
			t.Errorf("dynamic tooltip explanation is missing %q", dynamicHelp)
		}
	}

	for _, want := range []string{
		`y="203"`, `>UTC</text>`, "formatChartTimestamp", "chart-cursor",
		"not collected", `event.key==="ArrowRight"`, `event.key==="Home"`,
		`id="pressure-chart"`, `id="http-rate-chart"`, `id="http-latency-chart"`,
		`id="http-status-chart"`, `id="archive-rate-chart"`, "cpuPressureSomePct",
		"httpStatus1xxPct", "httpStatus2xxPct", "httpStatus4xxPct", "httpServerErrorPct",
	} {
		if !strings.Contains(healthPageHTML, want) {
			t.Errorf("interactive charts are missing %q", want)
		}
	}
	for _, removed := range []string{
		"Host PSI</strong><span>not collected", "Request volume</strong><span>no series",
		"route rate not collected", "Archive rate history is not collected",
	} {
		if strings.Contains(healthPageHTML, removed) {
			t.Errorf("implemented dashboard metric still appears as a gap: %q", removed)
		}
	}
}

func TestProductionHealthPageScriptsAddressExistingUniqueElements(t *testing.T) {
	markup, script, ok := strings.Cut(healthPageHTML, "<script>")
	if !ok {
		t.Fatal("health page has no inline script")
	}
	idPattern := regexp.MustCompile(`\bid="([^"]+)"`)
	ids := make(map[string]bool)
	for _, match := range idPattern.FindAllStringSubmatch(markup, -1) {
		if ids[match[1]] {
			t.Fatalf("health page repeats id %q", match[1])
		}
		ids[match[1]] = true
	}
	byPattern := regexp.MustCompile(`by\("([^"]+)"\)`)
	for _, match := range byPattern.FindAllStringSubmatch(script, -1) {
		if !ids[match[1]] {
			t.Errorf("dashboard script addresses missing id %q", match[1])
		}
	}
	textPattern := regexp.MustCompile(`text\("([^"]+)"`)
	for _, match := range textPattern.FindAllStringSubmatch(script, -1) {
		if !ids[match[1]] {
			t.Errorf("dashboard script writes missing id %q", match[1])
		}
	}
}

func TestProductionHealthSeriesUsesTheSharedGzipTransport(t *testing.T) {
	dir := t.TempDir()
	metrics := filepath.Join(dir, "service-host.jsonl")
	now := time.Now().UTC().Truncate(time.Second)
	writeHealthFixture(t, filepath.Join(dir, "completed-at"), fmt.Sprint(now.Unix()))
	writeHealthFixture(t, filepath.Join(dir, "checks"), "units relay-healthz archive-healthz subscribed status-age peers disk replay backup")
	for _, id := range strings.Fields("units relay-healthz archive-healthz subscribed status-age peers disk replay backup") {
		writeHealthFixture(t, filepath.Join(dir, "sev."+id), "OK")
	}
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, hostFixture(now.Add(time.Duration(i-19)*time.Minute).Format(time.RFC3339), 20+i, 500-i, 10+i, ""))
	}
	writeHealthFixture(t, metrics, strings.Join(lines, "\n")+"\n")
	a := &Archive{cfg: Config{MonitorStateDir: dir, HostMetricsFile: metrics}}
	ts := httptest.NewServer(a.httpHandler())
	t.Cleanup(ts.Close)

	_, plain := rawGet(t, ts.URL+"/api/health", "")
	zippedResponse, zipped := rawGet(t, ts.URL+"/api/health", "gzip")
	if zippedResponse.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("health series Content-Encoding = %q, want gzip", zippedResponse.Header.Get("Content-Encoding"))
	}
	if len(zipped) >= len(plain) {
		t.Fatalf("health series grew under gzip: %d bytes against %d", len(zipped), len(plain))
	}
	var decoded productionHealthView
	if err := json.Unmarshal(gunzip(t, zipped), &decoded); err != nil {
		t.Fatalf("compressed health series is not JSON: %v", err)
	}
	if decoded.ServiceHost.SampleCount != 20 {
		t.Fatalf("compressed health series has %d samples, want 20", decoded.ServiceHost.SampleCount)
	}
}
