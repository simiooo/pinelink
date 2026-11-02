param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[a-f0-9]{7,40}$')][string]$Version,
    [string]$Context = 'default',
    [string]$PeerHost = '127.0.0.1'
)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$headVersion = (& git -C $taskRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or -not $headVersion.StartsWith($Version)) { throw 'Version must identify the checked-out source commit' }
if (& git -C $taskRoot status --porcelain --untracked-files=no) { throw 'Commit source changes before deploying' }
$Version = $headVersion
$bundle = Join-Path $taskRoot ".stage/local-peer/$Version"
New-Item -ItemType Directory -Force -Path $bundle | Out-Null
$oldGoOS, $oldGoArch, $oldCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
    $env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    Push-Location (Join-Path $taskRoot 'cli')
    try {
        go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/simiooo/pinelink/cli/internal/cmd.BuildVersion=$Version" -o (Join-Path $bundle 'pinelink') ./cmd/pinelink
        if ($LASTEXITCODE -ne 0) { throw 'Peer build failed' }
    } finally { Pop-Location }
    $env:GOOS='windows'
    Push-Location (Join-Path $taskRoot 'cli')
    try {
        go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/simiooo/pinelink/cli/internal/cmd.BuildVersion=$Version" -o (Join-Path $bundle 'pinelink.exe') ./cmd/pinelink
        if ($LASTEXITCODE -ne 0) { throw 'Windows peer build failed' }
    } finally { Pop-Location }
} finally { $env:GOOS=$oldGoOS; $env:GOARCH=$oldGoArch; $env:CGO_ENABLED=$oldCgo }
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/Dockerfile.peer') -Destination $bundle -Force
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/compose.peer.yaml') -Destination $bundle -Force
$oldVersion, $oldHost = $env:PINELINK_VERSION, $env:PINELINK_PEER_HOST
try {
    $env:PINELINK_VERSION=$Version; $env:PINELINK_PEER_HOST=$PeerHost
    docker --context $Context compose -f (Join-Path $bundle 'compose.peer.yaml') up -d --build --wait --wait-timeout 120
    if ($LASTEXITCODE -ne 0) { throw 'Local peer deployment failed' }
    $container = (& docker --context $Context compose -f (Join-Path $bundle 'compose.peer.yaml') ps -q peer).Trim()
    $actual = (& docker --context $Context exec $container pinelink --version).Trim()
    if ($actual -ne "pinelink version $Version") { throw 'Wrong peer version' }
    $ready = $null
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        $readyText = & docker --context $Context exec $container cat /state/ready.json 2>$null
        if ($LASTEXITCODE -eq 0) { $ready=$readyText | ConvertFrom-Json; break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $ready -or $ready.peer.address -ne "https://${PeerHost}:18691") { throw 'Peer did not advertise the expected endpoint' }
    $evidence = [ordered]@{version=$Version; deployed_at=[DateTimeOffset]::UtcNow.ToString('o'); endpoint=$ready.peer.address; peer_id=$ready.peer.id; container=$container; binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $bundle 'pinelink')).Hash; windows_binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $bundle 'pinelink.exe')).Hash; auth='pinned-device-key; manual pairing and receive consent'}
    $evidence | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $bundle 'deployment.json')
    $evidence | ConvertTo-Json -Depth 5
} finally { $env:PINELINK_VERSION=$oldVersion; $env:PINELINK_PEER_HOST=$oldHost }
