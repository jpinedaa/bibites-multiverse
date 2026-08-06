# M4 livelock findings — the slot-6 replay storm of 2026-08-06

This document records the first serious incident of the six-world LAN rig: a
self-sustaining livelock that held slot 6 at ~1 TPS for over an hour while every process,
socket and contract rule involved was individually behaving as specified. It exists for
the same reason `m1_findings.md` and `m4_portal_findings.md` do — the next person to touch
this code should not have to rediscover any of it — and because the fix landed as contract
amendments (`contract-a.md` §15 A29, `contract-b-m4.md` §14 B8), this is the narrative
those amendments compress.

## 1. Timeline, from the logs

All times local, 2026-08-06. Slot 6 is the far-end machine at grid (2,1); its sidecar log
is `sidecar-slot6.log`, the mod's is BepInEx `LogOutput.log`.

| When | What |
|---|---|
| ~06:00–09:00 | An arrival flood: ~10,786 organisms accepted into slot 6, 2,000–3,000/hr. |
| ~06:59 | First `MIGRATION_NACK code=OVERLOADED` — the paced backlog reached `inboundQueueMax` (64) and stopped draining. `pacedDepth=64`, all entries `Open`. |
| 07:00–14:39 | The dam holds. The world runs, but the flood-spawned migrants starve; population settles near 50. The upstream sender re-forwards every stuck entry every `forwardRetryMs` (5 s). |
| 14:39 | One `FixedUpdate` spends >3.5 s applying deliveries. `Update` never runs, no `HEARTBEAT` goes out, the sidecar closes `4004` (`silentFor=3.5s`). |
| 14:39–15:32+ | The livelock: on every reconnect the sidecar replays the same 64 un-ACKed deliveries (`queued un-ACKed inbound deliveries for replay count=64` — identical on all eleven generations), the mod re-ingests them in one `FixedUpdate`, blows the deadline again, `4004`, repeat. The game process burns ~2.3 cores at ~1 TPS with ~50 organisms alive. |
| Total collateral | 203,735 `duplicate MIGRATION_PAYLOAD, delivering nothing` — each one a full multi-MiB decode on the pegged machine. |

Diagnostic signature, for the next time: **compute-bound game process + idle sidecar +
healthy LAN + `count=<N> pacedDepth=<N>` repeating verbatim across reconnect generations.**
The tempting hypotheses — a dead peer, a stuck Send-Q, a blocking network call in the tick
path — were all false. Nothing on the game's main thread ever touches the network; the
architecture was right about that and it did not matter.

## 2. The mechanism — five correct rules, one wrong composition

1. **Contract A §5.7 (pre-A29)** told the mod to enqueue and process on the main thread —
   and the mod drained the *entire* queue in one `FixedUpdate`. 64 spawns ≈ 2.5–4 s.
2. **Contract A §8** told the sidecar to close `4004` after 3.5 s of heartbeat silence —
   and the heartbeat is built in `Update`, which a long `FixedUpdate` starves. The close
   is *correct*: the mod genuinely cannot process deliveries. But the cause of the silence
   was the deliveries.
3. **Contract A §7.5** told the sidecar to replay every un-ACKed inbound on reconnect,
   unconditionally — and the implementation marked the whole batch due-now and reset the
   pacing bucket on every socket connect, refunding a 5-token burst each generation.
4. **Contract A §7.3 (pre-A29)** scoped the `migrationId` ledger to the connection — so
   every reconnect wiped it, and the `entityId` world-scan backstop missed because the
   flood-spawned organisms had already died. Every replay re-ran the full spawn.
5. **Contract B §9.2/B5** told the sender the retry is free because the destination
   deduplicates — and the receiver paid a full decode before the dedup lookup, 203,735
   times, on the machine that had no CPU to spare.

Each generation of the cycle re-created the stall that caused it. The pacer could not
drain the dam because its clock is the sim's own time (correct — see §4), and the sim was
barely advancing because the main thread was re-ingesting the dam. Steady state.

## 3. The defences, as landed

Every link gets its own break; any one would have degraded the cycle, all of them together
are why it cannot re-form. No wire message, field or enum changed.

| # | Side | Change | Where |
|---|---|---|---|
| 1 | mod | Ingest budget: ≤4 spawns / ~8 ms per `FixedUpdate`, ≥1 always; ≤16 frames / ~2 ms parsed per `Update`; strictly in order; `pendingIn` counts the whole deferred backlog | `MultiverseClient.cs` `ApplyMutations`/`DrainTransport`; contract-a §5.7, §5.2 (A29) |
| 2 | mod | The `migrationId` ledger survives socket reconnects; cleared only on `sessionId` change. A replayed handled delivery is an O(1) `duplicate:true` ACK even if the organism died | `MigrationImporter.cs` `OnHandshake`; contract-a §7.3 (A29) |
| 3 | mod | Hot-path SHA-256 and per-organism log chatter gated behind `MULTIVERSE_DEBUG_INGEST`; one line per ingest at info level | `MigrationImporter.cs`, `OrganismLinks.cs` |
| 4 | sidecar | Quiet-mod gate: `MIGRATE_IN` release held while the newest `HEARTBEAT` is older than `heartbeatDeliveryGraceMs` (1500 ms) — trips before the 3.5 s close, so a stalling mod is paused into, not flooded | `sidecar.go` `deliverLocked`; contract-a §7.5, §8 (A29) |
| 5 | sidecar | Pacing bucket resets only on a new `sessionId`, never on a same-session socket reconnect | `contracta_server.go`; contract-a §7.5 (A29) |
| 6 | sidecar | Replay batch waits `min((minAttempt−1)·500 ms, 5 s)` — one order-preserving delay, escalating per generation | `contracta_server.go` `replayInboundLocked`; contract-a §7.5 (A29) |
| 7 | sidecar | Dedup precedes the decode: `migrationId` extracted with a minimal parse, journal lookup, *then* (new ids only) the full decode | `contractb_client.go` `onMigrationPayload`; contract-b §6.6 (B8) |
| 8 | sidecar | Live-destination re-forward backs off 5 s → 60 s doubling; dark cadence and hold clock untouched (B5 stands) | `sidecar.go` `tickOutbound`; contract-b §9.2–§9.4, §12 (B8) |

Deliberately **not** done, and why:

- **The pacer keeps its simulated-time clock.** It is load-bearing: it is what stops a
  flood entering a world that cannot run it, and it is why a healthy 20× world drains a
  dam 20× faster. The livelock is broken by not re-loading the stalled world, not by
  draining into it harder.
- **The heartbeat stays on the main thread.** A transport-thread heartbeat would assert
  exactly the liveness that is false during a stall — "I can process deliveries" — and
  mask the fault. The honest fix is to not have long frames (defence 1).
- **No ACK-early.** The spawn is still the proof of delivery (§5.8, §6.7). The fix for
  replay cost is the durable ledger, not weaker custody.
- **`4004` still closes at 3500 ms.** With defence 1 the heartbeat survives any burst, so
  the timeout is finally what it was always meant to be: a dead-mod detector.

## 4. Verification

- `go test ./...` green, including six new tests in
  `go/internal/sidecar/livelock_test.go`: same-session reconnect keeps the bucket; replay
  delay escalates and drains; delivery pauses into a silent mod and resumes; live
  re-forward backs off and dark resets it; a journaled duplicate with an unparseable body
  is ignored without a decode; the two schedule formulas. No existing test weakened;
  `TestReconnectReplay` passes unchanged.
- `gofmt` clean, `go vet` clean, `CGO_ENABLED=0 go build ./cmd/...` and the
  `GOOS=windows` cross-builds of `sidecar` and `relay` succeed. (`-race` could not run in
  the WSL build env — no C compiler; the one new cross-thread read, `lastHeartbeat` in
  `deliverLocked`, was verified by inspection to be `s.mu`-owned at every site.)
- `dotnet build -c Release` of the mod: clean, 0 errors (only the pre-existing
  `MSB3277` warning). No C# test suite exists; the mod-side changes were review-verified.

## 5. Recovery and deployment notes

The livelock does not self-heal on old binaries — the 64-entry journal batch replays
forever. Recovery is: stop the slot-6 game and sidecar, deploy the new sidecar binary and
mod DLL, restart. The journal is durable, so the 64 arrive again — now paced, budgeted,
and deduped — and the world absorbs them over a few simulated minutes. The **sender-side**
defences (7, 8) only take effect when the *other* machine's sidecar is updated too; both
machines run the same binary, so one rebuild of the farend bundle covers it. Nothing in
the journal format, config format or wire changed: old journals replay under new binaries.
