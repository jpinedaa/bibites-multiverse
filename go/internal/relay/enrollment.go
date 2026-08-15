package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"multiverse/internal/peercred"
)

const (
	// PublicEnrollmentPath is outside Contract B. It creates the credential
	// that a later Contract B upgrade presents.
	PublicEnrollmentPath = "/api/enroll"
	publicPeerPrefix     = "public-"
	enrollmentRequestV1  = "bibites-multiverse/enrollment-request/1"
	enrollmentResponseV1 = "bibites-multiverse/enrollment-response/1"
)

var installIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// PublicEnrollmentOptions controls open installer enrollment. The endpoint is
// off by default. When it is on, both limits are required and finite.
type PublicEnrollmentOptions struct {
	Enabled          bool
	MaxCredentials   int
	MaxPerAddress    int
	PerAddressWindow time.Duration
}

func (o *PublicEnrollmentOptions) applyDefaults() error {
	if !o.Enabled {
		return nil
	}
	if o.MaxCredentials <= 0 {
		return errors.New("relay: public enrollment needs a positive credential limit")
	}
	if o.MaxPerAddress <= 0 {
		return errors.New("relay: public enrollment needs a positive per-address limit")
	}
	if o.PerAddressWindow <= 0 {
		o.PerAddressWindow = 24 * time.Hour
	}
	return nil
}

type enrollmentRequest struct {
	Format    string `json:"format"`
	InstallID string `json:"installId"`
	Secret    string `json:"secret"`
	Release   string `json:"release"`
}

type enrollmentResponse struct {
	Format   string `json:"format"`
	RelayURL string `json:"relayUrl"`
	PeerID   string `json:"peerId"`
	Created  bool   `json:"created"`
}

func (s *Server) servePublicEnrollment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeEnrollmentError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	if s.advertiseURL == "" || !strings.HasPrefix(s.advertiseURL, "wss://") {
		writeEnrollmentError(w, http.StatusServiceUnavailable,
			"public enrollment has no secure relay address")
		return
	}

	var req enrollmentRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeEnrollmentError(w, http.StatusBadRequest, "the enrollment request is not valid JSON")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeEnrollmentError(w, http.StatusBadRequest, "the enrollment request contains extra data")
		return
	}
	if req.Format != enrollmentRequestV1 {
		writeEnrollmentError(w, http.StatusBadRequest, "the enrollment request format is not supported")
		return
	}
	if !installIDPattern.MatchString(req.InstallID) {
		writeEnrollmentError(w, http.StatusBadRequest, "installId must be a UUID")
		return
	}
	if req.Release == "" || len(req.Release) > 32 {
		writeEnrollmentError(w, http.StatusBadRequest, "release is required")
		return
	}
	if err := peercred.ValidSecret(req.Secret); err != nil {
		writeEnrollmentError(w, http.StatusBadRequest, "the generated credential has an invalid shape")
		return
	}

	peerID := publicPeerPrefix + strings.ReplaceAll(strings.ToLower(req.InstallID), "-", "")
	s.enrollmentMu.Lock()
	created, status, retryAfter, err := s.enrollPublicLocked(sourceKey(r), peerID, req.Secret)
	s.enrollmentMu.Unlock()
	if retryAfter > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
	}
	if err != nil {
		writeEnrollmentError(w, status, err.Error())
		return
	}

	if created {
		status = http.StatusCreated
	} else {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(enrollmentResponse{
		Format: enrollmentResponseV1, RelayURL: s.advertiseURL, PeerID: peerID, Created: created,
	})
	s.log.Info("relay: public installer enrollment",
		"peer", peerID, "remote", sourceKey(r), "release", req.Release,
		"created", created, "publicCredentials", s.creds.CountPrefix(publicPeerPrefix),
		"publicCredentialLimit", s.publicEnrollment.MaxCredentials)
}

func (s *Server) enrollPublicLocked(source, peerID, secret string) (bool, int, time.Duration, error) {
	if _, exists := s.creds.GrantOf(peerID); exists {
		created, err := s.creds.Enroll(peerID, secret, peercred.GrantPeer)
		if err != nil {
			return false, http.StatusConflict, 0,
				errors.New("this installation id already has a different credential")
		}
		return created, http.StatusOK, 0, nil
	}
	if s.creds.CountPrefix(publicPeerPrefix) >= s.publicEnrollment.MaxCredentials {
		return false, http.StatusServiceUnavailable, 0,
			errors.New("the public map has no automatic-enrollment capacity")
	}

	now := time.Now()
	cutoff := now.Add(-s.publicEnrollment.PerAddressWindow)
	recent := s.enrollmentByAddr[source][:0]
	for _, at := range s.enrollmentByAddr[source] {
		if at.After(cutoff) {
			recent = append(recent, at)
		}
	}
	if len(recent) >= s.publicEnrollment.MaxPerAddress {
		retry := recent[0].Add(s.publicEnrollment.PerAddressWindow).Sub(now)
		if retry < time.Second {
			retry = time.Second
		}
		s.enrollmentByAddr[source] = recent
		return false, http.StatusTooManyRequests, retry,
			errors.New("this network address reached the automatic-enrollment limit")
	}

	created, err := s.creds.Enroll(peerID, secret, peercred.GrantPeer)
	if err != nil {
		return false, http.StatusInternalServerError, 0,
			errors.New("the relay could not store the new credential")
	}
	if created {
		s.enrollmentByAddr[source] = append(recent, now)
	}
	return created, http.StatusCreated, 0, nil
}

func writeEnrollmentError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
