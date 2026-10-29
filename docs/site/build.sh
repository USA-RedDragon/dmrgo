#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
out=${1:-artifacts/docs-site}
rm -rf "$out"
mkdir -p "$out"
for src in README.md docs/*.md docs/*/README.md; do
	case $src in
	README.md) page=index ;;
	docs/*/README.md)
		page=$(basename "$(dirname "$src")")
		[ ! -f "docs/$page.md" ] || continue
		;;
	*) page=$(basename "$src" .md) ;;
	esac
	pandoc -f gfm -t html5 --wrap=none --standalone --template docs/site/template.html --lua-filter docs/site/site.lua -M outdir="$out" -o "$out/$page.html" "$src"
done
echo "$out/index.html"
