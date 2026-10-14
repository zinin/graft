#!/bin/sh
# install.sh installs graft from its GitHub releases:
#
#   curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | sh
#
# It takes the build of the latest release for this system, checks it against
# the release's SHA256SUMS and installs it as ~/.local/bin/graft. GRAFT_VERSION
# picks another release, such as 0.1.0, and GRAFT_INSTALL_DIR another directory.
set -eu

repo=zinin/graft

fail() {
	echo "graft install: $*" >&2
	exit 1
}

# fetch downloads the URL $1 into the file $2.
fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "neither curl nor wget is installed"
	fi
}

# sha256 prints the SHA-256 of the file $1.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		fail "neither sha256sum nor shasum is installed to check the download"
	fi
}

# The whole script is read before main runs, so a download cut short runs nothing.
main() {
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "graft runs on Linux and macOS, not on $(uname -s)" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "graft has builds for amd64 and arm64, not for $(uname -m)" ;;
	esac

	if [ -n "${GRAFT_VERSION:-}" ]; then
		release="https://github.com/$repo/releases/download/v${GRAFT_VERSION#v}"
	else
		release="https://github.com/$repo/releases/latest/download"
	fi
	dir=${GRAFT_INSTALL_DIR:-${HOME:+$HOME/.local/bin}}
	[ -n "$dir" ] || fail "HOME is not set: set GRAFT_INSTALL_DIR to the directory for graft"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' INT TERM

	fetch "$release/SHA256SUMS" "$tmp/SHA256SUMS" || fail "cannot download $release/SHA256SUMS"
	# The line of this system's archive: "<sha256>  graft_<version>_<os>_<arch>.tar.gz".
	line=$(awk -v s="_${os}_${arch}.tar.gz" 'substr($2, length($2) - length(s) + 1) == s { print $1, $2; exit }' "$tmp/SHA256SUMS")
	[ -n "$line" ] || fail "the release has no build for $os/$arch"
	want=${line%% *}
	archive=${line#* }

	echo "graft install: downloading $archive"
	fetch "$release/$archive" "$tmp/$archive" || fail "cannot download $release/$archive"
	got=$(sha256 "$tmp/$archive")
	[ "$got" = "$want" ] || fail "$archive does not match its SHA256SUMS"
	tar -xzf "$tmp/$archive" -C "$tmp" graft

	mkdir -p "$dir"
	install -m 755 "$tmp/graft" "$dir/graft"
	version=$("$dir/graft" version) || fail "$dir/graft does not run on this system"
	echo "graft install: $version is in $dir"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "graft install: $dir is not on your PATH; add it, for example: export PATH=\"$dir:\$PATH\"" ;;
	esac
}

main "$@"
