# Install the latest NetRunner release (Windows).
#   irm https://raw.githubusercontent.com/kxngHADES/NetRunner/main/scripts/install.ps1 | iex
# Optional: $env:VERSION = "v0.1.0"; $env:INSTALL_DIR = "C:\tools\netrunner"
$ErrorActionPreference = "Stop"

$repo = "kxngHADES/NetRunner"
$installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "NetRunner\bin" }

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$version = $env:VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
if (-not $version) { throw "could not determine latest version (no releases yet?)" }

$ver = $version.TrimStart("v")
$archive = "netrunner_${ver}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("netrunner-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "Downloading NetRunner $version (windows/$arch)..."
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive)
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { $_ -match [regex]::Escape($archive) + "$" }
    $expected = ($line -split "\s+")[0]
    $actual = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
    if (-not $expected -or $expected -ne $actual) { throw "checksum verification failed" }

    Expand-Archive (Join-Path $tmp $archive) -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    Copy-Item (Join-Path $tmp "netrunner.exe") (Join-Path $installDir "netrunner.exe") -Force
}
finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "Installed to $installDir\netrunner.exe"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to your user PATH. Open a new terminal, then run: netrunner"
}
