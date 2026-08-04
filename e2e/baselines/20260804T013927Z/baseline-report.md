# M3 LAN rig baseline — T0 (20260804T013927Z)

Captured 2026-08-04T01:39:31Z (wall clock, UTC) by `e2e/baseline.sh`.
Repo HEAD `8d5021d`. Nothing was started, stopped, rebuilt or reconfigured to take it;
the only dev commands sent were `census`, `count` and `save`, on the two LOCAL slots.

## The rig at this instant

| | |
|---|---|
| Wall clock (UTC) | 2026-08-04T01:39:31Z |
| Wall clock (local) | 2026-08-03 21:39:38 EDT |
| Local slots | 1 and 3, on this machine |
| Remote slot | 2, on the owner's second computer — observed, never driven |
| Ring | `{"ring":[{"slot":1,"peerId":"slot-1"},{"slot":2,"peerId":"slot-2"},{"slot":3,"peerId":"slot-3"}],"maxSlotEverIssued":3}` |
| Time scale, slot 1 | `3.00 (Time.timeScale=3.00)` — the last value this rig commanded, read out of the BepInEx log. `census` does not report it and asking for it would mean writing it. |
| Time scale, slot 3 | `3.00 (Time.timeScale=3.00)` — the last value this rig commanded, read out of the BepInEx log. `census` does not report it and asking for it would mean writing it. |
| BepInEx log, slot 1 | `/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/LogOutput.log` |
| BepInEx log, slot 3 | `/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/LogOutput.log.1` |

## Worlds at T0

| | slot 1 (M3-Slot1) | slot 3 (M3-Slot3) |
|---|---|---|
| Population (living) | 20 | 24 |
| Dead bodies in save | 10 | 9 |
| Simulated time (s) | 37394.101 | 34538.501 |
| Unique genomes | 20 | 24 |
| Distinct species | 4 | 7 |
| Generation mean | 10 | 9.667 |
| Generation max | 12 | 12 |
| Synapses mean | 4.8 | 7.625 |
| Synapses max | 9 | 12 |
| Hidden nodes mean | 0.55 | 1.75 |
| Hidden nodes max | 4 | 4 |
| Age mean (s) | 2310.59 | 2261.801 |
| Maturity mean | 1.17 | 1.043 |
| Health mean | 141.771 | 131.547 |
| Energy mean | 96.628 | 75.745 |
| Mean x | -251.457 | -213.587 |
| Mean y | 76.512 | 249.96 |
| Stddev x | 954.366 | 1045.091 |
| Mean distance to E edge | 2251.457 | 2213.587 |
| West entry quarter | 6 | 5 |
| Middle half | 13 | 17 |
| East export quarter | 1 | 2 |
| In capture band (x>=S-W) | 0 | 0 |
| Outside square, east | 0 | 0 |
| x histogram [-S..S] | `0 4 1 1 4 4 4 0 1 0` | `3 0 0 3 3 8 3 0 0 2` |

Full numbers, including every gene's mean/stddev/min/max and the raw per-organism
table, are in `worldstat-slot1.json` and `worldstat-slot3.json`. The one-screen
summaries are in `worldstat-slot1.txt` / `worldstat-slot3.txt`.

### Census and exactly-once

```
slot1 t1785807571256188229 OK population=22 eggs=1 ids=[2029083964,662860515,670025147,-2072690327,308922080,1782803851,375917346,1281992326,218592216,-1653344353,-1197988455,-1344241946,1382584824,1957933593,2105370520,25726165,428592686,-403088161,-522334698,-1125392863,-42064639,2115190312]
slot3 t1785807571542141379 OK population=22 eggs=1 ids=[-744357222,114066618,516215168,-1425950560,-353420180,-122153489,947799556,1543881242,878674314,-1095714484,915780041,382148017,-17652892,839528051,1831253265,1592397772,1203994444,2099914690,-259505509,1320967923,-644256354,-1798510694]

askedSlot=1 entityFromSlot=1 t1785807571836775326 OK entityId=2029083964 present=1 alive=1 population=22
askedSlot=3 entityFromSlot=1 t1785807572115878211 OK entityId=2029083964 present=0 alive=0 population=23
askedSlot=1 entityFromSlot=3 t1785807572173659712 OK entityId=-744357222 present=0 alive=0 population=21
askedSlot=3 entityFromSlot=3 t1785807572447520794 OK entityId=-744357222 present=1 alive=1 population=24
```

### The mod's own crossing counters

Cumulative since each game started (`[M2-CROSSING]`, `CrossingStats.cs`):

```
slot1 [Info   :Bibites Multiverse] [M2-CROSSING] edge=E simMinute=615 stripEntries=1 crossings=0 totalStripEntries=294 totalCrossings=112 cumulativeStripEntriesPerSimMin=0.48 population=20 S=2000.0 W=40.0 simTick=1476001
slot3 [Info   :Bibites Multiverse] [M2-CROSSING] edge=E simMinute=575 stripEntries=0 crossings=0 totalStripEntries=308 totalCrossings=109 cumulativeStripEntriesPerSimMin=0.54 population=23 S=2000.0 W=40.0 simTick=1380001
```

### Population trend — read this before reading the population delta

The population above is ONE SAMPLE of an oscillating system. `CrossingStats` logs the
population once per simulated minute, and over the recent window it moves like this:

```
slot1 summary n=240 min=19 max=40 mean=28.5
slot3 summary n=240 min=17 max=43 mean=30.2
```

A T1 population that differs from T0 by less than that min-to-max range is noise, not
a trend. Compare the T1 `summary` line against this one instead. The full per-minute
series is in `population-trend.txt`.

## The archive — cross-world migration flow

`migrations.jsonl` holds 1994 lines. It is CUMULATIVE across every earlier run, so a
T1 comparison must subtract these totals rather than read the T1 totals as "overnight".

```
recordTypes ACK=695 GENOME=604 MIGRATION=695
migrations 695
acks 695
genomeRecords 604
distinctMigrantEntities 546
distinctMigrantGenomes 528
firstRecordedAt 2026-08-03T20:26:34.415000Z (1785788794415)
lastRecordedAt 2026-08-04T01:39:33.036000Z (1785807573036)
lanes:
  slot 2 -> slot 3  282
  slot 3 -> slot 1  210
  slot 1 -> slot 2  203
bySourcePeer:
  slot-1  203
  slot-2  282
  slot-3  210
```

The 546 distinct migrant entity IDs are listed in full at the end of
`archive-summary.txt`; the full per-migration record, with lineage and genome
custody, is `archive-list.txt`.

## Overnight safety: can a world restart under the rig?

**No.** Two independent links of the only chain that could do it are broken, and
neither was touched to take this baseline.

1. **The mod disables the game's autosave at runtime.** `DevCommands.DisableAutosaveOnce`
   calls `SaveController.ToggleAutoSave(false)` the first frame the simulation is ready,
   for every instance started with `MULTIVERSE_CMD_FILE` — which is both local games. It
   is runtime only and does not write the user's saved preference. With autosave off,
   `SaveController.AutoSave()` never runs and `ReloadAfterAutoSave` is never subscribed.
2. **`ReloadAfterAutosaves` is off in this profile anyway.** It is the setting that turns
   a finished autosave into `GameManager.StartGame(<autosave>)` — a full scene reload.
   The value below is read from the game's own persisted user settings.

```
gameDefaults: UserSettings.AutoSave=true  ReloadAfterAutosaves=false  AutoSavePeriod=TenMinutes  AutoSaveRealTime=true
  (UserSettings.cs:27-75 — the shipped defaults, not this profile)
persistedUserSettings (Unity PlayerPrefs, HKCU\Software\The Bibites\The Bibites):
  ReloadAfterAutoSaves_h727348301 = 0
autosaveFolder: /mnt/c/Users/j_jor/AppData/LocalLow/The Bibites/The Bibites/Savefiles/Autosaves exists, 0 file(s)
slot1 runtimeAutosave=DISABLED by the mod: [Info   :Bibites Multiverse] [M2-CMD] autosave disabled for this session (runtime only) — the rig drives every world save.
slot3 runtimeAutosave=DISABLED by the mod: [Info   :Bibites Multiverse] [M2-CMD] autosave disabled for this session (runtime only) — the rig drives every world save.
bepinexPluginConfig: AutoTest = false 
```

If a scene reload ever DID happen, the custody design absorbs it rather than losing
organisms: `MultiverseClient` mints a **fresh `sessionId` on every world load**
(`contract-a.md` §5.1), the sidecar sees a sessionId it has not seen and runs
`reassertCustodyLocked` (§7.4) — re-sending an unsolicited `MIGRATE_OUT_ACK` for every
outbound custody still in its journal, because a reloaded world may contain an organism
that already left — and then `replayInboundLocked` (§7.5) redelivers every journaled
inbound organism that was never ACKed. The journal in `e2e/data/slot-N/journal/` is what
makes that work, so it must survive any restart.

The far end is out of scope here: slot 2's autosave state is its own machine's, and
nothing on this side reads or writes it.

## SLOT 2 — the second computer

Slot 2 runs on the owner's other machine. Nothing in this capture reached it: the
far end is never driven (`m3_considerations.md` Risk 6). What follows is everything
about slot 2 that is knowable from THIS machine.

### Centrally observable

```
ringMembership: slot1=slot-1 slot2=slot-2 slot3=slot-3
edgeStatusRipple(slot1 -> its east neighbour slot 2): [Info   :Bibites Multiverse] [M2] EDGE_STATUS epoch=3 exportEdge=E open=True reason=peer_live peerS=2000.00 entries=1 applied=1 ignored=0 changed=True (entryEdge W is passive and never appears here)

  slot 2 -> slot 3  282
  slot 1 -> slot 2  203
```

* Lane `slot 1 -> slot 2` is what slot 2 RECEIVED from here.
* Lane `slot 2 -> slot 3` is what slot 2 EXPORTED, on its own, with nobody driving it.
* Ring membership above proves slot 2 still holds its reservation.
* Slot 1's `EDGE_STATUS` line is slot 2's liveness: slot 1's export edge opens only
  while its east neighbour is live and mod-connected.

### PLACEHOLDER — slot 2's own save and census are NOT in this snapshot

There is no `M3-Slot2.zip` and no `census-slot2.txt` here, and there will not be one
unless the owner chooses to run the command below **on the second computer**. It is
optional: everything the ring needs is already above. It only adds slot 2's own
genes, brains and geography, which nothing on this side can see.

Run it in an ordinary (non-elevated) PowerShell on the SECOND computer, while the
game is running. It writes two commands into slot 2's dev-command file — the same
atomic write-then-rename channel this side uses — and changes no setting.

```powershell
# ---- run this on the SECOND COMPUTER, in PowerShell, with the game running ----
# It sends `census` then `save` down slot 2's dev-command file. Both verbs are
# read-only with respect to the simulation: nothing is paused, no setting moves,
# and the time scale is untouched.
$cmd = Join-Path $env:TEMP 'bibites-m3\cmd-2.txt'
function Send-Bibites([string]$verb) {
    $token = 't' + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $tmp   = "$cmd.tmp"
    # Atomic write-then-rename, and the trailing newline is REQUIRED: the mod
    # treats content that does not end in one as a partial write.
    [IO.File]::WriteAllText($tmp, "$token $verb`n", (New-Object Text.UTF8Encoding $false))
    Move-Item -Force $tmp $cmd
    $deadline = (Get-Date).AddSeconds(180)
    while ((Get-Date) -lt $deadline) {
        if (Test-Path "$cmd.log") {
            $hit = Select-String -Path "$cmd.log" -Pattern "^$token " | Select-Object -Last 1
            if ($hit) { return $hit.Line }
        }
        Start-Sleep -Milliseconds 200
    }
    return "TIMEOUT waiting for '$verb' - is the game running with MULTIVERSE_CMD_FILE set?"
}

Send-Bibites 'census'
Send-Bibites 'save M3-Slot2'

$zip = Join-Path $env:USERPROFILE 'AppData\LocalLow\The Bibites\The Bibites\Savefiles\M3-Slot2.zip'
Write-Host ""
Write-Host "M3-Slot2.zip is here:" -ForegroundColor Green
Write-Host "  $zip"
Get-Item $zip | Format-List Name, Length, LastWriteTime
```

Then copy the printed `M3-Slot2.zip` to this machine, drop it in this snapshot
directory, and run:

```sh
bin/worldstat stat --world M3-Slot2 --slot 2 \
  --out e2e/baselines/20260804T013927Z/worldstat-slot2.json \
  e2e/baselines/20260804T013927Z/M3-Slot2.zip
```

## Tomorrow: the T1 capture and the comparison

```sh
e2e/baseline.sh --label T1                 # same capture, new timestamped directory
e2e/baseline.sh compare e2e/baselines/20260804T013927Z <the new directory>
```

`compare` runs `worldstat compare` for each world and prints the archive lane deltas
beside them: population change, new and extinct genomes, the largest gene-mean shifts,
brain-complexity change, whether the population moved east, and per-genome survival.
