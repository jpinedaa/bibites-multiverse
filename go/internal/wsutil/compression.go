package wsutil

import "github.com/coder/websocket"

// The Contract B peer wire's ONE compression setting, in ONE place
// (contracts/contract-b-m4.md §3 Transport, §24 B35).
//
// WHY IT EXISTS AT ALL. Every frame on this wire is UTF-8 JSON with a fixed
// envelope and a repetitive body, and nothing compressed it at any layer:
// nginx sets `gzip off` on /contract-b/ because a WebSocket is not a response
// body, and TLS does not compress. A 2026-08-16 loopback capture between nginx
// and the relay measured seven peer connections at ~2,508 GiB a month against a
// 3,072 GiB allowance — 82.7% of it MIGRATION_PAYLOAD at ~17.9 KB a crossing,
// 11.6% PEER_STATUS at 12.8 KB broadcast to every peer and the archive. Offline
// deflate over the same capture returned 9.0x, 9.8x and 8.5x overall.
//
// NEGOTIATION IS THE WHOLE COMPATIBILITY STORY, AND IT IS RFC 7692's, NOT OURS.
// permessage-deflate is a WebSocket EXTENSION: the client offers it in
// Sec-WebSocket-Extensions on the HTTP upgrade and the server either echoes it
// or says nothing. It is therefore used only when BOTH ends offer it.
//
//   - An old sidecar that does not offer it gets EXACTLY today's behaviour, on
//     the same URL, with the same frames. There is no version bump to gate on,
//     no field to detect and no fallback path to write, which is why §24 B35
//     answers §4's test with "neither major nor minor".
//   - The relay MUST therefore go on accepting uncompressed connections
//     FOREVER. This project moves its fleet by publication (release/README.md)
//     and cannot make a participant upgrade. A relay that required the
//     extension would evict every world that had not updated.
//   - Both halves are enabled here, because the saving is on both halves:
//     relay->peer was 1,364 GiB a month and peer->relay 1,144 GiB.
//
// WHY CONTEXT TAKEOVER. CompressionContextTakeover keeps one 32 KiB sliding
// window per connection across messages, so the second PEER_STATUS costs a back
// reference where the first cost a dictionary. That is the difference between
// ~8.5x and ~4.5x on this traffic, and this wire is the case the mode was
// written for: long-lived connections carrying near-identical JSON.
//
// WHAT IT COSTS, AND WHEN TO CHANGE IT. coder/websocket holds a flate.Writer
// (~1.2 MB) and a 32 KiB read window for the life of each connection under this
// mode, so ~1.25 MB per connection per process. The hosted relay carries seven
// peers and the archive: about 10 MB against a MemoryMax of 512M, which is
// nothing. IT DOES NOT STAY NOTHING. At a hundred peers it is ~125 MB and at
// several hundred it is the relay's whole budget, so the fallback is
// CompressionNoContextTakeover — a pooled writer per message, no per-connection
// window, roughly half the ratio and near-zero fixed cost. Change the constant
// below and every one of the six call sites moves with it; that is why they all
// read it from here.
//
// THE RELAY HAS AN OFF SWITCH AND THE CLIENTS DO NOT, deliberately.
// --ws-compression / MULTIVERSE_RELAY_WS_COMPRESSION lets the operator stop
// offering the extension without rolling a binary back, and because the
// extension needs both ends, a relay that stops offering it puts the WHOLE map
// back on uncompressed frames at the next reconnect. A participant's sidecar
// gets no knob for the same reason B23 gives its TLS verification none: there
// is no metric in a participant's hands that the setting answers to.
const (
	// PeerCompressionMode is the mode all six Contract B endpoints negotiate:
	// the relay's four accepts, the sidecar's dial and the archive's dial.
	//
	// It is NOT used on Contract A (contract-a.md §2, unchanged). That wire is
	// loopback between two processes on one machine, its bytes cross no
	// network and cost no transfer allowance, and compressing them would buy a
	// per-connection megabyte in every participant's game host for nothing.
	PeerCompressionMode = websocket.CompressionContextTakeover

	// PeerCompressionThreshold is the smallest message that is compressed
	// rather than sent as it is.
	//
	// The library's default under context takeover is 128 bytes, which is a
	// hair under the smallest frame this wire emits: a PONG is ~145 bytes of
	// envelope before its nonce. 64 is chosen so the floor sits BELOW the whole
	// catalogue rather than inside it, and so a future smaller frame cannot
	// fall off the compressed path by accident. Small frames are worth
	// compressing here precisely because of context takeover: the envelope
	// prefix is already in the window, so a PING costs a back reference
	// instead of 145 bytes. Below ~64 bytes an incompressible message can grow
	// by a few bytes of deflate block header, which is the only reason there is
	// a floor at all.
	PeerCompressionThreshold = 64
)

// PeerAcceptOptions applies the peer wire's compression settings to a
// server-side upgrade and returns the same struct, so a caller can keep writing
// its other fields as a literal. A nil o is allocated.
//
// It sets ONLY the two compression fields. Everything else about an upgrade —
// the origin check, the subprotocol, the read limit — belongs to the caller.
func PeerAcceptOptions(o *websocket.AcceptOptions) *websocket.AcceptOptions {
	if o == nil {
		o = &websocket.AcceptOptions{}
	}
	o.CompressionMode = PeerCompressionMode
	o.CompressionThreshold = PeerCompressionThreshold
	return o
}

// PeerDialOptions is PeerAcceptOptions for the client half — the sidecar and
// the archive. A client that calls it OFFERS the extension; whether it is used
// is the relay's answer.
func PeerDialOptions(o *websocket.DialOptions) *websocket.DialOptions {
	if o == nil {
		o = &websocket.DialOptions{}
	}
	o.CompressionMode = PeerCompressionMode
	o.CompressionThreshold = PeerCompressionThreshold
	return o
}
