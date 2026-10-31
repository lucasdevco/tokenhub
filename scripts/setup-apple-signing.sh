#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${APPLE_SIGNING_P12:-}" && -z "${APPLE_SIGNING_PASSWORD:-}" && "${REQUIRE_NOTARIZATION:-false}" != true ]]; then
  echo "Developer build: Apple signing credentials are unavailable."
  exit 0
fi
: "${APPLE_SIGNING_P12:?Missing APPLE_SIGNING_P12}"
: "${APPLE_SIGNING_PASSWORD:?Missing APPLE_SIGNING_PASSWORD}"
: "${RUNNER_TEMP:?Missing RUNNER_TEMP}"

umask 077
keychain="$RUNNER_TEMP/tokenhub-signing.keychain-db"
identity="Developer ID Application: chaoqun huang (93MJ8HDSM3)"
python3 - <<'PY'
import base64, os, pathlib
folder = pathlib.Path(os.environ['RUNNER_TEMP'])
(folder / 'tokenhub-signing.p12').write_bytes(base64.b64decode(os.environ['APPLE_SIGNING_P12'], validate=True))
PY
password="$(openssl rand -hex 32)"
echo "::add-mask::$password"
security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$password" "$keychain"
security import "$RUNNER_TEMP/tokenhub-signing.p12" -k "$keychain" -P "$APPLE_SIGNING_PASSWORD" -T /usr/bin/codesign -T /usr/bin/security
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$password" "$keychain" >/dev/null
security list-keychains -d user -s "$keychain" "$HOME/Library/Keychains/login.keychain-db"
security find-identity -v -p codesigning "$keychain" | grep -F "$identity"

if [[ "${REQUIRE_NOTARIZATION:-false}" == true ]]; then
  : "${APPLE_NOTARY_KEY:?Missing APPLE_NOTARY_KEY}"
  : "${APPLE_NOTARY_KEY_ID:?Missing APPLE_NOTARY_KEY_ID}"
  : "${APPLE_NOTARY_ISSUER_ID:?Missing APPLE_NOTARY_ISSUER_ID}"
  python3 - <<'PY'
import os, pathlib
pathlib.Path(os.environ['RUNNER_TEMP'], 'tokenhub-notary.p8').write_text(os.environ['APPLE_NOTARY_KEY'])
PY
  xcrun notarytool store-credentials tokenhub-notary --key "$RUNNER_TEMP/tokenhub-notary.p8" --key-id "$APPLE_NOTARY_KEY_ID" --issuer "$APPLE_NOTARY_ISSUER_ID" --keychain "$keychain"
fi

export APPLE_SIGNING_IDENTITY="$identity" APPLE_SIGNING_KEYCHAIN="$keychain"
node --input-type=module -e '
  import fs from "node:fs";
  const file = "desktop/mygo.json";
  const config = JSON.parse(fs.readFileSync(file, "utf8"));
  config.macos.signingIdentity = process.env.APPLE_SIGNING_IDENTITY;
  if (process.env.REQUIRE_NOTARIZATION === "true") {
    config.macos.notarize = { keychainProfile: "tokenhub-notary", keychain: process.env.APPLE_SIGNING_KEYCHAIN };
  }
  fs.writeFileSync(file, JSON.stringify(config, null, 2) + "\n");
'
{
  echo "APPLE_SIGNING_IDENTITY=$identity"
  echo "APPLE_SIGNING_KEYCHAIN=$keychain"
} >> "$GITHUB_ENV"
rm -f "$RUNNER_TEMP/tokenhub-signing.p12" "$RUNNER_TEMP/tokenhub-notary.p8"
