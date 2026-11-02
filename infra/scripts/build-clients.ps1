param([string]$Version='development',[string]$ApiUrl='http://127.0.0.1:18680')
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sdkVersion=(Get-Content -LiteralPath (Join-Path $taskRoot '.fvmrc') -Raw | ConvertFrom-Json).flutter
if ($Version -ne 'development' -and $Version -notmatch '^[a-f0-9]{40}$') { throw 'Invalid source version' }
$bundle=Join-Path $taskRoot ".stage/clients/$Version"
New-Item -ItemType Directory -Force -Path $bundle | Out-Null
$info=Invoke-RestMethod "$ApiUrl/v1/replication/info?nonce=$('a'*43)"
if (-not $info.public_key) { throw 'History public key unavailable' }
$wasm=Join-Path $taskRoot 'mobile/web/sqlite3.wasm'
$wasmHash='EFD308BE92A34F3AA15F289A37F213B0230FD80888A1364326EE29579D892D37'
if (-not (Test-Path -LiteralPath $wasm)) { Invoke-WebRequest 'https://github.com/simolus3/drift/releases/download/drift-2.35.3/sqlite3.wasm' -OutFile $wasm }
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $wasm).Hash -ne $wasmHash) { throw 'SQLite wasm checksum mismatch' }
$oldApi,$oldOwner,$oldPin=$env:VITE_PINELINK_API_URL,$env:VITE_PINELINK_OWNER,$env:VITE_PINELINK_HISTORY_KEY
try {
    $env:VITE_PINELINK_API_URL='/api'; $env:VITE_PINELINK_OWNER='staging-persistence'; $env:VITE_PINELINK_HISTORY_KEY=$info.public_key
    Push-Location (Join-Path $taskRoot 'web')
    try { npm run build; if ($LASTEXITCODE -ne 0) { throw 'Web build failed' } } finally { Pop-Location }
} finally { $env:VITE_PINELINK_API_URL=$oldApi; $env:VITE_PINELINK_OWNER=$oldOwner; $env:VITE_PINELINK_HISTORY_KEY=$oldPin }
Push-Location (Join-Path $taskRoot 'mobile')
try {
    & (Join-Path $taskRoot ".tools/flutter-$sdkVersion/bin/dart.bat") compile js -O4 --no-source-maps web/drift_worker.dart -o web/drift_worker.js
    if ($LASTEXITCODE -ne 0) { throw 'Drift worker build failed' }
    & (Join-Path $taskRoot 'infra/scripts/flutter.ps1') build web --release --no-web-resources-cdn --no-wasm-dry-run '--dart-define=PINELINK_API_URL=/api' '--dart-define=PINELINK_OWNER=staging-persistence' "--dart-define=PINELINK_HISTORY_KEY=$($info.public_key)"
    if ($LASTEXITCODE -ne 0) { throw 'Flutter build failed' }
} finally { Pop-Location }
foreach ($client in @(@{source='web/dist';name='web'},@{source='mobile/build/web';name='flutter'})) {
    $destination=Join-Path $bundle $client.name
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    Get-ChildItem -LiteralPath (Join-Path $taskRoot $client.source) | Copy-Item -Destination $destination -Recurse -Force
    @{version=$Version;client=$client.name;built_at=[DateTimeOffset]::UtcNow.ToString('o');flutter=$sdkVersion;history_public_key=$info.public_key} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $destination 'version.json')
}
python (Join-Path $taskRoot 'infra/scripts/flutter-offline-shell.py') (Join-Path $bundle 'flutter')
if ($LASTEXITCODE -ne 0) { throw 'Flutter offline shell generation failed' }
Write-Output $bundle
