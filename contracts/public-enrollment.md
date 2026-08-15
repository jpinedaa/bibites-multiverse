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

## Request

```json
{
  "format": "bibites-multiverse/enrollment-request/1",
  "installId": "9af42a17-6167-42e7-a6e8-c62cb8b95f4f",
  "secret": "<64 lower-case hexadecimal characters>",
  "release": "0.2.0"
}
```

| Field | Rule |
|---|---|
| `format` | Exact request format above |
| `installId` | UUID text. The installer creates it once and keeps it for a retry |
| `secret` | Client-generated Contract B secret: 32–256 printable ASCII characters with no spaces. The `0.2.0` installer uses 32 random bytes encoded as 64 lower-case hexadecimal characters |
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

The installer stores a protected pending record before its first request. It removes that record
only after it stores `peer-secret.txt`. A lost HTTP response is therefore a retry, not a second
identity.

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
