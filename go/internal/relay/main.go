package relay

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/logging"
	"multiverse/internal/peercred"
)

// Main is the multiverse-relay entry point, factored out of package main so a
// test can run the same code path in a subprocess.
func Main(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("multiverse-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", env("MULTIVERSE_RELAY_LISTEN",
		fmt.Sprintf("0.0.0.0:%d", contractb.DefaultRelayPort)),
		"host:port for the Contract B WebSocket server; M4 binds a LAN-reachable address")
	dataDir := fs.String("data-dir", env("MULTIVERSE_RELAY_DATA_DIR", "multiverse-relay-data"),
		"directory for ring.json, the durable map and its slot reservations")
	// There is NO --token-file and no MULTIVERSE_TOKEN any more (§22, B22). The
	// shared token is gone, the credentials live in <data-dir>/peers.json as
	// verifiers, and the only way a secret enters this process is the mint
	// command below, which prints it once and keeps a hash.
	mintCredential := fs.String("mint-credential", "",
		"<peerId>: mint a credential for this peer, print its join string ONCE, and exit "+
			"(contract-b-m4.md §3.1). The relay keeps a VERIFIER and cannot print it again")
	grant := fs.String("grant", peercred.GrantPeer,
		"with --mint-credential: peer, subscribe or admin. The three are DISJOINT — a subscribe "+
			"credential cannot claim a slot and a peer credential cannot subscribe (§22, B22, B27)")
	advertise := fs.String("advertise-url", env("MULTIVERSE_RELAY_ADVERTISE_URL", ""),
		"the URL a join string tells a peer to dial. Defaults to this relay's own scheme, "+
			"listen address and path; set it when the relay is behind a name or a proxy")
	insecure := fs.Bool("insecure-no-token", false,
		"accept unauthenticated connections; for a single-machine test rig ONLY. It also "+
			"REFUSES TO BIND ANYTHING BUT LOOPBACK, and no document this project ships may "+
			"instruct anyone to pass it (contract-b-m4.md §3.1)")
	minContract := fs.String("min-contract-version", env("MULTIVERSE_MIN_CONTRACT_VERSION", ""),
		"the lowest protocolVersion this relay admits, e.g. \"contract-b/4.0\" (§22, B25). "+
			"UNSET MEANS NO MINIMUM and that is the default: a floor is a deployment decision. "+
			"Raise it only AFTER the release that satisfies it is published. It is a "+
			"COMPATIBILITY control and never a security one — the string is chosen by the peer")
	tlsCert := fs.String("tls-cert", env("MULTIVERSE_RELAY_TLS_CERT", ""),
		"PEM certificate chain for this relay's own TLS listener (§22, B23). With --tls-key it "+
			"makes the relay speak wss://; without it, plain ws:// is refused with HTTP 426 on "+
			"every non-loopback address")
	tlsKey := fs.String("tls-key", env("MULTIVERSE_RELAY_TLS_KEY", ""),
		"PEM private key for --tls-cert. A renewed pair is picked up by the NEXT handshake "+
			"without dropping a session")
	tlsMin := fs.String("tls-min-version", env("MULTIVERSE_RELAY_TLS_MIN_VERSION", contractb.RelayTLSMinVersion),
		"lowest TLS version this relay's listener accepts: 1.2 or 1.3 (§12, relayTLSMinVersion)")
	releaseSlot := fs.Int("release-slot", 0,
		"release slot n at startup and exit, leaving its position a hole (contract-b-m4.md §7.5)")
	handover := fs.String("handover-slot", "",
		"<n>=<newPeerId>: rebind slot n's reservation — number and position — to another peer id, "+
			"then exit. The old machine keeps its journal (contract-b-m4.md §7.5)")
	yes := fs.Bool("yes", false,
		"skip the confirmation prompt for --release-slot and --handover-slot")
	var reserveSlots peerList
	fs.Var(&reserveSlots, "reserve-slot",
		"<peerId>[@<col>,<row>]: reserve a slot for this peer id at startup and exit; "+
			"repeat once per peer. Without a position the placement rules of §7.2 rule 6 apply")
	logLevel := fs.String("log-level", env("MULTIVERSE_LOG_LEVEL", "info"), "debug, info, warn or error")
	logFile, logRotateMB, logKeep := logging.Flags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log, logCloser, err := logging.New(stderr, logging.Options{
		Level: *logLevel, File: *logFile,
		RotateBytes: int64(*logRotateMB) << 20, Keep: *logKeep,
	})
	if err != nil {
		fmt.Fprintf(stderr, "relay: %v\n", err)
		return 1
	}
	defer logCloser.Close()

	// §22 B23: --insecure-no-token MUST ALSO REFUSE TO BIND ANYTHING BUT
	// LOOPBACK. A relay with no credential check on a reachable address is the
	// exact shape §3.1 wrote the flag's discipline for, and the check belongs
	// here — at the bind — rather than at the first connection, so the failure is
	// a startup error an operator reads and not a map that quietly admits
	// anybody.
	if *insecure && !bindIsLoopback(*listen) {
		log.Error("relay: --insecure-no-token refuses to bind anything but loopback",
			"listen", *listen,
			"remedy", "bind 127.0.0.1 for a single-machine rehearsal, or mint credentials with "+
				"--mint-credential and drop the flag (contract-b-m4.md §3.1)")
		return 1
	}

	tlsOn := *tlsCert != "" || *tlsKey != ""
	srv, err := New(Options{
		Logger:             log,
		DataDir:            *dataDir,
		InsecureNoToken:    *insecure,
		MinContractVersion: *minContract,
	})
	if err != nil {
		log.Error("relay: startup failed", "err", err)
		return 1
	}
	defer srv.Close()

	if *mintCredential != "" {
		return mintCommand(srv, *mintCredential, *grant, joinURL(*advertise, *listen, tlsOn), stdout, log)
	}
	if *releaseSlot > 0 {
		return releaseCommand(srv, *releaseSlot, *yes, stdout, stderr, log)
	}
	if *handover != "" {
		return handoverCommand(srv, *handover, *yes, joinURL(*advertise, *listen, tlsOn), stdout, stderr, log)
	}

	if len(reserveSlots) > 0 {
		for _, spec := range reserveSlots {
			id, at, err := parseReservation(spec)
			if err != nil {
				log.Error("relay: bad --reserve-slot", "spec", spec, "err", err)
				return 1
			}
			res, created, err := srv.ReserveSlot(id, at)
			if err != nil {
				log.Error("relay: reservation failed", "peer", id, "err", err)
				return 1
			}
			if created {
				log.Info("relay: reserved a slot before the peer connected",
					"slot", res.Slot, "position", res.Position(), "peer", res.PeerID)
			} else {
				log.Info("relay: this peer already holds a slot; left alone",
					"slot", res.Slot, "position", res.Position(), "peer", res.PeerID)
			}
			// §3.1: THE RELAY MINTS THE SECRET AT FIRST CLAIM AND PRINTS A JOIN
			// STRING. A reservation is the operator's claim of an identity on a
			// peer's behalf, and it is the only claim that can come first: a peer
			// with no credential cannot dial, so there is no trust-on-first-use path
			// and this is where the credential has to be born.
			if _, held := srv.Credentials().GrantOf(res.PeerID); held {
				log.Info("relay: this peer already holds a credential; it was NOT reminted, "+
					"because a relay keeps a verifier and cannot reprint a join string",
					"peer", res.PeerID,
					"hint", "if it was lost, --handover-slot rebinds the reservation to a new "+
						"peerId with a fresh credential (§7.5)")
				continue
			}
			secret, err := srv.Credentials().Mint(res.PeerID, peercred.GrantPeer)
			if err != nil {
				log.Error("relay: minting the credential failed", "peer", res.PeerID, "err", err)
				return 1
			}
			fmt.Fprint(stdout, peercred.JoinString{
				RelayURL: joinURL(*advertise, *listen, tlsOn),
				PeerID:   res.PeerID, Secret: secret, Grant: peercred.GrantPeer,
			}.Report())
		}
		log.Info("relay: map pre-seeded; start it again to serve",
			"map", srv.MapShape(), "slots", srv.Snapshot())
		return 0
	}

	// §3.1's start rule, checked at the moment it means something: a relay about
	// to SERVE with no credential in its store can admit nobody.
	if err := srv.CheckServable(); err != nil {
		log.Error("relay: refusing to serve", "err", err)
		return 1
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("relay: listen failed", "addr", *listen, "err", err)
		return 1
	}
	scheme := "ws"
	if tlsOn {
		minVersion, err := TLSMinVersion(*tlsMin)
		if err != nil {
			log.Error("relay: bad --tls-min-version", "err", err)
			_ = ln.Close()
			return 1
		}
		reloader, err := NewCertReloader(*tlsCert, *tlsKey)
		if err != nil {
			log.Error("relay: TLS startup failed", "err", err,
				"remedy", "--tls-cert and --tls-key must both name a readable PEM file")
			_ = ln.Close()
			return 1
		}
		ln = TLSListener(ln, reloader, minVersion)
		scheme = "wss"
		log.Info("relay: terminating TLS at its own front door",
			"cert", *tlsCert, "key", *tlsKey, "minVersion", *tlsMin,
			"rotation", "a renewed pair is picked up by the NEXT handshake; established "+
				"sessions are untouched (contract-b-m4.md §22, B23)")
	} else if !bindIsLoopback(*listen) {
		// B23 again, said at startup as well as per connection, because a relay
		// that 426s every upgrade looks from the outside exactly like a relay
		// nobody can reach.
		log.Error("relay: serving PLAINTEXT on a non-loopback bind; every upgrade off loopback "+
			"will be refused with HTTP 426",
			"listen", *listen,
			"remedy", "pass --tls-cert/--tls-key, or front this relay with a proxy that "+
				"terminates TLS and reaches it over loopback (contract-b-m4.md §22, B23)")
	}
	log.Info("relay: listening", "addr", ln.Addr().String(), "scheme", scheme,
		"path", contractb.ContractBPath,
		"retiredPaths", contractb.RetiredContractBPaths, "dataDir", *dataDir,
		"credentials", srv.Credentials().Len(), "credentialStore", srv.Credentials().Path(),
		"minContractVersion", orNone(srv.MinContractVersion()),
		"map", srv.MapShape(), "slots", srv.Snapshot(), "relaySessionId", srv.SessionID())

	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("relay: serve failed", "err", err)
			return 1
		}
	case <-ctx.Done():
		log.Info("relay: shutting down")
		srv.Drain()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}
	return 0
}

// releaseCommand prints the full map-side consequence and asks before acting
// (§7.5). What it CANNOT print is which entries name the slot: journals live on
// the peers' own machines and D2 keeps custody local, so a relay that could
// enumerate them would be a relay that reads journals. §7.5 splits the question
// instead, and the report says where the other half lives.
func releaseCommand(srv *Server, slot int, yes bool, stdout, stderr io.Writer, log *slog.Logger) int {
	report, err := srv.consequenceOfRelease(slot)
	if err != nil {
		log.Error("relay: release failed", "slot", slot, "err", err)
		return 1
	}
	fmt.Fprint(stdout, report)
	if !confirm(stdout, stderr, yes, fmt.Sprintf("release slot %d", slot)) {
		fmt.Fprintln(stdout, "aborted; nothing changed")
		return 1
	}
	if err := srv.ReleaseSlot(slot); err != nil {
		log.Error("relay: release failed", "slot", slot, "err", err)
		return 1
	}
	log.Info("relay: slot released; the number is retired and will not be reused", "slot", slot)
	return 0
}

// mintCommand is §3.1's issuance, and it is the SMALLEST DESIGN THAT WORKS: the
// relay mints a secret, prints a join string once at its own console, and keeps
// a verifier. No accounts, no email, no password reset (DQ1).
//
// The cost is stated wherever the flow is, because it is real and was chosen
// with the price in view: THERE IS NO RECOVERY PATH IN THE SOFTWARE. A stranger
// who loses their join string loses that world's identity until an operator
// hands the slot over by name — bounded only by the fact that the reservation
// never expires, so the slot, the position and every journal entry addressed to
// it are still there when the operator gets to it.
func mintCommand(srv *Server, peerID, grant, relayURL string, stdout io.Writer, log *slog.Logger) int {
	if !peercred.ValidGrant(grant) {
		log.Error("relay: bad --grant", "grant", grant,
			"want", "peer, subscribe or admin")
		return 1
	}
	secret, err := srv.Credentials().Mint(peerID, grant)
	if errors.Is(err, peercred.ErrExists) {
		log.Error("relay: this peerId already holds a credential and it was not replaced",
			"peer", peerID,
			"why", "the store keeps a VERIFIER, so a join string cannot be reprinted; replacing "+
				"one silently would strand whichever copy the peer still holds",
			"remedy", "--handover-slot <n>=<newPeerId> rebinds the reservation to a new identity "+
				"with a freshly minted credential (contract-b-m4.md §7.5, §22 B22)")
		return 1
	}
	if err != nil {
		log.Error("relay: minting the credential failed", "peer", peerID, "err", err)
		return 1
	}
	fmt.Fprint(stdout, peercred.JoinString{
		RelayURL: relayURL, PeerID: peerID, Secret: secret, Grant: grant,
	}.Report())
	log.Info("relay: minted a credential; the join string was printed once and is not recoverable",
		"peer", peerID, "grant", grant, "store", srv.Credentials().Path())
	return 0
}

func handoverCommand(srv *Server, spec string, yes bool, relayURL string, stdout, stderr io.Writer, log *slog.Logger) int {
	slot, newPeer, err := parseHandover(spec)
	if err != nil {
		log.Error("relay: bad --handover-slot", "spec", spec, "err", err)
		return 1
	}
	report, err := srv.consequenceOfHandover(slot, newPeer)
	if err != nil {
		log.Error("relay: handover failed", "slot", slot, "err", err)
		return 1
	}
	fmt.Fprint(stdout, report)
	if !confirm(stdout, stderr, yes, fmt.Sprintf("hand slot %d to %s", slot, newPeer)) {
		fmt.Fprintln(stdout, "aborted; nothing changed")
		return 1
	}
	old, now, err := srv.HandoverSlot(slot, newPeer)
	if err != nil {
		log.Error("relay: handover failed", "slot", slot, "err", err)
		return 1
	}
	// §22 B22: HANDOVER IS THE CREDENTIAL RECOVERY PATH, and there is no other.
	// The new identity gets a freshly minted credential and the old one's is
	// dropped, so an identity that gave up a slot cannot go on authenticating
	// against a reservation it no longer holds.
	secret, mintErr := srv.Credentials().Mint(now.PeerID, peercred.GrantPeer)
	if errors.Is(mintErr, peercred.ErrExists) {
		secret, mintErr = srv.Credentials().Remint(now.PeerID)
	}
	if mintErr != nil {
		log.Error("relay: the slot moved but its new credential could not be minted",
			"slot", now.Slot, "peer", now.PeerID, "err", mintErr,
			"remedy", "mint it by hand: multiverse-relay --mint-credential "+now.PeerID)
		return 1
	}
	if err := srv.Credentials().Forget(old.PeerID); err != nil {
		log.Error("relay: the old identity's credential could not be dropped",
			"peer", old.PeerID, "err", err)
		return 1
	}
	fmt.Fprint(stdout, peercred.JoinString{
		RelayURL: relayURL, PeerID: now.PeerID, Secret: secret, Grant: peercred.GrantPeer,
	}.Report())
	log.Info("relay: slot handed over; the map did not change shape and no lane moved",
		"slot", now.Slot, "position", now.Position(), "from", old.PeerID, "to", now.PeerID,
		"oldCredential", "dropped", "newCredential", "minted and printed once")
	return 0
}

func confirm(stdout, stderr io.Writer, yes bool, what string) bool {
	if yes {
		return true
	}
	fmt.Fprintf(stdout, "\nType YES to %s: ", what)
	in := bufio.NewReader(os.Stdin)
	line, err := in.ReadString('\n')
	if err != nil {
		fmt.Fprintf(stderr, "relay: could not read a confirmation: %v\n", err)
		return false
	}
	return strings.TrimSpace(line) == "YES"
}

// consequenceOfRelease is the pre-act report §7.5 REQUIRES: the slot, its
// position, its peerId, how long it has been dark, which peers' effective lanes
// change, and which positions become holes.
func (s *Server) consequenceOfRelease(slot int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return "", errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		return "", fmt.Errorf("relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nRELEASE slot %d\n", res.Slot)
	s.describeSlotLocked(&b, res)
	fmt.Fprintf(&b, "  after       position (%d,%d) becomes a HOLE; the map stays %dx%d\n",
		res.Col, res.Row, s.grid.Width, s.grid.Height)
	fmt.Fprintf(&b, "              slot %d is retired for good: maxSlotEverIssued (%d) never decreases,\n",
		res.Slot, s.grid.MaxSlotEverIssued)
	fmt.Fprintf(&b, "              so every journaled destSlot %d now answers SLOT_VACANT, permanently\n", res.Slot)
	s.describeLaneChangesLocked(&b, res)
	s.describeCustodySplitLocked(&b, res)
	return b.String(), nil
}

func (s *Server) consequenceOfHandover(slot int, newPeerID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return "", errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		return "", fmt.Errorf("relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	if held := s.grid.SlotOfPeer(newPeerID); held > 0 {
		return "", fmt.Errorf("relay: %s already holds slot %d; a peer holds at most one", newPeerID, held)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nHANDOVER slot %d to %s\n", res.Slot, newPeerID)
	s.describeSlotLocked(&b, res)
	fmt.Fprintf(&b, "  after       slot %d and position (%d,%d) are unchanged; only the identity moves\n",
		res.Slot, res.Col, res.Row)
	fmt.Fprintf(&b, "              the map does not change shape and NO LANE MOVES\n")
	fmt.Fprintf(&b, "              %s keeps its journal, its genome cache and its logs; a handover NEVER\n", res.PeerID)
	fmt.Fprintf(&b, "              moves a journal, and %s inherits NOTHING but the address\n", newPeerID)
	fmt.Fprintf(&b, "              in-flight work addressed to slot %d arrives at %s, because routing is\n",
		res.Slot, newPeerID)
	fmt.Fprintf(&b, "              on the slot. If you do not want that, you want --release-slot\n")
	s.describeCustodySplitLocked(&b, res)
	return b.String(), nil
}

func (s *Server) describeSlotLocked(b *strings.Builder, res Reservation) {
	m := s.metaLocked(res.PeerID)
	dark := "never connected to this relay session"
	if m.darkSinceMs > 0 {
		dark = fmt.Sprintf("dark since %s (%s ago)",
			time.UnixMilli(m.darkSinceMs).UTC().Format(time.RFC3339),
			time.Since(time.UnixMilli(m.darkSinceMs)).Truncate(time.Second))
	}
	fmt.Fprintf(b, "  peerId      %s\n", res.PeerID)
	fmt.Fprintf(b, "  position    (%d,%d) in a %dx%d map of %d slots\n",
		res.Col, res.Row, s.grid.Width, s.grid.Height, s.grid.Size())
	fmt.Fprintf(b, "  liveness    %s\n", dark)
}

// describeLaneChangesLocked lists every peer whose effective lane moves.
func (s *Server) describeLaneChangesLocked(b *strings.Builder, going Reservation) {
	fmt.Fprintf(b, "  lanes       ")
	moved := 0
	for _, res := range s.grid.Slots {
		if res.Slot == going.Slot {
			continue
		}
		ok := s.deliverableLocked(res)
		// All four, because the ripple is symmetric under two-way lanes: a slot
		// going dark re-targets every lane that pointed at it, and every one of its
		// neighbours has one (§17, B13).
		for _, edge := range contracta.CanonicalEdges() {
			target, _, found := s.grid.Effective(res, edge, ok)
			if !found || target.Slot != going.Slot {
				continue
			}
			if moved > 0 {
				fmt.Fprintf(b, "              ")
			}
			fmt.Fprintf(b, "slot %d's %s lane points at slot %d today and will re-pair\n",
				res.Slot, edge, going.Slot)
			moved++
		}
	}
	if moved == 0 {
		fmt.Fprintf(b, "no live peer's effective lane points at slot %d right now\n", going.Slot)
	}
}

func (s *Server) describeCustodySplitLocked(b *strings.Builder, res Reservation) {
	fmt.Fprintf(b, "  custody     this relay CANNOT list the entries that name slot %d: journals live on\n", res.Slot)
	fmt.Fprintf(b, "              six other machines and D2 keeps custody local. Read heldDepth per peer on\n")
	fmt.Fprintf(b, "              the status page or in ringstat, then run on the machine that owns them:\n")
	fmt.Fprintf(b, "                  multiverse-sidecar --list-inflight --dest-slot %d\n", res.Slot)
}

// parseReservation reads "<peerId>" or "<peerId>@<col>,<row>".
func parseReservation(spec string) (string, *contractb.Position, error) {
	id, at, found := strings.Cut(spec, "@")
	if id == "" {
		return "", nil, errors.New("no peer id")
	}
	if !found {
		return id, nil, nil
	}
	colStr, rowStr, ok := strings.Cut(at, ",")
	if !ok {
		return "", nil, errors.New("a position is <col>,<row>")
	}
	col, err := strconv.Atoi(strings.TrimSpace(colStr))
	if err != nil || col < 0 {
		return "", nil, fmt.Errorf("col %q is not a non-negative integer", colStr)
	}
	row, err := strconv.Atoi(strings.TrimSpace(rowStr))
	if err != nil || row < 0 {
		return "", nil, fmt.Errorf("row %q is not a non-negative integer", rowStr)
	}
	return id, &contractb.Position{Col: col, Row: row}, nil
}

// parseHandover reads "<slot>=<newPeerId>".
func parseHandover(spec string) (int, string, error) {
	slotStr, peerID, ok := strings.Cut(spec, "=")
	if !ok {
		return 0, "", errors.New("--handover-slot takes <n>=<newPeerId>")
	}
	slot, err := strconv.Atoi(strings.TrimSpace(slotStr))
	if err != nil || slot < 1 {
		return 0, "", fmt.Errorf("slot %q is not a slot number", slotStr)
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return 0, "", errors.New("--handover-slot needs a new peer id")
	}
	return slot, peerID, nil
}

// peerList collects a repeated --reserve-slot flag, in the order it was given.
type peerList []string

func (p *peerList) String() string     { return strings.Join(*p, ",") }
func (p *peerList) Set(v string) error { *p = append(*p, v); return nil }

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func orNone(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}

// bindIsLoopback answers B23's carve-out and §3.1's --insecure-no-token rule
// from the SAME test, because both rules mean the same thing by "loopback": a
// listener no other machine can reach. A wildcard bind is not loopback even
// though loopback can reach it — that is the whole point of the distinction.
func bindIsLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		host = listen
	}
	if host == "" {
		// ":8795" is the wildcard, spelled shortest.
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// joinURL is the URL a join string tells a peer to dial. It is the operator's
// --advertise-url when they gave one, and otherwise this relay's own scheme,
// bind and path — with the host left as a legible placeholder on a wildcard
// bind, because a relay genuinely does not know which of its addresses a
// stranger can reach.
func joinURL(advertise, listen string, tlsOn bool) string {
	if advertise != "" {
		return advertise
	}
	scheme := "ws"
	if tlsOn {
		scheme = "wss"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host, port = listen, ""
	}
	if ip := net.ParseIP(host); host == "" || host == "0.0.0.0" || host == "::" ||
		(ip != nil && ip.IsUnspecified()) {
		host = "<relay-host>"
	}
	if port == "" {
		return scheme + "://" + host + contractb.ContractBPath
	}
	return scheme + "://" + net.JoinHostPort(host, port) + contractb.ContractBPath
}
