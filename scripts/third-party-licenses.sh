#!/bin/sh
# Copy the license files of every third-party module linked into the mrimpact
# binary into third_party_licenses/<module path>/. Works offline from the module
# cache. Fails when a linked module ships no license file.
set -eu

out=third_party_licenses
main=$(go list -m)
rm -rf "$out"
mkdir -p "$out"

mods=$(go list -deps -f '{{with .Module}}{{.Path}}{{end}}' ./cmd/mrimpact | sort -u)
count=0
for mod in $mods; do
	[ "$mod" = "$main" ] && continue
	dir=$(go list -m -f '{{.Dir}}' "$mod")
	[ -n "$dir" ] || { echo "third-party-licenses: no directory for $mod" >&2; exit 1; }
	found=0
	for f in "$dir"/LICENSE* "$dir"/COPYING* "$dir"/NOTICE*; do
		[ -f "$f" ] || continue
		mkdir -p "$out/$mod"
		cp "$f" "$out/$mod/"
		found=1
	done
	if [ "$found" -eq 0 ]; then
		echo "third-party-licenses: no license file in $mod ($dir)" >&2
		exit 1
	fi
	count=$((count + 1))
done
# The Go standard library and runtime are linked into the binary too.
goroot=$(go env GOROOT)
[ -f "$goroot/LICENSE" ] || { echo "third-party-licenses: no LICENSE in $goroot" >&2; exit 1; }
mkdir -p "$out/go"
cp "$goroot/LICENSE" "$out/go/LICENSE"
echo "third-party-licenses: $count modules written to $out/"
