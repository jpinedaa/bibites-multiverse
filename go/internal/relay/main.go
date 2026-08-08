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
	"multiverse/internal/lantoken"
	"multiverse/internal/logging"
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
	tokenFile := fs.String("token-file", env("MULTIVERSE_TOKEN_FILE", ""),
		"file whose first line is the shared LAN token; MULTIVERSE_TOKEN is the alternative")
	insecure := fs.Bool("insecure-no-token", false,
		"accept unauthenticated connections; for a single-machine test rig only, never on the LAN")
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

	// There is no flag that takes the token literally: it would put the secret
	// in every process listing (contract-b-m4.md §3.1).
	token, err := lantoken.Load(*tokenFile)
	if err != nil && !*insecure {
		log.Error("relay: no LAN token", "err", err,
			"hint", "set MULTIVERSE_TOKEN or pass --token-file; --insecure-no-token is test-rig only")
		return 1
	}

	srv, err := New(Options{
		Logger:          log,
		DataDir:         *dataDir,
		Token:           token,
		InsecureNoToken: *insecure,
	})
	if err != nil {
		log.Error("relay: startup failed", "err", err)
		return 1
	}
	defer srv.Close()

	if *releaseSlot > 0 {
		return releaseCommand(srv, *releaseSlot, *yes, stdout, stderr, log)
	}
	if *handover != "" {
		return handoverCommand(srv, *handover, *yes, stdout, stderr, log)
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
		}
		log.Info("relay: map pre-seeded; start it again to serve",
			"map", srv.MapShape(), "slots", srv.Snapshot())
		return 0
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("relay: listen failed", "addr", *listen, "err", err)
		return 1
	}
	log.Info("relay: listening", "addr", ln.Addr().String(), "path", contractb.ContractBPath,
		"retiredPath", contractb.RetiredContractBPath, "dataDir", *dataDir,
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

func handoverCommand(srv *Server, spec string, yes bool, stdout, stderr io.Writer, log *slog.Logger) int {
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
	log.Info("relay: slot handed over; the map did not change shape and no lane moved",
		"slot", now.Slot, "position", now.Position(), "from", old.PeerID, "to", now.PeerID)
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
