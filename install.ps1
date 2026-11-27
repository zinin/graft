# install.ps1 installs graft on Windows from its GitHub releases:
#
#   irm https://raw.githubusercontent.com/zinin/graft/master/install.ps1 | iex
#
# It takes the build of the latest release for this machine, checks it
# against the release's SHA256SUMS and installs it as graft.exe in
# %USERPROFILE%\.local\bin, where Claude Code's native installer puts
# claude.exe. $env:GRAFT_VERSION picks another release, such as 0.5.0, and
# $env:GRAFT_INSTALL_DIR another folder. It does not change PATH.
#
# iex runs the script in the session that calls it, so all of it lives in one
# script block: its functions and variables go when it ends. Errors are
# thrown, since exit would close the caller's window. The block runs only once
# it has been read whole, so a download cut short runs nothing. The script
# calls no .NET type, and where Constrained Language Mode refuses
# Get-FileHash and Expand-Archive, it takes certutil and tar.exe instead.
& {
    # Get-GraftArch returns the architecture of the builds that run here.
    # PROCESSOR_ARCHITEW6432 names the machine's in a 32-bit PowerShell.
    function Get-GraftArch {
        $arch = $env:PROCESSOR_ARCHITEW6432
        if (-not $arch) {
            $arch = $env:PROCESSOR_ARCHITECTURE
        }
        switch ($arch) {
            'AMD64' { return 'amd64' }
            'ARM64' { return 'arm64' }
        }
        throw "graft install: graft has builds for amd64 and arm64, not for $arch"
    }

    # Get-GraftFile downloads url into the file at path.
    function Get-GraftFile($url, $path) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $path
        } catch {
            throw "graft install: cannot download ${url}: $($_.Exception.Message)"
        }
    }

    # Get-GraftHash returns the SHA-256 of the file at path in hex.
    # Get-FileHash is a script that creates a .NET type, which Constrained
    # Language Mode may refuse; certutil, a program, computes it there.
    function Get-GraftHash($path) {
        try {
            return (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash
        } catch {
            # The hash is the line of hex: the others are in the system's
            # language, and old Windows put spaces between the bytes.
            foreach ($line in certutil -hashfile $path SHA256) {
                $hex = $line -replace ' ', ''
                if ($hex -match '^[0-9a-fA-F]{64}$') {
                    return $hex
                }
            }
            throw "graft install: cannot compute the SHA-256 of $path"
        }
    }

    # Expand-GraftArchive unpacks the zip at path into the folder to: with
    # Expand-Archive, or where Constrained Language Mode refuses that script,
    # with tar.exe, a program of Windows 10 1803 and later.
    function Expand-GraftArchive($path, $to) {
        try {
            Expand-Archive -LiteralPath $path -DestinationPath $to -Force
        } catch {
            New-Item -ItemType Directory -Force -Path $to | Out-Null
            tar.exe -xf $path -C $to
            if ($LASTEXITCODE -ne 0) {
                throw "graft install: cannot unpack $path"
            }
        }
    }

    function Install-Graft {
        $ErrorActionPreference = 'Stop'
        # Without its progress bar, Invoke-WebRequest of Windows PowerShell
        # 5.1 downloads many times faster.
        $ProgressPreference = 'SilentlyContinue'

        $release = 'https://github.com/zinin/graft/releases/latest/download'
        if ($env:GRAFT_VERSION) {
            $release = 'https://github.com/zinin/graft/releases/download/v' + ($env:GRAFT_VERSION -replace '^v', '')
        }
        $arch = Get-GraftArch
        $dir = $env:GRAFT_INSTALL_DIR
        if (-not $dir) {
            if (-not $env:USERPROFILE) {
                throw 'graft install: USERPROFILE is not set: set GRAFT_INSTALL_DIR to the folder for graft'
            }
            $dir = Join-Path $env:USERPROFILE '.local\bin'
        }

        $tmp = Join-Path $env:TEMP ('graft-install-' + (Get-Random))
        New-Item -ItemType Directory -Force -Path $tmp | Out-Null
        try {
            $sums = Join-Path $tmp 'SHA256SUMS'
            Get-GraftFile "$release/SHA256SUMS" $sums
            # The line of this machine's archive: "<sha256>  graft_<version>_windows_<arch>.zip".
            $want = $null
            $archive = $null
            foreach ($line in Get-Content -LiteralPath $sums) {
                $fields = -split $line
                if ($fields.Count -eq 2 -and $fields[1] -like "*_windows_$arch.zip") {
                    $want = $fields[0]
                    $archive = $fields[1]
                    break
                }
            }
            if (-not $archive) {
                throw "graft install: the release has no build for windows/$arch"
            }

            Write-Host "graft install: downloading $archive"
            $zip = Join-Path $tmp $archive
            Get-GraftFile "$release/$archive" $zip
            # -ne compares strings in any case: Get-FileHash writes upper case.
            if ((Get-GraftHash $zip) -ne $want) {
                throw "graft install: $archive does not match its SHA256SUMS"
            }
            $unpacked = Join-Path $tmp 'unpacked'
            Expand-GraftArchive $zip $unpacked

            New-Item -ItemType Directory -Force -Path $dir | Out-Null
            $exe = Join-Path $dir 'graft.exe'
            Copy-Item -LiteralPath (Join-Path $unpacked 'graft.exe') -Destination $exe -Force
            $version = & $exe version
            if ($LASTEXITCODE -ne 0) {
                throw "graft install: $exe does not run on this system"
            }
            Write-Host "graft install: $version is in $dir"

            $onPath = $env:PATH -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -eq $dir.TrimEnd('\') }
            if (-not $onPath) {
                $quoted = $dir -replace "'", "''"
                Write-Host ("graft install: $dir is not on your PATH; add it for new terminals with: " +
                    "[Environment]::SetEnvironmentVariable('PATH', [Environment]::GetEnvironmentVariable('PATH', 'User') + ';$quoted', 'User')")
            }
        } finally {
            Remove-Item -Recurse -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
        }
    }

    Install-Graft
}
