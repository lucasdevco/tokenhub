#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${APPLE_SIGNING_IDENTITY:-}" ]]; then
  if [[ "${REQUIRE_NOTARIZATION:-false}" == true ]]; then
    echo "A release requires a Developer ID signing identity" >&2
    exit 1
  fi
  exit 0
fi
codesign --force --sign "$APPLE_SIGNING_IDENTITY" --options runtime --timestamp build/tokenhub
codesign --verify --strict --verbose=2 build/tokenhub

if [[ "${REQUIRE_NOTARIZATION:-false}" == true ]]; then
  archive="$RUNNER_TEMP/tokenhub-cli-notarization.zip"
  result="$RUNNER_TEMP/tokenhub-cli-notarization.json"
  ditto -c -k --keepParent build/tokenhub "$archive"
  xcrun notarytool submit "$archive" --keychain-profile tokenhub-notary --keychain "$APPLE_SIGNING_KEYCHAIN" --wait --output-format json > "$result"
  python3 - "$result" <<'PY'
import json, pathlib, sys
result = json.loads(pathlib.Path(sys.argv[1]).read_text())
if result.get('status') != 'Accepted':
    raise SystemExit(f"CLI notarization failed: {result}")
print('CLI notarization accepted:', result['id'])
PY
fi
