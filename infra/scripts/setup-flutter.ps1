$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sdkVersion=(Get-Content -LiteralPath (Join-Path $taskRoot '.fvmrc') -Raw | ConvertFrom-Json).flutter
if ($sdkVersion -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid pinned Flutter SDK version' }
$releases=Invoke-RestMethod 'https://storage.googleapis.com/flutter_infra_release/releases/releases_windows.json'
$release=$releases.releases | Where-Object { $_.version -eq $sdkVersion -and $_.channel -eq 'stable' } | Select-Object -First 1
if (-not $release -or $release.hash -notmatch '^[a-f0-9]{40}$') { throw 'Pinned SDK not found in official stable releases' }
$sdkRoot=Join-Path $taskRoot ".tools/flutter-$sdkVersion"
if (-not (Test-Path -LiteralPath $sdkRoot)) {
    New-Item -ItemType Directory -Force -Path (Join-Path $taskRoot '.tools') | Out-Null
    git -c core.longpaths=true clone --depth 1 --branch $sdkVersion https://github.com/flutter/flutter.git $sdkRoot
    if ($LASTEXITCODE -ne 0) { throw 'SDK checkout failed' }
}
$actual=(& git -C $sdkRoot rev-parse HEAD).Trim()
if ($actual -ne $release.hash) { throw 'SDK does not match official release commit' }
& (Join-Path $sdkRoot 'bin/flutter.bat') --suppress-analytics --version
if ($LASTEXITCODE -ne 0) { throw 'SDK bootstrap failed' }
