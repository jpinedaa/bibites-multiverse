#!/usr/bin/env bash
# Mint the relay's TLS material for the M5 crossing: a private CA and one server
# certificate for the M4 LAN rig's relay (contract-b-m4.md §22, B23).
#
# WHY A PRIVATE CA AND NOT A SELF-SIGNED LEAF. The clients verify with their
# platform's trust store and there is deliberately no knob between them and it
# (go/internal/sidecar/contractb_client.go: "NO TLS CONFIGURATION IS SET HERE AND
# NONE MAY BE"). So *something* has to be trusted. A CA is the smaller thing to
# trust: the far end imports one certificate once, and a renewed leaf — a moved
# LAN address, a new SAN, an expiry — costs a re-run of this script and NOTHING on
# the second computer. A self-signed leaf would make every renewal a far-end
# errand, and D9 says this machine cannot run those errands.
#
# WHERE THE OUTPUT GOES. $TLS_DIR, default e2e/tls-m4-lan/, which is GITIGNORED.
# The private keys never enter the repository. The only file that leaves this
# machine is ca.crt, which is public by construction.
#
#   e2e/crossing/mint-tls.sh              # mint (refuses to overwrite)
#   e2e/crossing/mint-tls.sh --renew-leaf # keep the CA, mint a new server cert
#   TLS_DIR=/tmp/scratch e2e/crossing/mint-tls.sh   # a rehearsal, off the rig
#
# WHAT IT DOES NOT DO. It does not touch a running process, it does not install
# anything into a trust store, and it does not copy anything to the far end. The
# runbook (e2e/crossing/RUNBOOK.md) says where each output is consumed.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E="$(cd "$HERE/.." && pwd)"

TLS_DIR="${TLS_DIR:-$E2E/tls-m4-lan}"

# The names a client will actually dial, and nothing else. A SAN list is a
# statement about who may present this certificate, so a spare entry is a real
# widening.
#
#   127.0.0.1        the five local sidecars and the archive. With --tls-cert the
#                    relay's listener is TLS for EVERY address including loopback
#                    (crypto/tls wraps the net.Listener whole), so the local
#                    clients dial wss://127.0.0.1 and verify this name.
#   192.168.1.227    the far end's RelayHost. It is this machine's Windows LAN
#                    address, recorded in dev_environment.md, *Owner steps*, and
#                    confirmed by the far end itself on 2026-08-03. The Windows
#                    portproxy forwards it into the WSL VM; the far end's TLS
#                    client verifies the name it DIALLED, which is this one.
#   172.24.110.174   the WSL address, the portproxy's connectaddress. Nothing
#                    dials it by name today; it is here so that a debugging dial
#                    from Windows against the VM directly does not fail on the
#                    name, and because it costs nothing to state a name this
#                    machine already owns. Override with WSL_IP= to drop it.
#   localhost        the same loopback under its name, for a hand dial.
LAN_HOST="${LAN_HOST:-192.168.1.227}"
WSL_IP="${WSL_IP:-$(hostname -I 2>/dev/null | awk '{print $1}')}"

CA_DAYS="${CA_DAYS:-3650}"
LEAF_DAYS="${LEAF_DAYS:-397}"

renew_leaf=0
[ "${1:-}" = "--renew-leaf" ] && renew_leaf=1

say() { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
die() { printf '\nSTOP: %s\n' "$*" >&2; exit 1; }

command -v openssl >/dev/null || die "openssl is not on PATH"

umask 077
mkdir -p "$TLS_DIR"
chmod 700 "$TLS_DIR"

CA_KEY="$TLS_DIR/ca.key"
CA_CRT="$TLS_DIR/ca.crt"
SRV_KEY="$TLS_DIR/relay.key"
SRV_CRT="$TLS_DIR/relay.crt"
SRV_CSR="$TLS_DIR/relay.csr"

if [ "$renew_leaf" = 0 ] && [ -e "$CA_KEY" ]; then
  die "$CA_KEY already exists. A second CA would strand the far end's imported
     copy of the first one. Use --renew-leaf to keep the CA and mint a new server
     certificate, or move $TLS_DIR aside deliberately."
fi
if [ "$renew_leaf" = 1 ] && [ ! -e "$CA_KEY" ]; then
  die "--renew-leaf needs an existing CA at $CA_KEY."
fi

# The SAN list, built once and used for both the request and the signature.
# openssl needs it in both places: a CSR's SAN is not carried into the
# certificate unless -copy_extensions or an explicit -extfile says so, and this
# uses the explicit form so the list in force is the list written here.
sans="IP:127.0.0.1,DNS:localhost,IP:$LAN_HOST"
[ -n "$WSL_IP" ] && sans="$sans,IP:$WSL_IP"

step "the names this certificate will carry"
say "$sans"

if [ "$renew_leaf" = 0 ]; then
  step "1 of 3 — the CA (valid $CA_DAYS days)"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
    -nodes -keyout "$CA_KEY" -out "$CA_CRT" -days "$CA_DAYS" -sha256 \
    -subj "/CN=Bibites Multiverse relay CA" \
    -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null
  say "CA key  $CA_KEY"
  say "CA cert $CA_CRT"
else
  step "1 of 3 — keeping the existing CA"
  say "CA cert $CA_CRT"
fi

step "2 of 3 — the relay's server certificate (valid $LEAF_DAYS days)"
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
  -keyout "$SRV_KEY" -out "$SRV_CSR" -sha256 \
  -subj "/CN=bibites-multiverse-relay" 2>/dev/null

# serverAuth only. This certificate authenticates a listener and nothing else.
openssl x509 -req -in "$SRV_CSR" -CA "$CA_CRT" -CAkey "$CA_KEY" \
  -CAcreateserial -out "$SRV_CRT" -days "$LEAF_DAYS" -sha256 \
  -extfile <(printf '%s\n' \
    "basicConstraints=critical,CA:FALSE" \
    "keyUsage=critical,digitalSignature,keyEncipherment" \
    "extendedKeyUsage=serverAuth" \
    "subjectAltName=$sans") 2>/dev/null
rm -f "$SRV_CSR"

# The relay reads both files on every handshake through its CertReloader, so the
# only requirement is that the process can read them. 0600 owned by the rig's
# user is what that means here.
chmod 600 "$CA_KEY" "$SRV_KEY"
chmod 644 "$CA_CRT" "$SRV_CRT"

step "3 of 3 — verify"
openssl verify -CAfile "$CA_CRT" "$SRV_CRT" >/dev/null || die "the leaf does not verify against the CA"
say "leaf verifies against the CA"
openssl x509 -in "$SRV_CRT" -noout -ext subjectAltName | sed 's/^/     /'
openssl x509 -in "$SRV_CRT" -noout -enddate | sed 's/^/     /'

cat <<EOF

Minted into $TLS_DIR (gitignored — nothing here may be committed except by
deliberate act, and the two .key files never).

  relay.crt / relay.key   the relay's --tls-cert / --tls-key
  ca.crt                  trusted by every client:
                            WSL side  — SSL_CERT_FILE=$TLS_DIR/ca.crt on the five
                                        sidecars and the archive (no sudo, no
                                        system trust store change)
                            far end   — imported into the Windows trust store by
                                        setup-farend.ps1 -CaFile .\\ca.crt
  ca.key                  KEEP. --renew-leaf needs it, and re-minting the CA
                          costs a far-end trust import that D9 forbids this
                          machine to perform.

The far end needs a copy of ca.crt and nothing else from this directory.
EOF
