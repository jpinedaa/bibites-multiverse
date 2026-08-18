# Observability standard

How this service is measured, where the measurements live, and what an operator
does with them. It covers the two hosts, the four processes, the network path
between them, and the money.

This document is the standard. It is not a description of what is deployed
today — [Rollout](#rollout) states plainly which parts exist and which do not.

## Why this exists

On 2026-08-15 every cloud world dropped off the map at once, came back four to
six minutes later, and did it again six times in ninety minutes. Diagnosing it
took a day of reasoning from outside the service.

Almost none of that day was spent gathering data. The data existed:

- `metrics.jsonl` had recorded every slot's `live` flag once a minute since the
  service began.
- `performance.jsonl` had recorded each world's load, memory and relay fault
  every five minutes.
- CloudWatch had recorded the world host's CPU for free, at five-minute
  granularity, from the moment the instance launched.
- The relay's log had recorded each disconnect as it happened.

Nothing read any of it. The account had **zero CloudWatch alarms**. The
monitor kept no history. No deployment record contained a number. The eventual
answer came from four read-only API calls against metrics that had been
collecting, unwatched, the whole time.

So the problem this standard solves is not a shortage of instruments. It is
that measurement without a **consumer** is not observability — it is only
storage. Every rule below exists to attach a reader to a number.

## Principles

**Unknown is a value.** A reading that is missing, or older than its freshness
rule, is published as unknown and never as zero. This is already §10.1 of the
status contract and it binds every tool here. A comparison that silently reads
a gap as a zero manufactures a regression that did not happen, or hides one
that did.

**The observer must not die with the observed.** Nothing that stores evidence
runs only on the host it is watching. The service host is the one most likely
to be sick, and querying a time-series database is memory-hungry precisely when
memory is short. Collectors may live on a watched host; stores and query
engines may not.

**A number with a floor publishes its floor.** The map is a rectangle and the
peer count need not fill it, so a 3×3 grid holding seven peers has two
permanently empty cells and the lanes around them are bypassed forever.
Reporting the bypass count alone makes a static map shape read as a worsening
incident. Any metric with a structural component publishes that component
beside it, and never subtracts one from the other unless the arithmetic is
exact.

**A probe measures the path from where the probe is.** A synthetic prober's own
error rate is not a service metric. The front door rate-limits on source
address, so several tools sharing one egress address pool into one bucket and
generate their own failures. A probe's reachability series is labelled by
vantage point and is never the basis of a service alarm.

**Resolution is a property of the instrument, not the event.** Nothing sampled
can resolve an event shorter than twice its interval. A five-minute average
cannot see a thirty-second stall, and reading one as though it could is how a
day gets spent on the wrong hypothesis. Every series here states its interval,
and every claim states the interval it rests on.

**Do not collapse a three-state signal into two.** A world publishes `live` and
`modConnected` and they are independent: `live` is the sidecar's membership of
the relay registry, `modConnected` is whether the game is attached to that
sidecar. The three real states are *live*, *connected but no game attached*, and
*dark*. The public map draws all three. A reader that asks only "is it live"
reports a world whose game has died as perfectly healthy — which is exactly what
happened during a 151-restart crash loop that a `live`-only prober recorded as
uneventful, and what let a deployment record state "seven live peers, none dark"
eight seconds before the failing world last restarted. Read every state a signal
can take, or the one you dropped is the one that fails.

**Counters survive sampling; gauges do not.** A gauge read once a minute misses
a thirty-second flap about half the time. A monotonic counter read once a
minute captures every event in the gap, because two readings bracket
everything between them. Prefer a counter wherever the question is "how often".

**Instrumentation must not block the work.** Nothing on the migration path
waits for a reader. New measurement is a counter increment under a lock already
held, or an atomic, or it does not ship.

**Every field in `metrics.jsonl` is permanent.** The archive serialises its
whole status object to that file every sixty seconds and never rewrites it. A
scalar added there costs a few hundred bytes a minute forever; an array added
there is a design error. Per-event and per-connection detail belongs in logs or
in `/my-slot`, never in the status object.

## The four layers

### Layer 1 — Host

What the machine is doing. Both hosts, same shape.

| Host | Collector | Interval | Lands in |
|---|---|---|---|
| World host | `bibites-performance-sample` | 5 min | `/srv/bibites/metrics/performance.jsonl` |
| Service host | `deploy/service-host-sample` | 1 min | `/var/lib/multiverse/metrics/service-host.jsonl` |

The service-host sampler exists because of a gap the incident exposed: the
world host had sampled itself since the day it was built, and the box running
nginx, the relay, the archive and the stream origin had never sampled itself at
all. When every peer dropped together, the one end common to every link was the
one end with no record of its own state.

Both samplers are unprivileged, read `/proc` and `systemctl show` only, and
report anything they cannot read as unknown. Neither walks a directory: on a
two-vCPU box a `du` over the genome store is the most expensive thing a monitor
could do, and it is forbidden.

On the service host the fields that matter most are the **cumulative TCP
counters** — `estabResets`, `retransSegs`, `outResets`, `listenOverflows`,
`synRetrans`. They are monotonic, so a reader differencing two samples sees
every reset in the gap whether or not anything was sampled while it happened.
This is the counters-survive-sampling principle in its most load-bearing form:
it is how a one-minute sampler says something true about a thirty-second event.

Per-unit `MemoryCurrent` is sampled against the relay's `MemoryMax=512M`
tripwire, so "did the cap get near" stops being unanswerable. `NRestarts` is
sampled so a restart that healed itself between two monitor ticks stops being
invisible.

**`MemoryCurrent` on its own is the wrong number for a growing heap, and this
is the sharpest example the project has.** It is anonymous memory plus page
cache plus kernel memory for the whole cgroup. The archive appends to a
multi-gigabyte ledger and to a rotating log, so its page-cache term moves by a
hundred megabytes between two samples and the kernel reclaims it at will. In
the first forty-five minutes of the service-host sampler's life the archive's
`MemoryCurrent` fell by 166 MB while the ledger it holds in memory grew. A
trend drawn from that series has the wrong sign. So the sampler records the
decomposition beside it:

| Field | Source | Why it is there |
|---|---|---|
| `memoryBytes` | `MemoryCurrent` | unchanged, and what the relay's cap is compared against |
| `anonBytes` | cgroup `memory.stat` `anon` | the unreclaimable half. This is the term that grows with the ledger and the term the out-of-memory killer eventually acts on |
| `fileBytes` | cgroup `memory.stat` `file` | the reclaimable half, recorded so the swing in `memoryBytes` is explained by the record rather than mistaken for one |
| `mainRssAnonBytes` | `/proc/<pid>/status` `RssAnon` | the main process's anonymous resident set. On a single-process unit it agrees with `anonBytes`, and recording both proves that rather than assuming it |
| `mainVmHwmBytes` | `/proc/<pid>/status` `VmHWM` | **nothing else on the box remembers the replay peak.** It occurs in the first minutes after a start, which no one-minute sampler is guaranteed to observe. Uncapped it equals the settled resident set to within 0.06%, so the value of recording it is the proof of that equality — and it separates from the settled figure, by up to 14%, exactly when a binding `GOMEMLIMIT` is doing its job |
| `mainPid` | `MainPID` | a restart *between* two samples. `NRestarts` counts only what systemd restarted by itself; every archive restart on this host so far has been an operator's `systemctl restart`, which leaves it at zero |

The `/proc` reads are taken for named single-process units only — the archive
by default. A per-process resident set describes a unit truthfully only when
the unit is one process, and nginx is a master with workers.

### Utilisation is not saturation

The most expensive lesson this service has produced. A host pinned at 96–99% CPU
ran for 7.1 hours without dropping a single peer; the same host dropped peers 47
times out of 48 while memory sat between 85 and 99 percent. Utilisation says how
busy a resource is. It does not say whether anything is *waiting*, and waiting is
what breaks a deadline.

Pressure stall information says it directly, and both hosts sample it:

```text
/proc/pressure/cpu     full total    time no task could run for want of CPU
/proc/pressure/memory  full total    time no task could run for want of memory
/proc/pressure/io      full total    time no task could run for want of IO
```

On the world host these read **zero** and **several hundred seconds**
respectively. That single pair reversed a diagnosis that two days of CPU graphs
had pointed the wrong way. Per-cgroup `memory.pressure` narrows it to a process.

**Read PSI before utilisation, and never conclude from a utilisation graph that
a resource is the constraint.** A 5-minute CPU average in particular cannot
resolve a 30-second stall, and reading one as though it could is how the wrong
answer survives for a day.

### Memory on a host that runs worlds

World processes leak at a wall-clock rate independent of what they simulate.
[`SIZING.md`](../deploy/SIZING.md) carries the formula and the exhaustion model.
The observability consequence is that **per-process memory must be sampled, not
just host totals** — a host-level "29 of 30 GiB used" says nothing about which
world to act on, and the sampler that recorded only host memory is why the
failure read as a CPU problem for a day.

Sample per-unit `MemoryCurrent` **and the cgroup's `anon` term beside it**,
per-cgroup `memory.pressure`, and `/proc/vmstat` `pswpin`, `pswpout` and
`oom_kill`. The last is the one that turns "something restarted" into "the
kernel killed it". [Layer 1](#layer-1--host) says why `MemoryCurrent` alone
misreads a growing heap.

### Layer 2 — Service

What the processes are doing.

`/api/status` is the live view and is already complete for its purpose. Its
freshness rule is fixed at 30 seconds: a stats block older than that makes
every stat on that slot unknown. `metrics.jsonl` is the same object once a
minute, durably.

**The record roll-up added a layer under it, and eleven fields that describe
it.** The archive's aggregates are now durable and its raw ledger is a run of
daily segments, so the page has to answer two questions that used to be one:
what can this archive still say, and what raw does it still hold to say it
from. Every field below is on `/api/status`; the first two are the ones a
person reads and the rest are what `monitor.sh` gates on.

| Field | What it says |
|---|---|
| `ledgerFirstRecordMs` | Where the AGGREGATES reach back to. The first record this archive ever folded, kept for ever. |
| `ledgerRawWindowFromMs` | Where the RAW LINES reach back to. **The honesty field**: it is what stops "no crossings before this" being read as "none recorded" when it means "none kept". Present even when no window is set, because it is a fact about the disk and not about the policy. |
| `rollupCoveredRecords`, `rollupSavedAtMs` | How much of the record the durable fold covers, and when it last saved. |
| `replayRawRecords`, `replayRawSeconds` | What the LAST start actually cost, measured by the archive itself. A claim about restart time that nobody measures is a claim nobody can check. |
| `replayFromRetired` | The one bad case, and it is loud: the saved cursor named a raw segment that has left this host, so the fold has a hole it cannot close by itself. Absent on every healthy archive. |
| `duplicatesRefused` | Records the duplicate guard refused, all-time. **Not omitted when it is zero**, because zero is the answer that matters: it is the evidence that the duplicate window may safely come down, and a refusal leaves no other trace anywhere. |
| `ledgerSegments`, `ledgerRawBytes` | Closed segments present on this host and what they occupy as stored. |
| `ledgerWindowMs` | The raw window in force. |
| `ledgerSegmentsAwaitingColdCopy` | **The number that should be zero.** Segments past the window with no confirmed off-host copy. |
| `ledgerRetiredTotal`, `ledgerRetiredBytes` | What this process removed after confirming a copy, and the bytes it freed. |

**These fields are refreshed by the archive's maintenance pass and not by the
status request**, so a page load never walks a directory. That has one
consequence for anything that reads them: the pass at start compresses before
it refreshes, which is 21 seconds on a real legacy segment, so a document
captured in the first minute after a restart shows zero segments on a perfectly
healthy archive. `monitor.sh` treats a zero `rollupSavedAtMs` beside zero
segments as "not reported yet" rather than as "nothing there", and anything
else reading these fields has to do the same.

**The sampler is unaffected.** `service-host-sample` reads `/proc` and systemd
unit properties and takes nothing from `/api/status`, so none of the above
changes it, its interval, or its file. Layer 1 is where the archive's memory and
its replay peak are recorded, and it records them the same way it did before.

Two things this layer does not yet answer, both of which are the difference
between a disconnect that explains itself and one that does not:

**Why a connection ended.** The relay's read loop discards the error on every
path except a liveness timeout, and its `client gone` line records the peer,
role, slot and whether the reservation was kept — but not the close code, not
the reason, not the session duration, not the bytes. A peer that closed
cleanly, a peer whose process was killed, and a path that vanished all produce
byte-identical log output. Recording the close code is the single highest-value
change available to this service and it is roughly forty lines.

**How often a peer has reconnected.** The relay's dark-since timestamp is
in-memory and lost on restart. The sidecar keeps exactly one fault, overwritten
each time. A slot that flapped ten times in twenty minutes remembers one of
them. A monotonic session counter per slot fixes this and survives the
sixty-second sampler, where the boolean does not.

### Layer 3 — Path

Whether the two hosts can reach each other, measured from the side that loses.

The six cloud peers already measure reachability, badly: they either work or
they are gone. A path probe measures the two things they cannot — loss and
latency on a healthy link, and whether a *new* connection can be established
while existing ones are fine. The TLS handshake time is the load-bearing
number, because it is what a saturated box inflates and what a dialling peer
eventually times out on.

The probe runs on the world host, targets `/healthz` as the cheapest public
handler, and writes to the retained volume — so its record survives the service
host being unreachable, which is the case it exists to document.

An outside-in probe from a third vantage point answers a different question
again: whether the public front door works for the public. Its error series is
labelled by vantage and never feeds a service alarm.

### Layer 4 — Cost

Spend is a health metric here, because two of the three cost risks are also
capacity risks.

| Risk | Signal | Cadence |
|---|---|---|
| Spot price drifting toward on-demand | `describe-spot-price-history` for the instance type, **in the AZ actually in use** | hourly, free |
| Egress eating the transfer allowance | `monitor.sh` `transfer`: `/proc/net/dev` on the billed interface, against a month-to-date burn-down line and a 24-hour trailing rate | 5 min, free |
| A transfer driver that has just appeared | `monitor.sh` `transfer-rate`: consecutive closed hours above an hourly limit | 5 min, free |
| The loopback pin that keeps the archive's subscription off the billed interface | `monitor.sh` `hosts-pin` | 5 min, free |
| A publisher spending a quarter of the allowance on an empty room | `deploy/viewers-presence.sh`: MediaMTX `hls_sessions` and non-loopback `/watch` and `/stream/` requests in the front-door access log, published at `/api/viewers` | 10 s, free |
| Billing truth behind the transfer rows above | `deploy/ce-reconcile.sh`: one Cost Explorer `get-cost-and-usage` call — DAILY granularity, `UsageQuantity` and `UnblendedCost`, grouped by usage type, filtered to the one service — plus two free instance-metric calls | daily, `$0.01` a day and `$0.31` a month |
| Whether that reconciliation is still running | `monitor.sh` `billing`: the age of the file the reconciliation ships, and the overage quantity in it | 5 min, free |
| The hourly off-host segment copy drawing on the same allowance | `monitor.sh` `transfer` counts it like any other byte: the upload leaves the instance, and the allowance is counted in both directions. `deploy/SIZING.md` "The raw ledger window" has the volume | 5 min, free |
| An object store that is being paid for and is not receiving anything | `monitor.sh` `cold-copy`: `ledgerSegmentsAwaitingColdCopy`. Nothing else on this host looks at the store | 5 min, free |
| Anything unexpected | A cost budget and an anomaly monitor scaled to this account | continuous |

These facts shape this layer and each has cost someone an hour:

- **The viewer-presence row is a control signal, not an alarm.** Nothing pages a
  person when it says nobody is watching; the publisher reads it and stops. It
  belongs in this table because it is a measurement with a cadence and a reader,
  and because a measurement that quietly stops updating decides something — the
  publisher holds its last answer. Its liveness is `asOf`, and the
  verification phase checks that the endpoint answers at all.
- **The service host does not publish to CloudWatch.** There is no
  `AWS/Lightsail` namespace, so no CloudWatch alarm can watch its CPU, its
  burst capacity or its egress. Those come from the Lightsail API instead, and
  burst capacity *rising* means the box is under baseline and was never
  throttled.
- **The host's own NIC counter is the free source, and the only credential-free
  one.** `/proc/net/dev` on the billed interface tracks the provider's
  `NetworkIn` plus `NetworkOut` to 0.002% outbound and 0.9% inbound — the
  inbound residual is the twelve minutes of provisioning that preceded the first
  boot. This host holds no cloud credential on purpose, and an egress monitor is
  not a good enough reason to put one on a public-facing box. So the alerting
  path reads the kernel and the daily Cost Explorer call reconciles it.
- **The allowance is counted in BOTH directions, and its GB is 2^30 bytes.**
  Reconciling the NIC counter against Cost Explorer gives a ratio of 1.01 on
  2^30 and 1.08 on 10^9, so the decimal base is the wrong one: a 3,072 GB
  allowance is 3,072 GiB. Everything here counts in 2^30 and calls it GB, as the
  provider does and as the free-disk check already did. Choosing the other base
  buys a month of false comfort.
- **Egress is billed as usage types with a zero rate.** The billed quantity is
  readable before any overage is charged, which makes it a leading indicator
  rather than a bill.
- **The Cost Explorer API is not free.** It is charged per paginated request.
  Polling it hourly costs more than it can possibly reveal, because the
  underlying data refreshes only a few times a day, and it lags about fourteen
  hours behind them. Daily is the correct cadence, and `$0.31/month` is its
  whole price. The reconciliation therefore makes **exactly one call per run**
  and passes `--no-paginate`; a `NextPageToken` in the answer stops the run
  rather than buying a second page, because a query this narrow does not page
  and a token means the question changed.
- **The reconciliation cannot run on the host.** It needs a cloud credential,
  and the service host deliberately holds none. So it runs from an operator
  machine and ships one small JSON file to the host over `ssh`, atomically,
  where the monitor reads it. That is the only reason the whole cost layer can
  reach an alert channel without putting a key on a public-facing box.

**Two projections, because each is blind where the other sees.** The
month-to-date burn-down cannot be fooled by a quiet afternoon and is meaningless
in a month whose first days were never observed. The 24-hour trailing rate sees
a change of behaviour today and forgets last week. The severity comes from
whichever is higher, and the alert says which one drove it. A trailing rate with
no closed hour behind it is *unknown*, and unknown is not zero.

**A threshold this close to the steady state needs hysteresis.** The bundle's
3,072 GB over 31 days is 99.1 GB/day and the corrected service draws about 98.5,
so the trailing projection sits at about 99% of the allowance and stays there.
Without hysteresis that is an alert and a recovery every five minutes until
somebody mutes the channel. Severity steps up immediately and steps down only
with `MV_TRANSFER_HYST_PCT` points of clearance below the threshold that raised
it. The same reasoning gives the check a six-hour warm-up: the first alert an
operator ever receives decides whether the channel is trusted.

**The rate trip is the leading half.** A month-to-date line moves slowly by
construction, so it is the wrong instrument for "something started". Three
consecutive hours above 9 GB/h — against a 4–5 GB/h corrected baseline and a
4.1 GB/h sustainable rate — would have caught the developer machine holding
`/watch` tabs open sixteen hours before a person did, and does not fire at the
current baseline.

**One line of `/etc/hosts` is worth a check of its own.** The archive subscribes
to the relay by name, and the name resolves publicly to this host's own address.
The `127.0.0.1` pin is what keeps that ~54 GB/day subscription on loopback and
off the bill. Lose it and billed transfer roughly doubles while every health
signal stays green, which is precisely the class of failure this document exists
to catch.

**A reading that arrives from off-box has absence as its first failure.** Every
other check in this document fails by reading something bad. The `billing` check
fails by reading nothing, because the machine that writes its input is an
operator's, and operator machines close. So a missing or stale reconciliation is
reported rather than skipped: it is a WARNING and never a critical, since the
NIC counter is still watching the allowance and what has been lost is the audit
rather than the alarm. The window is 36 hours — a daily job, plus one missed
run, plus the provider's own lag — so one failed run is not an alert and two
are. What *is* critical is a billed overage quantity, which is the provider
stating that the meter is running and is the one reading here that is not an
inference.

**The two instruments are compared over the same days, and their residual is
published rather than corrected.** The instance metric sums about `1.01` times
the Cost Explorer quantity in the same window. Nobody can explain the last
percent, so correcting it would hide the day it changes; the ratio is written
into the file and the monitor warns outside a band of `0.10` either side of 1.
The comparison uses only the days Cost Explorer has settled — never
month-to-date against month-to-date, because the fourteen-hour lag alone would
put that comparison tens of percent out and cost somebody a day.

**And a day is not settled because it has data in it.** The newest day Cost
Explorer returns always has data and always has *less* data than the day
contains, so counting it makes the same error one level down: a whole day of
counter against a part day of invoice. The first real run of the reconciliation
reported `1.37` and `1.17` from two instruments that agree to `1.01`, and that
would have been the monitor's first-ever billing alert — saying the instruments
disagree, while both were correct. A day now counts only once it has been over
for longer than the lag, which at 06:30 UTC makes the newest settled day the day
before yesterday. That is the honest reading; a fresher one is a part day. This
is the general shape of the trap: **a lagging source makes its most recent
record look complete, and the reader is the only party that knows it is not.**

**The principal is the account root, read-only, and that is a recorded
acceptance rather than an oversight.** On 2026-08-16 the owner accepted continued
read-only use of the existing profile for metrics and billing reads, and declined
a scoped IAM user for it. The written reconciliation carries that sentence in its
`principal` field, so the fact travels with the reading instead of living only in
a decision record. Every call the reconciliation makes is read-only.

The default anomaly detection on a new account will not fire below a large
absolute impact, which on an account of this size means it can never fire. It
must be replaced with thresholds scaled to the actual floor, or it is
decoration.

## Deployment tracking

A deployment is a change to a running service, so the record of one has to
answer "is it worse now". Today it cannot.

The provisioning script runs seventeen verification checks and every one is
binary pass/fail on **presence** — a unit is active, a health endpoint answers,
a listener is on loopback. A deployment can pass all seventeen and leave the
service measurably degraded, and the record will say every check passed.

### The health window

Every deployment record carries a window, not a moment:

| Metric | T−15 | T+0 | T+15 | T+60 |
|---|---|---|---|---|
| live / dark slots | | | | |
| population | | | | |
| achieved time-scale sum | | | | |
| status age | | | | |
| worst per-slot stats age | | | | |
| bypassed lanes (and empty cells) | | | | |
| ledger records | | | | |
| archive replay seconds, if it restarted | | | | |

`deploy/health-snapshot.sh` produces this. It keeps numbers and decides
nothing: no thresholds, no severity, no exit code that means "bad". A single
reading before and after cannot show a flap that resolves between them, which
is why the window is sampled rather than sampled twice.

The achieved time-scale sum is on the list because it is the workload variable
every capacity formula in the sizing model is written against. A deployment
that changes it has changed the cost model, and that should be visible in the
record rather than discovered a month later.

### Gates

**Install the provisioning kit before expecting it to write anything.** The
on-box kit is versioned separately from the binaries and the shipping script
does not update it. A stale kit renders a valid environment file that is simply
missing the keys the current source expects — and the script exits zero and
reports success, because it did write a file. Two separate work streams lost
settings to this in one day.

The gate is therefore an assertion on **file content, per expected key by
name**. An exit code proves nothing here and neither does the log line.

**Assert ancestry.** Refuse to install a source older than or unrelated to what
is running, unless an approved rollback names it.

**Take the lease.** One deployment owner per target at a time.

### Restart cost is part of the plan

The archive rebuilds its view of the record on start, and crossings during that
replay are absent rather than delayed — a permanent gap in the record. So
archive-side instrumentation is batched into one restart, its replay cost stated
in advance, and never shipped to collect a single diagnostic number.

**The cost is now measured rather than modelled, and it is bounded.** A restart
rebuilds its aggregates from a state sidecar and parses only one duplicate
window of raw records; the archive publishes what that cost at its own last
start, and `monitor.sh`'s `replay-cost` check projects the next one from it. An
earlier version of this paragraph cited a restart that replayed 3.7 million
records in 57 seconds and said the cost grew with the record. It did, on that
build. Stating the cost in advance now means reading a projection, not
multiplying a record count.

**Two things nothing else on the host watches**, both added for that reason: a
state sidecar that stopped saving, which breaks nothing and turns the next
restart back into a full replay, and closed segments with no confirmed off-host
copy, which is the only signal that the cold archive has stopped working. Each
one is invisible in every other reading.

A relay restart is cheaper in records and more expensive in blast radius: it
disconnects every peer at once. Neither is done casually, and both leave a
record.

**The gap stopped being hypothetical on 2026-08-17**, and the shape it took is
worth carrying here because no reading on this page would have prevented it. A
package manager's own restart hook restarted both services after an unrelated
`apt` transaction, outside every gate a deployment sets. The worst part was not
the outage: it was that the **relay came back first and the archive did not**,
so for a minute and a half the map was live with nothing recording it, and those
crossings are absent rather than delayed. **`duplicatesRefused` climbing
afterwards is evidence the duplicate guard worked, not evidence the gap was
filled** — a crossing forwarded into that window generates no duplicate, because
nothing ever recorded it. There is no counter that measures a gap; the only
evidence is the restart timestamps against the relay's own connection log.
[`RESTART-POLICY.md`](../deploy/RESTART-POLICY.md), "Package installs and
needrestart", carries the rule that prevents it.

## Acting on a measurement

Most of this document is about seeing. One rule governs acting.

**An automatic remedy is a relief valve, not a schedule.** The world recycler
restarts a world when host memory falls below a threshold; it does not restart
worlds every N minutes. The distinction is not stylistic. The leak it
compensates for is a defect, and a fixed schedule would go on restarting healthy
worlds long after the defect was fixed, while a threshold quietly stops acting.
Anything automatic here states the condition it responds to, and does nothing
when that condition is absent.

**A remedy that can deepen an outage must refuse to run during one.** The
recycler will not touch a world while any world is already dark, will not act
twice inside its cool-off, and refuses outright if it cannot confirm that the
world it picked saves on quit. Each guard exists because the failure it prevents
is worse than the delay it causes.

**Prefer taking the action the failure would take anyway, earlier and gently.**
The recycler does not prevent restarts — the OOM killer was already performing
them, with `SIGKILL`, at its own moment, after dragging every other world through
a swap storm. Doing the same restart early with `SIGTERM` and save-on-quit costs
no simulated time. When a remedy looks drastic, check whether the failure is
already doing something worse.

**Automatic action needs its own record.** The recycler logs the world, its
resident size and the memory reading that triggered it, and leaves that decision
in `/run/bibites-ops/recycle.state`; the Spot watcher leaves the same kind of
breadcrumb for every poll. Both files are rewritten on each tick, so the age of
the file says whether the watcher is alive and its contents say what it last
decided. No collector reads either file yet — today an operator does. An
unexplained restart in a metrics series is worse than no restart at all.

**A record must not live where the thing it observes can delete it.** The first
version of both breadcrumbs was written to `/run/bibites`, which is a systemd
`RuntimeDirectory` of the world unit — so the first real recycle destroyed its
own evidence, and the Spot watcher's, in the same run. Storage owned by the
lifecycle under observation is not storage.

## Investigating an incident

Order matters, because the cheap evidence is also the most decisive and it
expires.

1. **Read what already exists before adding anything.** `metrics.jsonl` on the
   service host, `performance.jsonl` on the world host, and the relay log. Two
   of the three carry per-peer detail nothing else has.
2. **Read pressure before utilisation.** `/proc/pressure/{cpu,memory,io}` on
   both hosts says which resource anything actually *waited* for. A utilisation
   graph cannot answer that and will mislead you if you let it.
3. **Check the free platform metrics.** Host CPU, and burst capacity for the
   service host from the Lightsail API. State the granularity in the finding —
   EC2 without detailed monitoring is 5-minute averages and cannot resolve a
   30-second event.
4. **Preserve the logs.** Raw logs are evidence and must not be committed to
   either repository. Copy them outside both, record the path and a checksum,
   and cite that in the incident record.
5. **Establish which hosts are implicated before theorising about mechanism.**
   The single most useful question in the 2026-08-15 incident turned out to be
   whether the one peer on a *different* host was also affected. It was not,
   which eliminated the entire service host and its front door in one step.
6. **Exclude the known confounds.** An archive restart makes every world look
   dark for the duration of its replay, and the only health signal most
   runbooks use is served by the archive itself — so it cannot distinguish
   "worlds left" from "the archive is rebuilding its view of them". The status
   epoch resetting is the tell.
7. **State resolution with every claim.** "CPU did not dip" from five-minute
   averages does not mean CPU did not dip.

## Alerting

The monitor alerts on severity change with a repeat while bad and a daily
heartbeat. That shape is right and stays. What it does mean is that a check
reports only as finely as its severity does: while a check sits at one severity
the channel stays quiet however much worse the reading gets underneath it, and
the repeat is the only thing that speaks. A check that saturates early is
therefore a check that goes quiet early, which is what the peer floor below
turns into a blind spot.

**Severity travels in the alert, never in the exit code.** A check runner exits
non-zero when *it* could not run, and zero when it ran and found something —
including something critical. `monitor.sh` once did the opposite, and the cost
was a `Type=oneshot` unit that a five-minute timer left in `failed` almost
permanently: `systemctl is-failed multiverse-monitor.service` stopped
distinguishing a monitor with a warning from a monitor that never started, and
a deployment pipeline under `set -o pipefail` broke on a healthy tick. The
verdict belongs in the channel, in the printed `worst:` line and in the stored
per-check severity, all of which say *which* check and *how bad*. An exit code
says neither, and spending it on severity throws away the only signal that
could have reported the watcher's own death.

Three things about it are wrong today and are part of this standard:

- The error check counts only `ERROR`-level lines. The relay logs a
  disconnection at `INFO`. Every world can leave and the error count stays
  zero.
- The expected-peer floor ships as zero, so the check watches dark slots only
  and "every live slot vanished" is a warning rather than a critical. On a map
  that already has a dark slot it is not even that: the peers check is already
  at `WARN`, so a collapse changes no severity, sends nothing, and waits for the
  `MV_ALERT_REPEAT_HOURS` repeat — twelve hours by default — to say it in the
  same words it used for one dark slot. That is not a hypothetical. On
  `2026-08-18` a world-host failure took the map down to one live slot and ten
  dark and no alert was sent, because dark slots had held the check at `WARN`
  for days. Raising `MV_EXPECTED_PEERS` to the map's peer count once worlds have
  joined is what closes it, and it is the one threshold in the monitor that has
  to be raised by hand as the map grows.
- Silence is indistinguishable from health. A monitor on the service host
  cannot report that the service host is gone. Something off-host must watch
  for the absence of a heartbeat, or an outage of the whole box is
  self-concealing.

## Rollout

Ordered by diagnostic value per unit of risk to a live public experiment. The
rest of this document is the standard; this section is the only part that says
what is actually running.

### What exists today

- **Both host samplers.** The world host's sampler now also records PSI,
  `/proc/vmstat` and per-world resident memory, keeping every field it already
  had. The service host's sampler and its one-minute timer are installed and
  enabled. Layer 1's table gives the interval and the path for each.
- **The deployment health window.** `deploy/health-snapshot.sh` is in the
  hosting kit and fills in [the health window](#the-health-window) on request.
  It is a tool a person runs, not something a deployment does by itself yet.
- **The world recycler.** `bibites-recycle.timer` is enabled on the world host
  and asks every ten minutes. `bibites-recycle-world` acts only below its memory
  threshold and records what it decided.
- **The Spot watchers.** Both read the HTTP status of each IMDS poll instead of
  discarding it, refresh the token at half its lifetime, log a heartbeat every
  ten minutes, and leave a liveness breadcrumb. Before this they went blind
  after six hours and said nothing.
- **The transfer budget check.** `monitor.sh` reads `/proc/net/dev` every five
  minutes, accumulates month-to-date transfer and 24 closed hourly buckets in
  `/var/lib/multiverse/monitor`, and alerts on the higher of the two projections
  in [Layer 4](#layer-4--cost). Two companion checks ship with it: consecutive
  hot hours, and the `/etc/hosts` loopback pin. It needs no cloud credential,
  which is the only reason it can run on this host at all.
  `deploy/test-monitor.sh` exercises its arithmetic off-host.
- **The daily billing reconciliation.** `deploy/ce-reconcile.sh` runs on an
  operator machine, makes one Cost Explorer call and two free instance-metric
  calls, and ships `billing.json` to the host over `ssh`. `monitor.sh`
  `billing` reads it every five minutes: a billed overage is critical, and a
  reconciliation that stopped arriving is a warning that names the script and
  the machine it runs from. `deploy/test-ce-reconcile.sh` parses a saved
  response under `deploy/testdata/` against a fake metric provider, so the
  parser is covered without spending `$0.01` a run or depending on a figure
  that changes daily.
- **The archive memory gate, divided by physical RAM alone.** `monitor.sh`
  `replay` projects the archive's resident set and replay peak from the ledger
  record count and compares the larger against `MemTotal`. **Swap is not in
  that denominator.** It once was, which meant a critical archive-memory
  verdict could be cleared by adding a swap file while the ledger, the retained
  state and the replay were all exactly as they were — a tripwire a one-line
  change turns green without touching what it watches. Swap is reported by its
  own `swap` check instead, which says whether any exists and warns when
  sustained use makes it a sizing signal rather than a crash barrier. The two
  per-record constants are parameters (`MV_REPLAY_PEAK_B`,
  `MV_REPLAY_RESIDENT_B`) so a measurement on a real ledger can retune the gate
  without a release; [`SIZING.md`](../deploy/SIZING.md) "Archive memory" owns
  their values. `deploy/test-monitor.sh` covers this arithmetic too, including
  the swap case as a regression. **Those constants are a property of the
  deployed build**: a change to what the archive retains per record leaves the
  gate describing a binary that is no longer running, so retuning them belongs
  in the same deployment as the change.
- **The record layer's four checks.** `monitor.sh` gained `replay-cost`,
  `cold-copy`, `rollup` and `duplicates` beside the memory gate, and
  `--only archive` runs the last three on their own. `replay-cost` projects the
  next restart's held-down-relay time from `replayRawSeconds` and
  `replayRawRecords` — which the archive measures at its own start — and from
  the record rate this box samples for itself; it falls back to saying so, and
  naming the record-count model as the only remaining estimate, against a
  binary that publishes neither field. `cold-copy` watches
  `ledgerSegmentsAwaitingColdCopy`, and it is **the only thing on this host that
  watches the object store at all**: no segment is removed without a receipt, so
  a cold archive that stopped working costs disk rather than record, weeks
  later, in a different check. `rollup` watches `rollupSavedAtMs`. `duplicates`
  watches `duplicatesRefused`, which is the evidence for lowering the duplicate
  window and therefore the restart cost. `deploy/test-monitor.sh` covers all
  four off-host, including the first-maintenance-pass reading that would
  otherwise fire on every healthy restart.
- **The off-host copy of the ledger segments.** `deploy/coldcopy.sh` runs hourly
  under `multiverse-coldcopy.timer`, copies each closed compressed segment once,
  verifies it by reading the object back out of the store, and writes the
  receipt without which the archive removes nothing. `--check` answers "would
  this work" without writing an object, and `provision.sh --only verify` runs it.
  The timer is enabled only when a destination is configured; with none, no
  receipt is written and nothing is ever retired, which is the safe default.
- **The archive's anonymous memory and replay peak, in the record.** The
  service-host sampler records `anonBytes`, `fileBytes`, `mainRssAnonBytes`,
  `mainVmHwmBytes` and `mainPid` per unit beside the `MemoryCurrent` it already
  had. [Layer 1](#layer-1--host) says why the original field could not see the
  problem it was added to watch.
- **A world's time scale survives a restart it performs itself.**
  `bibites-game@%i` wants `bibites-timescale@%i`, so a world that systemd brings
  back applies its target scale again instead of running silently at `x1`.

Every reading above is a file on the host that produced it, and a person is the
only reader. The one exception is the reconciliation, which is produced
elsewhere and read by the monitor — and which is exactly why the monitor has to
report its absence.

### What does not exist yet

No *continuous* measurement ships off-box: no metrics agent, no time-series
store, no log shipping, no dashboard, no continuous profiling. The daily
reconciliation is the one exception and it travels the other way — an operator
machine reads the provider and writes one file to the host. The Layer 3 path
probe has not been written. The relay still does not log a close code, so the
highest-value change named in this document is still unmade. On the cost side
there is still no budget and no replacement for the default anomaly threshold.

The phases below keep their order. Everything in them remains to be done,
except where a phase says otherwise.

**Phase 0 — no new daemon, no production change.** Read the existing JSONL
files. Import them into a time-series database on a workstation; they are
already time series and cover the whole service period. Create the free budgets
and replace the default anomaly threshold; the daily reconciliation above
already covers the daily-granularity half of this phase. Point the existing webhook alert channel at somewhere a person
looks. Confirm both clocks are disciplined, because every cross-host ordering
claim depends on it.

**Phase 1 — one read-only daemon per host.** Its first half is done: the
service-host sampler and its timer are deployed. What remains is a metrics agent
that ships off-box and buffers with an **explicit** disk cap, because the
default is unlimited and the disk is shared with the archive.

**Phase 2 — the stateful tier, off the fragile box.** The time-series store and
log store live on the host with memory to spare, on its retained volume, so
they survive an interruption of the instance. A free external mirror carries
the dead-man's switch, because that is the only thing that can alert when the
service host is the thing that failed.

**Phase 3 — code.** Close-code logging on the relay first: it is small,
log-only, changes no wire format, and it is the difference between a disconnect
that names its cause and one that does not. Then the per-slot session counter.
Then diagnostic listeners, loopback-only, on a **separate mux** — the public
front door proxies the archive's whole mux by rule, so anything added there is
public the moment the binary ships.

## What this standard does not cover

- **The network between the hosts.** Neither endpoint can see the middle. A
  silent path failure and a peer that stopped talking look identical from both
  ends, and correlation of both sides' counters is inference, not proof.
- **Frame contents.** Nothing here records payloads. A semantic failure will
  not be found by any of it.
- **Sub-second ordering across hosts**, unless both clocks are disciplined and
  verified.
- **Whether a measurement is worth its cost.** That judgement is the
  operator's, and this document's bias is that a permanent per-minute disk cost
  needs a much better reason than a log line does.
