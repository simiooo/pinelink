param(
    [Parameter(Mandatory=$true)][string]$DatabaseUrl,
    [string]$Context='default',
    [Parameter(Mandatory=$true)][ValidatePattern('^/home/[A-Za-z0-9_-]+/\.cache/pinelink-concurrency$')][string]$GoCacheHostPath
)
$ErrorActionPreference='Stop'
# Keep compiler/module scratch data off Docker's shared data partition.
$endpoint=(& docker context inspect $Context --format '{{.Endpoints.docker.Host}}').Trim()
if ($endpoint -notmatch '^ssh://([A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+)$') { throw 'This check expects an SSH Docker context' }
$sshTarget=$Matches[1]
& ssh $sshTarget "mkdir -p '$GoCacheHostPath/mod' '$GoCacheHostPath/build' '$GoCacheHostPath/tmp'"
if ($LASTEXITCODE -ne 0) { throw 'Could not prepare isolated Go scratch cache' }
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$bundle=Join-Path $taskRoot '.artifacts/strict-race'
New-Item -ItemType Directory -Force -Path $bundle | Out-Null
Push-Location $taskRoot
try {
    $files=@(& git ls-files --cached --others --exclude-standard -- backend cli protocol infra/migrations)
    $files | Set-Content -Encoding utf8NoBOM -LiteralPath (Join-Path $bundle 'files.txt')
    & tar -cf (Join-Path $bundle 'source.tar') -T (Join-Path $bundle 'files.txt')
    if ($LASTEXITCODE -ne 0) { throw 'Source snapshot failed' }
} finally { Pop-Location }
@'
FROM golang:1.26
WORKDIR /source
COPY source.tar /source.tar
RUN tar -xf /source.tar -C /source
COPY run.sh /run.sh
ENTRYPOINT ["bash", "/run.sh"]
'@ | Set-Content -Encoding utf8NoBOM -LiteralPath (Join-Path $bundle 'Dockerfile')
@'
set -euo pipefail
cd /source/backend
go test -race -count=1 ./internal/service ./internal/httpapi
cd /source/cli
go test -race -count=1 ./internal/lan ./internal/p2p
cd /source/protocol
go test -race -count=1 ./...
'@.Replace("`r`n","`n") | Set-Content -Encoding utf8NoBOM -NoNewline -LiteralPath (Join-Path $bundle 'run.sh')
& docker --context $Context build -t pinelink-concurrency-check:local $bundle
if ($LASTEXITCODE -ne 0) { throw 'Concurrency test image build failed' }
$oldDatabaseUrl=$env:PINELINK_SERVICE_TEST_DATABASE_URL
try {
    $env:PINELINK_SERVICE_TEST_DATABASE_URL=$DatabaseUrl
    & docker --context $Context run --rm --network host --label pinelink.scope=concurrency-check -e PINELINK_SERVICE_TEST_DATABASE_URL `
        -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOTMPDIR=/cache/tmp `
        --mount "type=bind,source=$GoCacheHostPath,target=/cache" pinelink-concurrency-check:local
    if ($LASTEXITCODE -ne 0) { throw 'Concurrency tests failed' }
} finally { $env:PINELINK_SERVICE_TEST_DATABASE_URL=$oldDatabaseUrl }
