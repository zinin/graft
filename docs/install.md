# Install

[graft](../README.md) › Install

graft runs on Linux, macOS and Windows with Claude Code 2.1.327 or later. On Linux and macOS, to install the latest release into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | sh
```

[install.sh](../install.sh) takes the build for your system from the [latest release](https://github.com/zinin/graft/releases/latest), checks it against the release's `SHA256SUMS` and installs it; run it again to update. `GRAFT_VERSION` picks another release and `GRAFT_INSTALL_DIR` another directory:

```sh
curl -fsSL https://raw.githubusercontent.com/zinin/graft/master/install.sh | GRAFT_VERSION=0.1.0 GRAFT_INSTALL_DIR=~/bin sh
```

A release also has the archives for a manual install, on x86-64 (`amd64`) and ARM (`arm64`). The macOS builds are not notarized: after a download in a browser, run `xattr -d com.apple.quarantine graft` before the first start.

With Go 1.26, `go install github.com/zinin/graft/cmd/graft@latest` builds graft from source. From a clone, `make install` puts the binary into `~/.local/bin`.

## Windows

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/zinin/graft/master/install.ps1 | iex
```

[install.ps1](../install.ps1) takes the build for your machine from the latest release, checks it against the release's `SHA256SUMS` and installs `graft.exe` into `%USERPROFILE%\.local\bin`, where Claude Code's native installer puts `claude.exe`; run it again to update. It does not change PATH: when the folder is not there, it prints the command that adds it. `$env:GRAFT_VERSION` picks another release and `$env:GRAFT_INSTALL_DIR` another folder:

```powershell
$env:GRAFT_VERSION = '0.5.0'; irm https://raw.githubusercontent.com/zinin/graft/master/install.ps1 | iex
```

graft starts the `claude.exe` of Claude Code's native installer, `irm https://claude.ai/install.ps1 | iex`, and never the `claude.cmd` that npm puts in PATH: cmd.exe would garble graft's arguments.

A release also has zip archives for Windows, on x86-64 (`amd64`) and ARM (`arm64`). `graft.exe` is not signed. install.ps1 leaves it without the mark of a download, but a zip downloaded in a browser and unpacked in Explorer passes the mark on, and Windows may warn before the first start: `Unblock-File graft.exe` removes it.
