# `docs/` — the support surface

**This directory is written for people who did not build this.** A participant is somebody who
installed a mod, joined a map, and has no build toolchain, no access to anybody else's machine,
and no context. Everything here is written to be read by that person, or — in the case of the
two reference documents — to make sure somebody can answer them.

**It is not the operator's book.** `dev_environment.md` at the repository root is the rig's own
record: how the project's development deployment is brought up, what it last read, and which
traps it has fallen into. It is where several of the checks and symptoms here were first
learned, and it stays where it is. Nothing in `docs/` is a copy of it.

## What is here

| Document | Who reads it | What it answers |
|---|---|---|
| [`participant/install.md`](participant/install.md) | a participant | What do I need, what does the installer do, and what does a bare install actually do once it runs? |
| [`participant/join.md`](participant/join.md) | a participant | What is a join string, what happens on my first claim, where do I land, and what does joining publish about my world? |
| [`participant/diagnose.md`](participant/diagnose.md) | a participant | Something is wrong. What do I read, in what order, and what do I send when I ask for help? |
| [`participant/leave.md`](participant/leave.md) | a participant | What does stopping mean, what does leaving mean, and what becomes of my world and its place on the map? |
| [`error-taxonomy.md`](error-taxonomy.md) | a participant **and** the operator | Every refusal this system can hand somebody, with the remedy **and who must act** |
| [`sidecar-diagnose-spec.md`](sidecar-diagnose-spec.md) | whoever implements `--diagnose` | Each check, its pass criterion, and the taxonomy entry a failure points at |

## The rule the taxonomy exists for

**Every refusal names the remedy and who must act.** The second half is the novel part, and the
reason is a failure mode this system has already produced: a world's lanes run badly, its queues
fill, and **the cause is in somebody else's install**. The machine that suffers is not the
machine at fault, and the operator is the only party positioned to see both ends
(`m5_considerations.md`, DQ8).

So the taxonomy has three actors — **you**, **operator**, **nobody** — and no entry is complete
without one.

## Slots

Several entries carry a **slot**: a marked to-do naming the work package that fills it.

> **SLOT — WP*n* (name).** What is missing, and why it cannot be written yet.

**A slot is a to-do with an owner, never a blank.** They exist because this documentation spine
was drafted against WP1's contracts while the packages that invent what a participant actually
sees were still being built. **WP2's and WP4's are closed**: the credential refusals and the
join string's printed form, the published capacity table, the admin path's participant-visible
effects and the lineage-gap and export-edge texture are now quoted from what ships. What
remains is owned by **WP3** (the hosted deployment), **WP6** (the package), **WP8** (the bands
only a playtest can measure) and this package's own later arcs. Each document collects its own
slots in a table at the end.

## Where the authority lives

Everything here is derived. When this directory and one of these disagrees, these win:

| For | Read |
|---|---|
| The wire between a game's mod and its sidecar | `contracts/contract-a.md` |
| The wire between a sidecar and the relay | `contracts/contract-b-m4.md` |
| Genome identity | `contracts/genome-hash.md` |
| Project-wide decisions | `system_decomposition.md` |
| This milestone's design, and the reasoning behind the support surface | `m5_considerations.md` |
| The development rig | `dev_environment.md` |
