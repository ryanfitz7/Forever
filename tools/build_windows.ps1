param(
    [string]$GoBin = '',
    [string]$SiteBase = '/classic/',
    [string]$SiteRepo = 'ryanfitz7/Forever',
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
$savedPath = $env:PATH
$savedGoos = $env:GOOS
$savedGoarch = $env:GOARCH
$savedSiteBase = $env:SITE_BASE
$savedSiteRepo = $env:SITE_REPO
try {
    if (-not $GoBin) {
        $localGoBin = Join-Path (Split-Path $projectRoot -Parent) '.toolchain/go/bin'
        if (Test-Path (Join-Path $localGoBin 'go.exe')) { $GoBin = $localGoBin }
    }
    $pluginBin = Join-Path (Split-Path $projectRoot -Parent) '.toolchain/bin'
    $env:PATH = "$GoBin;$pluginBin;$projectRoot/node_modules/.bin;$savedPath"
    $env:SITE_BASE = $SiteBase
    $env:SITE_REPO = $SiteRepo
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Install Go 1.23.4 or newer, or pass -GoBin.' }
    if (-not (Test-Path node_modules/.bin/protoc.cmd)) {
        & npm.cmd ci --no-audit --no-fund
        if ($LASTEXITCODE) { throw 'npm ci failed' }
    }
    if (-not (Get-Command protoc-gen-go -ErrorAction SilentlyContinue)) {
        throw 'Install protoc-gen-go v1.36.6 and add its bin directory to PATH.'
    }
    New-Item -ItemType Directory -Force sim/core/proto,ui/core/proto,dist/classic | Out-Null
    $protoSources = Get-ChildItem proto/*.proto | ForEach-Object { 'proto/' + $_.Name }
    & protoc.cmd '--proto_path=proto' '--go_out=sim/core' @protoSources
    if ($LASTEXITCODE) { throw 'Go protobuf generation failed' }
    & protoc.cmd '--ts_opt=generate_dependencies' '--ts_out=ui/core/proto' '--proto_path=proto' proto/api.proto
    if ($LASTEXITCODE) { throw 'API TypeScript protobuf generation failed' }
    & protoc.cmd '--ts_out=ui/core/proto' '--proto_path=proto' proto/test.proto proto/ui.proto
    if ($LASTEXITCODE) { throw 'UI TypeScript protobuf generation failed' }

    $coreRoot = (Resolve-Path ui/core).Path
    $imports = Get-ChildItem ui/core -Recurse -Filter *.ts |
        Where-Object { $_.FullName -ne (Join-Path $coreRoot 'index.ts') } |
        Sort-Object FullName |
        ForEach-Object { 'import "./' + [IO.Path]::GetRelativePath($coreRoot, $_.FullName).Replace('\','/').Replace('.ts','') + '";' }
    [IO.File]::WriteAllLines((Join-Path $coreRoot 'index.ts'), $imports)
    & tsc.cmd --noEmit
    if ($LASTEXITCODE) { throw 'TypeScript check failed' }
    if (-not $SkipTests) {
        & go test --tags=with_db ./sim/core ./sim/priest/...
        if ($LASTEXITCODE) { throw 'Priest or core tests failed' }
    }
    $env:GOOS = 'js'
    $env:GOARCH = 'wasm'
    & go build -o dist/classic/lib.wasm ./sim/wasm/
    if ($LASTEXITCODE) { throw 'WASM build failed' }
    $env:GOOS = $savedGoos
    $env:GOARCH = $savedGoarch
    & node --import tsx vite.build-workers.ts
    if ($LASTEXITCODE) { throw 'Worker build failed' }
    & vite.cmd build
    if ($LASTEXITCODE) { throw 'Site build failed' }
    New-Item -ItemType Directory -Force dist/classic/assets | Out-Null
    Get-ChildItem assets | Where-Object Name -ne 'db_inputs' |
        Copy-Item -Destination dist/classic/assets -Recurse -Force
    Write-Host 'Built dist/classic. Serve with: python -m http.server 8766 --bind 127.0.0.1 --directory dist'
}
finally {
    $env:PATH = $savedPath
    $env:GOOS = $savedGoos
    $env:GOARCH = $savedGoarch
    $env:SITE_BASE = $savedSiteBase
    $env:SITE_REPO = $savedSiteRepo
    Pop-Location
}
