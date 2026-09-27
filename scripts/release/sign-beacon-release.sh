#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 <ed25519-private-key.pem> <checksums.txt> [version download-url sha256]" >&2
  exit 2
}

[ "$#" -eq 2 ] || [ "$#" -eq 5 ] || usage
private_key=$1
manifest=$2

[ -f "$private_key" ] || { echo "private key not found" >&2; exit 1; }
[ -f "$manifest" ] || { echo "checksum manifest not found" >&2; exit 1; }

signature_tmp=$(mktemp)
trap 'rm -f "$signature_tmp"' EXIT
openssl pkeyutl -sign -rawin -inkey "$private_key" -in "$manifest" -out "$signature_tmp"
openssl base64 -A -in "$signature_tmp" > "$manifest.sig"
printf '\n' >> "$manifest.sig"
chmod 600 "$manifest.sig"
echo "wrote $manifest.sig"

if [ "$#" -eq 5 ]; then
  version=$3
  download_url=$4
  checksum=$(printf '%s' "$5" | tr '[:upper:]' '[:lower:]')
  payload_tmp=$(mktemp)
  payload_signature_tmp=$(mktemp)
  trap 'rm -f "$signature_tmp" "$payload_tmp" "$payload_signature_tmp"' EXIT
  printf '%s\n%s\n%s' "$version" "$download_url" "$checksum" > "$payload_tmp"
  openssl pkeyutl -sign -rawin -inkey "$private_key" -in "$payload_tmp" -out "$payload_signature_tmp"
  openssl base64 -A -in "$payload_signature_tmp"
  printf '\n'
fi
