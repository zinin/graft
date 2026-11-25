# Install

[graft](../README.md) › Install

graft runs on Linux and macOS with Claude Code 2.1.327 or later. To install the latest release into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | sh
```

[install.sh](../install.sh) takes the build for your system from the [latest release](https://github.com/zinin/graft/releases/latest), checks it against the release's `SHA256SUMS` and installs it; run it again to update. `GRAFT_VERSION` picks another release and `GRAFT_INSTALL_DIR` another directory:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | GRAFT_VERSION=0.1.0 GRAFT_INSTALL_DIR=~/bin sh
```

A release also has the archives for a manual install, on x86-64 (`amd64`) and ARM (`arm64`). The macOS builds are not notarized: after a download in a browser, run `xattr -d com.apple.quarantine graft` before the first start.

With Go 1.26, `go install github.com/zinin/graft/cmd/graft@latest` builds graft from source. From a clone, `make install` puts the binary into `~/.local/bin`.
