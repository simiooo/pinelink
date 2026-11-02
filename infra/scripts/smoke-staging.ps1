param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[a-f0-9]{40}$')][string]$Version,
    [string]$SshHost='localhost',
    [string]$ApiUrl='http://127.0.0.1:18680'
)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
& git -C $taskRoot diff --quiet $Version -- cli infra/integration
if ($LASTEXITCODE -ne 0) { throw 'Smoke CLI/tests must match the target source version' }
$evidenceRoot=Join-Path $taskRoot ".artifacts/sim847/$Version"
New-Item -ItemType Directory -Force -Path $evidenceRoot | Out-Null
$health=Invoke-RestMethod "$ApiUrl/healthz"
if ($health.version -ne $Version) { throw 'Staging source version does not match smoke target' }
$credentials=@{}
Get-Content -LiteralPath (Join-Path $taskRoot '.stage/staging.env') | ForEach-Object {
    $parts=$_.Split('=',2)
    if ($parts.Length -eq 2) { $credentials[$parts[0]]=$parts[1] }
}
if ($credentials.PINELINK_STAGING_DB_PASSWORD -notmatch '^[a-f0-9]+$') { throw 'Invalid local staging DB configuration' }
$portProbe=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
$portProbe.Start()
$forwardPort=$portProbe.LocalEndpoint.Port
$portProbe.Stop()
$taskTunnel=Start-Process -FilePath 'ssh.exe' -ArgumentList @('-N','-o','BatchMode=yes','-o','ExitOnForwardFailure=yes','-L',"127.0.0.1:${forwardPort}:127.0.0.1:55435",$SshHost) -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $evidenceRoot 'tunnel.log')
$envNames=@('PINELINK_TEST_SERVER','PINELINK_TEST_AUTH_SERVER','PINELINK_TEST_CLI','PINELINK_DATABASE_URL')
$oldValues=@{}
foreach ($name in $envNames) { $oldValues[$name]=[Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $deadline=[DateTimeOffset]::UtcNow.AddSeconds(10)
    do {
        if ($taskTunnel.HasExited) { throw 'Staging DB tunnel could not start' }
        $probe=[Net.Sockets.TcpClient]::new()
        try { $probe.Connect('127.0.0.1',$forwardPort); $ready=$true } catch { $ready=$false } finally { $probe.Dispose() }
        if (-not $ready) { Start-Sleep -Milliseconds 100 }
    } until ($ready -or [DateTimeOffset]::UtcNow -ge $deadline)
    if (-not $ready) { throw 'Staging DB tunnel did not become ready' }
    $cli=Join-Path $evidenceRoot 'pinelink.exe'
    Push-Location (Join-Path $taskRoot 'cli')
    try { go build -trimpath -o $cli ./cmd/pinelink; if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' } } finally { Pop-Location }
    $env:PINELINK_TEST_SERVER=$ApiUrl
    $env:PINELINK_TEST_AUTH_SERVER=$ApiUrl
    $env:PINELINK_TEST_CLI=$cli
    $env:PINELINK_DATABASE_URL="postgres://pinelink:$($credentials.PINELINK_STAGING_DB_PASSWORD)@127.0.0.1:${forwardPort}/pinelink?sslmode=disable"
    Push-Location (Join-Path $taskRoot 'infra')
    try {
        go test -v ./integration -run '^(TestP2PDirectRelayAndFallback|TestRelayPreuploadConsentAndRestore)$' -count=1 2>&1 | Tee-Object -FilePath (Join-Path $evidenceRoot 'signed-p2p-smoke.log')
        if ($LASTEXITCODE -ne 0) { throw 'Signed staging P2P smoke failed' }
    } finally { Pop-Location }
    $unsigned=Invoke-WebRequest "$ApiUrl/v1/p2p/$([guid]::NewGuid())/progress?owner_id=outsider&device_id=outsider" -SkipHttpErrorCheck
    if ($unsigned.StatusCode -ne 401) { throw 'Unsigned staging request was not rejected' }
    $evidence=[ordered]@{version=$Version;cli_source_revision=(& git -C $taskRoot rev-parse HEAD).Trim();tested_at=[DateTimeOffset]::UtcNow.ToString('o');api=$ApiUrl;health=$health;signed_device_paths=@('direct','relay','auto-fallback','mid-transfer-fallback','preupload-before-consent','sender-offline-receive','explicit-restore','empty-file');verification='ciphertext/plaintext integrity, exact file equality, route projection, independent cloud failure';unsigned_request_status=$unsigned.StatusCode;cli_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $cli).Hash}
    $evidence | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidenceRoot 'smoke.json')
    $evidence | ConvertTo-Json -Depth 5
} finally {
    foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name,$oldValues[$name],'Process') }
    if (-not $taskTunnel.HasExited) { $taskTunnel.Kill();$taskTunnel.WaitForExit() }
}
