# Security policy

## Supported versions

Security fixes apply to the latest GitHub release and the current `main` branch.

| Version | Security fixes |
|---|---|
| Latest GitHub release | Yes |
| `main` | Yes, before the next release |
| Older releases and branches | No |

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/jpinedaa/bibites-multiverse/security/advisories/new).
Do not report a vulnerability in a public issue.

Include this information:

- The affected component and version or commit
- The security impact
- The conditions that trigger the vulnerability
- Minimal reproduction steps or a proof of concept
- A proposed fix, if you have one

Remove credentials, join strings, personal data, game saves, and unrelated logs from the report.

CAUTION: Do not probe the hosted service in a way that can interrupt it or expose participant
data. Get written authorization before an active security test against the public service.

The maintainer will assess the report privately. A coordinated fix and disclosure can follow when
the impact and affected versions are known.

## Security boundaries

Useful reports include authentication bypasses, credential exposure, unsafe installer behavior,
TLS failures, path traversal, remote code execution, and unauthorized access to private data.

Compatibility errors, simulation balance, and expected public archive data are not security
vulnerabilities.
