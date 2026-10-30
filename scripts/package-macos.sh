#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
version="${1:?usage: package-macos.sh VERSION ARCH}"
arch="${2:?usage: package-macos.sh VERSION ARCH}"
[[ "$(go env GOOS)" == darwin && "$(go env GOARCH)" == "$arch" ]]
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]

app="build/darwin-$arch/tokenhub.app"
dmg="build/darwin-$arch/tokenhub $version $arch.dmg"
prefix="tokenhub-v$version-macos-$arch"
out="build/release"
mkdir -p "$out"

test -x "$app/Contents/MacOS/tokenhub"
test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$app/Contents/Info.plist")" = "$version"
codesign --verify --deep --strict "$app"
hdiutil verify "$dmg"
./build/tokenhub -h

mountpoint="$(mktemp -d)"
cleanup() {
  hdiutil detach "$mountpoint" -force >/dev/null 2>&1 || true
  rmdir "$mountpoint"
}
trap cleanup EXIT
hdiutil attach -readonly -nobrowse -noautoopen -mountpoint "$mountpoint" "$dmg"
test -x "$mountpoint/tokenhub.app/Contents/MacOS/tokenhub"
test -L "$mountpoint/Applications"
cmp "$app/Contents/MacOS/tokenhub" "$mountpoint/tokenhub.app/Contents/MacOS/tokenhub"
hdiutil detach "$mountpoint"
rmdir "$mountpoint"
trap - EXIT

cp "$dmg" "$out/$prefix.dmg"
tar -czf "$out/$prefix-cli.tar.gz" -C build tokenhub -C .. config.example.yaml README.md
unpacked="$(mktemp -d)"
trap 'rm -rf "$unpacked"' EXIT
tar -xzf "$out/$prefix-cli.tar.gz" -C "$unpacked"
test -f "$unpacked/config.example.yaml"
test -f "$unpacked/README.md"
"$unpacked/tokenhub" -h
(
  cd "$out"
  shasum -a 256 "$prefix.dmg" "$prefix-cli.tar.gz" > "SHA256SUMS-$arch.txt"
)
