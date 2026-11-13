#!/bin/sh
# Proves the App Store build holds none of the 3D mascot (BRD-001): builds Release for the
# simulator and fails if the model, the motion file or the view got in. Run by `task ios:check`.
#
# Three checks, because one is not enough: the linker dead-strips unused code, so a binary
# without the symbol does not show the file was left out. The object file's absence does.
#
# Usage: ios/tools/release-proof.sh [Release|Beta]. Beta is the negative check, run by hand
# after changing the exclusion: it must fail, or this script proves nothing.
set -eu
cd "$(dirname "$0")/../.."
configuration="${1:-Release}"

derived="$(mktemp -d)"
trap 'rm -rf "$derived"' EXIT

xcodebuild build -project ios/Postitheka.xcodeproj -scheme Postitheka -configuration "$configuration" \
	-destination 'generic/platform=iOS Simulator' -derivedDataPath "$derived" CODE_SIGNING_ALLOWED=NO -quiet

app="$(find "$derived/Build/Products" -name Postitheka.app -type d | head -1)"
[ -n "$app" ] || { echo "release-proof: no Postitheka.app built" >&2; exit 1; }

status=0
found="$(find "$app" \( -name '*.usdz' -o -name 'mascot-motion.json' \) -print)"
if [ -n "$found" ]; then
	echo "release-proof: $configuration bundles 3D mascot files:" >&2
	echo "$found" >&2
	status=1
fi

objects="$(find "$derived/Build/Intermediates.noindex" -name 'Mascot3DView.o' -print)"
if [ -n "$objects" ]; then
	echo "release-proof: $configuration compiled Mascot3DView.swift:" >&2
	echo "$objects" >&2
	status=1
fi

# The binary may sit beside a debug dylib in Debug-like builds; check every Mach-O in the app.
for binary in $(find "$app" -maxdepth 1 -type f -perm -u+x) $(find "$app" -maxdepth 1 -name '*.dylib'); do
	if strings -a "$binary" | grep -q 'Mascot3DView\|MascotStage'; then
		echo "release-proof: $configuration's $(basename "$binary") names Mascot3DView" >&2
		status=1
	fi
done

if [ "$status" -eq 0 ]; then
	echo "release-proof: $configuration holds no 3D mascot model, motion file or view"
fi
exit "$status"
