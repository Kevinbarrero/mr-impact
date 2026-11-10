#!/bin/sh
# install.sh - download, verify and install a mrimpact release binary.
#
#   install.sh [-b bindir] [version]
#
# bindir defaults to ./bin, version to "latest" (a tag such as v0.3.0, or 0.3.0).
# The archive is verified against the release's checksums.txt before anything is
# written to bindir. Environment overrides (mostly for tests):
#   MRIMPACT_DOWNLOAD_URL  base of the release downloads
#   MRIMPACT_LATEST_URL    URL whose redirect names the latest tag
#   MRIMPACT_OS, MRIMPACT_ARCH  skip platform detection
set -eu

REPO_URL="https://github.com/Kevinbarrero/mr-impact/releases"
DOWNLOAD_URL="${MRIMPACT_DOWNLOAD_URL:-$REPO_URL/download}"
LATEST_URL="${MRIMPACT_LATEST_URL:-$REPO_URL/latest}"

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

usage() {
	echo "usage: install.sh [-b bindir] [version]" >&2
	exit 1
}

bindir=./bin
while getopts "b:h" opt; do
	case "$opt" in
	b) bindir=$OPTARG ;;
	*) usage ;;
	esac
done
shift $((OPTIND - 1))
[ $# -le 1 ] || usage
version=${1:-latest}

# --- platform ---------------------------------------------------------------
os=${MRIMPACT_OS:-}
if [ -z "$os" ]; then
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	MINGW* | MSYS* | CYGWIN*) os=windows ;;
	*) fail "unsupported operating system: $(uname -s)" ;;
	esac
fi
case "$os" in
linux | darwin | windows) ;;
*) fail "unsupported operating system: $os" ;;
esac

arch=${MRIMPACT_ARCH:-}
if [ -z "$arch" ]; then
	arch=$(uname -m)
fi
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture: $arch" ;;
esac

# --- tools ------------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
	fetcher=curl
elif command -v wget >/dev/null 2>&1; then
	fetcher=wget
else
	fail "curl or wget is required"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
	fail "sha256sum or shasum is required"
fi

if [ "$os" = windows ]; then
	ext=zip
	command -v unzip >/dev/null 2>&1 || fail "unzip is required"
else
	ext=tar.gz
	command -v tar >/dev/null 2>&1 || fail "tar is required"
fi

download() { # url dest
	if [ "$fetcher" = curl ]; then
		curl -fsSL -o "$2" "$1" || fail "download failed: $1"
	else
		wget -q -O "$2" "$1" || fail "download failed: $1"
	fi
}

# --- version ----------------------------------------------------------------
if [ "$version" = latest ]; then
	if [ "$fetcher" = curl ]; then
		headers=$(curl -fsSI "$LATEST_URL") || fail "cannot resolve the latest release from $LATEST_URL"
	else
		headers=$(wget -S --spider --max-redirect=0 "$LATEST_URL" 2>&1) || true
	fi
	tag=$(printf '%s\n' "$headers" | tr -d '\r' |
		sed -n 's/^[[:space:]]*[Ll][Oo][Cc][Aa][Tt][Ii][Oo][Nn]:[[:space:]]*//p' | sed 's/[[:space:]]*\[following\]$//; s/[[:space:]]*$//' | tail -n 1)
	tag=${tag%/}
	tag=${tag##*/}
	[ -n "$tag" ] || fail "cannot determine the latest release tag from $LATEST_URL"
else
	tag=$version
	case "$tag" in v*) ;; *) tag="v$tag" ;; esac
fi

archive="mrimpact_${tag#v}_${os}_${arch}.${ext}"

# --- download and verify ----------------------------------------------------
tmp=$(mktemp -d) || fail "cannot create a temporary directory"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

download "$DOWNLOAD_URL/$tag/$archive" "$tmp/$archive"
download "$DOWNLOAD_URL/$tag/checksums.txt" "$tmp/checksums.txt"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "no checksum entry for $archive in checksums.txt; refusing to install unverified"
got=$(sha256 "$tmp/$archive")
[ "$got" = "$want" ] || fail "checksum mismatch for $archive: expected $want, got $got"

# --- extract and install ----------------------------------------------------
bin=mrimpact
[ "$os" = windows ] && bin=mrimpact.exe
mkdir "$tmp/x"
if [ "$ext" = zip ]; then
	unzip -q "$tmp/$archive" "$bin" -d "$tmp/x" || fail "cannot extract $archive"
else
	tar -xzf "$tmp/$archive" -C "$tmp/x" "$bin" || fail "cannot extract $archive"
fi
[ -f "$tmp/x/$bin" ] || fail "$archive does not contain $bin"
chmod 755 "$tmp/x/$bin"

mkdir -p "$bindir" || fail "cannot create $bindir"
# Same directory as the target, so the final rename is atomic and never leaves a partial file.
cp "$tmp/x/$bin" "$bindir/.$bin.new.$$" || fail "cannot write to $bindir"
mv -f "$bindir/.$bin.new.$$" "$bindir/$bin" || {
	rm -f "$bindir/.$bin.new.$$"
	fail "cannot install into $bindir"
}

echo "mrimpact $tag installed to $bindir/$bin"
