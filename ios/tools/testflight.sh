#!/bin/sh
# Archives the Beta configuration for devices and, with `upload`, sends it to App Store Connect
# for TestFlight. Beta talks to production (Release.xcconfig) and carries the 3D mascot.
#
# Usage: ios/tools/testflight.sh archive   # signed archive only, to check signing works
#        ios/tools/testflight.sh upload    # archive, then export and upload
#
# Signing is automatic (team 84X9WYBF36). Upload authenticates one of two ways:
#   - an App Store Connect API key, when ASC_KEY_ID, ASC_ISSUER_ID and ASC_KEY_PATH are set
#     (the .p8 file stays outside the repo; nothing here reads or prints it);
#   - otherwise the Apple ID signed in to Xcode (Settings > Accounts).
#
# The build number is the UTC date and time, YYYYMMDD.HHMM: every upload needs a higher one
# than the last, and this needs no state. MARKETING_VERSION stays as the project sets it.
set -eu
cd "$(dirname "$0")/../.."
mode="${1:-archive}"
case "$mode" in archive | upload) ;; *) echo "usage: $0 archive|upload" >&2; exit 2 ;; esac

# Beta carries the 3D mascot, whose model is built outside the repo (BRD-001). Without it the
# archive would build and ship a mascot view with nothing to show.
if [ ! -f ios/Postitheka/Mascot3D/Mascot.usdz ]; then
	echo "testflight: ios/Postitheka/Mascot3D/Mascot.usdz is missing; build it with brand/tools/build_mascot3d.py" >&2
	exit 1
fi

build="$(date -u +%Y%m%d.%H%M)"
out="${TESTFLIGHT_OUT:-build/testflight/$build}"
mkdir -p "$out"

auth=""
if [ -n "${ASC_KEY_ID:-}" ] && [ -n "${ASC_ISSUER_ID:-}" ] && [ -n "${ASC_KEY_PATH:-}" ]; then
	auth="-authenticationKeyPath $ASC_KEY_PATH -authenticationKeyID $ASC_KEY_ID -authenticationKeyIssuerID $ASC_ISSUER_ID"
fi

# shellcheck disable=SC2086 # $auth is a list of flags on purpose
xcodebuild archive -project ios/Postitheka.xcodeproj -scheme Postitheka -configuration Beta \
	-destination 'generic/platform=iOS' -archivePath "$out/Postitheka.xcarchive" \
	-allowProvisioningUpdates $auth CURRENT_PROJECT_VERSION="$build" -quiet
echo "testflight: archived build $build at $out/Postitheka.xcarchive"

[ "$mode" = upload ] || exit 0

# shellcheck disable=SC2086
xcodebuild -exportArchive -archivePath "$out/Postitheka.xcarchive" \
	-exportOptionsPlist ios/tools/ExportOptions.plist -exportPath "$out/export" \
	-allowProvisioningUpdates $auth
echo "testflight: uploaded build $build; it shows in App Store Connect > TestFlight once processed"
