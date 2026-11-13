#!/bin/sh
# Brand checks that `task check` and CI run (BRD-001).
#
# 1. Each client's copy of design/mascot-motion.json matches it byte for byte. The clients
#    bundle their own copy because neither can reach the other's tree at build time; a copy
#    edited alone would make web and iOS move differently while both tests still passed. A
#    missing copy passes: the iOS Release build excludes its copy on purpose.
# 2. No hex colour literal in the 3D code. Its colours are brand tokens (styles.css on the web,
#    the asset catalog on iOS); a literal there is a colour that drifts from the palette.
set -eu
cd "$(dirname "$0")/.."

status=0
fail() {
	echo "brand-guard: $1" >&2
	status=1
}

for copy in web/src/lib/mascot3d/mascot-motion.json ios/Postitheka/Mascot3D/mascot-motion.json; do
	if [ -f "$copy" ] && ! cmp -s design/mascot-motion.json "$copy"; then
		fail "$copy differs from design/mascot-motion.json; edit the design file, then: cp design/mascot-motion.json $copy"
	fi
done

for file in web/src/lib/mascot-motion.ts web/src/lib/mascot3d/scene.ts web/src/components/Mascot3D.tsx \
	ios/Postitheka/Mascot3D/Mascot3DView.swift; do
	[ -f "$file" ] || continue
	if grep -nE '#[0-9A-Fa-f]{6}\b|#[0-9A-Fa-f]{3}\b|0x[0-9A-Fa-f]{6}\b' "$file"; then
		fail "$file has a hex colour literal; use a brand token"
	fi
done

exit "$status"
