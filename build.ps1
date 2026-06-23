param(
    [string]$GoExe = $env:GO_EXE,
    [string]$UpxExe = $env:UPX_EXE
)

$ErrorActionPreference = "Stop"
$Repo = $PSScriptRoot
$Build = Join-Path $Repo "build"
$Release = Join-Path $Repo "release"
$Binary = Join-Path $Build "zlan-telemetryd"
$RootfsBinary = Join-Path $Repo "rootfs\usr\bin\zlan-telemetryd"
$Archive = Join-Path $Release "zlan-hybrid-persistent.tar.gz"
$Tailscale = Join-Path $Repo "assets\tailscale.combined"
$ExpectedTailscaleHash = "626120fc20dac637772e1f045d5e2568f5875af36435da7221897a9d274fbe11"
$Version = (Get-Content -Raw (Join-Path $Repo "VERSION")).Trim()
$PayloadVersion = (Get-Content -Raw (Join-Path $Repo "rootfs\usr\share\zlan-hybrid\VERSION")).Trim()

if (-not $GoExe) {
    $GoExe = (Get-Command go -ErrorAction SilentlyContinue).Source
}
if (-not $UpxExe) {
    $UpxExe = (Get-Command upx -ErrorAction SilentlyContinue).Source
}
if (-not $GoExe) { throw "Go nao encontrado. Defina GO_EXE ou instale Go 1.22+." }
if (-not $UpxExe) { throw "UPX nao encontrado. Defina UPX_EXE; a compressao e obrigatoria para a flash de 16 MB." }
if (-not (Test-Path $Tailscale)) { throw "assets/tailscale.combined nao encontrado." }
if ($Version -ne $PayloadVersion) { throw "VERSION ($Version) difere da versao do payload ($PayloadVersion)." }

$actualTailscaleHash = (Get-FileHash -Algorithm SHA256 $Tailscale).Hash.ToLowerInvariant()
if ($actualTailscaleHash -ne $ExpectedTailscaleHash) {
    throw "SHA-256 inesperado para tailscale.combined: $actualTailscaleHash"
}

New-Item -ItemType Directory -Force $Build, $Release | Out-Null

Push-Location $Repo
try {
    & $GoExe test ./...
    if ($LASTEXITCODE -ne 0) { throw "Testes Go falharam." }

    $oldGoos = $env:GOOS
    $oldGoarch = $env:GOARCH
    $oldGomips = $env:GOMIPS
    $oldCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "mipsle"
        $env:GOMIPS = "softfloat"
        $env:CGO_ENABLED = "0"
        & $GoExe build -trimpath -ldflags "-s -w -buildid=" -o $Binary ./cmd/zlan-telemetryd
        if ($LASTEXITCODE -ne 0) { throw "Cross-build MIPS falhou." }
    }
    finally {
        $env:GOOS = $oldGoos
        $env:GOARCH = $oldGoarch
        $env:GOMIPS = $oldGomips
        $env:CGO_ENABLED = $oldCgo
    }

    & $UpxExe --best --lzma $Binary
    if ($LASTEXITCODE -ne 0) { throw "UPX falhou." }
    Copy-Item -Force $Binary $RootfsBinary

    if (Test-Path $Archive) { Remove-Item -Force $Archive }
    & tar.exe -czf $Archive -C (Join-Path $Repo "rootfs") .
    if ($LASTEXITCODE -ne 0) { throw "Falha criando pacote persistente." }

    $archiveHash = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
    Set-Content -NoNewline -Encoding ascii -Path "$Archive.sha256" -Value "$archiveHash  zlan-hybrid-persistent.tar.gz`n"

    $archiveSize = (Get-Item $Archive).Length
    if ($archiveSize -gt 2500000) {
        throw "Pacote persistente excedeu o limite de 2.500.000 bytes: $archiveSize"
    }
    Write-Host "Pacote: $Archive ($archiveSize bytes)"
    Write-Host "SHA-256: $archiveHash"
}
finally {
    Pop-Location
}
