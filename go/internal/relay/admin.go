package relay

// B28's authenticated admin path (contract-b-m4.md §7.5, §22 B28).
//
// FOUR PROPERTIES MAKE IT SAFE, and every one of them is a line of code here.
//
//  1. IT IS NOT ON THIS WIRE. A separate listener, never the Contract B
//     WebSocket. No frame of §6's catalogue invokes an act and no peer or
//     subscriber can reach one — a relay that accepted an admin instruction on
//     the peer wire would have to authorize per message on the path D1 keeps
//     free of decisions. The message catalogue grows by nothing here.
//  2. AUTHENTICATION IS §3.1'S, WITH A THIRD GRANT. The same credential
//     mechanism, the admin grant, disjoint from peer and subscribe. Loopback by
//     default and TLS if bound anywhere else (B23) — both enforced in Main.
//  3. THE ACT STAYS DELIBERATE. Two calls. The first returns the same
//     consequence report §7.5 has always required an operator to read, plus a
//     single-use token bound to the act AND to the current ring.json state. The
//     second performs the act and is REFUSED if the map moved underneath the
//     token. A confirmation an operator cannot see is not a confirmation.
//  4. EVERY ACT IS AUDITED. One line per act naming the grant, the act, the
//     slot, the state before and the operator's reason string.
//
// WHAT IT DELIBERATELY DOES NOT BECOME. It is not the control surface. It
// changes the map's REGISTRY — a reservation, an identity, a peer's admission —
// and touches nothing inside a world: no time scale, no save policy, no
// exclusion list, no setting a mod reported. D23 defers that surface to M6.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
	"multiverse/internal/wire"
)

// The three acts. They are §7.5's three relay commands and no others: the
// fourth escape hatch, --release-inflight, stays on the sidecar's own machine
// because custody is local (D2) and that machine is a stranger's.
const (
	ActReleaseSlot  = "release-slot"
	ActHandoverSlot = "handover-slot"
	ActEvictPeer    = "evict-peer"
)

// adminConfirmTTL bounds how long a reported consequence may sit unconfirmed.
// It is short on purpose: the token is bound to a ring state, and a report an
// operator read an hour ago describes a map that has had an hour to move.
const adminConfirmTTL = 10 * time.Minute

// eviction is one peer's admission ban (B28). Until is the zero time for "until
// lifted".
type eviction struct {
	Until  time.Time
	Reason string
	At     time.Time
}

// adminPending is one reported-but-unconfirmed act.
type adminPending struct {
	act       string
	slot      int
	peerID    string
	newPeerID string
	until     time.Time
	forSpec   string
	reason    string
	ringHash  string
	grant     string
	createdAt time.Time
}

// ---------------------------------------------------------------- the reports
//
// The wire shape of §7.5's consequence report, over the path. It is the SAME
// report the console prints — the map's half — because the relay cannot
// enumerate journals it is forbidden to read, and HeldEntriesAddressedHere says
// so rather than pretending otherwise.

// AdminReport is the first call's answer.
type AdminReport struct {
	Act    string `json:"act"`
	Slot   int    `json:"slot,omitempty"`
	PeerID string `json:"peerId"`
	// Position, BecomesHole and LanesChanged are the map's half of the
	// consequence: what moves, and for whom.
	Position     *contractb.Position  `json:"position,omitempty"`
	DarkForMs    int64                `json:"darkForMs,omitempty"`
	Live         bool                 `json:"live"`
	BecomesHole  []contractb.Position `json:"becomesHole,omitempty"`
	LanesChanged []LaneChange         `json:"lanesChanged"`
	// AddressRetiredForever is release's whole point: the slot NUMBER never
	// comes back, so every journal entry that names it still gets its permanent
	// SLOT_VACANT, which is what lets an orphaned entry re-route at all.
	AddressRetiredForever bool `json:"addressRetiredForever,omitempty"`
	// HolesAfter is every position the map would hold with nobody in it once
	// this act applied, in structural order (added for B29's slot-space policy).
	// It is the list the NEXT newcomer is filled from, before any axis grows, so
	// it is the operator's picture of what releasing this slot actually offers
	// the map: not an empty space kept for a departed peer, but the next
	// stranger's position.
	HolesAfter []contractb.Position `json:"holesAfter"`
	// MaxSlotEverIssued is the address counter that never decreases, reported
	// beside the act because release is the one act that adds to the gap between
	// it and slotCount.
	MaxSlotEverIssued int `json:"maxSlotEverIssued,omitempty"`
	// NewPeerID is handover's.
	NewPeerID string `json:"newPeerId,omitempty"`
	// EvictedUntilMs and EvictedFor are eviction's. A zero EvictedUntilMs with
	// Act evict-peer means "until lifted".
	EvictedUntilMs int64  `json:"evictedUntilMs,omitempty"`
	EvictedFor     string `json:"evictedFor,omitempty"`
	Lifts          bool   `json:"lifts,omitempty"`
	// HeldEntriesAddressedHere IS A SENTENCE AND NOT A LIST, on purpose. The
	// relay CANNOT enumerate journals — they live on other people's machines and
	// D2 keeps custody local — so the field says what the relay does not know
	// and where the answer is. After M5 the machine that answers it is usually a
	// stranger's, and the operator's honest answer becomes "ask the peer".
	HeldEntriesAddressedHere string `json:"heldEntriesAddressedHere"`
	// Notes are the act's own consequences in the operator's language, the same
	// sentences the console prints above its confirmation prompt.
	Notes []string `json:"notes,omitempty"`

	ConfirmToken  string `json:"confirmToken"`
	RingStateHash string `json:"ringStateHash"`
}

// LaneChange is one peer whose effective lane moves because of the act.
type LaneChange struct {
	PeerID string `json:"peerId"`
	Edge   string `json:"edge"`
	From   int    `json:"from"`
	// To is null when that edge has no deliverable target after the act, which
	// is what closes it with no_peer (§8).
	To *int `json:"to"`
}

// AdminApplied is the second call's answer.
type AdminApplied struct {
	Act               string             `json:"act"`
	Slot              int                `json:"slot,omitempty"`
	PeerID            string             `json:"peerId,omitempty"`
	Applied           bool               `json:"applied"`
	Map               contractb.MapShape `json:"map"`
	SlotCount         int                `json:"slotCount"`
	MaxSlotEverIssued int                `json:"maxSlotEverIssued"`
	Epoch             int64              `json:"epoch"`
	// JoinString is present exactly for a handover, which is also the credential
	// recovery path (B22): the new identity gets a freshly minted credential and
	// this is the ONE moment its secret exists anywhere but the peer's machine.
	JoinString string `json:"joinString,omitempty"`
	// EvictedUntilMs and Lifted are eviction's.
	EvictedUntilMs int64 `json:"evictedUntilMs,omitempty"`
	Lifted         bool  `json:"lifted,omitempty"`
}

const heldEntriesSentence = "not knowable from the relay — read stats.heldDepth on PEER_STATUS, " +
	"and multiverse-sidecar --list-inflight --dest-slot %d on the machine that holds the journal"

const heldEntriesSentenceNoSlot = "not knowable from the relay — read stats.heldDepth on " +
	"PEER_STATUS, and multiverse-sidecar --list-inflight on the machine that holds the journal"

// ---------------------------------------------------------------- the listener

// AdminHandler is B28's separate listener. It is returned rather than served so
// Main owns the bind, the TLS decision and the lifetime — the same division
// Handler() already has.
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	for _, act := range []string{ActReleaseSlot, ActHandoverSlot, ActEvictPeer} {
		name := act
		mux.HandleFunc("/admin/"+name, func(w http.ResponseWriter, r *http.Request) {
			s.adminReport(w, r, name)
		})
		mux.HandleFunc("/admin/"+name+"/confirm", func(w http.ResponseWriter, r *http.Request) {
			s.adminConfirm(w, r, name)
		})
	}
	mux.HandleFunc("/admin/healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminGrant(w, r); !ok {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// adminGrant is property 2. It authenticates with §3.1's mechanism and then
// demands the ADMIN grant specifically: a peer credential and a subscribe
// credential are refused here exactly as an admin credential is refused both
// roles on the Contract B wire. That disjointness is the whole reason the three
// grants exist.
func (s *Server) adminGrant(w http.ResponseWriter, r *http.Request) (string, bool) {
	peerID, grant, ok := s.creds.Verify(peercred.FromRequest(r))
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		adminError(w, http.StatusUnauthorized, "unauthorized")
		// No peerId in the log: the id on a refused credential is
		// attacker-chosen text, and echoing it turns the log into a search index
		// of guesses (§3.1, B22).
		s.log.Error("relay: refused an admin request whose credential did not verify",
			"remote", r.RemoteAddr, "path", r.URL.Path)
		return "", false
	}
	if grant != peercred.GrantAdmin {
		adminError(w, http.StatusForbidden,
			"this credential carries the "+grant+" grant and the admin path needs "+peercred.GrantAdmin)
		s.log.Error("relay: refused an admin act to a credential without the admin grant",
			"peer", peerID, "grant", grant, "path", r.URL.Path,
			"remedy", "mint one: multiverse-relay --mint-credential <id> --grant admin. The three "+
				"grants are disjoint and a peer's is never an operator's (contract-b-m4.md §22, B22)")
		return "", false
	}
	return peerID, true
}

type adminRequest struct {
	Slot          int    `json:"slot"`
	PeerID        string `json:"peerId"`
	NewPeerID     string `json:"newPeerId"`
	For           string `json:"for"`
	Reason        string `json:"reason"`
	ConfirmToken  string `json:"confirmToken"`
	RingStateHash string `json:"ringStateHash"`
}

func (s *Server) adminReport(w http.ResponseWriter, r *http.Request, act string) {
	operator, ok := s.adminGrant(w, r)
	if !ok {
		return
	}
	req, ok := decodeAdminRequest(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		// The audit line carries the operator's reason, so an act with no reason
		// is an act nobody can account for later. It is cheap to demand and it is
		// the only field the relay cannot reconstruct.
		adminError(w, http.StatusBadRequest, "reason is required: it is what the audit line records")
		return
	}
	report, err := s.consequence(act, req)
	if err != nil {
		adminError(w, http.StatusConflict, err.Error())
		s.log.Warn("relay: an admin report was refused", "act", act, "operator", operator, "err", err)
		return
	}
	token := wire.NewUUID()
	report.ConfirmToken = token
	s.mu.Lock()
	s.sweepAdminTokensLocked(time.Now())
	s.adminTokens[token] = &adminPending{
		act: act, slot: report.Slot, peerID: report.PeerID, newPeerID: report.NewPeerID,
		until: adminUntil(report), forSpec: report.EvictedFor, reason: req.Reason,
		ringHash: report.RingStateHash, grant: operator, createdAt: time.Now(),
	}
	s.mu.Unlock()
	s.log.Info("relay: admin consequence reported; nothing has changed yet",
		"act", act, "operator", operator, "slot", report.Slot, "peer", report.PeerID,
		"confirmToken", token, "ringStateHash", report.RingStateHash)
	adminJSON(w, http.StatusOK, report)
}

func adminUntil(rep AdminReport) time.Time {
	if rep.EvictedUntilMs == 0 {
		return time.Time{}
	}
	return time.UnixMilli(rep.EvictedUntilMs)
}

func (s *Server) adminConfirm(w http.ResponseWriter, r *http.Request, act string) {
	operator, ok := s.adminGrant(w, r)
	if !ok {
		return
	}
	req, ok := decodeAdminRequest(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	s.sweepAdminTokensLocked(time.Now())
	pending, known := s.adminTokens[req.ConfirmToken]
	if known {
		// SINGLE USE. Consumed whether or not the act goes on to succeed: a token
		// that survived a refusal would be a confirmation an operator could
		// replay against a map that had moved twice.
		delete(s.adminTokens, req.ConfirmToken)
	}
	current := ringStateHash(s.grid)
	s.mu.Unlock()

	switch {
	case !known:
		adminError(w, http.StatusConflict,
			"no such confirmToken: it was used, it expired, or this relay restarted")
		return
	case pending.act != act:
		adminError(w, http.StatusConflict,
			"that confirmToken belongs to "+pending.act+", not "+act)
		return
	case req.RingStateHash != "" && req.RingStateHash != pending.ringHash:
		adminError(w, http.StatusConflict,
			"the ringStateHash you sent is not the one that was reported to you")
		return
	case current != pending.ringHash:
		// THE MAP MOVED UNDERNEATH THE TOKEN. This is the refusal property 3
		// exists for: the operator read a consequence that is no longer the
		// consequence, and applying it anyway would act on a report nobody has
		// seen.
		adminError(w, http.StatusConflict,
			"the map changed since that report was made ("+pending.ringHash+" -> "+current+
				"); ask for the consequence again and read it")
		s.log.Warn("relay: refusing an admin act because the map moved under its token",
			"act", act, "operator", operator, "reportedRingState", pending.ringHash,
			"currentRingState", current)
		return
	}

	applied, err := s.applyAdmin(pending, operator)
	if err != nil {
		adminError(w, http.StatusConflict, err.Error())
		s.log.Error("relay: an admin act failed after confirmation", "act", act,
			"operator", operator, "err", err)
		return
	}
	adminJSON(w, http.StatusOK, applied)
}

func decodeAdminRequest(w http.ResponseWriter, r *http.Request) (adminRequest, bool) {
	if r.Method != http.MethodPost {
		adminError(w, http.StatusMethodNotAllowed, "the admin path takes POST")
		return adminRequest{}, false
	}
	var req adminRequest
	// 64 KiB is far more than any of these three acts needs and far less than a
	// body worth buffering from a listener an operator reaches over loopback.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		adminError(w, http.StatusBadRequest, "the body is not the JSON object this act takes")
		return adminRequest{}, false
	}
	return req, true
}

func adminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func adminError(w http.ResponseWriter, status int, msg string) {
	adminJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------- the slot-space policy

// slotSpacePolicy is DQ3's question answered in the operator's own words:
// WHAT DO YOU DO WITH A POSITION THAT WILL NEVER BE FILLED AGAIN
// (m5_considerations.md DQ3; contract-b-m4.md §7.5, §22 B29).
//
// It is written ONCE and rendered by both doors — the console's printed report
// and the admin path's JSON — because an operator who read one of them and
// acted through the other must not have been told two different things. The
// sentences are the quotable form: the participant documentation WP7 owes
// quotes them rather than paraphrasing, so the map's rule and the support
// answer cannot drift apart.
//
// The decision the operator is actually making has two arms and the report has
// to state both, because doing nothing is a choice here rather than a delay:
//
//   - LEAVE IT. The reservation never expires (§7.2 rule 1). The position stays
//     held for that peerId for as long as this map exists, no newcomer can have
//     it, and a peer that returns after two weeks lands exactly where it was
//     with reason:"reclaimed". That is the right answer for a peer that MIGHT
//     come back, and it costs the map one bypassed position that both axes
//     route around.
//   - RELEASE IT. The POSITION becomes an ordinary hole that the next newcomer
//     fills before any axis grows (B29). The ADDRESS is retired forever. That
//     is the right answer for a peer that will not come back, and it is
//     irreversible: the returning peer would be a new slot number at whatever
//     position the map had left.
func slotSpacePolicy(g *Grid, res Reservation) []string {
	holes := len(g.Holes())
	return []string{
		fmt.Sprintf("the map stays %dx%d and position (%d,%d) becomes a HOLE that both axes route "+
			"around (§8)", g.Width, g.Height, res.Col, res.Row),
		fmt.Sprintf("that hole is where the NEXT newcomer goes: rule 6 fills the first hole in "+
			"structural order before any axis grows, and under contract-b/4.0 a newcomer that "+
			"asked to extend an axis instead is refused the extension while any hole exists "+
			"(§7.2 rule 4, §22 B29). After this act the map offers %s",
			plural(holes+1, "hole", "holes")),
		fmt.Sprintf("slot %d is retired for good: maxSlotEverIssued (%d) never decreases, so every "+
			"journaled destSlot %d answers SLOT_VACANT permanently — which is what lets an "+
			"orphaned entry prove non-delivery and re-route (§6.8)",
			res.Slot, g.MaxSlotEverIssued, res.Slot),
		fmt.Sprintf("IF YOU DO NOTHING INSTEAD, the reservation never expires: position (%d,%d) "+
			"stays held for %s for as long as this map exists, no newcomer can have it, and a "+
			"return is an ordinary reason=\"reclaimed\" at the same slot and the same position. "+
			"Release is for a peer that will NOT come back, and it cannot be undone",
			res.Col, res.Row, res.PeerID),
		res.PeerID + " keeps its journal, its genome cache and its logs; a release never moves a journal",
	}
}

// plural writes a count an operator reads at three in the morning.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// holesAfterRelease is the hole list the map would hold once slot n is gone. It
// is computed without mutating anything, because a consequence report that
// changed the map to describe it would be the one thing B28's first call must
// never do.
func holesAfterRelease(g *Grid, going Reservation) []contractb.Position {
	out := []contractb.Position{}
	for row := 0; row < g.Height; row++ {
		for col := 0; col < g.Width; col++ {
			if res, ok := g.At(col, row); !ok || res.Slot == going.Slot {
				out = append(out, contractb.Position{Col: col, Row: row})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- consequences

// consequence builds the pre-act report for one act. It is the map's half of
// §7.5's division and it CHANGES NOTHING.
func (s *Server) consequence(act string, req adminRequest) (AdminReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := ringStateHash(s.grid)

	switch act {
	case ActReleaseSlot:
		res, ok := s.grid.ResOfSlot(req.Slot)
		if !ok {
			return AdminReport{}, fmt.Errorf("slot %d names no reservation", req.Slot)
		}
		if _, live := s.peers[res.PeerID]; live {
			return AdminReport{}, fmt.Errorf(
				"slot %d is held by a live peer (%s); stop it first", req.Slot, res.PeerID)
		}
		pos := res.Position()
		rep := AdminReport{
			Act: act, Slot: res.Slot, PeerID: res.PeerID, Position: &pos,
			BecomesHole:              []contractb.Position{pos},
			AddressRetiredForever:    true,
			HolesAfter:               holesAfterRelease(s.grid, res),
			MaxSlotEverIssued:        s.grid.MaxSlotEverIssued,
			LanesChanged:             s.laneChangesLocked(res),
			HeldEntriesAddressedHere: fmt.Sprintf(heldEntriesSentence, res.Slot),
			RingStateHash:            hash,
			// DQ3's answer, in the words both doors use (§22, B29).
			Notes: slotSpacePolicy(s.grid, res),
		}
		s.fillDarkLocked(&rep, res.PeerID)
		return rep, nil

	case ActHandoverSlot:
		res, ok := s.grid.ResOfSlot(req.Slot)
		if !ok {
			return AdminReport{}, fmt.Errorf("slot %d names no reservation", req.Slot)
		}
		newPeer := strings.TrimSpace(req.NewPeerID)
		if newPeer == "" {
			return AdminReport{}, errors.New("handover-slot needs newPeerId")
		}
		if _, live := s.peers[res.PeerID]; live {
			return AdminReport{}, fmt.Errorf(
				"slot %d is held by a live peer (%s); stop it first", req.Slot, res.PeerID)
		}
		if held := s.grid.SlotOfPeer(newPeer); held > 0 {
			return AdminReport{}, fmt.Errorf("%s already holds slot %d; a peer holds at most one",
				newPeer, held)
		}
		pos := res.Position()
		rep := AdminReport{
			Act: act, Slot: res.Slot, PeerID: res.PeerID, NewPeerID: newPeer, Position: &pos,
			// A handover changes no shape and moves no lane: the reservation is
			// rebound, and the position it names does not move. The hole list is
			// reported unchanged for the same reason — a handover offers the map
			// nothing new, which is exactly what tells it apart from a release.
			LanesChanged:             []LaneChange{},
			HolesAfter:               s.grid.Holes(),
			MaxSlotEverIssued:        s.grid.MaxSlotEverIssued,
			HeldEntriesAddressedHere: fmt.Sprintf(heldEntriesSentence, res.Slot),
			RingStateHash:            hash,
			Notes: []string{
				fmt.Sprintf("slot %d and position (%d,%d) are unchanged; only the identity moves, and no lane moves",
					res.Slot, res.Col, res.Row),
				res.PeerID + " keeps its journal, its genome cache and its logs; " + newPeer +
					" inherits NOTHING but the address and starts with an empty journal",
				fmt.Sprintf("in-flight work addressed to slot %d arrives at %s, because routing is on the "+
					"slot. If you do not want that, you want release-slot", res.Slot, newPeer),
				"this is also the credential recovery path: " + newPeer + " gets a freshly minted " +
					"credential and " + res.PeerID + "'s is dropped",
			},
		}
		s.fillDarkLocked(&rep, res.PeerID)
		return rep, nil

	case ActEvictPeer:
		peerID := strings.TrimSpace(req.PeerID)
		if peerID == "" {
			return AdminReport{}, errors.New("evict-peer needs peerId")
		}
		d, lift, err := parseEvictionFor(req.For)
		if err != nil {
			return AdminReport{}, err
		}
		rep := AdminReport{
			Act: act, PeerID: peerID, Lifts: lift,
			LanesChanged: []LaneChange{},
			// An eviction RELEASES NOTHING, so the hole list is the map's current
			// one and the address counter does not move. Reporting them anyway is
			// what makes the three acts comparable at a glance.
			HolesAfter:               s.grid.Holes(),
			MaxSlotEverIssued:        s.grid.MaxSlotEverIssued,
			HeldEntriesAddressedHere: heldEntriesSentenceNoSlot,
			RingStateHash:            hash,
		}
		if res, ok := s.grid.ResOfPeer(peerID); ok {
			pos := res.Position()
			rep.Slot = res.Slot
			rep.Position = &pos
			rep.HeldEntriesAddressedHere = fmt.Sprintf(heldEntriesSentence, res.Slot)
			// An eviction is a LIVENESS act: the map treats an evicted peer
			// exactly as it treats a dark one, so the lanes that will re-pair are
			// the same ones a departure would move.
			rep.LanesChanged = s.laneChangesLocked(res)
		}
		s.fillDarkLocked(&rep, peerID)
		if lift {
			cur, evicted := s.evictionLocked(peerID)
			if !evicted {
				return AdminReport{}, fmt.Errorf("%s is not evicted; there is nothing to lift", peerID)
			}
			rep.EvictedUntilMs = untilMs(cur)
			rep.Notes = []string{
				peerID + " is admitted again from the moment this is confirmed",
				"nothing about the map changes: an eviction released nothing, so there is nothing to give back",
			}
			return rep, nil
		}
		rep.EvictedFor = "until lifted"
		if d > 0 {
			until := time.Now().Add(d)
			rep.EvictedUntilMs = until.UnixMilli()
			rep.EvictedFor = d.String()
		}
		rep.Notes = []string{
			"this RELEASES NOTHING: the reservation, the slot number and the position all survive, so " +
				peerID + "'s return is an ordinary reason:\"reclaimed\" and its journal stays addressable throughout",
			"the map treats an evicted peer exactly as it treats a dark one; its neighbours route around it (§8)",
			"the peer is told what a draining relay tells it (close 4005) and is given no other signal — " +
				"eviction is deliberately the weaker tool, and suppressing a name at the renderer costs " +
				"that world nothing",
			"this eviction lives in this relay process only: a relay restart drops every session anyway, " +
				"and it does not survive one",
		}
		return rep, nil
	}
	return AdminReport{}, errors.New("unknown act")
}

func (s *Server) fillDarkLocked(rep *AdminReport, peerID string) {
	if _, live := s.peers[peerID]; live {
		rep.Live = true
		return
	}
	if m, ok := s.meta[peerID]; ok && m.darkSinceMs > 0 {
		rep.DarkForMs = time.Now().UnixMilli() - m.darkSinceMs
	}
}

// laneChangesLocked lists every peer whose effective lane points at this
// reservation today, and where that lane goes once it is gone. It is the same
// walk the console's report runs, over all four edges, because the ripple is
// symmetric under two-way lanes (§17, B13).
func (s *Server) laneChangesLocked(going Reservation) []LaneChange {
	out := []LaneChange{}
	for _, res := range s.grid.Slots {
		if res.Slot == going.Slot {
			continue
		}
		ok := s.deliverableLocked(res)
		for _, edge := range contracta.CanonicalEdges() {
			target, _, found := s.grid.Effective(res, edge, ok)
			if !found || target.Slot != going.Slot {
				continue
			}
			change := LaneChange{PeerID: res.PeerID, Edge: edge, From: going.Slot}
			if next, _, ok2 := s.grid.Effective(res, edge, s.deliverableWithoutLocked(res, going.Slot)); ok2 {
				n := next.Slot
				change.To = &n
			}
			out = append(out, change)
		}
	}
	return out
}

// deliverableWithoutLocked is §8's filter with one slot forced undeliverable,
// which is how the report answers "and where does that lane go instead".
func (s *Server) deliverableWithoutLocked(me Reservation, gone int) Deliverable {
	base := s.deliverableLocked(me)
	return func(res Reservation) string {
		if res.Slot == gone {
			return contractb.SkipPeerOffline
		}
		return base(res)
	}
}

// ---------------------------------------------------------------- application

// AuditAct is B28's fourth property: ONE DURABLE LINE PER ACT, naming which
// grant, which act, which slot, the state before, and the reason string the
// operator supplied. It runs BEFORE the change, so an act that dies half-way
// still leaves a record of having been attempted — the opposite ordering
// records only the acts that worked, which is the wrong half.
//
// It is the relay's log line, and the relay's log is the durable surface it
// already has: --log-file with logRotateMb and logKeep bound it (§20, B20). The
// same line is written for a console act and for one over the path, so an
// operator reading a log after the fact cannot tell the two apart by accident
// of format — only by the `via` field, which is the honest difference.
func (s *Server) AuditAct(act, operator, grant, reason string, slot int, peerID, newPeerID string) {
	s.mu.Lock()
	before := ringStateHash(s.grid)
	shape := s.grid.Shape()
	count := s.grid.Size()
	maxSlot := s.grid.MaxSlotEverIssued
	s.mu.Unlock()
	if strings.TrimSpace(reason) == "" {
		reason = "<none given>"
	}
	s.log.Warn("relay: ADMIN ACT",
		"act", act, "via", operator, "grant", grant,
		"slot", slot, "peer", peerID, "newPeer", newPeerID, "reason", reason,
		"stateBefore", fmt.Sprintf("ring %s, map %dx%d, %d slot(s), maxSlotEverIssued %d",
			before, shape.Width, shape.Height, count, maxSlot))
}

func (s *Server) applyAdmin(p *adminPending, operator string) (AdminApplied, error) {
	s.AuditAct(p.act, operator, peercred.GrantAdmin, p.reason, p.slot, p.peerID, p.newPeerID)

	switch p.act {
	case ActReleaseSlot:
		if err := s.ReleaseSlot(p.slot); err != nil {
			return AdminApplied{}, err
		}
	case ActHandoverSlot:
		_, _, err := s.HandoverSlot(p.slot, p.newPeerID)
		if err != nil {
			return AdminApplied{}, err
		}
	case ActEvictPeer:
		return s.applyEviction(p, operator)
	default:
		return AdminApplied{}, errors.New("unknown act")
	}

	s.mu.Lock()
	out := AdminApplied{
		Act: p.act, Slot: p.slot, PeerID: p.peerID, Applied: true,
		Map: s.grid.Shape(), SlotCount: s.grid.Size(),
		MaxSlotEverIssued: s.grid.MaxSlotEverIssued,
	}
	s.mu.Unlock()
	out.Epoch = s.publishNow()

	if p.act == ActHandoverSlot {
		// B22: the handover IS the credential recovery path, and there is no
		// other. The new identity gets a freshly minted credential, the old one's
		// is dropped, and the secret exists here for the only moment it will.
		secret, err := s.creds.Mint(p.newPeerID, peercred.GrantPeer)
		if errors.Is(err, peercred.ErrExists) {
			secret, err = s.creds.Remint(p.newPeerID)
		}
		if err != nil {
			return out, fmt.Errorf(
				"the slot moved but its new credential could not be minted (%w); mint it by hand: "+
					"multiverse-relay --mint-credential %s", err, p.newPeerID)
		}
		if err := s.creds.Forget(p.peerID); err != nil {
			return out, fmt.Errorf("the old identity's credential could not be dropped: %w", err)
		}
		out.PeerID = p.newPeerID
		out.JoinString = peercred.JoinString{
			RelayURL: s.advertiseURL, PeerID: p.newPeerID, Secret: secret,
			Grant: peercred.GrantPeer,
		}.Line()
	}
	return out, nil
}

func (s *Server) applyEviction(p *adminPending, operator string) (AdminApplied, error) {
	out := AdminApplied{Act: p.act, Slot: p.slot, PeerID: p.peerID, Applied: true}
	if p.until.IsZero() && p.forSpec == "" {
		// A lift.
		s.mu.Lock()
		delete(s.evictions, p.peerID)
		shape, count, maxSlot := s.grid.Shape(), s.grid.Size(), s.grid.MaxSlotEverIssued
		s.mu.Unlock()
		out.Lifted = true
		out.Map, out.SlotCount, out.MaxSlotEverIssued = shape, count, maxSlot
		out.Epoch = s.publishNow()
		s.log.Warn("relay: eviction lifted; this peer is admitted again",
			"peer", p.peerID, "operator", operator, "reason", p.reason)
		return out, nil
	}
	s.mu.Lock()
	s.evictions[p.peerID] = eviction{Until: p.until, Reason: p.reason, At: time.Now()}
	live := s.peers[p.peerID]
	shape, count, maxSlot := s.grid.Shape(), s.grid.Size(), s.grid.MaxSlotEverIssued
	s.mu.Unlock()
	if live != nil {
		// §7.5: the act closes that peer's connection with 4005 and refuses it
		// for the stated period. 4005 is what a DRAINING relay says, and the
		// contract gives the refusal no shape of its own: an evicted peer's
		// experience is deliberately indistinguishable from a routine drain
		// except by its persistence.
		live.conn.Close(contractb.CloseShuttingDown, "this relay is not accepting this peer")
	}
	out.Map, out.SlotCount, out.MaxSlotEverIssued = shape, count, maxSlot
	out.EvictedUntilMs = untilMs(p.until)
	out.Epoch = s.publishNow()
	s.log.Warn("relay: peer evicted; the reservation, the slot number and the position all survive",
		"peer", p.peerID, "operator", operator, "reason", p.reason,
		"until", evictionUntil(p.until), "wasLive", live != nil,
		"note", "this is a LIVENESS act and not a placement one; the map treats this peer as dark "+
			"and its neighbours route around it. It does not survive a relay restart")
	return out, nil
}

// publishNow flushes the registry change the act made, so the map an operator
// reads next is the map the act produced rather than the one the coalescing
// window still holds.
func (s *Server) publishNow() int64 {
	s.publish()
	s.mu.Lock()
	defer s.mu.Unlock()
	var epoch int64
	for _, p := range s.peers {
		p.mu.Lock()
		if p.epoch > epoch {
			epoch = p.epoch
		}
		p.mu.Unlock()
	}
	return epoch
}

// ---------------------------------------------------------------- evictions

// EvictPeer is the console half of the third act (§7.5). It takes the same
// arguments the path does and produces the same audit line.
func (s *Server) EvictPeer(peerID string, d time.Duration, lift bool, reason string) (time.Time, error) {
	if peerID == "" {
		return time.Time{}, errors.New("relay: --evict-peer needs a peer id")
	}
	p := &adminPending{act: ActEvictPeer, peerID: peerID, reason: reason}
	if !lift && d > 0 {
		p.until = time.Now().Add(d)
		p.forSpec = d.String()
	} else if !lift {
		p.forSpec = "until lifted"
	}
	if lift {
		s.mu.Lock()
		_, evicted := s.evictionLocked(peerID)
		s.mu.Unlock()
		if !evicted {
			return time.Time{}, fmt.Errorf("relay: %s is not evicted; there is nothing to lift", peerID)
		}
	}
	if _, err := s.applyAdmin(p, "console"); err != nil {
		return time.Time{}, err
	}
	return p.until, nil
}

// evictionLocked answers whether peerID is currently refused admission.
func (s *Server) evictionLocked(peerID string) (time.Time, bool) {
	ev, ok := s.evictions[peerID]
	if !ok {
		return time.Time{}, false
	}
	if !ev.Until.IsZero() && time.Now().After(ev.Until) {
		// EXPIRY IS SILENT AND AUTOMATIC. A stated period that needed an operator
		// to end it would be an indefinite eviction with a friendlier name.
		delete(s.evictions, peerID)
		return time.Time{}, false
	}
	return ev.Until, true
}

func (s *Server) evictedUntil(peerID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictionLocked(peerID)
}

// Evictions lists the peers this relay is currently refusing, for the operator
// console and for a test.
func (s *Server) Evictions() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]time.Time{}
	for id := range s.evictions {
		if until, ok := s.evictionLocked(id); ok {
			out[id] = until
		}
	}
	return out
}

// closeEvicted answers an evicted peer's reconnect. It completes the upgrade
// and closes 4005, which is EXACTLY what a draining relay does.
//
// THE INDISTINGUISHABILITY IS THE CONTRACT'S, NOT AN OVERSIGHT HERE. B28 gives
// eviction close 4005 SHUTTING_DOWN and gives its refusal no shape of its own —
// no close code, no lastRefusal axis, no wire field. An evicted peer therefore
// sees what a routine drain looks like and can tell them apart only by
// persistence. Inventing a distinguishable refusal would be a wire change this
// package has no authority to make.
func (s *Server) closeEvicted(w http.ResponseWriter, r *http.Request, peerID string, until time.Time) {
	s.log.Warn("relay: refusing an evicted peer's connection",
		"peer", peerID, "remote", r.RemoteAddr, "until", evictionUntil(until),
		"note", "the peer is told what a draining relay tells it (4005) and nothing else; B28 gives "+
			"this refusal no distinguishable shape")
	// s.acceptOptions() and not a literal, and §24 B35's own comment on that
	// helper explains why THIS door in particular: an evicted peer must see the
	// handshake a draining relay gives it, and a Sec-WebSocket-Extensions header
	// that differed here would be a distinguishable refusal B28 does not grant.
	conn, err := websocket.Accept(w, r, s.acceptOptions())
	if err != nil {
		return
	}
	_ = conn.Close(contractb.CloseShuttingDown, "this relay is not accepting this peer")
}

func evictionUntil(until time.Time) string {
	if until.IsZero() {
		return "until lifted"
	}
	return until.UTC().Format(time.RFC3339)
}

func untilMs(until time.Time) int64 {
	if until.IsZero() {
		return 0
	}
	return until.UnixMilli()
}

// parseEvictionFor reads --for. An empty spec is "until lifted"; "0" is a LIFT,
// which is how §7.5's "or until lifted" is spelled without inventing a fourth
// act on a table that has exactly three.
func parseEvictionFor(spec string) (time.Duration, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, false, nil
	}
	if spec == "0" || spec == "lift" {
		return 0, true, nil
	}
	d, err := time.ParseDuration(spec)
	if err != nil {
		return 0, false, fmt.Errorf("for %q is not a duration (e.g. 24h, 30m); 0 lifts an eviction", spec)
	}
	if d <= 0 {
		return 0, true, nil
	}
	return d, false, nil
}

func (s *Server) sweepAdminTokensLocked(now time.Time) {
	for token, p := range s.adminTokens {
		if now.Sub(p.createdAt) > adminConfirmTTL {
			delete(s.adminTokens, token)
		}
	}
}

// ringStateHash is what the confirmation token is bound to: the map's shape,
// every reservation in it, and the slot counter. Sixteen hex characters is
// enough for a value an operator compares by eye and a confirm call echoes.
func ringStateHash(g *Grid) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d:%d:%d\n", g.Width, g.Height, g.MaxSlotEverIssued)
	for _, res := range g.Order() {
		fmt.Fprintf(h, "%d:%d:%d:%s\n", res.Slot, res.Col, res.Row, res.PeerID)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
