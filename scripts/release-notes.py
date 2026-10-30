#!/usr/bin/env python3
"""Generate release notes from conventional commits and macOS assets."""

import os
import subprocess
import sys


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def main():
    tag = sys.argv[1]
    repository = os.environ.get("GH_REPO", "lucasdevco/tokenhub")
    base = f"https://github.com/{repository}"
    previous = subprocess.run(
        ["git", "describe", "--tags", "--abbrev=0", f"{tag}^"],
        capture_output=True,
        text=True,
    ).stdout.strip()
    revision = f"{previous}..{tag}" if previous else tag
    groups = {
        "Features": [],
        "Bug fixes": [],
        "Improvements": [],
        "Build and release": [],
        "Documentation": [],
        "Other changes": [],
    }
    categories = {
        "feat": "Features",
        "fix": "Bug fixes",
        "perf": "Improvements",
        "refactor": "Improvements",
        "build": "Build and release",
        "ci": "Build and release",
        "docs": "Documentation",
    }
    for line in git("log", "--no-merges", "--format=%H%x09%s", revision).splitlines():
        sha, subject = line.split("\t", 1)
        kind = subject.split(":", 1)[0].split("(", 1)[0].rstrip("!")
        groups[categories.get(kind, "Other changes")].append(
            f"- {subject} ([{sha[:7]}]({base}/commit/{sha}))"
        )
    for heading, commits in groups.items():
        if commits:
            print(f"## {heading}\n")
            print("\n".join(commits) + "\n")

    download = f"{base}/releases/download/{tag}"
    print("## Install\n")
    print("| Mac | Desktop installer | Command line |")
    print("| --- | --- | --- |")
    for label, arch in (("Apple Silicon", "arm64"), ("Intel", "amd64")):
        dmg = f"tokenhub-darwin-{arch}.dmg"
        cli = f"tokenhub-cli-darwin-{arch}"
        print(f"| {label} | [{dmg}]({download}/{dmg}) | [{cli}]({download}/{cli}) |")
    print("\nOpen the DMG and drag tokenhub.app into Applications.")
    print("For the CLI, download the binary for your Mac and run:\n")
    print("```sh\nchmod +x tokenhub-cli-darwin-arm64")
    print('./tokenhub-cli-darwin-arm64 -config "$HOME/.tokenhub/config.yaml"\n```')
    print("\nOn Intel Macs, replace arm64 with amd64. The web UI is embedded in the binary.")
    print("The configuration file is created on first launch; add providers in Settings.")
    print(f"\n[SHA-256 checksums]({download}/SHA256SUMS) cover all four packages.")
    print("The app uses ad hoc signing and is not notarized by Apple.\n")
    history = f"compare/{previous}...{tag}" if previous else f"commits/{tag}"
    print(f"[Full changelog]({base}/{history})")


if __name__ == "__main__":
    main()
