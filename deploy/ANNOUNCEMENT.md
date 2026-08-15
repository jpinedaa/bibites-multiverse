# Service communication policy

This file replaces the historical announcement copy.
It defines what the operator communicates and when.
It does not provide promotional text for a mass announcement.

## Discovery approach

Use soft discovery for the public map.
Share the project where it is relevant to an existing discussion.
Do not post repeated advertisements or imitate independent interest.

When the operator participates in a community discussion:

- Identify the relationship to the project.
- Explain why the project is relevant to that discussion.
- Link to the public repository or release page.
- Do not publish a join string.
- Follow the community rules and moderator direction.

Keep channel research, contact history, draft posts, and outcomes in private operations storage.
The public repository owns only this policy and participant-facing service facts.

## Information required before a join

Give each participant these facts before you transfer a join string:

1. The service start and end dates.
2. The rule for an extension or early closure.
3. The retention rule for the ledger and genome blobs.
4. The data that the map publishes about their world.
5. The normal effect of relay and archive restarts.
6. The wind-down timeline and record disposition.
7. The support and update channel.

Use one set of dates and terms in all participant documents.
The deployed `MV_PERIOD_START` and `MV_PERIOD_END` values must match those documents.

Do not shorten the service period without a dated notice.
An extension also needs a dated notice.

## Routine service notices

Send a notice before a planned relay, archive, or host restart.
State the affected service, expected duration, and participant action.

Usually, a participant does not need to act during a restart.
The sidecar reconnects and reclaims its reserved slot.

After an unplanned outage, send a short explanation.
State the actual start and end times when they are known.
Do not speculate about a cause before the evidence supports it.

If the archive was unavailable while the map ran, state that the permanent record has a gap.
Do not imply that missed archive records can be reconstructed.

## Capacity and security notices

Communicate a participant action only when the action is required.
Examples include a minimum-version change, a credential handover, or a certificate-authority change.

Do not include these values in a public notice:

- Join strings or credential secrets.
- Alert URLs or webhook capabilities.
- Private addresses and administration paths.
- Cloud account or physical resource identifiers.
- Support case identifiers.

Transfer a replacement credential through an approved private channel.

## Wind-down notices

Use the timeline in `WIND-DOWN.md`.
At minimum, give notices 30 days, 14 days, and 7 days before the planned end.

Each notice must repeat these facts:

- A participant world remains on the participant machine.
- The shared relay, archive, and website are the services that end.
- The final record follows the published retention rule.
- The operator will announce any extension.

After the end, publish a closing notice and a sanitized final summary.
Do not publish credential stores, raw logs, participant saves, or private infrastructure records.

## Record of communication

For each service notice, record these fields in private operations storage:

- Date and channel.
- Audience and purpose.
- Exact public link used.
- Service period or incident identifier.
- Required participant action.
- Outcome and follow-up date.

This record supports consistent communication without turning the public repository into an outreach log.
