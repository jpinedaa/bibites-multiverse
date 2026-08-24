// Package relay implements multiverse-relay: the deliberately dumb M4
// transport (D1). It forwards Contract B frames across a rectangular map of
// slots, arbitrates placement, computes the effective neighbour on each export
// edge, proves what it did and did not forward, copies every routed migration
// to the archive, and never parses a bb8 body or a lineage annex (D4).
//
// M4 gives the relay exactly two rules that are not frame forwarding: the
// effective-neighbour walk (§8) and the forwarding record behind the
// non-delivery proof (§5.2). It stays dumb in the sense that matters — it never
// parses a body, never validates a payload, never indexes a genome and never
// takes organism custody or retains decoded organism state. B43's bounded
// destination queue retains only opaque transport bytes. The walk reads the
// registry it already keeps; the record is a set of ids it already routed.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
	"multiverse/internal/wsutil"
)

// Version is reported in HANDSHAKE_ACK.
const Version = "m5.0"

// Options configures a Server.
type Options struct {
	Logger *slog.Logger
	// DataDir holds ring.json AND peers.json. Empty keeps both in memory, which
	// is what a test rig wants and what production must never do (§7.4, §12
	// credentialVerifierStore).
	DataDir string
	// Credentials is the §3.1 verifier store. A relay with no credential in it
	// can admit nobody, so New refuses to start unless InsecureNoToken is set.
	Credentials *peercred.Store
	// InsecureNoToken accepts unauthenticated connections and logs one loud
	// warning per accepted connection (§3.1). It exists for a single-machine test
	// rig, it ALSO refuses to bind anything but loopback (enforced in Main), and
	// no installer, script or document this project ships may instruct a stranger
	// to pass it (m5_considerations.md, decision 7).
	InsecureNoToken bool
	// MinContractVersion is B25's admission floor. Empty means NO MINIMUM, which
	// is the default and the only honest one for a map whose operator has not
	// decided a floor.
	MinContractVersion string
	// Limits is §3.3's capacity table as this relay runs it (added — §22, B24).
	// Any entry left at zero takes the shipped default; what the relay PUBLISHES
	// is what it ends up running with, never the table in the contract.
	Limits contractb.Limits
	// AdvertiseURL is the relay URL a join string tells a peer to dial. B28's
	// handover mints a credential over the path, so the answer has to be able to
	// carry a usable join string rather than half of one.
	AdvertiseURL string
	// PublicEnrollment enables the HTTPS bootstrap endpoint for installers.
	// It is disabled unless an operator sets explicit capacity limits.
	PublicEnrollment PublicEnrollmentOptions
	PingInterval     time.Duration
	PeerTimeout      time.Duration
	// ArchiveQueue is the per-subscriber copy queue (§5.1).
	ArchiveQueue int
	// StatusCoalesce is the minimum spacing between PEER_STATUS broadcasts and
	// between grants to one peer (§7.2). The last frame of a burst is always
	// sent. Under contract-b/4.0 it is the FLOOR of a window that widens
	// (amended — §22, B29).
	StatusCoalesce time.Duration
	// StatusCoalesceMax is the ceiling that window widens toward under sustained
	// churn (added — §22, B29). It bounds the broadcast RATE, which is what a
	// public map's slotCount stats blocks per frame make expensive.
	StatusCoalesceMax time.Duration
	// StatusChurnBurstThreshold is how many REGISTRY changes inside one window
	// make the relay widen it (added — §22, B29).
	StatusChurnBurstThreshold int
	// StatsBroadcast republishes PEER_STATUS on a timer, because stats change
	// without the registry changing (§6.5).
	StatsBroadcast time.Duration
	// ForwardRecordRetention is how long a forwarded migrationId is remembered
	// for the neverForwarded proof (§5.2). It used to be sized at twice the
	// sender's hold; §25's B37 removed the hold, and what it covers now is a
	// re-routed entry's later attempts and an old sidecar still retrying across
	// the transition.
	ForwardRecordRetention time.Duration
	// NoWireCompression stops this relay OFFERING permessage-deflate (§24, B35).
	//
	// THE ZERO VALUE IS THE SHIPPED BEHAVIOUR, which is compression offered, so
	// every existing caller and every test gets §24's wire without being
	// rewritten. It is expressed as a negative for exactly that reason.
	//
	// It is the operator's kill switch and it is complete on its own: the
	// extension is used only when both ends offer it, so a relay that stops
	// offering it puts the whole map back on uncompressed frames as each peer
	// reconnects, with no participant action and no binary rollback.
	NoWireCompression bool
}

// Server is the relay. The zero value is not usable; call New.
type Server struct {
	log             *slog.Logger
	creds           *peercred.Store
	insecureNoToken bool
	minContract     string
	limits          contractb.Limits
	advertiseURL    string
	pingInterval    time.Duration
	peerTimeout     time.Duration
	archiveQueue    int
	statusCoalesce  time.Duration
	// B29's two new tunables. statusCoalesce is the floor of the window these
	// two shape; the window itself lives in the publish loop, which is the one
	// goroutine allowed to touch it.
	statusCoalesceMax time.Duration
	churnThreshold    int
	statsBroadcast    time.Duration
	forwardRetain     time.Duration
	// wireCompression is whether this relay OFFERS permessage-deflate on its
	// upgrades (§3, §24 B35). It is read on every accept and never changes after
	// New, so it needs no lock.
	wireCompression  bool
	publicEnrollment PublicEnrollmentOptions
	enrollmentMu     sync.Mutex
	enrollmentByAddr map[string][]time.Time

	// B24's two socket counters. They are outside s.mu deliberately: an upgrade
	// must not queue behind a PEER_STATUS broadcast, and a connection storm is
	// exactly when the registry lock is busiest.
	perAddress *slotCounter
	perPeer    *slotCounter

	// evictions is B28's admission ban, in memory and per relay process. It is
	// NOT durable: an eviction is a LIVENESS act and a relay restart already
	// drops every peer's session, so a ban that survived one would outlive the
	// state it was reasoning about. The log line says so at the moment it is set.
	evictions map[string]eviction
	// adminTokens are B28's single-use confirmation tokens, each bound to an act
	// AND to the ring state it was reported against.
	adminTokens map[string]*adminPending

	// sessionID is minted once at process start and constant for the life of it.
	// It is the scope of the forwarding record: a relay restart is exactly the
	// event that invalidates the proof, so nothing about it is durable (§5.2).
	sessionID string

	stopOnce sync.Once
	stop     chan struct{}
	wg       sync.WaitGroup

	mu   sync.Mutex
	grid *Grid
	// connections includes current and replaced connections until their writer
	// exits. Relay-wide drain must reach every accepted destination transport
	// queue, including one on a connection that was replaced concurrently.
	connections map[*peer]struct{}
	// live connections by peer id, role "peer".
	peers map[string]*peer
	// live connections by peer id, role "archive".
	subscribers map[string]*peer
	// what the relay knows about a peer id, live or not. PEER_STATUS keeps
	// reporting a reserved slot after its peer goes away (§6.5).
	meta map[string]*peerMeta
	// pending counts the REGISTRY changes the next broadcast must carry. It is
	// drained by the publisher, and it is what guarantees B29's "the last frame
	// of a burst is always sent": nothing but a publish that actually happened
	// clears it.
	//
	// A STATS BLOCK DOES NOT BUMP IT (amended — §24, B36). It used to, and that
	// made every peer's PING a broadcast trigger; §6.5 gives stats the
	// statsBroadcastIntervalMs timer and nothing else. See the note beside
	// markChurnLocked.
	pending int
	// churn counts the REGISTRY changes inside the current window, which is the
	// narrower thing B29 widens the window on. It has never counted a stats
	// block, for the reason churn.go states at length: widening the window on a
	// quiet map's heartbeat would be a bound that fired when nothing was
	// happening.
	//
	// SINCE §24 B36 THE TWO COUNTERS HOLD THE SAME NUMBER, because a registry
	// change is now the only thing either of them counts. They stay separate
	// because the RULES stay separate: §7.2 says what must be published and B29
	// says what may widen the window, and the next event that is one without the
	// other should find the distinction already here rather than have to
	// reintroduce it. Stats were that event until §24 removed them from both.
	churn int
	// broadcasts counts PEER_STATUS broadcast rounds for the life of the
	// process, for the operator log line and for the churn harness's measured
	// rate against B29's arithmetic.
	broadcasts int64
	// coalesceWindow is the width the publisher is currently waiting, published
	// here so an operator and a test can read the window a storm produced.
	coalesceWindow time.Duration
	// lastPublishAt is when the last broadcast round went out, from WHICHEVER
	// path made it — the coalescing window or an admin act's publishNow. §14 B4
	// makes the stats timer a bound on how far apart broadcasts may DRIFT, so it
	// has to see every publish, not only the ones its own loop made.
	lastPublishAt time.Time
	draining      bool
	// forwarded is the §5.2 record: the migrationIds this PROCESS has attempted
	// to write to some peer's connection, with the time of the first attempt.
	forwarded map[string]time.Time
	// receiptsSent and receiptsDropped count B26's frame (§6.12). They are the
	// operator's reading of a term that is one frame per migration and therefore
	// grows with exactly the thing a public map grows, and the harness's input
	// for frames-per-migration at rate. A drop is benign — a missing receipt is
	// silence — so it is COUNTED rather than acted on.
	receiptsSent       int64
	receiptsDropped    int64
	lastReceiptDropLog time.Time
	// inherited names the peer ids that took a slot by operator handover, so
	// their first grant says so (§6.4's "handover" reason). It is consumed once.
	inherited map[string]bool
}

type peerMeta struct {
	gameVersion  string
	simSize      float64
	modConnected bool
	exportEdges  []string
	lastSeenMs   int64
	darkSinceMs  int64
	lastRefusal  string
	// borderEdges is kept for ONE reason: B29's second rule names it among the
	// fields whose change forbids the suppression of a broadcast. Nothing
	// publishes it — §6.5's SlotInfo carries exportEdges and not this — so it is
	// stored to be compared and for nothing else.
	borderEdges []string
	stats       *contractb.PeerStats
	statsAsOfMs int64
	// claims is maxClaimsPerMinute's counter, kept HERE rather than on the
	// connection because §3.3 scopes it per peer: this struct outlives a
	// reconnect and a per-connection counter would hand a storming peer a fresh
	// allowance every time it redialled.
	claims *claimMeter
	// migrationPace is the per-destination physical-write schedule. It is stored
	// on the identity metadata so overlapping reconnects share one allowance.
	// It holds no frame or organism state.
	migrationPace *migrationForwardPace
}

type peer struct {
	id      string
	role    string
	conn    *wsutil.Conn
	claimed bool // a SECTOR_CLAIM was already answered on this connection

	mu        sync.Mutex
	epoch     int64
	lastGrant string // the last SECTOR_GRANT body, so a repeat is not re-sent
	// lastPos is the coordinate the last grant carried. A grant whose position
	// moved is a "repositioned", not an "updated": the map grew under this peer
	// and its neighbours changed with it (§6.4).
	lastPos *contractb.Position
	// out is the bounded copy queue of a subscriber (§5.1). It is nil for a
	// peer.
	out         chan []byte
	dropped     int64
	lastDropLog time.Time
}

// New builds a relay.
func New(opts Options) (*Server, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = contractb.RelayPingInterval
	}
	if opts.PeerTimeout <= 0 {
		opts.PeerTimeout = contractb.PeerTimeout
	}
	if opts.ArchiveQueue <= 0 {
		opts.ArchiveQueue = contractb.ArchiveQueueMax
	}
	if opts.StatusCoalesce <= 0 {
		opts.StatusCoalesce = contractb.StatusCoalesce
	}
	if opts.StatusCoalesceMax <= 0 {
		opts.StatusCoalesceMax = contractb.StatusCoalesceMax
	}
	if opts.StatusCoalesceMax < opts.StatusCoalesce {
		// A ceiling under the floor is a configuration that cannot be obeyed. It
		// resolves to "no widening" rather than to a startup failure, because the
		// floor alone is exactly the pre-B29 behaviour and a relay that refused to
		// start over a coalescing knob would take a live map down for it.
		opts.Logger.Warn("relay: --status-coalesce-max-ms is below --status-coalesce-ms; the "+
			"window will not widen",
			"statusCoalesceMs", opts.StatusCoalesce, "statusCoalesceMaxMs", opts.StatusCoalesceMax,
			"remedy", "the ceiling must be at or above the floor (contract-b-m4.md §12, §22 B29)")
		opts.StatusCoalesceMax = opts.StatusCoalesce
	}
	if opts.StatusChurnBurstThreshold <= 0 {
		opts.StatusChurnBurstThreshold = contractb.StatusChurnBurstThreshold
	}
	if opts.StatsBroadcast <= 0 {
		opts.StatsBroadcast = contractb.StatsBroadcastInterval
	}
	if opts.ForwardRecordRetention <= 0 {
		opts.ForwardRecordRetention = contractb.ForwardRecordRetention
	}
	if opts.Credentials == nil {
		store, err := peercred.OpenStore(opts.DataDir)
		if err != nil {
			return nil, err
		}
		opts.Credentials = store
	}
	if opts.MinContractVersion != "" {
		if _, err := wire.CompareProtocol(wire.ProtocolB, opts.MinContractVersion); err != nil {
			return nil, fmt.Errorf("relay: --min-contract-version %q is not comparable with %q: %w",
				opts.MinContractVersion, wire.ProtocolB, err)
		}
	}
	if err := opts.PublicEnrollment.applyDefaults(); err != nil {
		return nil, err
	}
	opts.Limits.ApplyDefaults()
	if opts.Limits.MaxFramesPerSecond < contractb.MinimumMaxFramesPerSecond {
		return nil, fmt.Errorf("relay: maxFramesPerSecond must be at least %d to preserve "+
			"migration control-frame headroom (got %d)",
			contractb.MinimumMaxFramesPerSecond, opts.Limits.MaxFramesPerSecond)
	}
	if opts.Limits.MaxFramesPerSecond > int64(^uint(0)>>1) {
		return nil, fmt.Errorf("relay: maxFramesPerSecond %d cannot size a transport queue on this platform",
			opts.Limits.MaxFramesPerSecond)
	}
	grid, err := LoadGrid(opts.DataDir)
	if err != nil {
		return nil, err
	}
	// A production data directory must have a durable map even before its first
	// reservation. Besides making the empty map survive as explicit state, this
	// gives the identity backup both files from its first run and proves at
	// startup that ring.json can be replaced and the directory can be synced.
	if err := grid.Save(); err != nil {
		return nil, fmt.Errorf("relay: initialize ring.json: %w", err)
	}
	s := &Server{
		log:               opts.Logger,
		creds:             opts.Credentials,
		insecureNoToken:   opts.InsecureNoToken,
		minContract:       opts.MinContractVersion,
		limits:            opts.Limits,
		advertiseURL:      opts.AdvertiseURL,
		pingInterval:      opts.PingInterval,
		peerTimeout:       opts.PeerTimeout,
		archiveQueue:      opts.ArchiveQueue,
		statusCoalesce:    opts.StatusCoalesce,
		statusCoalesceMax: opts.StatusCoalesceMax,
		churnThreshold:    opts.StatusChurnBurstThreshold,
		coalesceWindow:    opts.StatusCoalesce,
		statsBroadcast:    opts.StatsBroadcast,
		forwardRetain:     opts.ForwardRecordRetention,
		wireCompression:   !opts.NoWireCompression,
		publicEnrollment:  opts.PublicEnrollment,
		enrollmentByAddr:  map[string][]time.Time{},
		sessionID:         wire.NewUUID(),
		stop:              make(chan struct{}),
		grid:              grid,
		connections:       map[*peer]struct{}{},
		peers:             map[string]*peer{},
		subscribers:       map[string]*peer{},
		meta:              map[string]*peerMeta{},
		forwarded:         map[string]time.Time{},
		inherited:         map[string]bool{},
		perAddress:        newSlotCounter(),
		perPeer:           newSlotCounter(),
		evictions:         map[string]eviction{},
		adminTokens:       map[string]*adminPending{},
	}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.publishLoop() }()
	return s, nil
}

// SessionID is the relay's forwarding-record scope (§5.2). It is exported for
// tests and for an operator who has to reason about a proof.
func (s *Server) SessionID() string { return s.sessionID }

// Credentials is the §3.1 verifier store, for the operator commands in Main that
// mint a join string, remint one on a handover, and drop the identity that gave
// a slot up. Nothing on the serving path calls it.
func (s *Server) Credentials() *peercred.Store { return s.creds }

// CheckServable is §3.1's *No credential store configured* row: THE RELAY MUST
// REFUSE TO START, unless --insecure-no-token. A relay with an empty store can
// admit nobody, so serving is a listener that answers 401 to every peer on the
// map — a failure that looks like a network problem and is a configuration one.
//
// It is a check on SERVING rather than on construction, and that is not a
// technicality: minting the first join string needs the same Server, and a relay
// that could not be built before it had a credential could never be given one.
func (s *Server) CheckServable() error {
	if s.insecureNoToken || s.creds.Len() > 0 {
		return nil
	}
	return errors.New("relay: the credential store holds no credentials, so this relay can " +
		"admit nobody. Mint the join strings first — multiverse-relay --mint-credential " +
		"<peerId> [--grant peer|subscribe|admin], or --reserve-slot <peerId>, which mints one " +
		"with the reservation. --insecure-no-token is for a single-machine test rig and binds " +
		"loopback only")
}

// MinContractVersion is B25's published floor, or "" for no minimum.
func (s *Server) MinContractVersion() string { return s.minContract }

// ReleaseSlot implements the operator escape hatch of §7.5. It is a startup
// command, not a wire message: an operator command is a rare, deliberate,
// physical act on the machine that owns the data, and giving it a network
// surface in a milestone with one shared token is a poor trade.
func (s *Server) ReleaseSlot(slot int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		// §7.5: releasing a slot whose peer is live is a mis-operation.
		return fmt.Errorf("relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	s.grid.Release(slot)
	if err := s.grid.Save(); err != nil {
		return err
	}
	s.log.Warn("relay: released a slot by operator command",
		"slot", res.Slot, "position", res.Position(), "peer", res.PeerID,
		"slotCount", s.grid.Size(), "positionIsNowAHole", res.Position())
	s.markChurnLocked()
	return nil
}

// HandoverSlot rebinds a reservation — slot number AND position — to a
// different peerId (§7.5, new in M4). The map does not change shape and no lane
// moves.
//
// The relay refuses a handover while the old peer is live: a live peer with its
// slot taken out from under it would keep claiming, keep being refused, and
// keep a world running with nowhere to export.
func (s *Server) HandoverSlot(slot int, newPeerID string) (Reservation, Reservation, error) {
	if newPeerID == "" {
		return Reservation{}, Reservation{}, errors.New("relay: --handover-slot needs a new peer id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.grid.ResOfSlot(slot)
	if !ok {
		return Reservation{}, Reservation{}, errors.New("relay: no such slot")
	}
	if _, live := s.peers[res.PeerID]; live {
		return Reservation{}, Reservation{}, fmt.Errorf(
			"relay: slot %d is held by a live peer (%s); stop it first", slot, res.PeerID)
	}
	if held := s.grid.SlotOfPeer(newPeerID); held > 0 {
		return Reservation{}, Reservation{}, fmt.Errorf(
			"relay: %s already holds slot %d; a peer holds at most one", newPeerID, held)
	}
	old, now, _ := s.grid.Handover(slot, newPeerID)
	if err := s.grid.Save(); err != nil {
		s.grid.Handover(slot, old.PeerID)
		return Reservation{}, Reservation{}, err
	}
	// The old identity keeps its journal and its genome cache; nothing about the
	// handover moves data. Its meta is dropped so the new occupant does not
	// inherit a stale version, size or stats block.
	delete(s.meta, old.PeerID)
	s.inherited[newPeerID] = true
	s.log.Warn("relay: handed a slot to a new peer identity by operator command",
		"slot", now.Slot, "position", now.Position(), "from", old.PeerID, "to", newPeerID)
	s.markChurnLocked()
	return old, now, nil
}

// ReserveSlot pre-seeds one reservation for a peer that has not connected yet,
// optionally at a named position. Like ReleaseSlot it is a startup command.
//
// Why the map needs it. Auto-placement (§7.2 rule 6) makes slot order start
// order, and the rig on one machine simply starts its sidecars in the order it
// wants. Across a LAN that is not available: the second computer is started by
// a person, and demanding that they start it after slot 1 and before slot 3
// makes the map depend on human timing. Pre-seeding removes the ordering
// constraint completely — rule 1 then hands each peer the slot that is already
// keyed to its peerId, whenever it arrives.
//
// It is idempotent: a peerId that already holds a slot keeps it, and created is
// false.
func (s *Server) ReserveSlot(peerID string, at *contractb.Position) (Reservation, bool, error) {
	if peerID == "" {
		return Reservation{}, false, errors.New("relay: --reserve-slot needs a peer id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if res, ok := s.grid.ResOfPeer(peerID); ok {
		return res, false, nil
	}
	res := s.grid.Place(peerID, Preference{Position: at})
	if err := s.grid.Save(); err != nil {
		s.grid.Release(res.Slot)
		return Reservation{}, false, err
	}
	// A reservation is a registry change like any other. At startup, which is
	// where this command lives, nothing is connected and the mark costs nothing;
	// it is here so that the counter means "the registry moved" rather than "the
	// registry moved through one of the paths somebody remembered to annotate".
	s.markChurnLocked()
	return res, true, nil
}

// Snapshot returns the current reservations in structural order.
func (s *Server) Snapshot() []Reservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.Order()
}

// MapShape is the rectangle right now.
func (s *Server) MapShape() contractb.MapShape {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.Shape()
}

// Drain closes migration admission, then asks every current or overlapping
// connection to drain its accepted paced transport queue before close 4005.
// The shared deadline prevents a dead reader from holding shutdown forever. A
// hijacked WebSocket is not tracked by net/http, so http.Server.Shutdown alone
// would leave peers hanging.
func (s *Server) Drain() {
	s.mu.Lock()
	s.draining = true
	conns := make([]*peer, 0, len(s.connections))
	for p := range s.connections {
		conns = append(conns, p)
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), contractb.MigrationDrainTimeout)
	defer cancel()
	var drains sync.WaitGroup
	for _, p := range conns {
		drains.Add(1)
		go func(p *peer) {
			defer drains.Done()
			if err := p.conn.ClosePaced(ctx, contractb.CloseShuttingDown, "relay draining"); err != nil && !errors.Is(err, wsutil.ErrClosed) {
				s.log.Warn("relay: paced connection drain ended before its queue emptied",
					"peer", p.id, "role", p.role, "err", err,
					"attemptedForwardsRemainConservative", true)
			}
		}(p)
	}
	drains.Wait()
	s.Close()
}

// Close promptly stops every remaining connection and the publisher. Drain is
// the only call that preserves paced migration queues; error and test cleanup
// use this prompt path so a bad socket cannot make a close linger.
func (s *Server) Close() {
	s.mu.Lock()
	s.draining = true
	conns := make([]*peer, 0, len(s.connections))
	for p := range s.connections {
		conns = append(conns, p)
	}
	s.mu.Unlock()
	for _, p := range conns {
		p.conn.Close(contractb.CloseShuttingDown, "relay stopping")
	}
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// Handler returns the relay's HTTP handler, serving contract-b-m4.md's path and
// the retired one.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(contractb.ContractBPath, s.serveWS)
	if s.publicEnrollment.Enabled {
		mux.HandleFunc(PublicEnrollmentPath, s.servePublicEnrollment)
	}
	// §3, and again at §22 B32: a relay MUST keep serving EVERY retired path and
	// MUST close every connection on one immediately with 4000, so a sidecar left
	// behind gets the defined loud error instead of a bare HTTP 404.
	for _, path := range contractb.RetiredContractBPaths {
		retired := path
		mux.HandleFunc(retired, func(w http.ResponseWriter, r *http.Request) {
			s.serveRetired(w, r, retired)
		})
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// acceptOptions is the ONE set of upgrade options this relay serves, and every
// websocket.Accept in this package takes it (§3 Transport, §24 B35).
//
// COMPRESSION IS OFFERED, NEVER REQUIRED. permessage-deflate is negotiated, so
// a sidecar that does not offer it is served uncompressed and MUST NOT be
// refused — that is what makes this change invisible to an un-updated
// participant and what lets the fleet cross by publication rather than in
// lockstep.
//
// THE REFUSAL DOORS OFFER IT TOO, AND THAT IS DELIBERATE. serveRetired,
// shedUpgrade and closeEvicted complete the upgrade only to close it, so
// compression saves them nothing — a close frame is a control frame and RFC
// 7692 never compresses one. They share these options because B28 requires an
// evicted peer to see EXACTLY what a draining relay shows it: if a refusal door
// declined the extension while the live door accepted it, the response's
// Sec-WebSocket-Extensions header would separate "refused at the door" from
// "accepted and then closed" before the peer had read a single frame. One set
// of options, one observable handshake, and the close code stays the only
// difference.
//
// InsecureSkipVerify is coder/websocket's ORIGIN check and not a TLS one: a
// Contract B client is a process, not a browser, and it sends no Origin header.
func (s *Server) acceptOptions() *websocket.AcceptOptions {
	opts := &websocket.AcceptOptions{InsecureSkipVerify: true}
	if s.wireCompression {
		return wsutil.PeerAcceptOptions(opts)
	}
	// --ws-compression=false. CompressionDisabled is the zero value, so the
	// relay simply never echoes the extension and every peer, old or new, runs
	// on the pre-§24 wire.
	return opts
}

func (s *Server) serveRetired(w http.ResponseWriter, r *http.Request, path string) {
	// A retired path is served over TLS like the live one (B23), so the same
	// transport rule applies to it: a plaintext upgrade off loopback is refused
	// before it becomes a socket.
	if !s.allowPlainUpgrade(w, r) {
		return
	}
	ws, err := websocket.Accept(w, r, s.acceptOptions())
	if err != nil {
		return
	}
	s.log.Error("relay: refusing a connection on a retired contract-b path",
		"remote", r.RemoteAddr, "path", path,
		"hint", "this relay speaks "+wire.ProtocolB+" on "+contractb.ContractBPath)
	_ = ws.Close(contractb.CloseProtocolUnsupported,
		path+" is retired; this relay serves "+contractb.ContractBPath)
}

// allowPlainUpgrade is B23's scheme rule, and it is the ONE place the transport
// decides anything.
//
//	"Clients dial wss://. A public relay MUST refuse a plain ws:// upgrade —
//	 HTTP 426 with Upgrade: TLS/1.2, HTTP/1.1, NOT A REDIRECT, because a redirect
//	 to a scheme the client did not ask for is how a downgrade goes unnoticed.
//	 Plain ws:// survives ONLY on a loopback bind for a single-machine
//	 rehearsal."
//
// The test is the LOCAL address of this connection, not a flag and not a
// configured bind string, and that choice is what makes it composable with both
// deployment shapes §3 names. A relay that terminates its own TLS answers
// r.TLS != nil and never reaches here. A relay behind a fronting proxy that
// terminates for it is reached over loopback, and loopback is the carve-out. A
// relay serving plaintext on a reachable interface is the one shape B23 refuses,
// and it is refused per connection, so a single-machine rehearsal on the same
// process keeps working over 127.0.0.1 while a remote peer is told exactly what
// to do about it.
func (s *Server) allowPlainUpgrade(w http.ResponseWriter, r *http.Request) bool {
	if r.TLS != nil || localIsLoopback(r) {
		return true
	}
	// 426 and not 301/302/307: a redirect to a scheme the client did not ask for
	// is how a downgrade goes unnoticed.
	w.Header().Set("Upgrade", "TLS/1.2, HTTP/1.1")
	w.Header().Set("Connection", "Upgrade")
	http.Error(w, "this relay is reachable off loopback and speaks TLS only; dial wss://",
		http.StatusUpgradeRequired)
	s.log.Error("relay: refusing a plain ws:// upgrade on a non-loopback address",
		"remote", r.RemoteAddr, "local", localAddr(r), "status", http.StatusUpgradeRequired,
		"remedy", "run this relay with --tls-cert/--tls-key, or put it behind a proxy that "+
			"terminates TLS and reaches it over loopback; the client dials wss:// either way "+
			"(contract-b-m4.md §22, B23)")
	return false
}

func localAddr(r *http.Request) string {
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && a != nil {
		return a.String()
	}
	return ""
}

func localIsLoopback(r *http.Request) bool {
	addr := localAddr(r)
	if addr == "" {
		// No local address means no listener the relay can reason about — an
		// in-process handler under httptest, for instance. Resolve toward the safe
		// answer for a wire rule and toward the usable one for a rehearsal: this is
		// not a socket a stranger reached.
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// authed is what the HTTP upgrade proved about a connection, carried into the
// handshake where the frame that claims a peerId finally arrives.
type authed struct {
	// peerID is the id the CREDENTIAL named — never the one the frame claims.
	peerID string
	grant  string
	// checked is false only under --insecure-no-token, where nothing was proved
	// and the binding cannot be enforced.
	checked bool
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	// B23 first: a wire this relay will not serve is refused before its
	// credential is read off it.
	if !s.allowPlainUpgrade(w, r) {
		return
	}
	// B24, maxConnectionsPerAddress, and it is the FIRST limit because it is the
	// only one that costs nothing to enforce: the relay has the source address
	// before it has read a header, so the ninth socket from one machine is
	// refused without a digest, an upgrade or a goroutine. It is the one limit
	// answered with HTTP 429 rather than a close code, because there is no
	// WebSocket yet to close (§3.3).
	src := sourceKey(r)
	n, ok := s.perAddress.acquire(src, s.limits.MaxConnectionsPerAddress)
	defer s.perAddress.release(src)
	if !ok {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "too many simultaneous connections from this address",
			http.StatusTooManyRequests)
		s.log.Error("relay: refusing an upgrade over maxConnectionsPerAddress",
			"remote", r.RemoteAddr, "open", n, "maxConnectionsPerAddress", s.limits.MaxConnectionsPerAddress,
			"status", http.StatusTooManyRequests,
			"remedy", "raise --max-connections-per-address if one machine legitimately runs this "+
				"many peers; the rig itself runs five (contract-b-m4.md §3.3)")
		return
	}
	// §3.1: the credential is checked on the HTTP UPGRADE. A missing, malformed
	// or wrong one gets HTTP 401 with WWW-Authenticate: Bearer and NO upgrade, so
	// there is no WebSocket and there is no close code — deliberately, because a
	// refusal before the upgrade is the one refusal that costs the relay nothing.
	var auth authed
	if s.insecureNoToken {
		s.log.Warn("relay: accepting a connection with NO CREDENTIAL CHECK; " +
			"--insecure-no-token is for a single-machine test rig and never for a map with peers on it")
	} else {
		peerID, grant, ok := s.creds.Verify(peercred.FromRequest(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			// The log names no peerId, because the id on a refused credential is
			// attacker-chosen text and echoing it turns the log into a search index
			// of guesses.
			s.log.Error("relay: refused a connection whose credential did not verify",
				"remote", r.RemoteAddr,
				"remedy", "the peer's own operator re-applies its join string, or asks this "+
					"relay's operator for a slot handover (contract-b-m4.md §3.1)")
			return
		}
		auth = authed{peerID: peerID, grant: grant, checked: true}
	}
	// B24, maxConnectionsPerPeer, and it applies EXACTLY when the connection is
	// authenticated, because §3.3 counts "simultaneous authenticated
	// connections". Under --insecure-no-token nothing was authenticated and
	// there is no identity to count against, which is one more thing that flag
	// switches off and one more reason it binds loopback only.
	//
	// The refusal is a close and not a 429: this connection IS a WebSocket by
	// the time the count is known to matter, and §3.2 gives capacity its own
	// close code so a client can tell it from a credential failure.
	if auth.checked {
		open, within := s.perPeer.acquire(auth.peerID, s.limits.MaxConnectionsPerPeer)
		defer s.perPeer.release(auth.peerID)
		if !within {
			breach := fmt.Sprintf("maxConnectionsPerPeer %d exceeded (%d open for this peerId)",
				s.limits.MaxConnectionsPerPeer, open)
			s.log.Error("relay: shedding a connection over maxConnectionsPerPeer",
				"peer", auth.peerID, "open", open,
				"maxConnectionsPerPeer", s.limits.MaxConnectionsPerPeer,
				"remedy", "one peer runs one sidecar; the second connection is the 4006 overlap "+
					"during a reconnect (contract-b-m4.md §3.3)")
			s.shedUpgrade(w, r, auth.peerID, breach)
			return
		}
		// B28's eviction, checked at the door for the same reason the credential
		// is: a peer refused for its admission never reaches the registry, so
		// nothing on the map moves when it tries.
		if until, evicted := s.evictedUntil(auth.peerID); evicted {
			s.closeEvicted(w, r, auth.peerID, until)
			return
		}
	}
	ws, err := websocket.Accept(w, r, s.acceptOptions())
	if err != nil {
		s.log.Warn("relay: websocket upgrade failed", "err", err)
		return
	}
	// maxFrameBytes is a knob rather than a constant from B24 onward (§3.3).
	// Over it the library closes 1009 TOO_BIG, which is what §3.2 asks for. The
	// paced destination queue reuses the published one-second frame and byte
	// ceilings as its retention bounds; it introduces no hidden capacity knob.
	paceBinding := &migrationPaceBinding{}
	conn := wsutil.NewPacedLimited(ws, 128, s.limits.MaxFrameBytes, wsutil.PacedConfig{
		Pacer:        paceBinding,
		MaxFrames:    int(s.limits.MaxFramesPerSecond),
		MaxBytes:     s.limits.MaxBytesPerSecond,
		ControlBurst: int(contractb.MigrationControlBurst),
	})
	s.handle(r.Context(), conn, auth, paceBinding)
}

// shedUpgrade turns a capacity refusal that is known before the handshake into
// the close §3.2 defines for it. The upgrade is completed first and closed
// immediately: a client that was refused with a bare HTTP status would learn
// "something went wrong", and a client that reads 4007 knows to hold at
// relayBackoffMaxMs rather than to hammer.
func (s *Server) shedUpgrade(w http.ResponseWriter, r *http.Request, peerID, breach string) {
	if peerID != "" {
		s.mu.Lock()
		if _, reserved := s.grid.ResOfPeer(peerID); reserved {
			// §6.5: the capacity axis of lastRefusal, so an operator reading the
			// map sees WHICH limit shed this peer rather than a slot that merely
			// went quiet. Only a reserved slot has anywhere to write it.
			s.metaLocked(peerID).lastRefusal = capacityRefusal(breach)
			s.markChurnLocked()
		}
		s.mu.Unlock()
	}
	ws, err := websocket.Accept(w, r, s.acceptOptions())
	if err != nil {
		return
	}
	_ = ws.Close(contractb.CloseCapacity, breach)
}

func (s *Server) handle(ctx context.Context, conn *wsutil.Conn, auth authed,
	paceBinding *migrationPaceBinding) {
	// B24's frame and byte meters live for exactly as long as the connection
	// they measure, and they start counting at the HANDSHAKE: §3.3 says "frames
	// of any type", and a peer that could spend its whole allowance before it
	// identified itself would have found the gap rather than the ceiling.
	meter := newConnMeter()
	p, err := s.handshake(ctx, conn, auth, meter, paceBinding)
	if err != nil {
		s.log.Warn("relay: handshake failed", "err", err)
		<-conn.Done()
		return
	}
	// The writer can stop while this handler is pacing an accepted migration
	// for another destination. Remove the exact dead pointer promptly; the
	// deferred call below is an idempotent fallback for every other exit path.
	go func() {
		<-p.conn.Done()
		s.drop(p)
	}()
	defer s.drop(p)

	go s.pingLoop(p)
	if p.role == contractb.RoleArchive {
		go s.copyLoop(p)
	}

	for {
		readCtx, cancel := context.WithTimeout(ctx, s.peerTimeout)
		frame, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				s.log.Warn("relay: peer silent, dropping", "peer", p.id)
				conn.Close(contractb.CloseLivenessTimeout, "no traffic within peerTimeoutMs")
			}
			<-conn.Done()
			return
		}
		// The count happens BEFORE the dispatch, on the frame's LENGTH and on
		// nothing inside it. That ordering is what makes the limit countable at
		// the frame level (D1): a relay that decoded first and counted second
		// would have paid the decode it is trying to bound.
		if breach := meter.observe(time.Now(), len(frame), s.limits); breach != "" {
			s.shedForCapacity(p, breach)
			<-conn.Done()
			return
		}
		s.touch(p.id)
		if !s.dispatch(p, frame) {
			<-conn.Done()
			return
		}
	}
}

// shedForCapacity is B24's "the relay sheds the connection, never the map".
// This peer's socket closes with 4007 and its slot's lastRefusal names the
// limit. No other peer's connection is shed and no lane closes. Accepted
// migrations queued to this connection follow B43's conservative connection-
// error drop rule. Its neighbours route around it exactly as they route around
// any dark peer (§8).
func (s *Server) shedForCapacity(p *peer, breach string) {
	s.mu.Lock()
	if p.role == contractb.RolePeer {
		s.metaLocked(p.id).lastRefusal = capacityRefusal(breach)
		s.markChurnLocked()
	}
	s.mu.Unlock()
	s.log.Error("relay: shedding a connection over a published capacity limit",
		"peer", p.id, "role", p.role, "breach", breach,
		"remedy", "the peer's own operator slows this connection down, or this relay's operator "+
			"raises the limit and restarts; the published limits object on HANDSHAKE_ACK and "+
			"PEER_STATUS is what a peer must be built against (contract-b-m4.md §3.3, §22 B24)")
	p.conn.Close(contractb.CloseCapacity, breach)
}

// handshake reads the mandatory first frame and registers the client.
func (s *Server) handshake(ctx context.Context, conn *wsutil.Conn, auth authed,
	meter *connMeter, paceBinding *migrationPaceBinding) (*peer, error) {
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	frame, err := conn.Read(readCtx)
	if err != nil {
		conn.Close(contractb.CloseMalformedFrame, "no HANDSHAKE")
		return nil, err
	}
	if breach := meter.observe(time.Now(), len(frame), s.limits); breach != "" {
		// A single frame can only break the BYTE rate, and only when the
		// operator set maxBytesPerSecond below maxFrameBytes. It is still a
		// capacity shed and it still says which limit fired.
		s.log.Error("relay: shedding a connection over a published capacity limit at the handshake",
			"credentialPeer", auth.peerID, "breach", breach)
		conn.Close(contractb.CloseCapacity, breach)
		return nil, errors.New("relay: " + breach)
	}
	env, err := wire.Decode(frame)
	if err != nil {
		conn.Close(contractb.CloseMalformedFrame, "malformed first frame")
		return nil, err
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolB); err != nil {
		conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocol major")
		return nil, err
	}
	if env.Type != contractb.TypeHandshake {
		conn.Close(contractb.CloseMalformedFrame, "first frame is not HANDSHAKE")
		return nil, errors.New("relay: first frame is not HANDSHAKE")
	}
	var hs contractb.Handshake
	if err := contractb.DecodeData(env.Data, &hs); err != nil {
		conn.Close(contractb.CloseMalformedFrame, "malformed HANDSHAKE")
		return nil, err
	}
	if err := hs.Validate(); err != nil {
		conn.Close(contractb.CloseMalformedFrame, err.Error())
		return nil, err
	}

	// ------------------------------------------------- §3.1's whole security property
	//
	// THE BINDING. The credential named a peerId on the upgrade; this frame
	// claims one. They must be the same, and a connection where they differ is
	// refused HERE, at the handshake, with 4003.
	//
	// AND THE PEER WHOSE ID WAS BORROWED OBSERVES NOTHING AT ALL: no close, no
	// 4006, no PEER_STATUS change, no lastRefusal on its slot. That is why this
	// check sits ahead of every line that touches s.meta, s.peers or the publish
	// counters —
	// the refusal must be unable to reach the map, not merely decline to publish
	// to it. Under M4's shared token this exact sequence took a peer off the map
	// in one frame (§3.1, §6.1; m5_considerations.md, Risk 1; D21).
	//
	// Writing the refusal on the borrowed slot would be worse than useless twice
	// over: it would tell an innocent peer it had been attacked, and it would
	// hand the attacker a confirmation surface for a guessed peerId (§6.5, B22).
	if auth.checked && hs.PeerID != auth.peerID {
		s.log.Error("relay: refusing a handshake whose peerId does not match its credential",
			"credentialPeer", auth.peerID, "claimedPeer", hs.PeerID, "role", hs.Role,
			"note", "the peer whose id was claimed is not told and is not touched")
		conn.Close(contractb.CloseMalformedFrame, "peerId does not match the authenticated credential")
		return nil, errors.New("relay: peerId does not match the authenticated credential")
	}

	// §5.1 B27 / §7.5 B28: THE GRANT DECIDES THE ROLE, and the three grants are
	// disjoint. A subscribe credential cannot claim a slot and a peer credential
	// cannot subscribe, so neither compromise becomes the other. Nothing appears
	// on any slot's lastRefusal for this either: it is a role error on ONE
	// connection, not a refusal of that peer (B27's worked example).
	if auth.checked && !peercred.GrantAllowsRole(auth.grant, hs.Role) {
		reason := "credential for " + auth.peerID + " does not carry the " +
			grantForRole(hs.Role) + " grant"
		s.log.Error("relay: refusing a role the credential's grant does not carry",
			"peer", auth.peerID, "role", hs.Role, "grant", auth.grant,
			"needs", grantForRole(hs.Role))
		conn.Close(contractb.CloseMalformedFrame, reason)
		return nil, errors.New("relay: " + reason)
	}

	if err := wire.CheckProtocol(hs.ProtocolVersion, wire.ProtocolB); err != nil {
		conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocolVersion")
		return nil, err
	}

	p := &peer{id: hs.PeerID, role: hs.Role, conn: conn}
	if p.role == contractb.RoleArchive {
		p.out = make(chan []byte, s.archiveQueue)
	}

	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		conn.Close(contractb.CloseShuttingDown, "relay draining")
		return nil, errors.New("relay: draining")
	}
	// B25's floor, and it sits BESIDE §6.1's game-version refusal rather than
	// replacing it: two axes, two tests, and D22 is the decision that they never
	// meet. This one is the map's MEMBERSHIP test — may this build join this map —
	// and it is a COMPATIBILITY control and NEVER a security one, because
	// protocolVersion is chosen by the peer that sends it and anyone who edits one
	// string walks through it. It keeps the HONESTLY stale peer off a map it would
	// degrade, which is the failure dev_environment.md's *The minors* records.
	if s.minContract != "" {
		if cmp, err := wire.CompareProtocol(hs.ProtocolVersion, s.minContract); err == nil && cmp < 0 {
			if p.role == contractb.RolePeer {
				s.metaLocked(p.id).lastRefusal = contractb.RefusalContractVersion + ": " +
					hs.ProtocolVersion + " < " + s.minContract
				s.markChurnLocked()
			}
			s.mu.Unlock()
			s.log.Error("relay: refusing a peer below this relay's minimum contract version",
				"peer", p.id, "peerContractVersion", hs.ProtocolVersion,
				"minContractVersion", s.minContract,
				"remedy", "the peer's OWN operator upgrades from the published release; nobody on "+
					"this relay's side can do it for them (D25)")
			conn.Close(contractb.CloseMalformedFrame,
				"protocolVersion "+hs.ProtocolVersion+" is below this relay's minimum "+s.minContract)
			return nil, errors.New("relay: protocolVersion below the published minimum")
		}
	}
	// §6.1: compatibility enforcement at connect, and it must be loud. A silent
	// version mismatch is indistinguishable from a dead peer — under M4 both end
	// with a lane routed around them — and M4 crosses two independently updated
	// installs, so this is the failure most likely to waste an evening.
	//
	// §22 DOES NOT TOUCH THIS RULE and that is deliberate (B31). D22 makes the
	// CONTRACT version the map's membership test and the game version a
	// per-machine matter answered by a published support matrix, which reads like
	// a reason to retire this paragraph — and the owner decided on 2026-08-11 not
	// to. It is the fourth of the four kept exceptions to the game version's
	// diagnostic-only rule, and what it costs is a map that PARTITIONS along a
	// version boundary after a staggered game update.
	if p.role == contractb.RolePeer {
		if mapVersion := s.mapVersionLocked(""); mapVersion != "" && hs.GameVersion != "" &&
			mapVersion != hs.GameVersion {
			s.metaLocked(p.id).lastRefusal = contractb.RefusalGameVersion + ": " +
				hs.GameVersion + " != the map's " + mapVersion
			s.markChurnLocked()
			s.mu.Unlock()
			s.log.Error("relay: refusing a peer on gameVersion grounds",
				"peer", p.id, "peerGameVersion", hs.GameVersion, "mapGameVersion", mapVersion)
			conn.Close(contractb.CloseMalformedFrame,
				"gameVersion "+hs.GameVersion+" is incompatible with the map's "+mapVersion)
			return nil, errors.New("relay: incompatible gameVersion")
		}
	}

	// B28's eviction again, for the one path that could not check it at the
	// door: under --insecure-no-token nothing was authenticated, so the peerId
	// is not known until this frame. An evicted peer gets 4005 here too.
	if !auth.checked {
		if until, evicted := s.evictionLocked(p.id); evicted {
			s.mu.Unlock()
			s.log.Warn("relay: refusing an evicted peer", "peer", p.id, "until", evictionUntil(until))
			conn.Close(contractb.CloseShuttingDown, "this relay is not accepting this peer")
			return nil, errors.New("relay: peer is evicted")
		}
	}

	// B24, maxSubscribers: it bounds the FAN-OUT COST of §5.1's copy queues, and
	// B27's grant is what bounds the trust. A subscriber already in the registry
	// is REPLACING ITSELF (§6.1) and is not a new one, so a flapping archive
	// never eats its own ceiling.
	if p.role == contractb.RoleArchive {
		if _, already := s.subscribers[p.id]; !already &&
			int64(len(s.subscribers)) >= s.limits.MaxSubscribers {
			open := len(s.subscribers)
			s.mu.Unlock()
			breach := fmt.Sprintf("maxSubscribers %d exceeded (%d already connected)",
				s.limits.MaxSubscribers, open)
			s.log.Error("relay: shedding a subscriber over maxSubscribers",
				"peer", p.id, "open", open, "maxSubscribers", s.limits.MaxSubscribers,
				"remedy", "raise --max-subscribers, or stop an archive that is no longer read; "+
					"each subscriber costs one archiveQueueMax copy queue (contract-b-m4.md §3.3)")
			conn.Close(contractb.CloseCapacity, breach)
			return nil, errors.New("relay: " + breach)
		}
	}

	registry := s.peers
	if p.role == contractb.RoleArchive {
		registry = s.subscribers
	}
	// Bind the writer to this identity's shared physical migration schedule
	// before the connection can become a routing target. The binding contains
	// no frame and survives only through the identity metadata.
	paceBinding.bind(s.migrationPaceLocked(p.id))
	ack := contractb.HandshakeAck{
		RelayVersion:    Version,
		ProtocolVersion: wire.ProtocolB,
		RelaySessionID:  s.sessionID,
		Map:             s.grid.Shape(),
		SlotCount:       s.grid.Size(),
		ReceivedAt:      time.Now().UnixMilli(),
		// B25: published at connect so a peer can say what it will need BEFORE it
		// needs it. Absent is no minimum.
		MinContractVersion: s.minContract,
		// B24: the FIRST thing on this wire the relay tells a peer about the
		// relay, and it is here rather than on a later frame because a peer that
		// learns its ceilings after it has already exceeded them learns them from
		// a 4007. What is published is what this relay is RUNNING with.
		Limits: s.limits.Published(),
	}
	if p.role == contractb.RolePeer {
		if res, ok := s.grid.ResOfPeer(p.id); ok {
			slot, pos := res.Slot, res.Position()
			ack.AssignedSlot = &slot
			ack.AssignedPosition = &pos
		}
	}
	ackFrame := mustFrame(s.log, contractb.TypeHandshakeAck, ack)
	if ackFrame == nil {
		s.mu.Unlock()
		conn.Close(contractb.CloseShuttingDown, "relay could not encode HANDSHAKE_ACK")
		return nil, errors.New("relay: could not encode HANDSHAKE_ACK")
	}
	// Queue HANDSHAKE_ACK before this connection becomes a routing target. A
	// destination can answer a migration immediately, so publishing p first can
	// put that reply ahead of the mandatory ACK on p's outbound queue.
	if err := p.conn.Send(ackFrame); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("relay: send HANDSHAKE_ACK: %w", err)
	}
	s.connections[p] = struct{}{}
	if old, ok := registry[p.id]; ok {
		// §6.1: a newer connection THAT AUTHENTICATED AS this peerId takes over,
		// which makes a crashed-and-restarted sidecar self-healing.
		//
		// The narrowing of §22 B22 is enforced above and not here, and it is worth
		// saying where: p.id is the id the credential proved, because a connection
		// whose frame disagreed with its credential never reached this line. A
		// connection presenting somebody else's peerId therefore never reaches this
		// rule at all — it is refused at the credential check, and the live peer is
		// not told, because nothing happened to it.
		old.conn.Close(contractb.CloseReplaced, "a newer connection claimed this peerId")
	}
	registry[p.id] = p

	m := s.metaLocked(p.id)
	m.lastRefusal = ""
	m.lastSeenMs = time.Now().UnixMilli()
	m.darkSinceMs = 0
	if p.role == contractb.RolePeer {
		if hs.GameVersion != "" {
			m.gameVersion = hs.GameVersion
		}
		if hs.SimulationSize > 0 {
			m.simSize = hs.SimulationSize
		}
	}
	s.markChurnLocked()
	s.mu.Unlock()

	s.log.Info("relay: client connected", "peer", p.id, "role", p.role,
		"assignedSlot", derefSlot(ack.AssignedSlot), "map", ack.Map, "slotCount", ack.SlotCount,
		"relaySessionId", s.sessionID)
	return p, nil
}

// mapVersionLocked is the game version the map is running: the first non-empty
// version among live peers other than self, in structural order.
func (s *Server) mapVersionLocked(self string) string {
	for _, res := range s.grid.Slots {
		if res.PeerID == self {
			continue
		}
		if _, live := s.peers[res.PeerID]; !live {
			continue
		}
		if m, ok := s.meta[res.PeerID]; ok && m.gameVersion != "" {
			return m.gameVersion
		}
	}
	for id := range s.peers {
		if id == self {
			continue
		}
		if m, ok := s.meta[id]; ok && m.gameVersion != "" {
			return m.gameVersion
		}
	}
	return ""
}

// markChurnLocked records a REGISTRY change: a placement, a release, a
// handover, a peer arriving or going dark, an eviction, or a refusal written to
// a slot. It bumps both counters, because a registry change is both something
// the next broadcast must carry and something B29 widens the window on.
func (s *Server) markChurnLocked() {
	s.churn++
	s.pending++
}

// THERE IS NO markStatsLocked, AND THAT IS THE POINT (§6.5, §14 B4; §24, B36).
//
// A STATS ARRIVAL SCHEDULES NOTHING. It is neither churn nor pending: it does
// not change what the map IS, and §6.5 gives it one publisher by name — "also
// sent on a statsBroadcastIntervalMs timer, BECAUSE STATS CHANGE WITHOUT THE
// REGISTRY CHANGING". Storing the block against the peer is all an arrival
// does; the next broadcast, timer-driven or registry-driven, carries whatever
// is stored. Nothing is delayed by not scheduling — only the extra frame is.
//
// UNTIL §24 THIS BUMPED pending, and the cost was the map's whole broadcast
// rate. Stats arrivals do not line up: seven peers each PINGing once every
// statsIntervalMs land in seven different coalescing windows, so the relay
// published PEER_STATUS at 1.32/s measured, one map-wide frame per arrival,
// where §6.5 designs for one per 5 s. On the 2026-08-16 capture that was
// ~246 GiB a month of PEER_STATUS the contract never asked for — and
// publishLoop below already said so in its own words: the stats timer is "A
// FLOOR ON FRESHNESS, NOT A SECOND SOURCE OF FRAMES".
//
// WHAT IT COSTS TO STOP. Stats freshness on the map moves from ~0.76 s to at
// most statsBroadcastIntervalMs (5 s). §12's statsStaleMs is 30 s, so no
// reader's staleness rule changes and nothing renders as unknown that did not
// before. statsAsOfMs is unaffected in meaning: §6.5 defines it as the relay
// clock when the block ARRIVED, never when it was published, so a reader ages
// the stats from the arrival either way.

func (s *Server) metaLocked(peerID string) *peerMeta {
	m, ok := s.meta[peerID]
	if !ok {
		m = &peerMeta{}
		s.meta[peerID] = m
	}
	return m
}

// claimMeterLocked is maxClaimsPerMinute's per-peer counter, created on first
// use so a relay that has never seen a peer carries no counter for it.
func (s *Server) claimMeterLocked(peerID string) *claimMeter {
	m := s.metaLocked(peerID)
	if m.claims == nil {
		m.claims = newClaimMeter()
	}
	return m.claims
}

// migrationPaceLocked returns the physical-write schedule for one destination
// identity. Overlapping old and new connections receive the same pointer, so a
// reconnect cannot refill the allowance. The connection writer calls Reserve
// without holding Server.mu or Conn.mu.
func (s *Server) migrationPaceLocked(peerID string) *migrationForwardPace {
	m := s.metaLocked(peerID)
	if m.migrationPace == nil {
		m.migrationPace = &migrationForwardPace{
			interval: contractb.MigrationFanInInterval(s.limits.MaxFramesPerSecond),
		}
	}
	return m.migrationPace
}

// Limits is §3.3's table as this relay is running it, for the operator console
// and for a test that has to assert what was published.
func (s *Server) Limits() contractb.Limits { return s.limits }

func (s *Server) touch(peerID string) {
	s.mu.Lock()
	s.metaLocked(peerID).lastSeenMs = time.Now().UnixMilli()
	s.mu.Unlock()
}

// dispatch handles one frame. It returns false when the connection must end.
func (s *Server) dispatch(p *peer, frame []byte) bool {
	env, err := wire.Decode(frame)
	if err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed frame")
		return false
	}
	if err := wire.CheckProtocol(env.Protocol, wire.ProtocolB); err != nil {
		p.conn.Close(contractb.CloseProtocolUnsupported, "unsupported protocol major")
		return false
	}

	switch env.Type {
	case contractb.TypeSectorClaim:
		return s.onSectorClaim(p, env)
	case contractb.TypeMigrationPayload:
		return s.onMigrationPayload(p, env, frame)
	case contractb.TypeMigrationAck, contractb.TypeMigrationNack:
		if p.role == contractb.RoleArchive {
			// §5.1: a subscriber MUST NOT answer a copied frame.
			s.log.Warn("relay: ignoring an answer from a read-only subscriber",
				"peer", p.id, "type", env.Type)
			return true
		}
		return s.onDirected(p, env, frame, true)
	case contractb.TypeGenomeRequest:
		return s.onGenomeRequest(p, env, frame)
	case contractb.TypeGenomeResponse:
		if p.role == contractb.RoleArchive {
			s.log.Warn("relay: ignoring a GENOME_RESPONSE from a subscriber", "peer", p.id)
			return true
		}
		return s.onDirected(p, env, frame, false)
	case contractb.TypePing:
		var ping contractb.Ping
		if contractb.DecodeData(env.Data, &ping) == nil {
			// §6.11: a PING from a peer MAY carry the stats block. The relay
			// stores it against that peer with its own receivedAt, republishes
			// it, and never routes, refuses, SCHEDULES or filters on a stat.
			//
			// "Schedules" is load-bearing and it was the bug (§24, B36). Storing
			// the block is the whole of the work here; the frame that carries it
			// out is statsBroadcastIntervalMs's, not this arrival's. See the note
			// beside markChurnLocked above.
			if p.role == contractb.RolePeer && ping.Stats != nil {
				s.mu.Lock()
				m := s.metaLocked(p.id)
				m.stats = ping.Stats
				m.statsAsOfMs = time.Now().UnixMilli()
				s.mu.Unlock()
			}
			s.send(p, contractb.TypePong, contractb.Pong{Nonce: ping.Nonce})
		}
		return true
	case contractb.TypePong:
		return true
	case contractb.TypeHandshake:
		p.conn.Close(contractb.CloseMalformedFrame, "duplicate HANDSHAKE")
		return false
	default:
		// contract-a.md §3.1, mirrored here: an unknown type is a
		// forward-compatible addition, not a fault.
		s.log.Warn("relay: ignoring unknown type", "peer", p.id, "type", env.Type)
		return true
	}
}

func (s *Server) onSectorClaim(p *peer, env wire.Envelope) bool {
	var claim contractb.SectorClaim
	if err := contractb.DecodeData(env.Data, &claim); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed SECTOR_CLAIM")
		return false
	}
	if err := claim.Validate(); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, err.Error())
		return false
	}

	// §7.2 rule 7: a claim from a role:"archive" client never gets a slot.
	if p.role == contractb.RoleArchive {
		s.mu.Lock()
		grant := contractb.SectorGrant{
			Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
			Reason: contractb.GrantRoleHasNoSlot}
		s.mu.Unlock()
		s.answerClaim(p, grant)
		return true
	}

	s.mu.Lock()
	// B24, maxClaimsPerMinute, and it is the ONE limit in §3.3 that does not
	// close: the claim is answered granted:false / rate_limited and the
	// connection stays up. A claim storm is usually a peer whose measured time
	// scale is wandering — DQ3 counted 64 claims from one slot in a day — and a
	// refusal that peer can read beats a close it must recover from.
	//
	// It is checked before placement, because the point is to stop the registry
	// work, not to do it and then complain.
	if breach := s.claimMeterLocked(p.id).observe(time.Now(), s.limits); breach != "" {
		s.metaLocked(p.id).lastRefusal = capacityRefusal(breach)
		grant := contractb.SectorGrant{
			Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
			Reason: contractb.GrantRateLimited}
		s.markChurnLocked()
		s.mu.Unlock()
		s.log.Warn("relay: refusing a claim over maxClaimsPerMinute; the connection stays up",
			"peer", p.id, "breach", breach,
			"remedy", "a claim storm is usually a wandering time scale on that world, not abuse; "+
				"read its stats block before raising --max-claims-per-minute "+
				"(contract-b-m4.md §3.3, §6.4)")
		s.answerClaim(p, grant)
		return true
	}
	// §7.2 rule 8: a peer whose gameVersion disagrees with the map's is refused.
	// This is the ordinary path for the check, because a sidecar usually
	// connects before its mod and only learns a version here.
	if mapVersion := s.mapVersionLocked(p.id); mapVersion != "" &&
		claim.GameVersion != "" && mapVersion != claim.GameVersion {
		s.metaLocked(p.id).lastRefusal = contractb.RefusalGameVersion + ": " +
			claim.GameVersion + " != the map's " + mapVersion
		grant := contractb.SectorGrant{
			Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
			Reason: contractb.GrantVersionIncompatible}
		s.markChurnLocked()
		s.mu.Unlock()
		s.log.Error("relay: refusing a claim on gameVersion grounds",
			"peer", p.id, "peerGameVersion", claim.GameVersion, "mapGameVersion", mapVersion)
		s.answerClaim(p, grant)
		return true
	}

	// B29's second rule needs the shape BEFORE this claim is applied, so it is
	// taken before the first field moves.
	before := s.claimShapeLocked(p.id)

	m := s.metaLocked(p.id)
	m.simSize = claim.SimulationSize
	m.modConnected = claim.ModConnected
	m.exportEdges = append([]string(nil), claim.ExportEdges...)
	m.borderEdges = append([]string(nil), claim.BorderEdges...)
	m.lastRefusal = ""
	if claim.GameVersion != "" {
		m.gameVersion = claim.GameVersion
	}
	if claim.Stats != nil {
		// STATS ARE NOT PART OF THE SHAPE. B29's second rule lists seven fields
		// and none of them is a stat: a claim whose only news is a population
		// count rides the next statsBroadcastIntervalMs timer, which was going to
		// send it anyway (§6.5, §14 B4).
		m.stats = claim.Stats
		m.statsAsOfMs = time.Now().UnixMilli()
	}

	res, reason, placed := s.assignLocked(p, claim)
	if placed {
		// §7.2 ordering step 1: the map is on disk BEFORE the grant goes out. An
		// answered grant that is not durable can hand the same slot to two peers
		// across a restart.
		if err := s.grid.Save(); err != nil {
			s.log.Error("relay: ring.json write failed; refusing the claim", "err", err)
			s.grid.Release(res.Slot)
			grant := contractb.SectorGrant{
				Granted: false, Map: s.grid.Shape(), SlotCount: s.grid.Size(),
				Reason: contractb.GrantProtocolMismatch}
			s.mu.Unlock()
			s.answerClaim(p, grant)
			return true
		}
	}
	p.claimed = true
	grant := s.grantForLocked(res, reason)
	// ------------------------------------------- B29's second broadcast bound
	//
	// A REPEAT CLAIM THAT CHANGES NOTHING STRUCTURAL BROADCASTS NOTHING. A claim
	// answered reason:"updated" whose slot, position, exportEdges, borderEdges,
	// modConnected, gameVersion and simulationSize are all unchanged MUST NOT
	// raise the epoch or trigger a broadcast. The relay still answers the claim
	// with its SECTOR_GRANT below, because THE CLAIMANT IS OWED AN ANSWER; what
	// it stops doing is telling everybody else.
	//
	// This is the epoch rate's measured cause, not a hypothetical: slot 6 of the
	// living deployment issued 64 placement claims in one day against two or
	// three from each local slot, every one a re-claim as its measured time scale
	// wandered — 64 epochs, on a six-slot map, from one peer, for nothing
	// (m5_considerations.md, DQ3). Every one of those is a frame carrying
	// slotCount stats blocks, so the term this suppresses grows with the map on
	// both sides.
	after := s.claimShapeLocked(p.id)
	quiet := reason == contractb.GrantUpdated && before.same(after)
	if !quiet {
		s.markChurnLocked()
	}
	s.mu.Unlock()

	s.log.Info("relay: placement claim", "peer", p.id, "slot", res.Slot,
		"position", res.Position(), "reason", reason,
		"simulationSize", claim.SimulationSize, "modConnected", claim.ModConnected,
		"exportEdges", claim.ExportEdges, "broadcast", !quiet)
	// §7.2 ordering step 2: answer the claim. Steps 3 and 4 — the PEER_STATUS
	// broadcast and the fresh grants to every peer whose effective neighbour
	// changed — belong to the publisher, which coalesces them.
	s.answerClaim(p, grant)
	return true
}

// claimShapeLocked is B29's comparison subject: everything about this peer that
// a PEER_STATUS broadcast would carry differently if it changed. A peer with no
// reservation and no meta yields the zero shape, which differs from every real
// one, so a first claim is never mistaken for a repeat.
func (s *Server) claimShapeLocked(peerID string) claimShape {
	shape := claimShape{}
	if res, ok := s.grid.ResOfPeer(peerID); ok {
		shape.slot, shape.col, shape.row = res.Slot, res.Col, res.Row
	}
	m, ok := s.meta[peerID]
	if !ok {
		return shape
	}
	shape.exportEdges = edgeKey(m.exportEdges)
	shape.borderEdges = edgeKey(m.borderEdges)
	shape.modConnected = m.modConnected
	shape.gameVersion = m.gameVersion
	shape.simSize = m.simSize
	shape.refusal = m.lastRefusal
	return shape
}

// assignLocked implements §7.2's arbitration rules 1 to 6, in order.
func (s *Server) assignLocked(p *peer, claim contractb.SectorClaim) (Reservation, string, bool) {
	// Rules 1 and 2: this peerId already holds a reservation, or names one
	// reserved to itself. The reservation is keyed on peerId, never on a
	// connection, and it NEVER EXPIRES. This is the whole of "return needs no
	// insertion": a peer that comes back after two hours or two weeks lands
	// where it was.
	if res, ok := s.grid.ResOfPeer(p.id); ok {
		if p.claimed {
			return res, contractb.GrantUpdated, false
		}
		if s.inherited[p.id] {
			// §7.5: this peer inherited a slot by operator command. It inherits
			// the address and nothing else, and its first grant says which.
			delete(s.inherited, p.id)
			return res, contractb.GrantHandover, false
		}
		return res, contractb.GrantReclaimed, false
	}
	// Rule 3: a preferredSlot reserved to somebody else is ignored, never
	// honoured by eviction.
	if claim.PreferredSlot > 0 {
		if owner := s.grid.PeerOfSlot(claim.PreferredSlot); owner != "" {
			s.log.Warn("relay: ignoring a preferredSlot that belongs to another peer",
				"peer", p.id, "preferredSlot", claim.PreferredSlot, "owner", owner)
		}
	}
	res := s.grid.Place(p.id, Preference{
		Slot:            claim.PreferredSlot,
		Position:        claim.PreferredPosition,
		InsertAfterSlot: claim.InsertAfterSlot,
		InsertAxis:      claim.InsertAxis,
	})
	return res, contractb.GrantGranted, true
}

// deliverableLocked is §8's filter from one peer's point of view. It returns
// the empty string when the candidate is deliverable and the §6.4 skip reason
// otherwise.
func (s *Server) deliverableLocked(me Reservation) Deliverable {
	mine := s.metaLocked(me.PeerID)
	return func(res Reservation) string {
		if res.Slot == me.Slot {
			return contractb.SkipPeerOffline // unreachable: the walk never visits self
		}
		if _, live := s.peers[res.PeerID]; !live {
			return contractb.SkipPeerOffline
		}
		m := s.metaLocked(res.PeerID)
		if !m.modConnected {
			// A dead sim must not keep receiving organisms (contract-a.md §8).
			return contractb.SkipPeerModAbsent
		}
		if mine.gameVersion != "" && m.gameVersion != "" && mine.gameVersion != m.gameVersion {
			return contractb.SkipPeerIncompatible
		}
		if mine.simSize > 0 && m.simSize > 0 && !sameSize(mine.simSize, m.simSize) {
			return contractb.SkipSimSizeMismatch
		}
		return ""
	}
}

// sameSize is contract-a.md §13 A10's relative epsilon. §4.1 forbids exact
// float equality.
func sameSize(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// exportEdgesOf is the set of edges a peer declared. A peer that has not
// claimed yet, or has no mod, declares nothing and gets no lanes.
func (s *Server) exportEdgesLocked(peerID string) []string {
	if m, ok := s.meta[peerID]; ok {
		return m.exportEdges
	}
	return nil
}

// grantForLocked builds one peer's SECTOR_GRANT: its slot, its position, the
// map, and ONE EFFECTIVE NEIGHBOUR PER EXPORT EDGE — up to FOUR of them under
// two-way lanes (§17, B13). A key is absent when that axis has no deliverable
// target, and its absence is what closes that export edge with no_peer (§8).
//
// The ripple is symmetric for free, and it is worth naming why rather than
// adding a mechanism for it. sendGrants recomputes EVERY peer's grant on a
// liveness change and re-sends the ones whose content moved; when each peer had
// two lanes, a dark slot's own east and north neighbours had nothing pointing
// at it and were told nothing. With four lanes every neighbour of a dark slot
// both pointed at it and was pointed at by it, so they all land in the "content
// changed" set. statusCoalesceMs already bounds the burst.
func (s *Server) grantForLocked(res Reservation, reason string) contractb.SectorGrant {
	pos := res.Position()
	grant := contractb.SectorGrant{
		Granted:   true,
		Slot:      res.Slot,
		Position:  &pos,
		Map:       s.grid.Shape(),
		SlotCount: s.grid.Size(),
		Reason:    reason,
	}
	ok := s.deliverableLocked(res)
	edges := s.exportEdgesLocked(res.PeerID)
	if len(edges) == 0 {
		// Nothing declared: no lanes to publish. The sidecar's own EDGE_STATUS
		// is empty too, which closes every edge it might have had.
		return grant
	}
	grant.Neighbours = map[string]*contractb.Neighbour{}
	for _, edge := range edges {
		if !contracta.ValidEdge(edge) {
			// Not an edge at all. Under D17 (§17, B13) every one of the four IS an
			// export edge, so there is no longer an edge the map has no axis for.
			continue
		}
		// ONLY THE EDGES THE SIDECAR DECLARED. The relay never invents a lane a
		// peer did not ask for, which is what keeps a two-edge sidecar's grant
		// byte-identical to what contract-b/3.2 produced for it.
		target, skipped, found := s.grid.Effective(res, edge, ok)
		if !found {
			continue
		}
		m := s.metaLocked(target.PeerID)
		grant.Neighbours[edge] = &contractb.Neighbour{
			Slot:           target.Slot,
			PeerID:         target.PeerID,
			Position:       target.Position(),
			Live:           true,
			ModConnected:   true,
			GameVersion:    m.gameVersion,
			SimulationSize: m.simSize,
			Skipped:        skipped,
		}
	}
	return grant
}

func (s *Server) answerClaim(p *peer, grant contractb.SectorGrant) {
	body, err := json.Marshal(grant)
	if err == nil {
		p.mu.Lock()
		p.lastGrant = string(body)
		p.lastPos = grant.Position
		p.mu.Unlock()
	}
	s.send(p, contractb.TypeSectorGrant, grant)
}

// ---------------------------------------------------------------- publishing

// publishLoop is §7.2's "no storm" rule, and under contract-b/4.0 it is also
// B29's broadcast bound.
//
// THE OLD RULE, UNCHANGED. At most one PEER_STATUS broadcast per window and at
// most one SECTOR_GRANT per peer in it. Coalescing may drop intermediate
// states; it MUST NOT drop the last one, because every one of these messages is
// full state and the last one is the truth. The pending counter guarantees it:
// it survives every widening and every timer reset and is cleared only by a
// publish that happened.
//
// THE NEW RULE. The window is no longer a fixed ticker. It starts at
// statusCoalesceMs, DOUBLES toward statusCoalesceMaxMs after any window that
// saw more than statusChurnBurstThreshold registry changes, and narrows one
// step after a quieter one. That converts a bound on SPACING into a bound on
// RATE, which is the thing that matters once a broadcast costs slotCount stats
// blocks (§22, B29).
//
// THE STATS TIMER IS A FLOOR ON FRESHNESS, NOT A SECOND SOURCE OF FRAMES.
// §14 B4 states the interaction exactly: "coalescing bounds how CLOSELY two
// broadcasts may follow each other, and this timer bounds how FAR APART they
// may drift." So it publishes only when nothing has been published for
// statsBroadcastIntervalMs — which leaves every broadcast this relay makes
// emitted from one place, spaced at least one window apart, and makes B29's
// arithmetic (60000/window a minute) a bound a test can assert rather than an
// estimate.
func (s *Server) publishLoop() {
	window := newChurnWindow(s.statusCoalesce, s.statusCoalesceMax, s.churnThreshold)
	coalesce := time.NewTimer(window.current())
	defer coalesce.Stop()
	stats := time.NewTicker(s.statsBroadcast)
	defer stats.Stop()
	sweep := time.NewTicker(time.Minute)
	defer sweep.Stop()
	s.mu.Lock()
	s.lastPublishAt = time.Now()
	s.mu.Unlock()
	for {
		select {
		case <-s.stop:
			return
		case <-coalesce.C:
			s.mu.Lock()
			pending, churn := s.pending, s.churn
			s.pending, s.churn = 0, 0
			s.mu.Unlock()
			if pending > 0 {
				s.publish()
			}
			next := window.observe(churn)
			s.mu.Lock()
			s.coalesceWindow = next
			s.mu.Unlock()
			coalesce.Reset(next)
		case <-stats.C:
			// §6.5: also sent on a timer, because stats change without the
			// registry changing — and §14 B4's drift bound, so a map that has
			// just been published is not published again for it. The check reads
			// the SHARED timestamp rather than a local one, so an admin act's
			// immediate publish counts too.
			s.mu.Lock()
			since := time.Since(s.lastPublishAt)
			s.mu.Unlock()
			if since < s.statsBroadcast {
				continue
			}
			s.publish()
		case <-sweep.C:
			s.sweepForwardRecord()
		}
	}
}

func (s *Server) publish() {
	s.broadcastPeerStatus()
	s.sendGrants()
	s.mu.Lock()
	s.lastPublishAt = time.Now()
	s.mu.Unlock()
}

// BroadcastCount is how many PEER_STATUS broadcast ROUNDS this process has made
// — one per publish, not one per frame. It is what an operator reads against
// B29's arithmetic and what the churn harness measures a rate from.
func (s *Server) BroadcastCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.broadcasts
}

// CoalesceWindow is the width the publisher is currently waiting. On a quiet
// map it is statusCoalesceMs; under sustained churn it climbs toward
// statusCoalesceMaxMs, and reading it is how an operator tells a busy map from
// a stalled publisher.
func (s *Server) CoalesceWindow() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.coalesceWindow
}

// MaxSlotEverIssued is the monotone address counter of §7.2 and D8. It NEVER
// decreases — a released number is retired forever, because SLOT_VACANT is a
// permanent answer and therefore a valid proof of non-delivery (§6.8), and
// reissuing an address would silently convert that proof into a lie.
func (s *Server) MaxSlotEverIssued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.MaxSlotEverIssued
}

// Holes is every position inside the rectangle that no slot names — the
// positions B29 fills before any axis grows. An operator deciding whether to
// release a slot is deciding whether to add one to this list.
func (s *Server) Holes() []contractb.Position {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grid.Holes()
}

// sendGrants pushes a SECTOR_GRANT to every live slot holder whose grant body
// changed — which after a splice or a liveness change is usually more than one
// peer, and after a column insertion may be most of them.
func (s *Server) sendGrants() {
	s.mu.Lock()
	type item struct {
		p     *peer
		grant contractb.SectorGrant
	}
	items := make([]item, 0, len(s.peers))
	for _, res := range s.grid.Slots {
		p, live := s.peers[res.PeerID]
		if !live {
			continue
		}
		items = append(items, item{p: p, grant: s.grantForLocked(res, contractb.GrantUpdated)})
	}
	s.mu.Unlock()

	for _, it := range items {
		it.p.mu.Lock()
		// §6.4: "repositioned" is the same slot at a NEW coordinate, because the
		// map grew. A splice moves most of a row or a column, and a peer that
		// learns its neighbours changed for that reason deserves to be told which
		// reason it was.
		if it.p.lastPos != nil && it.grant.Position != nil && *it.p.lastPos != *it.grant.Position {
			it.grant.Reason = contractb.GrantRepositioned
		}
		body, err := json.Marshal(it.grant)
		if err != nil {
			it.p.mu.Unlock()
			continue
		}
		same := it.p.lastGrant == string(body)
		if !same {
			it.p.lastGrant = string(body)
			it.p.lastPos = it.grant.Position
		}
		it.p.mu.Unlock()
		if same {
			continue
		}
		s.send(it.p, contractb.TypeSectorGrant, it.grant)
	}
}

func (s *Server) broadcastPeerStatus() {
	s.mu.Lock()
	// One round, counted once. B29's bound is on ROUNDS: the per-client frame
	// count is len(peers)+len(subscribers) and grows with the map by design,
	// which is exactly why the rate of rounds had to become the bounded thing.
	s.broadcasts++
	slots := make([]contractb.SlotInfo, 0, s.grid.Size())
	for _, res := range s.grid.Slots {
		m := s.metaLocked(res.PeerID)
		_, live := s.peers[res.PeerID]
		info := contractb.SlotInfo{
			Slot:           res.Slot,
			Position:       res.Position(),
			PeerID:         res.PeerID,
			Live:           live,
			ModConnected:   live && m.modConnected,
			GameVersion:    m.gameVersion,
			SimulationSize: m.simSize,
			ExportEdges:    append([]string{}, m.exportEdges...),
			LastSeenMs:     m.lastSeenMs,
			LastRefusal:    m.lastRefusal,
			Stats:          m.stats,
		}
		if !live {
			// Risk 5: a healed map hides a dead world. "Bypassed since 04:12" is
			// what stops an operator missing it for a day.
			info.DarkSinceMs = m.darkSinceMs
		}
		if m.stats != nil {
			info.StatsAsOfMs = m.statsAsOfMs
		}
		slots = append(slots, info)
	}
	observers := len(s.subscribers)
	shape := s.grid.Shape()
	count := s.grid.Size()
	// One object, built once and shared by every frame of this broadcast: the
	// table is the relay's own configuration and cannot differ between two
	// clients of the same relay.
	limits := s.limits.Published()

	type target struct {
		p  *peer
		me contractb.You
	}
	targets := make([]target, 0, len(s.peers)+len(s.subscribers))
	for _, p := range s.peers {
		me := contractb.You{Neighbours: map[string]*int{}}
		if res, ok := s.grid.ResOfPeer(p.id); ok {
			slot, pos := res.Slot, res.Position()
			me.Slot = &slot
			me.Position = &pos
			ok := s.deliverableLocked(res)
			// §17 B13 leaves PEER_STATUS ALONE, deliberately: it publishes the
			// registry and the structural row-major order, and the lanes have always
			// been DERIVED from it — a client that wants four walks runs them itself
			// (mapwalk), which is exactly what the archive's map does. `you` stays
			// §6.5's two-key convenience so nothing on this frame changes shape; the
			// authority for a peer's routing is its own SECTOR_GRANT, and that now
			// carries all four.
			for _, edge := range []string{contracta.EdgeE, contracta.EdgeN} {
				if target, _, found := s.grid.Effective(res, edge, ok); found {
					n := target.Slot
					me.Neighbours[edge] = &n
				} else {
					me.Neighbours[edge] = nil
				}
			}
		}
		targets = append(targets, target{p: p, me: me})
	}
	for _, p := range s.subscribers {
		// §6.5: all null for a subscriber.
		targets = append(targets, target{p: p, me: contractb.You{Neighbours: map[string]*int{}}})
	}
	s.mu.Unlock()

	for _, t := range targets {
		t.p.mu.Lock()
		t.p.epoch++
		epoch := t.p.epoch
		t.p.mu.Unlock()
		s.send(t.p, contractb.TypePeerStatus, contractb.PeerStatus{
			Epoch:     epoch,
			Map:       shape,
			SlotCount: count,
			Slots:     slots,
			You:       t.me,
			Observers: observers,
			// B25: republished on every broadcast, so an operator surface can say
			// which peers are one release away from being refused and D25's
			// "publish, then raise the floor" is a sequence a reader can WATCH.
			//
			// EVERY CLIENT GETS THE SAME FRAME. A subscriber's copy differs in `you`
			// and in nothing else, which is what makes B27's boundary describable in
			// one sentence: a subscriber gets nothing a peer does not already get.
			MinContractVersion: s.minContract,
			// B24: republished on every broadcast, so a page fed only by broadcasts
			// can render each peer's behaviour against the ceilings it is measured
			// on. BESIDE the stats blocks, never inside one (§6.3.1).
			Limits: limits,
		})
	}
}

// ---------------------------------------------------------------- routing

type migrationEnqueueResult int

const (
	migrationEnqueued migrationEnqueueResult = iota
	migrationSlotVacant
	migrationPeerOffline
	migrationQueueFull
	migrationRelayDraining
)

// enqueueMigration is the routing and proof transaction. It resolves the
// current destination, admits the byte-identical frame to that connection's
// bounded paced transport queue, and records the attempted write before it
// releases Server.mu. Thus no proof reader can observe an accepted queue item
// with neverForwarded:true. A full paced queue is different from the ordinary
// required-frame queue: it refuses this migration without closing the peer.
func (s *Server) enqueueMigration(destSlot int, migrationID string,
	frame []byte) (migrationEnqueueResult, Reservation, *peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining {
		return migrationRelayDraining, Reservation{}, nil, nil
	}
	res, reserved := s.grid.ResOfSlot(destSlot)
	if !reserved {
		return migrationSlotVacant, Reservation{}, nil, nil
	}
	dest := s.peers[res.PeerID]
	if dest == nil {
		return migrationPeerOffline, res, nil, nil
	}
	select {
	case <-dest.conn.Done():
		return migrationPeerOffline, res, nil, nil
	default:
	}
	if err := dest.conn.TrySendPaced(frame); err != nil {
		if errors.Is(err, wsutil.ErrPacedQueueFull) {
			return migrationQueueFull, res, dest, nil
		}
		return migrationPeerOffline, res, dest, err
	}
	if migrationID != "" {
		if _, ok := s.forwarded[migrationID]; !ok {
			s.forwarded[migrationID] = time.Now()
		}
	}
	return migrationEnqueued, res, dest, nil
}

// onMigrationPayload routes on data.destSlot and nothing else. The frame is
// forwarded byte for byte; body.bb8 and data.lineage are never decoded (§5).
func (s *Server) onMigrationPayload(p *peer, env wire.Envelope, frame []byte) bool {
	var routing contractb.MigrationRouting
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	var id contractb.Identity
	_ = json.Unmarshal(env.Data, &id)
	if !wire.ValidUUID(id.MigrationID) {
		p.conn.Close(contractb.CloseMalformedFrame, "migrationId is not a UUID")
		return false
	}

	// §5.1: a MIGRATION_PAYLOAD from a subscriber is answered NOT_A_MEMBER and
	// is not forwarded.
	if p.role == contractb.RoleArchive {
		s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
			MigrationID: id.MigrationID,
			SourcePeer:  "",
			DestPeer:    p.id,
			Code:        contractb.NackNotAMember,
			Class:       contractb.ClassPermanent,
			Message:     "this connection is a read-only archive subscriber and holds no slot",
		})
		s.log.Warn("relay: refused a migration from a read-only subscriber", "peer", p.id)
		return true
	}
	if routing.DestSlot < 1 {
		p.conn.Close(contractb.CloseMalformedFrame, "destSlot is not a slot")
		return false
	}
	if routing.Reroute != nil && routing.Reroute.Count < 1 {
		p.conn.Close(contractb.CloseMalformedFrame, "reroute.count is not positive")
		return false
	}
	attempt := &contractb.MigrationAttempt{DestSlot: routing.DestSlot}
	if routing.Reroute != nil {
		attempt.RerouteCount = routing.Reroute.Count
	}

	result, res, dest, err := s.enqueueMigration(routing.DestSlot, id.MigrationID, frame)
	switch result {
	case migrationRelayDraining:
		s.nackNoDelivery(p, id.MigrationID, contractb.NackNotForwarded,
			"the relay is draining and declined to hand this frame over", nil, false)
		return true
	case migrationSlotVacant:
		s.nackNoDelivery(p, id.MigrationID, contractb.NackSlotVacant,
			fmt.Sprintf("slot %d names no reservation; slot numbers are never reused, so it never returns",
				routing.DestSlot), nil, true)
		return true
	case migrationPeerOffline:
		msg := fmt.Sprintf("slot %d (%d,%d) is reserved to %s, which is not connected",
			res.Slot, res.Col, res.Row, res.PeerID)
		s.mu.Lock()
		if m, ok := s.meta[res.PeerID]; ok && m.darkSinceMs > 0 {
			msg += fmt.Sprintf(", dark for %ds", (time.Now().UnixMilli()-m.darkSinceMs)/1000)
		}
		s.mu.Unlock()
		if err != nil {
			s.log.Warn("relay: destination transport queue stopped before migration admission",
				"peer", res.PeerID, "migrationId", id.MigrationID, "err", err)
		}
		s.nackNoDelivery(p, id.MigrationID, contractb.NackPeerOffline, msg, nil, true)
		return true
	case migrationQueueFull:
		s.nackNoDelivery(p, id.MigrationID, contractb.NackNotForwarded,
			fmt.Sprintf("slot %d (%s) has a full paced migration transport queue; "+
				"the destination connection stays live", res.Slot, res.PeerID), attempt, true)
		return true
	case migrationEnqueued:
		s.sendForwardReceipt(p, id.MigrationID, routing.DestSlot)
		s.fanOut(frame)
		return true
	default:
		s.log.Error("relay: impossible migration enqueue result", "result", result,
			"destinationPresent", dest != nil, "migrationId", id.MigrationID)
		return false
	}
}

// sendForwardReceipt is B26's whole relay-side obligation (§5.2, §6.12).
//
// ONE FORWARD, ONE RECEIPT. A re-route of the same migrationId produces another,
// because it is a statement about a WRITE and not about a migration — a sender
// holding two under one migrationId has written the frame twice, which is a fact
// about its own re-routes and never a duplicated organism. A conforming sender
// since §25's B37 never writes the same frame to the same destination twice at
// all; the relay does not care either way and counts writes.
//
// IT GOES TO THE SENDER'S OWN CONNECTION and nowhere else. That is not an
// optimisation: §5.1's fan-out set is unchanged and a subscriber is NOT copied,
// because a receipt is a fact about one sender's journal rather than about the
// migration, and every other frame this relay copies is the second kind.
//
// BEST EFFORT, THROUGH TrySend. A full outbound queue drops the receipt and the
// connection stays up (§6.12's *Bounded* row): the sender's entry stays `sent`,
// which is exactly where the receipt would have kept it. Nothing here can fail a
// forward, and nothing here is retried.
func (s *Server) sendForwardReceipt(sender *peer, migrationID string, destSlot int) {
	if migrationID == "" {
		// A frame the relay cannot name is a frame no journal can join a receipt
		// to. §5.2's record skips it for the same reason.
		return
	}
	now := time.Now().UnixMilli()
	frame := mustFrame(s.log, contractb.TypeForwardReceipt, contractb.ForwardReceipt{
		MigrationID: migrationID,
		DestSlot:    destSlot,
		// The session in force AT THE WRITE, so the sender learns the SCOPE of
		// the fact along with the fact (§5.2). It is constant for the life of
		// this process, which is precisely what makes it the scope.
		RelaySessionID: s.sessionID,
		ForwardedAt:    now,
	})
	if frame == nil {
		return
	}
	err := sender.conn.TrySend(frame)
	s.mu.Lock()
	if err == nil {
		s.receiptsSent++
	} else {
		s.receiptsDropped++
	}
	dropped := s.receiptsDropped
	shout := err != nil && time.Since(s.lastReceiptDropLog) > time.Minute
	if shout {
		s.lastReceiptDropLog = time.Now()
	}
	s.mu.Unlock()
	if shout {
		// At most one line a minute, because the failure is benign by design and
		// a line per migration would be the expensive half of a cheap frame.
		s.log.Warn("relay: dropping FORWARD_RECEIPTs for a sender whose outbound queue is full; "+
			"the forwards themselves are untouched",
			"peer", sender.id, "droppedTotal", dropped,
			"meaning", "a missing receipt is silence, and silence is never proof in this contract; "+
				"the sender's entry stays `sent`, which is where the receipt would have kept it "+
				"(contract-b-m4.md §6.12, §22 B26)")
	}
}

// ReceiptCounts is how many FORWARD_RECEIPTs this process has enqueued and how
// many it has dropped for a full outbound queue. It is what an operator reads
// against the crossing rate, and what the cost harness measures a per-migration
// frame count from.
func (s *Server) ReceiptCounts() (sent, dropped int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.receiptsSent, s.receiptsDropped
}

// nackNoDelivery answers the SENDER rather than dropping the frame. A dropped
// frame turns a bounded failure into a stall, and under M4 it also withholds
// the evidence a sender needs to re-route (§5.2).
func (s *Server) nackNoDelivery(p *peer, migrationID, code, message string,
	refusedAttempt *contractb.MigrationAttempt, includeLegacyProof bool) {
	never := !s.hasForwarded(migrationID)
	nack := contractb.MigrationNack{
		MigrationID:    migrationID,
		SourcePeer:     "",
		DestPeer:       p.id,
		Code:           code,
		Class:          contractb.ClassOf(code),
		Message:        message,
		RefusedAttempt: refusedAttempt,
	}
	if includeLegacyProof {
		nack.NeverForwarded = &never
		nack.RelaySessionID = s.sessionID
	}
	if nack.Class == contractb.ClassTransient {
		nack.RetryAfterMs = 15000
	}
	if !includeLegacyProof {
		nack.Message += "; relay drain carries no non-delivery proof"
	} else if never {
		nack.Message += "; this relay has never forwarded this migration"
	} else {
		nack.Message += "; this relay has already handed this migration to a peer at least once"
	}
	s.send(p, contractb.TypeMigrationNack, nack)
	s.fanOut(mustFrame(s.log, contractb.TypeMigrationNack, nack))
}

func (s *Server) recordForward(migrationID string) {
	if migrationID == "" {
		return
	}
	s.mu.Lock()
	if _, ok := s.forwarded[migrationID]; !ok {
		s.forwarded[migrationID] = time.Now()
	}
	s.mu.Unlock()
}

func (s *Server) hasForwarded(migrationID string) bool {
	if migrationID == "" {
		// An answer about a migration the relay cannot name is no proof at all.
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.forwarded[migrationID]
	return ok
}

func (s *Server) sweepForwardRecord() {
	cutoff := time.Now().Add(-s.forwardRetain)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, at := range s.forwarded {
		if at.Before(cutoff) {
			delete(s.forwarded, id)
		}
	}
}

// ForwardedCount is the size of the §5.2 record, for tests and operator logs.
func (s *Server) ForwardedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.forwarded)
}

// onDirected routes MIGRATION_ACK, MIGRATION_NACK and GENOME_RESPONSE on
// data.destPeer. copyToArchive marks the frames §5.1 fans out.
func (s *Server) onDirected(p *peer, env wire.Envelope, frame []byte, copyToArchive bool) bool {
	var routing contractb.Routing
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	s.mu.Lock()
	dest := s.peers[routing.DestPeer]
	if dest == nil {
		dest = s.subscribers[routing.DestPeer]
	}
	s.mu.Unlock()
	if dest == nil {
		if env.Type == contractb.TypeMigrationAck || env.Type == contractb.TypeMigrationNack {
			// §6.8: PEER_UNKNOWN, rather than a drop. These route on a peer id
			// rather than a slot.
			var id contractb.Identity
			_ = json.Unmarshal(env.Data, &id)
			s.send(p, contractb.TypeMigrationNack, contractb.MigrationNack{
				MigrationID:  id.MigrationID,
				SourcePeer:   "",
				DestPeer:     p.id,
				Code:         contractb.NackPeerUnknown,
				Class:        contractb.ClassTransient,
				Message:      "destPeer " + routing.DestPeer + " is not connected",
				RetryAfterMs: 15000,
			})
			return true
		}
		s.log.Warn("relay: dropping frame for absent peer", "type", env.Type, "destPeer", routing.DestPeer)
		return true
	}
	s.forward(dest, frame)
	if copyToArchive {
		s.fanOut(frame)
	}
	return true
}

// onGenomeRequest routes on destPeer and answers the requester itself when the
// asked-for peer is not connected (§5, §6.10).
func (s *Server) onGenomeRequest(p *peer, env wire.Envelope, frame []byte) bool {
	var routing contractb.Routing
	if err := json.Unmarshal(env.Data, &routing); err != nil {
		p.conn.Close(contractb.CloseMalformedFrame, "malformed routing fields")
		return false
	}
	if routing.SourcePeer != p.id {
		p.conn.Close(contractb.CloseMalformedFrame, "sourcePeer does not match this connection")
		return false
	}
	s.mu.Lock()
	dest := s.peers[routing.DestPeer]
	s.mu.Unlock()
	if dest == nil {
		var id contractb.Identity
		_ = json.Unmarshal(env.Data, &id)
		s.send(p, contractb.TypeGenomeResponse, contractb.GenomeResponse{
			RequestID:  id.RequestID,
			SourcePeer: "",
			DestPeer:   p.id,
			GenomeHash: id.GenomeHash,
			Found:      false,
			Reason:     contractb.GenomePeerOffline,
		})
		return true
	}
	s.forward(dest, frame)
	return true
}

// fanOut sends a byte-identical copy of a routed frame to every subscriber
// (§5.1). It never delays, blocks or fails a migration.
func (s *Server) fanOut(frame []byte) {
	if frame == nil {
		return
	}
	s.mu.Lock()
	subs := make([]*peer, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.out <- frame:
		default:
			// Bounded: on overflow drop the OLDEST copy, count it, and log at
			// most one line a minute. Never disconnect the migration.
			select {
			case <-sub.out:
			default:
			}
			select {
			case sub.out <- frame:
			default:
			}
			sub.mu.Lock()
			sub.dropped++
			dropped := sub.dropped
			shout := time.Since(sub.lastDropLog) > time.Minute
			if shout {
				sub.lastDropLog = time.Now()
			}
			sub.mu.Unlock()
			if shout {
				s.log.Warn("relay: archive subscriber is behind, dropping the oldest copies",
					"peer", sub.id, "droppedTotal", dropped)
			}
		}
	}
}

func (s *Server) copyLoop(sub *peer) {
	for {
		select {
		case <-sub.conn.Done():
			return
		case frame := <-sub.out:
			if err := sub.conn.Send(frame); err != nil {
				return
			}
		}
	}
}

func (s *Server) forward(dest *peer, frame []byte) {
	if err := dest.conn.Send(frame); err != nil {
		s.log.Warn("relay: forward failed", "peer", dest.id, "err", err)
	}
}

func (s *Server) drop(p *peer) {
	s.mu.Lock()
	delete(s.connections, p)
	registry := s.peers
	if p.role == contractb.RoleArchive {
		registry = s.subscribers
	}
	cur, ok := registry[p.id]
	if !ok || cur != p {
		s.mu.Unlock()
		return
	}
	delete(registry, p.id)
	if m, ok := s.meta[p.id]; ok {
		m.modConnected = false
		if p.role == contractb.RolePeer {
			m.darkSinceMs = time.Now().UnixMilli()
		}
	}
	slot := s.grid.SlotOfPeer(p.id)
	s.markChurnLocked()
	s.mu.Unlock()
	s.log.Info("relay: client gone", "peer", p.id, "role", p.role, "slot", slot,
		"reservationKept", slot > 0)
	// §8: the vacancy is announced BACKWARDS along each lane that pointed at it.
	// Every peer whose effective neighbour was that slot re-targets and KEEPS
	// ITS EXPORT EDGE OPEN; the slot itself stays in the map, live:false, with
	// darkSinceMs set.
}

func (s *Server) send(p *peer, typ string, data any) {
	frame := mustFrame(s.log, typ, data)
	if frame == nil {
		return
	}
	if err := p.conn.Send(frame); err != nil {
		s.log.Warn("relay: send failed", "peer", p.id, "type", typ, "err", err)
	}
}

func mustFrame(log *slog.Logger, typ string, data any) []byte {
	frame, err := wire.Encode(wire.ProtocolB, typ, time.Now().UnixMilli(), data)
	if err != nil {
		log.Error("relay: encode failed", "type", typ, "err", err)
		return nil
	}
	return frame
}

func (s *Server) pingLoop(p *peer) {
	t := time.NewTicker(s.pingInterval)
	defer t.Stop()
	for {
		select {
		case <-p.conn.Done():
			return
		case <-t.C:
			s.send(p, contractb.TypePing, contractb.Ping{Nonce: wire.NewUUID()})
		}
	}
}

func derefSlot(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// grantForRole names the grant a role needs, for a refusal string that states
// the remedy rather than the symptom (§5.1, B27).
func grantForRole(role string) string {
	switch role {
	case contractb.RoleArchive:
		return peercred.GrantSubscribe
	case contractb.RolePeer:
		return peercred.GrantPeer
	}
	return role
}
