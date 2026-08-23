# Public installer enrollment

Version: `bibites-multiverse/enrollment-request/1` and
`bibites-multiverse/enrollment-response/1`.

This HTTPS endpoint creates the peer credential that a later Contract B connection presents. It
is not part of Contract B. It sends no migration, placement, or world-status message.

## Endpoint

`POST /api/enroll`

The public reverse proxy terminates TLS and sends this exact path to the loopback relay. The relay
serves the path only when public enrollment is enabled. The advertised relay URL must use `wss://`.

All responses use JSON and `Cache-Control: no-store`. Other methods receive `405` from the relay.
The hosted nginx configuration denies them before they reach the relay.

## Packaged public join configuration

All participant archives include `public-map.json`:

```json
{
  "format": "bibites-multiverse/public-map/1",
  "enrollmentUrl": "https://bibitesmultiverse.com/api/enroll",
  "relayUrl": "wss://bibitesmultiverse.com/contract-b/v4"
}
```

This file is public join configuration. It contains no peer identity or secret. The installer
creates those values before it sends the enrollment request. A private-map
`multiverse-join/1` string contains an identity and secret and must not be shared between
installations.

## Request

```json
{
  "format": "bibites-multiverse/enrollment-request/1",
  "installId": "9af42a17-6167-42e7-a6e8-c62cb8b95f4f",
  "secret": "<64 lower-case hexadecimal characters>",
  "release": "0.3.4"
}
```

| Field | Rule |
|---|---|
| `format` | Exact request format above |
| `installId` | UUID text. The installer creates it once and keeps it for a retry |
| `secret` | Client-generated Contract B secret: 32–256 printable ASCII characters with no spaces. The installers encode 32 random bytes as 64 lower-case hexadecimal characters |
| `release` | Non-empty release identifier, at most 32 characters. It is diagnostic metadata, not an admission rule |

Unknown fields, extra JSON values, malformed JSON, and bodies above 1 KiB are refused.

## Identity and response

The identity is deterministic:

```text
public-<lower-case installId with hyphens removed>
```

A new credential receives `201 Created`. An exact retry receives `200 OK`.

```json
{
  "format": "bibites-multiverse/enrollment-response/1",
  "relayUrl": "wss://bibitesmultiverse.com/contract-b/v4",
  "peerId": "public-9af42a17616742e7a6e8c62cb8b95f4f",
  "created": true
}
```

The response never contains the secret. The installer accepts it only when the format, relay URL,
and derived peer ID match its request and packaged public-map configuration.

## Retry and replacement rules

The pair `(installId, secret)` is idempotent. The relay verifies an exact retry against the stored
verifier and does not write another credential or consume another per-address entry.

The same `installId` with a different secret receives `409 Conflict`. Enrollment never replaces an
existing credential. Slot handover remains the only identity-replacement operation.

The installer stores a protected pending record before its first request. Windows applies an ACL
for the current account. Linux applies mode `0600`. The installer removes the pending record only
after it stores `peer-secret.txt` and the install record. A lost response is therefore a retry,
not a second identity.

**An installer enrolls only for a data root with no world in it, or when the participant has said
in as many words that it may take a second identity.** When the data root already holds
`peer-secret.txt`, the installer adopts the identity that names it — from `install-record.json`,
the pending record whose secret matches, a launcher profile or start script for that same data
root, or `data/peer-id` and `data/relay-url` — and sends no request. It never overwrites
`peer-secret.txt` on that path: the client holds the only recoverable copy of the secret. A secret
with no identity beside it, and a join string naming a different identity, are refused
(`INS-ENROLL`); a join string naming the **same** identity is a slot handover and is the one case
that replaces a secret, and then only when a file the installer itself wrote proves which world the
folder holds. An uninstall keeps `peer-secret.txt`, `data/peer-id` and `data/relay-url` unless the
participant also asks for the world's data.

**The one path that spends a second credential over an existing world** is a data root that still
names a world whose `peer-secret.txt` is gone: nothing recovers a secret, so the only alternatives
are a slot handover on the operator's side — which mints a different identity, not another
enrollment — or a new identity here. The installers refuse both by default and act only under
`-ReplaceWorldIdentity` / `--replace-world-identity`, keeping the stranded world's name in
`data/peer-id.previous`. **No server behaviour changes for any of this**: the endpoint's rules,
limits and idempotency are exactly as above, and this paragraph describes only what the clients do
before they call it.

## Limits and errors

| Status | Meaning |
|---|---|
| `400` | Invalid JSON, format, UUID, release, or secret shape |
| `409` | The installation UUID already has another credential |
| `429` | The source address reached the configured enrollment-window limit. `Retry-After` states the remaining seconds |
| `503` | Enrollment has no secure advertised relay or the total automatic-credential limit is full |

The relay applies a persistent total limit to identities whose names start with `public-`. It
applies an in-memory per-address limit to new identities. nginx applies a separate outer request
rate. A relay restart clears only the per-address time window; it does not clear credentials or
the total count.

## Secret handling

The client creates the only recoverable secret. HTTPS protects it in transit. The relay stores a
random salt and verifier in `peers.json`; it does not store or log the secret. The enrollment log
contains the peer ID, source address, release, creation result, and current public count.

The endpoint is an explicit open-enrollment surface. It is disabled by default in the relay. A
host enables it with finite limits. Disabling it stops new enrollment and does not disconnect or
revoke an existing world.
