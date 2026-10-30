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
(
  cd "$out"
  shasum -a 256 "$installer" "$cli" > "SHA256SUMS-$arch"
)
