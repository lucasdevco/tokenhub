#!/usr/bin/env bash
set -euo pipefail

cd "${3:-$(dirname "$0")/..}"
version="${1:?usage: package-macos.sh VERSION ARCH}"
arch="${2:?usage: package-macos.sh VERSION ARCH}"
[[ "$(go env GOOS)" == darwin && "$(go env GOARCH)" == "$arch" ]]
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]

app="build/darwin-$arch/tokenhub.app"
dmg="build/darwin-$arch/tokenhub $version $arch.dmg"
installer="tokenhub-darwin-$arch.dmg"
cli="tokenhub-cli-darwin-$arch"
out="build/release"
mkdir -p "$out"

test -x "$app/Contents/MacOS/tokenhub"
test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$app/Contents/Info.plist")" = "$version"
codesign --verify --deep --strict "$app"
hdiutil verify "$dmg"
if [[ "${REQUIRE_NOTARIZATION:-false}" == true ]]; then
  metadata="$(codesign --display --verbose=4 "$app" 2>&1)"
  grep -F 'Authority=Developer ID Application: chaoqun huang (93MJ8HDSM3)' <<< "$metadata"
  grep -F 'TeamIdentifier=93MJ8HDSM3' <<< "$metadata"
  grep -F 'runtime' <<< "$metadata"
  codesign --verify --strict "$dmg"
  codesign --verify --strict build/tokenhub
  xcrun stapler validate "$app"
  xcrun stapler validate "$dmg"
  spctl --assess --type execute --verbose=4 "$app"
  spctl --assess --type open --context context:primary-signature --verbose=4 "$dmg"
fi
./build/tokenhub -h

mountpoint="$(mktemp -d)"
disk=""
cleanup() {
  if [[ -n "$disk" ]]; then
    hdiutil detach "$disk" -force >/dev/null 2>&1 || true
  fi
  rmdir "$mountpoint"
}
trap cleanup EXIT
disk="$(hdiutil attach -readonly -nobrowse -noautoopen -mountpoint "$mountpoint" "$dmg" |
  awk '/^\/dev\/disk/ && !disk { disk = $1 } END { print disk }')"
[[ "$disk" == /dev/disk* ]]
test -x "$mountpoint/tokenhub.app/Contents/MacOS/tokenhub"
test -L "$mountpoint/Applications"
cmp "$app/Contents/MacOS/tokenhub" "$mountpoint/tokenhub.app/Contents/MacOS/tokenhub"
hdiutil detach "$disk" -force
rmdir "$mountpoint"
trap - EXIT

cp "$dmg" "$out/$installer"
cp build/tokenhub "$out/$cli"
"$out/$cli" -h
manifest="build/darwin-$arch/update-darwin-$arch.json"
if [[ -f "$manifest" ]]; then
  go run "$(dirname "$0")/verify-update.go" "$manifest" desktop/mygo.json "$version" "$arch"
  cp "$manifest" "build/darwin-$arch/tokenhub-$version-darwin-$arch.tar.gz" "$out/"
elif [[ "${REQUIRE_UPDATES:-false}" == true ]]; then
  echo "Signed update archive is required for a release" >&2
  exit 1
fi
(
  cd "$out"
  files=("$installer" "$cli")
  if [[ -f "update-darwin-$arch.json" ]]; then
    files+=("update-darwin-$arch.json" "tokenhub-$version-darwin-$arch.tar.gz")
  fi
  shasum -a 256 "${files[@]}" > "SHA256SUMS-$arch"
)
