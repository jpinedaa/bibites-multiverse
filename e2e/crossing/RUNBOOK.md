# Contract B v4 crossing fixture

This directory records the test fixture that moved an M4 map to `contract-b/4.0`.
The original live execution receipt and machine-specific commands are not public operations guidance.

Use this fixture only in an isolated test map.
Do not apply its archived patch to a current deployment without checking the target files first.

## What the crossing proves

The crossing exercises these changes together:

- Contract B moves to `/contract-b/v4` over TLS.
- Each peer uses a credential bound to its `peerId`.
- The archive uses a separate `subscribe` credential.
- Contract A uses its bearer-token file.
- Existing slot reservations and journals survive the change.
- The archive can replay while map activity is stopped.
- A remote Windows peer can trust the test certificate authority and reclaim its slot.

The historical test completed with no discarded journal bytes and no archive gap.
Keep exact dates, machine addresses, and execution evidence in private operations storage.

## Files

| File | Purpose |
|---|---|
| `RUNBOOK.md` | This reusable fixture guide. |
| `contract-b-4.patch` | Archived patch for the version transition. |
| `mint-tls.sh` | Creates test TLS material in the ignored TLS directory. |
| `mint-credentials.sh` | Creates peer and subscriber credentials in protected local files. |
| `rig-check.sh` | Reads the test status API and checks the expected map state. |

The patch is historical evidence.
Current scripts can already contain its behavior.
Use `git apply --check` before any use.

## Safety boundary

Follow these rules:

- Run the fixture against copied data or a disposable test map.
- Stop if observed state differs from the selected runbook version.
- Keep build output on the same filesystem as the target binaries.
- Do not build over a binary that a process holds open.
- Do not pass an insecure token or TLS bypass flag.
- Do not print local peer secrets to a terminal or log.
- Do not operate a remote participant machine without its operator.
- Do not use live resource names or addresses from an old receipt.

The test certificate authority is not a production trust root.
Remove it from a remote test account after the fixture is complete.

## Required private inputs

Select these inputs before a run:

- The test relay hostnames and addresses.
- The test map data directory.
- The peer identifiers, coordinates, and remote peer.
- The protected credential directory.
- The build and rollback directories.
- The expected Contract A, Contract B, mod, and game versions.

Pass supported values through the environment variables of the fixture scripts.
Do not add one machine's values to this public guide.

## Phase 1: establish the baseline

Run the read-only fixture check:

```sh
e2e/crossing/rig-check.sh
```

Make sure that all expected peers and lanes are healthy.
Resolve an unexplained dark slot before the crossing.

Capture these baseline values in private test evidence:

- Live and dark slot counts.
- Lane state.
- Ledger record count.
- Journal error counts.
- Binary and plugin hashes.
- Current protocol versions.

## Phase 2: create rollback copies

Copy the current binaries, plugin, `ring.json`, and `peers.json` to protected local storage.
Keep both relay identity files from the same point in time.

Do not copy credential values into the repository.
Do not include raw saves or logs in a commit.

The archive ledger is append-only.
A full archive copy belongs to the approved backup process, not this fixture directory.

## Phase 3: build and check

Build the new Go commands in a scratch directory on the target filesystem:

```sh
go -C go fmt ./...
go -C go vet ./...
go -C go test -p 1 ./...
CGO_ENABLED=0 go -C go build -o <same-filesystem-build-dir>/ ./cmd/...
```

Check the mod build and supported game assembly through the normal project build.

Do not write build output over a running binary.
Use atomic replacements only after the test map stops.

## Phase 4: create test TLS and credentials

If a remote peer uses the LAN, set the selected Windows LAN address explicitly.
The following example uses the documentation address `192.0.2.10`:

```sh
LAN_HOST=192.0.2.10 e2e/crossing/mint-tls.sh
```

Make sure that the leaf certificate verifies against the test authority.
Make sure that every dialed name is present in its Subject Alternative Name list.

Point `RELAY_BIN` at the new relay build.
Pass the same LAN address to the credential script:

```sh
LAN_RELAY_HOST=192.0.2.10 \
  RELAY_BIN=<same-filesystem-build-dir>/relay \
  e2e/crossing/mint-credentials.sh
```

If you already have the complete remote URL, set `FAR_URL` instead:

```sh
FAR_URL=wss://192.0.2.10:8795/contract-b/v4 \
  RELAY_BIN=<same-filesystem-build-dir>/relay \
  e2e/crossing/mint-credentials.sh
```

If every peer is local, mint loopback-only TLS and clear `FAR_PEER`:

```sh
e2e/crossing/mint-tls.sh
FAR_PEER= RELAY_BIN=<same-filesystem-build-dir>/relay \
  e2e/crossing/mint-credentials.sh
```

The check mode reads state without a LAN address:

```sh
RELAY_BIN=<same-filesystem-build-dir>/relay \
  e2e/crossing/mint-credentials.sh --check
```

Check mode reports a missing remote URL, but it does not require one.
A mint run requires `FAR_URL` or `LAN_RELAY_HOST` when `FAR_PEER` is set.

The script must report an unchanged `ring.json` digest.
It must create one credential for each peer and one `subscribe` credential for the archive.

Keep secret files at mode `0600`.
Transfer a remote-peer secret only through an approved private channel.

## Phase 5: stop map activity

Archive the test logs before the games restart.
BepInEx can truncate its active log on launch.

Stop games, sidecars, the archive, and the relay through the current test harness.
Make sure that all fixture ports are free.

Stop relay activity before the archive restart when ledger continuity is required.
An archive that replays while crossings continue creates a permanent record gap.

## Phase 6: replace the test artifacts

Deploy the new plugin through the normal mod deployment command.
Replace the stopped Go binaries with the checked scratch builds.

If the archived patch is still required, check and apply it now:

```sh
git apply --check -p1 e2e/crossing/contract-b-4.patch
git apply -p1 e2e/crossing/contract-b-4.patch
```

Do not apply the patch before the new binaries are ready.
Old binaries do not understand the new credential and TLS flags.

Run `bash -n` on each changed harness script.
Make sure that no live `--token-file` argument remains.

## Phase 7: start and gate

Start the relay first.
Make sure that it reports TLS, `/contract-b/v4`, and the expected credential count.

Start the archive next.
Wait for replay and the subscription message.

Start each sidecar and wait for `reason=reclaimed` at its original position.
Start each game only after its sidecar receives the slot grant.

Apply the target time scale after each world loads.
Read the achieved value from `/api/status`.

## Phase 8: remote peer

The remote operator receives these items through approved channels:

- A far-end bundle, built on demand with `farend/make-farend-bundle.sh`.
- The test certificate-authority certificate.
- The remote peer credential file.
- The relay name that the certificate covers.

The remote operator runs the instructions in `farend/README.md`.
No command in this fixture controls that machine.

Wait for the relay to report that the remote peer reclaimed its original slot.
Do not infer success from silence.

## Verification gates

The crossing passes only when all applicable gates are true:

- All expected slots are live and mod-connected.
- Every slot reports `contract-a/2.4`.
- The relay uses `/contract-b/v4` over TLS.
- Every returning peer reports `reason=reclaimed` at its original coordinate.
- Journal logs contain zero `discardedBytes` errors.
- The archive reports `relayConnected: true`.
- Ledger count increases after map activity resumes.
- Contract A token files exist at mode `0600`.
- Each running process matches the intended binary digest.

Run the fixture check with wire checks enabled:

```sh
e2e/crossing/rig-check.sh --wire
```

Record exact outputs outside the public repository.

## Rollback

Before archive subscription, restore the copied binaries, plugin, and harness files.
Then start the old fixture.

After archive subscription, a rollback needs a second archive restart.
Stop relay activity first if the record must remain continuous.

Never reconstruct a missing join string from a verifier.
Use a formal slot handover to create a new peer identity.

Never replace a damaged ledger before you preserve and inspect the damaged copy.

## Historical result

The original crossing established these engineering facts:

- Restarting the archive inside a full map outage can avoid a ledger gap.
- Existing peer reservations can cross to bound credentials without renumbering.
- Sidecar journals can replay without discarded bytes.
- A full stop does not create the custody burst seen during a game-only restart.
- Replay time must be calculated from current ledger size.

The exact live receipt is operations data.
It is not part of this public fixture.
