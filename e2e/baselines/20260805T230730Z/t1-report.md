# M3 LAN rig — T1 (20260805T230730Z), the overnight run harvested

Assembled 2026-08-05T23:07Z, repo HEAD `cb6e1b4`, against a rig that is no longer
running. It is the counterpart of `e2e/baselines/20260804T013927Z/baseline-report.md`
(T0, 2026-08-04T01:39Z).

**Nothing was started to produce this.** No game was launched, no Go process was
stopped, no dev command was sent to any world. `e2e/baseline.sh` was left unmodified:
its `capture` path refuses to run without a live rig (`require_rig`), which is correct,
so this directory was assembled by hand into the same file layout. `baseline.sh compare`
then ran against it unchanged, and its output is in `compare.txt`.

---

## The verdict you need first: did we get the worlds, or only the migrant stream?

**Only the migrant stream. The overnight end state of the two local worlds is gone.**

The two `.zip` files here are real saves of real worlds, but they are dated
**2026-08-04T01:39:58Z and 01:39:59Z** — twenty-five seconds after T0 — and both games
then ran for another **12 hours 34 minutes without ever being saved again**.

| | slot 1 (M3-Slot1) | slot 3 (M3-Slot3) |
|---|---|---|
| Simulated minute in the zip here | 632.7 | 584.7 |
| Simulated minute the world actually reached | **20066** | **17621** |
| Share of the run the save holds | **3.2 %** | **3.3 %** |
| Last save receipt | `t1785807598756 … bytes=194026 population=26` | `t1785807599306 … bytes=207567 population=27` |

The evidence chain is in `save-vintage.txt`: the mod's own `[M2-CMD]` save receipts (the
last two in either log), the byte counts matching the files on disk, and a cross-check
that the population inside slot 3's zip — 26 living — is exactly what `[M2-CROSSING]`
logged at simulated minutes 584, 585 and 586. There is no autosave to fall back on:
`Autosaves/` is still empty, because `DevCommands.DisableAutosaveOnce` turned the game's
autosave off for the session. That is the same mechanism that made the overnight run
safe from a scene reload, and it is why closing the windows discarded the final state.

So the world-save half of the comparison in `compare.txt` measures **nine simulated
minutes**, not a night. Read it as a sanity check that the tooling works, and read
nothing else into it. Everything below that says "overnight" comes from three sources
that DID survive the whole night:

* the **archive ledger** — 22 595 migrations recorded between T0 and the last record,
  each with a timestamp and a `genomeHash`;
* the **genome store** — every one of those hashes resolves to a full organism, so the
  genes and brains that crossed are readable exactly;
* the **BepInEx logs** — `[M2-CROSSING]` once per simulated minute for the whole run,
  harvested before anything could overwrite them.

---

## The night, in one paragraph

Both local games ran a single unbroken session — simulated minute 1 through 20066
(slot 1) and 17621 (slot 3), no scene reload, no restart. From about 02:00Z the worlds
ran at roughly 17× to 33× real time, faster than the 3.00 this rig last commanded;
something outside the rig raised the speed (`timescale.txt`). Slot 2, on the second
computer, dropped out of the ring for about three hours in the morning and came back.
At 2026-08-04T14:13:14Z everything stopped at once: the games, the sidecars and the
archive, within three seconds of each other, with the Go logs ending mid-record and
NUL-padded — the signature of the host going away rather than a graceful shutdown
(`shutdown.txt`). The rig has been down for the 33 hours since.

### Timeline

| UTC | Event |
|---|---|
| 2026-08-04T01:39:33 | T0 baseline captured |
| 01:39:58 / 01:39:59 | last save of each local world — **the zips here** |
| ~02:00 | simulation speed rises to 17–33× |
| 08:46:39 | slot 2 exports its last organism before the gap |
| 09:49:44 | relay sees slot 2's mod disconnect |
| 09:49:45 | slot 1's export edge closes (`peer_mod_absent`), simulated minute 13220 |
| 10:08:59 | slot 2's sidecar disconnects too |
| 11:46:43 | slot 2 back: `modConnected=true simulationSize=2000 exportEdge=E` |
| 11:46:45 | slot 1's export edge reopens for good, simulated minute 16724 |
| 14:13:14 | everything stops |
| 2026-08-05T23:07 | this capture |

---

## Evolution

### What the world saves can and cannot say

`compare.txt` compares the T0 zips against the zips here. That is a 569-second
(slot 1) and 546-second (slot 3) window, so **every number in its per-world sections
is within-noise and none of it is an overnight result**. For the record it reports
population 20 → 26 and 24 → 26, 11 and 12 new genomes, 5 and 10 extinct, and gene shifts
of at most 0.67 T0 standard deviations. Those are nine minutes of ordinary churn.

### What the migrant stream says — this is the real answer

`evolution.txt` reads the genomes that actually crossed. Two things about `genomeHash`
govern how far it can be pushed, both from `contracts/genome-hash.md` §4: the projection
it hashes is **exactly** the 35-gene array plus the brain's nodes and synapses plus the
game version. So gene values and brain measurements below are exact. Generation and
speciesID are *not* in the projection, so they are reported as the values of the first
organism ever seen carrying that genome — a date stamp for when the genome arose, never
averaged as if it were a per-hop measurement.

One structural fact has to come first, because it changes every mean: **the ancestral
wild type never went away.** Five genome hashes — generation 0, speciesID 1, never
mutated, 3 synapses, 0 hidden nodes — account for **10 307 of the 22 595 overnight hops
(45.6 %)**, in every world at once (slot 1 47.0 %, slot 2 47.7 %, slot 3 40.6 %). An
unmutated descendant carries its parent's hash, so this is a clonal line that keeps
reproducing true and keeps crossing, not a handful of seed organisms. Everything below
excludes it and measures the **evolved fraction**.

#### Brain complexity — all three worlds, first 1000 evolved hops vs last 1000

| | slot 1 | slot 2 | slot 3 |
|---|---|---|---|
| Synapses | 8.17 → **12.69** (+4.52) | 7.65 → **13.50** (+5.86) | 8.19 → **14.29** (+6.10) |
| Enabled synapses | 7.13 → 11.14 | 6.74 → 12.01 | 7.17 → 12.41 |
| Hidden nodes | 0.75 → 1.16 | 0.84 → 1.00 | 0.80 → 1.45 |
| First-seen generation | 41.6 → 97.5 | 26.7 → 103.5 | 43.0 → 92.2 |
| First-seen speciesID | 134.7 → 525.4 | 93.0 → 194.8 | 154.9 → 504.4 |

Brains grew by **connections, not neurons**: synapse counts rose 55–77 % while hidden
nodes moved by less than one node. Per hour the evolved fraction's synapse mean went
5.2 → 15.5 (slot 1), and peaked above 14 in all three worlds.

#### Gene drift, in units of the T0 population's own spread

Largest shifts, first 1000 vs last 1000 evolved hops. `z` is the shift divided by that
gene's standard deviation in the T0 world save; slot 2 has no T0 save here, so it is
shown raw.

| Gene | slot 1 | slot 2 | slot 3 |
|---|---|---|---|
| **Diet** | 0.244 → 0.157 (**−21.0 sd**) | 0.252 → 0.147 | 0.247 → 0.161 (**−8.7 sd**) |
| **AverageMutationNumber** | 4.04 → 1.69 (−4.5 sd) | 3.85 → 0.91 | 4.20 → 0.98 (**−7.6 sd**) |
| MutationAmountSigma | 0.0153 → 0.0283 (+4.9 sd) | 0.0187 → 0.0296 | 0.0148 → 0.0305 (+2.9 sd) |
| PheroSense | 151.2 → 110.6 (−4.4 sd) | 149.1 → 97.2 | 151.9 → 107.1 (−6.0 sd) |
| EyeOffset | 0.413 → 0.543 (+4.6 sd) | 0.429 → 0.570 | 0.414 → 0.557 (+6.2 sd) |
| ThroatWAG | 1.58 → 2.31 (+4.2 sd) | 1.41 → 2.50 | 1.59 → 2.54 (+3.3 sd) |
| ViewAngle | 188.0 → 165.7 (−3.2 sd) | 192.0 → 159.9 | 185.1 → 163.8 (−2.0 sd) |
| ClockSpeed | 1.11 → 1.57 (+2.6 sd) | 1.08 → 1.68 | 1.11 → 1.58 (+3.0 sd) |
| SizeRatio | 1.00 → 1.28 (+2.6 sd) | 0.94 → 1.28 | 1.03 → 1.26 (+2.5 sd) |
| BroodTime | 27.1 → 21.6 (−2.2 sd) | 25.6 → 21.0 | 27.1 → 21.6 (−5.2 sd) |

The full 35-gene table for each world is in `evolution.txt` §3.

#### Population

The T0 report warned that a single population sample is noise and gave the band to
compare against. Here are both, over the same 240-simulated-minute window:

| | slot 1 | slot 3 |
|---|---|---|
| T0, last 240 sim minutes | min 19, max 40, mean 28.5 | min 17, max 43, mean 30.2 |
| T1, last 240 sim minutes | min 16, max 33, **mean 21.9** | min 17, max 33, **mean 24.5** |
| Whole night (20066 / 17621 samples) | min 10, max 51, mean 22.4 | min 10, max 61, mean 23.7 |
| Final sample | 20 | 27 |

The final instantaneous populations (20 and 27) sit inside T0's stated band, so on the
T0 report's own test they are **noise, not a trend**. The *mean* is a different matter:
it fell 6.6 and 5.7 organisms, and the whole-night minimum reached 10 in both worlds —
below anything T0 saw. The per-hour means in `population-hourly.txt` show the shape: a
decline from ~29 to ~18 across the first eight hours in both worlds, then a partial
recovery to 22–27. Both worlds carried a smaller, more volatile population than at T0.

---

## Geography

**The spatial verdict T0 set up cannot be answered.** Mean x, the x histogram and
east-quarter occupancy come from a world save, and the only saves that exist are the
stale ones. The archive is not a substitute: every genome it stored was captured as its
organism reached the export edge, so a 1499-genome sample puts the median x at 1960 —
they describe the edge, not the world (`geography.txt`).

What survives is the mod's own edge instrumentation, once per simulated minute for the
whole night, and it answers a related question well: **how hard was the population
pressing on its eastern boundary?**

| | slot 1 | slot 3 |
|---|---|---|
| Strip entries, whole night | 10 195 (T0: 294) | 7 134 (T0: 308) |
| Square crossings, whole night | 2 791 (T0: 112) | 992 (T0: 109) |
| Strip entries per sim-minute per 10 organisms, first hour | 0.118 | 0.133 |
| … last hour | **0.269** | **0.297** |

Edge pressure per organism roughly **doubled** over the night in both worlds. The
population did not simply get pushed east by numbers — it shrank while pressing harder,
which is a behavioural shift, and it sits beside a `ViewAngle` that narrowed by 21–32°
and a `ClockSpeed` that rose 42–56 % in every world.

Two events make the mechanism visible:

* **Slot 1, hours 09Z–11Z.** Its export edge was shut because slot 2 was gone. Square
  crossings jumped to 328, **803** and 611 per hour against a typical 66–181. With
  nowhere to be exported to, organisms that reached the strip walked out of the playable
  square instead. That is the containment path (D10) taking the load, visible as a 9×
  spike and nothing else breaking.
* **Slot 3, the same hours.** Its strip entries *collapsed* — 205, 144, 214 per hour
  against 580–950 in the hours on either side — although its own export edge stayed
  open the whole time.
  Slot 3's edge pressure was mostly through-traffic arriving from slot 2 at its western
  edge and walking across. Cut the inflow and the eastern edge goes quiet.

---

## The overnight migration story

`archive-overnight.txt`. The ledger is cumulative, so everything here is the slice after
T0's last record.

```
overnight migrations   22 595   over 12.56 hours   = 1 799 hops/hour
distinct migrant entities  19 951   (19 949 never seen before T0)
distinct migrant genomes    9 859   (9 857 never seen before T0)
repeat migrants (crossed more than once)  1 748
```

| Lane | T0 total | T1 total | Overnight |
|---|---|---|---|
| slot 2 → slot 3 | 282 | 9 926 | **+9 644** |
| slot 1 → slot 2 | 203 | 7 151 | **+6 948** |
| slot 3 → slot 1 | 210 | 6 213 | **+6 003** |

Net flow — the ring is not balanced:

| World | Exported | Imported | Net |
|---|---|---|---|
| slot 1 | 6 948 | 6 003 | **−945** |
| slot 2 | 9 644 | 6 948 | **−2 696** |
| slot 3 | 6 003 | 9 644 | **+3 641** |

**Slot 2 is the ring's net exporter** and slot 3 its net sink, by a wide margin, and it
stayed that way in the T0 sample too (282 out, 203 in). Slot 2 sent out 39 % more
organisms than it took in, over three hours *less* running time than the local worlds.

Busiest hour: **13:00Z with 2 769 hops**. Traffic climbed from 234 in the first partial
hour to a 2 400–2 700/hour plateau by 04:00Z, collapsed to 662 and 144 during the slot-2
blackout, and recovered to the plateau within an hour of slot 2 returning. The full
per-hour, per-lane table is in `archive-overnight.txt`.

**Exactly-once held.** 23 290 MIGRATION records against 23 289 ACK records: exactly one
hop unacknowledged, and it is the last record in the ledger — migration
`9d6db335-b1ae-433e-a44a-bb2109912913`, entity 2004967003, slot 1 → slot 2, recorded
2026-08-04T14:13:14.202Z, the instant everything stopped. Slot 1's journal still holds
its entry as `in_flight`, which is exactly what `reassertCustodyLocked`
(`contract-a.md` §7.4) exists to replay. Slot 3 had nothing in flight at all
(`journal-custody.txt`).

---

## Genome diversity over time

`evolution.txt` §2 and the per-hour table there. Cumulative distinct genomes exported,
per world, over the night:

| World | Distinct genomes at T0 | At the end | New overnight |
|---|---|---|---|
| slot 1 | 184 | 3 751 | 3 567 |
| slot 2 | 268 | 5 168 | 4 900 |
| slot 3 | 191 | 3 647 | 3 456 |

Novelty never saturated: in the last full hour the three worlds still produced 369, 561
and 152 genomes that had never been recorded anywhere. Slot 3 is consistently the least
novel — 152 new against 340 distinct in that hour — because so much of what it exports
arrived from slot 2 rather than being made in slot 3.

---

## The most interesting thing in the data

**The three worlds evolved as one population, and one of them was on a different
computer.**

The ring couples the worlds by migration alone. Nothing synchronises their genes.
Yet across the night the same 19 of 35 genes moved in the same direction in all three
worlds by more than three times the spread *between* the worlds' final values
(`evolution.txt` §4). The end points are nearly identical: Diet 0.157 / 0.147 / 0.161,
AverageMutationNumber 1.69 / 0.91 / 0.98, ClockSpeed 1.57 / 1.68 / 1.58, PheroSense
110.6 / 97.2 / 107.1, synapses 12.7 / 13.5 / 14.3. For `FatStorageThreshold` the shared
shift is **17×** the between-world spread; for `SizeRatio`, 11×.

Two things make this worth keeping. First, **it is a result about the multiverse, not
about a world** — it is the thing three isolated Bibites instances could not produce,
and the first evidence that the ring is doing what it was built to do. Second, the
strongest single trend inside it is one no single world would have shown you:
**`AverageMutationNumber` fell from ~4.0 to 0.9–1.7 while `MutationAmountSigma` rose
58–106 %** — selection for *fewer but larger* mutations, in three worlds at once, while
brains simultaneously grew 55–77 % more synapses. And it happened against a background
in which the unmutated ancestral clone still carried 46 % of all traffic: the evolved
fraction did all of this while never displacing the wild type.

The slot-2 blackout is the control that makes the coupling legible rather than assumed.
For three hours slot 3 was cut off from its upstream, and immediately its export stream
lost the wild type entirely (0.0 %, 0.0 %, 0.5 % in hours 09Z–11Z, against 43 % in the hour
before it and 29–47 % in the hours after), its edge pressure fell by a factor of four to six, and its first-seen
generation dropped from 137 to 9.7 as a single local lineage took the stream over. The
mixing is not a background detail; it is most of what each world exports.

---

## Slot 2 — the second computer

What is here: everything slot 2 did that passed through this machine — 9 644 exported
organisms with their full genomes, its liveness history, its ring membership, and its
gene and brain trajectory across the night, which is in `evolution.txt` alongside the
local worlds. That is more than T0 had.

What is **not** here, and cannot be: slot 2's own world save, its population, its
spatial distribution, and its species table. `M3-Slot2.zip` does not exist in this
directory. The T0 directory does not contain one either — the optional PowerShell
command in the T0 report was never run, so there is no slot-2 save at either end.

> There is a `M3-Slot2.zip` in this machine's `Savefiles/` folder dated
> 2026-08-03T20:25Z. **It is not the far end.** It is the slot-2 world of the earlier
> all-local three-world run, from before the rig moved to the LAN, and it predates the
> LAN session by four hours. Do not read it as slot 2's state.

**Slot 2's save can no longer be captured.** The T0 report's PowerShell block needs the
game running, and the owner has closed it. If that game was never saved on the far end
either — which is what happened on this side — then slot 2's overnight world state is
gone for the same reason. The placeholder stands for a future run, not for this one:

```powershell
# ---- run this on the SECOND COMPUTER, in PowerShell, WITH THE GAME RUNNING ----
# It sends `census` then `save` down slot 2's dev-command file. Both verbs are
# read-only with respect to the simulation.
$cmd = Join-Path $env:TEMP 'bibites-m3\cmd-2.txt'
function Send-Bibites([string]$verb) {
    $token = 't' + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $tmp   = "$cmd.tmp"
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
Get-Item $zip | Format-List Name, Length, LastWriteTime
```

Then copy the zip here and run:

```sh
bin/worldstat stat --world M3-Slot2 --slot 2 \
  --out e2e/baselines/20260805T230730Z/worldstat-slot2.json \
  e2e/baselines/20260805T230730Z/M3-Slot2.zip
```

What the second computer may still hold, whether or not the game is running:

* **Slot 2's BepInEx `LogOutput.log`** — its `[M2-CROSSING]` series, its population once
  per simulated minute for the whole night, and its `EDGE_STATUS` history. This is the
  one substantial piece of overnight data still recoverable, and it is **perishable**:
  BepInEx overwrites it the next time that game launches. It should be copied off before
  anything else happens on that machine.
* **Slot 2's sidecar log and journal** — its own view of custody and of the three-hour
  gap, including whether anything was in flight when its window closed.
* **`$env:TEMP\bibites-m3\cmd-2.txt.log`** — the dev-command receipts, which would say
  whether that world was ever saved.

---

## The one lesson for the next overnight run

The run was safe — no restart, no lost organism, exactly-once held across 22 595 hops —
and the world state was still lost, because "safe" and "saved" were the same switch. The
rig turns the game's autosave off so nothing can reload a scene under it, and then the
rig never sent a save. A periodic `save` from the rig (the verb is already on
`baseline.sh`'s safe list) would have cost nothing and would have made this a full
comparison instead of a partial one.

---

## What is in this directory

| File | What it is |
|---|---|
| `save-vintage.txt` | The evidence for the verdict at the top. Read this first. |
| `shutdown.txt` | How and when the rig stopped; slot 2's liveness timeline. |
| `compare.txt` | `e2e/baseline.sh compare` T0 → T1, unmodified. World sections span 9 simulated minutes; the archive section spans the night. |
| `evolution.txt` | Wild type, genome diversity per hour, gene and brain drift per world, and the three-world convergence test. |
| `geography.txt` | Edge pressure per hour per world, and why the spatial table cannot be produced. |
| `archive-overnight.txt` | The overnight slice: lanes, net flow, per-hour traffic. |
| `archive-summary.txt` | Cumulative ledger summary in T0's format (this is what `compare` parses). |
| `archive-list.txt.gz` | Full `archive list` output, 23 290 migrations, gzipped (5.9 MB raw). `archive-list-tail.txt` holds its summary line. |
| `crossing-series-slot{1,3}.csv` | Every `[M2-CROSSING]` sample of the whole run: 20 066 and 17 621 rows. |
| `population-by-wallclock-slot{1,3}.csv` | The same series with each simulated minute anchored to wall clock. |
| `population-hourly.txt`, `crossing-stats.txt` | Population summaries per hour and per quarter of the night. |
| `journal-custody.txt` | What was in custody when the rig died. |
| `worldstat-slot{1,3}.json/.txt`, `M3-Slot{1,3}.zip` | The stale saves and their statistics, kept so the comparison runs and so the vintage is auditable. |
| `census-slot{1,3}.txt`, `count.txt` | ABSENT markers — these need a live game. |
| `meta.txt`, `ring.json`, `relay-ring.log`, `slot2-observable.txt`, `timescale.txt`, `autosave.txt`, `crossing.txt` | The T0 report's own fields, brought forward. |

The two harvested BepInEx logs (98 MB and 49 MB) are too large for the repository. They
are preserved outside it, next to the rig's own archived logs, at
`e2e/logs-m3-lan/bepinex/LogOutput.log.t1harvest-slot1.20260805-230730` and
`LogOutput.log.1.t1harvest-slot3.20260805-230730`. Every series in this directory was
extracted from them. **They must not be lost: BepInEx overwrites the live
`LogOutput.log` the next time a game launches, and these copies are the only record of
the overnight run's simulated minutes.**
