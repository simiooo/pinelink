param(
    [Parameter(Mandatory=$true)][ValidatePattern('^v\d+\.\d+\.\d+-preview\.\d+$')][string]$Tag,
    [Parameter(Mandatory=$true)][ValidatePattern('^[a-f0-9]{40}$')][string]$Revision,
    [string]$FlutterSdk,
    [switch]$SkipClients
)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
if ((& git -C $taskRoot rev-parse HEAD).Trim() -ne $Revision) { throw 'Source revision mismatch' }
if (& git -C $taskRoot status --porcelain --untracked-files=no) { throw 'Commit source changes before packaging' }
$bundle=Join-Path $taskRoot ".artifacts/releases/$Tag/$Revision"
$scratch=Join-Path $taskRoot ".artifacts/release-build/$Tag/$Revision/$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Force -Path $bundle,$scratch | Out-Null
$licenses=Join-Path $scratch 'licenses'
python (Join-Path $PSScriptRoot 'collect-release-notices.py') --output $licenses
if ($LASTEXITCODE -ne 0) { throw 'Third-party notices incomplete' }
$artifacts=[Collections.Generic.List[object]]::new()
function Publish-Package([string]$Name,[string]$Directory,[string]$Platform) {
    Copy-Item -LiteralPath (Join-Path $taskRoot 'LICENSE'),(Join-Path $taskRoot 'NOTICE') -Destination $Directory
    Copy-Item -LiteralPath $licenses -Destination (Join-Path $Directory 'licenses') -Recurse -Force
    @{tag=$Tag;revision=$Revision;profile='self-host-development-preview';platform=$Platform;built_at=[DateTimeOffset]::UtcNow.ToString('o')} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $Directory 'version.json')
    $suffix=if ($Platform -like 'windows-*' -or $Platform -like '*web*') {'.zip'} else {'.tar.gz'}
    $file=Join-Path $bundle "$Name-$Tag-$Platform$suffix"
    python (Join-Path $PSScriptRoot 'archive-preview.py') $Directory $file
    if ($LASTEXITCODE -ne 0) { throw 'Archive failed' }
    $artifacts.Add(@{file=[IO.Path]::GetFileName($file);bytes=(Get-Item -LiteralPath $file).Length;sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant();platform=$Platform})
}
$savedGoOS,$savedGoArch,$savedCgo=$env:GOOS,$env:GOARCH,$env:CGO_ENABLED
try {
    foreach ($target in @('windows-amd64','linux-amd64','linux-arm64','darwin-amd64','darwin-arm64')) {
        $parts=$target.Split('-');$env:GOOS=$parts[0];$env:GOARCH=$parts[1];$env:CGO_ENABLED='0'
        $destination=Join-Path $scratch "cli-$target"
        New-Item -ItemType Directory -Force -Path $destination | Out-Null
        $executable=Join-Path $destination $(if ($env:GOOS -eq 'windows') {'pinelink.exe'} else {'pinelink'})
        Push-Location (Join-Path $taskRoot 'cli')
        try { go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/simiooo/pinelink/cli/internal/cmd.BuildVersion=$Revision" -o $executable ./cmd/pinelink; if ($LASTEXITCODE -ne 0) { throw "CLI build failed: $target" } } finally { Pop-Location }
        @"
# Pinelink CLI developer preview

Source: https://github.com/simiooo/pinelink/tree/$Revision

Run pinelink --help or pinelink lan --help. The central-service commands default to a local development backend; configure --server, owner/device and encryption scope for your own test service. Offline LAN pairing uses pinned device keys and explicit receive consent.

This preview is not the completed official consumer client. Keep the included LICENSE, NOTICE and licenses directory when redistributing.
"@ | Set-Content -LiteralPath (Join-Path $destination 'README.md')
        Publish-Package 'pinelink-cli' $destination $target
    }
    foreach ($target in @('linux-amd64','linux-arm64')) {
        $parts=$target.Split('-');$env:GOOS=$parts[0];$env:GOARCH=$parts[1];$env:CGO_ENABLED='0'
        $destination=Join-Path $scratch "server-$target"
        New-Item -ItemType Directory -Force -Path $destination | Out-Null
        Push-Location (Join-Path $taskRoot 'backend')
        try { go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/simiooo/pinelink/backend/internal/httpapi.BuildVersion=$Revision" -o (Join-Path $destination 'pinelinkd') ./cmd/pinelinkd; if ($LASTEXITCODE -ne 0) { throw "Server build failed: $target" } } finally { Pop-Location }
        Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/migrations') -Destination $destination -Recurse -Force
        @"
# Pinelink server developer preview

Source: https://github.com/simiooo/pinelink/tree/$Revision

Set PINELINK_MIGRATIONS_DIR to the included migrations directory. Configure PostgreSQL, NATS and S3 using the source repository's infra/README.md, then run ./pinelinkd. Device signatures must remain enabled outside controlled local tests.

Public multi-user identity bootstrap and official hosted-service policy are unfinished. This developer preview must not be exposed as a production service with shared development identities.
"@ | Set-Content -LiteralPath (Join-Path $destination 'README.md')
        Publish-Package 'pinelink-server' $destination $target
    }
} finally { $env:GOOS=$savedGoOS;$env:GOARCH=$savedGoArch;$env:CGO_ENABLED=$savedCgo }
if (-not $SkipClients) {
    $savedApi,$savedOwner,$savedPin=$env:VITE_PINELINK_API_URL,$env:VITE_PINELINK_OWNER,$env:VITE_PINELINK_HISTORY_KEY
    try {
        $env:VITE_PINELINK_API_URL='/api';$env:VITE_PINELINK_OWNER='dev-owner';$env:VITE_PINELINK_HISTORY_KEY=''
        Push-Location (Join-Path $taskRoot 'web')
        try { npm run build; if ($LASTEXITCODE -ne 0) { throw 'Web build failed' } } finally { Pop-Location }
    } finally { $env:VITE_PINELINK_API_URL=$savedApi;$env:VITE_PINELINK_OWNER=$savedOwner;$env:VITE_PINELINK_HISTORY_KEY=$savedPin }
    $destination=Join-Path $scratch 'web'
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    Get-ChildItem -LiteralPath (Join-Path $taskRoot 'web/dist') | Copy-Item -Destination $destination -Recurse -Force
    'Developer preview: serve this directory over HTTPS or localhost. Proxy /api/ to your own backend with the prefix removed; configure SPA fallback to index.html. Serve sw.js without long-lived caching. Set Cross-Origin-Opener-Policy: same-origin and Cross-Origin-Embedder-Policy: require-corp. This build uses development owner dev-owner; use source builds for other profiles. Official hosted identity/bootstrap, service switching and complete browser file transfers are unfinished.' | Set-Content -LiteralPath (Join-Path $destination 'README.txt')
    Publish-Package 'pinelink' $destination 'react-web'
    $sdkVersion=(Get-Content -Raw -LiteralPath (Join-Path $taskRoot '.fvmrc') | ConvertFrom-Json).flutter
    if (-not $FlutterSdk) { $FlutterSdk=Join-Path $taskRoot ".tools/flutter-$sdkVersion" }
    $flutter=Join-Path $FlutterSdk 'bin/flutter.bat';$dart=Join-Path $FlutterSdk 'bin/dart.bat'
    if (-not (Test-Path -LiteralPath $flutter)) { throw 'Pinned Flutter SDK required; pass -FlutterSdk or run setup-flutter.ps1' }
    $sdk=(& $flutter --suppress-analytics --version --machine | ConvertFrom-Json)
    if ($sdk.frameworkVersion -ne $sdkVersion) { throw 'Flutter SDK does not match .fvmrc' }
    $wasm=Join-Path $taskRoot 'mobile/web/sqlite3.wasm'
    if (-not (Test-Path -LiteralPath $wasm)) { Invoke-WebRequest 'https://github.com/simolus3/drift/releases/download/drift-2.35.3/sqlite3.wasm' -OutFile $wasm }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $wasm).Hash -ne 'EFD308BE92A34F3AA15F289A37F213B0230FD80888A1364326EE29579D892D37') { throw 'SQLite wasm checksum mismatch' }
    Push-Location (Join-Path $taskRoot 'mobile')
    try {
        & $dart compile js -O4 --no-source-maps web/drift_worker.dart -o web/drift_worker.js
        if ($LASTEXITCODE -ne 0) { throw 'Drift worker build failed' }
        & $flutter --suppress-analytics build web --release --no-web-resources-cdn --no-wasm-dry-run '--dart-define=PINELINK_API_URL=/api' '--dart-define=PINELINK_OWNER=dev-owner'
        if ($LASTEXITCODE -ne 0) { throw 'Flutter Web build failed' }
    } finally { Pop-Location }
    $destination=Join-Path $scratch 'flutter-web'
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    Get-ChildItem -LiteralPath (Join-Path $taskRoot 'mobile/build/web') | Copy-Item -Destination $destination -Recurse -Force
    'Flutter Web developer preview: serve over HTTPS or localhost with a same-origin /api/ backend reverse proxy, SPA fallback, Cross-Origin-Opener-Policy: same-origin and Cross-Origin-Embedder-Policy: require-corp. This build uses development owner dev-owner. Full Flutter file transfer, official service bootstrap and profile switching are unfinished; this package is for controlled developer testing.' | Set-Content -LiteralPath (Join-Path $destination 'README.txt')
    python (Join-Path $PSScriptRoot 'flutter-offline-shell.py') $destination
    if ($LASTEXITCODE -ne 0) { throw 'Flutter offline shell generation failed' }
    Publish-Package 'pinelink' $destination 'flutter-web'
}
$manifest=[ordered]@{tag=$Tag;revision=$Revision;repository='https://github.com/simiooo/pinelink';profile='self-host-development-preview';created_at=[DateTimeOffset]::UtcNow.ToString('o');go=(& go version);artifacts=@($artifacts);limits=@('Official hosted identity/bootstrap and service switching are unfinished.','Complete Web/Flutter file transfer and offline intent queue are unfinished.','Desktop and mobile node-graph migration is incomplete.','Web archives require a same-origin /api reverse proxy, trusted HTTPS or localhost, SPA fallback and the documented isolation headers.')}
$manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $bundle 'manifest.json')
$artifacts | Sort-Object file | ForEach-Object { "$($_.sha256)  $($_.file)" } | Set-Content -LiteralPath (Join-Path $bundle 'SHA256SUMS')
Write-Output $bundle
