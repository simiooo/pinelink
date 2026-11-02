param([Parameter(Mandatory=$true)][ValidatePattern('^[a-f0-9]{40}$')][string]$Version,[string]$Context='default',[string]$HostAddress='127.0.0.1',[string]$ApiUrl="http://${HostAddress}:18680")
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
if ((& git -C $taskRoot rev-parse HEAD).Trim() -ne $Version) { throw 'Checked out source must match deployment version' }
if (& git -C $taskRoot status --porcelain --untracked-files=no) { throw 'Commit changes before deploying' }
& (Join-Path $PSScriptRoot 'build-clients.ps1') -Version $Version -ApiUrl $ApiUrl
if ($LASTEXITCODE -ne 0) { throw 'Client builds failed' }
$bundle=Join-Path $taskRoot ".stage/clients/$Version"
$tls=Join-Path $taskRoot '.stage/client-tls'
New-Item -ItemType Directory -Force -Path $tls | Out-Null
$key=Join-Path $tls 'server.key'; $certificate=Join-Path $tls 'server.crt'
if (-not (Test-Path -LiteralPath $certificate)) {
    $openssl='C:/Program Files/Git/usr/bin/openssl.exe'
    & $openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout $key -out $certificate -days 30 -subj '/CN=pinelink-staging' -addext "subjectAltName=IP:${HostAddress},IP:127.0.0.1,DNS:localhost"
    if ($LASTEXITCODE -ne 0) { throw 'Staging certificate generation failed' }
}
New-Item -ItemType Directory -Force -Path (Join-Path $bundle 'tls') | Out-Null
Copy-Item -LiteralPath $key,$certificate -Destination (Join-Path $bundle 'tls') -Force
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/Dockerfile.clients') -Destination (Join-Path $bundle 'Dockerfile') -Force
Copy-Item -LiteralPath (Join-Path $taskRoot 'infra/staging/clients.conf') -Destination $bundle -Force
docker --context $Context build --build-arg "VERSION=$Version" -t "pinelink-staging-clients:$Version" $bundle
if ($LASTEXITCODE -ne 0) { throw 'Client image build failed' }
$existing=& docker --context $Context ps -aq --filter 'name=^pinelink-staging-clients$'
if ($existing) {
    $owned=(& docker --context $Context inspect --format '{{index .Config.Labels "pinelink.scope"}}' pinelink-staging-clients).Trim()
    if ($owned -ne 'staging-clients') { throw 'Container is not owned by this deployment' }
    docker --context $Context rm -f pinelink-staging-clients
    if ($LASTEXITCODE -ne 0) { throw 'Previous client container stop failed' }
}
docker --context $Context run -d --name pinelink-staging-clients --label pinelink.scope=staging-clients --restart unless-stopped --network host "pinelink-staging-clients:$Version"
if ($LASTEXITCODE -ne 0) { throw 'Client start failed' }
$urls=@("https://${HostAddress}:18682","https://${HostAddress}:18683")
foreach ($url in $urls) {
    $ready=$false
    for ($attempt=0;$attempt -lt 30;$attempt++) {
        try { $actual=Invoke-RestMethod "$url/version.json" -SkipCertificateCheck; if ($actual.version -eq $Version) { $ready=$true;break } } catch {}
        Start-Sleep -Milliseconds 300
    }
    if (-not $ready) { throw 'Wrong deployed client version' }
    $health=Invoke-RestMethod "$url/api/healthz" -SkipCertificateCheck
    if ($health.status -ne 'ok') { throw 'Client API proxy unavailable' }
}
$evidence=@{version=$Version;deployed_at=[DateTimeOffset]::UtcNow.ToString('o');urls=$urls;flutter='3.47.9';certificate_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $certificate).Hash;tls='self-signed staging certificate; no system trust-store changes'}
$evidence | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $bundle 'deployment.json')
$evidence | ConvertTo-Json -Depth 4
