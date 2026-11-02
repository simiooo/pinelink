param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[a-f0-9]{7,40}$')][string]$Version,
    [string]$Context = 'default',
    [string]$ApiUrl = 'http://127.0.0.1:18680'
)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$headVersion = (& git -C $taskRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or -not $headVersion.StartsWith($Version)) { throw 'Version must identify the checked-out source commit' }
if (& git -C $taskRoot status --porcelain --untracked-files=no) { throw 'Commit source changes before deploying' }
$Version = $headVersion
$stageRoot = Join-Path $taskRoot '.stage'
$bundle = Join-Path $stageRoot $Version
New-Item -ItemType Directory -Force -Path $bundle | Out-Null
$envFile = Join-Path $stageRoot 'staging.env'
if (-not (Test-Path -LiteralPath $envFile)) {
    $dbPassword = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
    $s3Secret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
    [IO.File]::WriteAllText($envFile, "PINELINK_STAGING_DB_PASSWORD=$dbPassword`nPINELINK_STAGING_S3_SECRET=$s3Secret`n")
}
if (-not (Select-String -LiteralPath $envFile -Pattern '^PINELINK_HISTORY_SIGNING_SEED=' -Quiet)) {
    $historySeed = [Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
    [IO.File]::AppendAllText($envFile,"`nPINELINK_HISTORY_SIGNING_SEED=$historySeed`n")
}
$oldGoOS, $oldGoArch, $oldCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Push-Location (Join-Path $taskRoot 'backend')
    try {
        go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/simiooo/pinelink/backend/internal/httpapi.BuildVersion=$Version" -o (Join-Path $bundle 'pinelinkd') ./cmd/pinelinkd
        if ($LASTEXITCODE -ne 0) { throw 'Backend build failed' }
    } finally { Pop-Location }
} finally { $env:GOOS=$oldGoOS; $env:GOARCH=$oldGoArch; $env:CGO_ENABLED=$oldCgo }
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/Dockerfile') -Destination $bundle -Force
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/compose.yaml') -Destination $bundle -Force
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/migrations') -Destination $bundle -Recurse -Force
$oldVersion = $env:PINELINK_VERSION
try {
    $env:PINELINK_VERSION = $Version
    docker --context $Context compose --env-file $envFile -f (Join-Path $bundle 'compose.yaml') up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'Staging deployment failed' }
    $health = Invoke-RestMethod "$ApiUrl/healthz"
    if ($health.status -ne 'ok' -or $health.version -ne $Version) { throw 'Wrong deployed version' }
    $evidence = [ordered]@{version=$Version;deployed_at=[DateTimeOffset]::UtcNow.ToString('o');url=$ApiUrl;health=$health;binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $bundle 'pinelinkd')).Hash}
    $evidence | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $bundle 'deployment.json')
    $evidence | ConvertTo-Json -Depth 5
} finally { $env:PINELINK_VERSION=$oldVersion }
