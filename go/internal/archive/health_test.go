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
	}

	for _, want := range []string{
		`y="203"`, `>UTC</text>`, "formatChartTimestamp", "chart-cursor",
		"not collected", `event.key==="ArrowRight"`, `event.key==="Home"`,
	} {
		if !strings.Contains(healthPageHTML, want) {
			t.Errorf("interactive charts are missing %q", want)
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
