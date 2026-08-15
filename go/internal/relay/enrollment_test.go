package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multiverse/internal/peercred"
)

func startEnrollmentRelay(t *testing.T, maxCredentials, maxPerAddress int) (*Server, *httptest.Server) {
	t.Helper()
	store, err := peercred.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	srv, err := New(Options{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		DataDir:      t.TempDir(),
		Credentials:  store,
		AdvertiseURL: "wss://map.example.test/contract-b/v4",
		PublicEnrollment: PublicEnrollmentOptions{
			Enabled:          true,
			MaxCredentials:   maxCredentials,
			MaxPerAddress:    maxPerAddress,
			PerAddressWindow: time.Hour,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return srv, ts
}

func postEnrollment(t *testing.T, ts *httptest.Server, installID, secret, source string) (int, []byte, http.Header) {
	t.Helper()
	body, err := json.Marshal(enrollmentRequest{
		Format: enrollmentRequestV1, InstallID: installID, Secret: secret, Release: "0.2.0",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+PublicEnrollmentPath, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if source != "" {
		req.Header.Set("X-Forwarded-For", source)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return resp.StatusCode, raw, resp.Header
}

func TestPublicEnrollmentCreatesOneBoundCredentialWithoutEchoingTheSecret(t *testing.T) {
	srv, ts := startEnrollmentRelay(t, 4, 2)
	secret := peercred.MintSecret()
	installID := "01234567-89ab-4cde-8fab-0123456789ab"
	status, raw, header := postEnrollment(t, ts, installID, secret, "203.0.113.10")
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, raw)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("the enrollment response echoed the credential secret")
	}
	if got := header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var response enrollmentResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	wantPeer := "public-0123456789ab4cde8fab0123456789ab"
	if response.Format != enrollmentResponseV1 || response.PeerID != wantPeer ||
		response.RelayURL != "wss://map.example.test/contract-b/v4" || !response.Created {
		t.Fatalf("response = %+v", response)
	}
	if _, grant, ok := srv.Credentials().Verify(peercred.Join(wantPeer, secret)); !ok || grant != peercred.GrantPeer {
		t.Fatal("the returned peer id does not verify with the client-generated secret")
	}
}

func TestPublicEnrollmentRetryIsIdempotent(t *testing.T) {
	srv, ts := startEnrollmentRelay(t, 1, 1)
	secret := peercred.MintSecret()
	installID := "11111111-2222-4333-8444-555555555555"
	if status, raw, _ := postEnrollment(t, ts, installID, secret, "203.0.113.11"); status != http.StatusCreated {
		t.Fatalf("first status = %d: %s", status, raw)
	}
	status, raw, _ := postEnrollment(t, ts, installID, secret, "203.0.113.11")
	if status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200: %s", status, raw)
	}
	var response enrollmentResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if response.Created {
		t.Fatal("an idempotent retry reported a new credential")
	}
	if got := srv.Credentials().CountPrefix(publicPeerPrefix); got != 1 {
		t.Fatalf("public credential count = %d, want 1", got)
	}
	status, _, _ = postEnrollment(t, ts, installID, peercred.MintSecret(), "203.0.113.11")
	if status != http.StatusConflict {
		t.Fatalf("different secret status = %d, want 409", status)
	}
}

func TestPublicEnrollmentHasFiniteGlobalAndAddressLimits(t *testing.T) {
	_, ts := startEnrollmentRelay(t, 2, 1)
	first := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	second := "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff"
	third := "cccccccc-dddd-4eee-8fff-000000000000"
	if status, raw, _ := postEnrollment(t, ts, first, peercred.MintSecret(), "203.0.113.12"); status != http.StatusCreated {
		t.Fatalf("first status = %d: %s", status, raw)
	}
	status, _, header := postEnrollment(t, ts, second, peercred.MintSecret(), "203.0.113.12")
	if status != http.StatusTooManyRequests || header.Get("Retry-After") == "" {
		t.Fatalf("same-address status = %d, Retry-After = %q", status, header.Get("Retry-After"))
	}
	if status, raw, _ := postEnrollment(t, ts, second, peercred.MintSecret(), "203.0.113.13"); status != http.StatusCreated {
		t.Fatalf("second source status = %d: %s", status, raw)
	}
	if status, _, _ := postEnrollment(t, ts, third, peercred.MintSecret(), "203.0.113.14"); status != http.StatusServiceUnavailable {
		t.Fatalf("global-cap status = %d, want 503", status)
	}
}

func TestPublicEnrollmentRejectsMalformedRequestsAndIsOffByDefault(t *testing.T) {
	store, err := peercred.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	srv, err := New(Options{Credentials: store})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	resp, err := http.Post(ts.URL+PublicEnrollmentPath, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled endpoint status = %d, want 404", resp.StatusCode)
	}

	_, enabled := startEnrollmentRelay(t, 2, 1)
	resp, err = http.Post(enabled.URL+PublicEnrollmentPath, "application/json", strings.NewReader(`{"format":"wrong"}`))
	if err != nil {
		t.Fatalf("Post malformed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400", resp.StatusCode)
	}
}
