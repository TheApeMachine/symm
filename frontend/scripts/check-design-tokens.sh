#!/usr/bin/env bash
# Fails when app code paints with Tailwind's own palette instead of the theme
# tokens. The UI library's contract is that re-skinning means redeclaring the
# custom properties in components/ui/theme.css — a literal `zinc-800` is
# invisible to that, and it is also a different colour temperature from this
# theme's warm ramp, which is how one panel ends up looking foreign.
#
# Use the ramps in theme.css instead:
#   text   -> --f1 (loudest) .. --f4 (quietest)
#   bg     -> --bg / --sunken / --surface / --raised
#   border -> --line / --line2
#   tone   -> --acc --info --success --warning --error --up --down
set -euo pipefail

cd "$(dirname "$0")/.."

PALETTE='(bg|text|border|ring|fill|stroke|from|via|to|outline|decoration|shadow|accent|caret|divide|placeholder)-(zinc|slate|gray|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}'

status=0

if hits=$(grep -rnE "$PALETTE" src --include='*.tsx' --include='*.ts' --include='*.css'); then
	echo "Raw Tailwind palette colours found — use theme.css tokens instead:"
	echo
	echo "$hits"
	status=1
fi

# A literal hex in a utility is invisible to a re-theme in exactly the same way,
# and it is how --sunken ended up spelled #0a0907 in seventeen places.
HEX='(bg|text|border|ring|fill|stroke|outline|shadow|decoration|caret|accent)-\[#[0-9a-fA-F]{3,8}\]'

if hits=$(grep -rnE "$HEX" src --include='*.tsx' --include='*.ts' \
	--exclude-dir=ui); then
	echo "Hardcoded hex colours found — use theme.css tokens instead:"
	echo
	echo "$hits"
	status=1
fi

if [ "$status" -eq 0 ]; then
	echo "design tokens ok — no raw palette or hex colours in src/"
fi

exit "$status"
