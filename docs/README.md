# `docs/` — the support surface

**This directory is written for people who did not build this.** A participant is somebody who
installed a mod, joined a map, and has no build toolchain, no access to anybody else's machine,
and no context. Everything here is written to be read by that person, or — in the case of the
two reference documents — to make sure somebody can answer them.

**It is not the developer guide.** [`dev_environment.md`](../dev_environment.md) explains local
builds, test rigs, and durable technical findings. Nothing in `docs/` copies that guide.

## What is here

| Document | Who reads it | What it answers |
|---|---|---|
| [`participant/install.md`](participant/install.md) | a participant | What do I need, what does the installer do, and what does a bare install actually do once it runs? |
| [`participant/join.md`](participant/join.md) | a participant | How does automatic public enrollment work, what is a private-map join string, where do I land, and what does joining publish? |
| [`participant/diagnose.md`](participant/diagnose.md) | a participant | Something is wrong. What do I read, in what order, and what do I send when I ask for help? |
| [`participant/leave.md`](participant/leave.md) | a participant | What does stopping mean, what does leaving mean, and what becomes of my world and its place on the map? |
| [`support-matrix.md`](support-matrix.md) | a participant **and** the operator | Which mod and sidecar build goes with which game version **on which platform**, how to look your own build up, and what a map with two game builds on it does |
| [`error-taxonomy.md`](error-taxonomy.md) | a participant **and** the operator | Every refusal this system can hand somebody, with the remedy **and who must act** |
| [`sidecar-diagnose-spec.md`](sidecar-diagnose-spec.md) | whoever implements or reviews `--diagnose` | Each check, its pass criterion, and the taxonomy entry a failure points at — plus the exit codes, the JSON schema, and the participant's own-slot view `--my-slot` |
| [`defaults-audit.md`](defaults-audit.md) | a reviewer, and the operator | Every default the release ships with, what a bare install does with it, and the verdict |

**Two of these travel with the release rather than only living here.** `support-matrix.md`'s
machine-readable block is copied into **every** release archive as `support-matrix.json`. Each
installer therefore uses the refusal text from the published page. `defaults-audit.md` is linked
from the release page, because a reader must know what a bare install does before running it.

**Every page here covers both platforms and the editions this release publishes.** The recommended
complete package for each platform includes an authorized game. The Windows GUI can instead use
an existing Steam copy. The Linux add-on uses an existing itch.io copy. A complete edition
installs a versioned game runtime. Where a command, a path or a
refusal differs, the page gives each form. Where something exists on one platform only, the page marks it
(`INS-MARKOFWEB` and `INS-EXECPOLICY` on Windows; `INS-NOTEXECUTABLE`, `INS-LINUXDEPS` and
`LOCAL-LOGSHRED` on Linux). The support matrix is honest about the two rows not carrying equal
weight.

## The rule the taxonomy exists for

**Every refusal names the remedy and who must act.** The second half is the novel part, and the
reason is a failure mode this system has already produced: a world's lanes run badly, its queues
fill, and **the cause is in somebody else's install**. The machine that suffers is not the
machine at fault, and the operator is the only party positioned to see both ends
(`m5_considerations.md`, DQ8).

So the taxonomy has three actors — **you**, **operator**, **nobody** — and no entry is complete
without one.

## Release state

The `0.2.1` participant documents contain no work-package placeholders.
Track a new documentation gap in a GitHub issue. Update the affected page with its fix.

## Where the authority lives

Everything here is derived. When this directory and one of these disagrees, these win:

| For | Read |
|---|---|
| The wire between a game's mod and its sidecar | `contracts/contract-a.md` |
| The wire between a sidecar and the relay | `contracts/contract-b-m4.md` |
| Public installer credential creation | `contracts/public-enrollment.md` |
| Genome identity | `contracts/genome-hash.md` |
| Project-wide decisions | `system_decomposition.md` |
| Current public phase | `STATUS.md` |
| This milestone's design, and the reasoning behind the support surface | `m5_considerations.md` |
| Local development and durable technical findings | `dev_environment.md` |
