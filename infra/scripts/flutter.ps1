param([Parameter(ValueFromRemainingArguments=$true)][string[]]$FlutterArguments)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$sdkVersion=(Get-Content -LiteralPath (Join-Path $taskRoot '.fvmrc') -Raw | ConvertFrom-Json).flutter
if ($sdkVersion -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid pinned Flutter SDK version' }
$sdkRoot=Join-Path $taskRoot ".tools/flutter-$sdkVersion"
$sdkCommand=Join-Path $sdkRoot 'bin/flutter.bat'
if (-not (Test-Path -LiteralPath $sdkCommand)) { throw 'Run infra/scripts/setup-flutter.ps1 first' }
& $sdkCommand --suppress-analytics @FlutterArguments
exit $LASTEXITCODE
