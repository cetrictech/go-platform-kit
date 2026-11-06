#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GPK_TLS_DIR="${GPK_TLS_DIR:-$PWD/integration/.runtime/certs}"
if [[ "${1:-up}" == down ]]; then
 docker compose -f integration/extended-compose.yaml down
 exit
fi
if [[ ! -f "$GPK_TLS_DIR/ca.crt" ]] || ! openssl x509 -checkend 3600 -noout -in "$GPK_TLS_DIR/ca.crt" >/dev/null; then
 GOWORK=off go -C integration run ./cmd/fixtures -dir "$GPK_TLS_DIR"
 docker compose -f integration/extended-compose.yaml down
fi
docker compose -f integration/extended-compose.yaml up -d --wait
# Administrative bootstrap stays on the loopback-only local test listener.
for bits in 256 512; do
 user="sdk-$bits"
 if curl --fail --silent --show-error http://127.0.0.1:29644/v1/security/users | python3 -c 'import json,sys;sys.exit(0 if sys.argv[1] in json.load(sys.stdin) else 1)' "$user"; then
  method=PUT; url="http://127.0.0.1:29644/v1/security/users/$user"
 else
  method=POST; url=http://127.0.0.1:29644/v1/security/users
 fi
 curl --fail --silent --show-error -X "$method" -H 'Content-Type: application/json' --data "{\"username\":\"$user\",\"password\":\"sdk-test-only\",\"algorithm\":\"SCRAM-SHA-$bits\"}" "$url"
done
