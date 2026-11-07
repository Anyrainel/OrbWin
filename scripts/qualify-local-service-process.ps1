param([string]$DaemonId = 'd9c00194-ea9d-439a-8561-6a0ec580fbd5')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $taskRoot 'artifacts/backend/orbwin-session.exe'
$asset = Join-Path $taskRoot 'artifacts/backend/orbwin-services'
$manifest = Get-Content (Join-Path $taskRoot 'artifacts/backend/services-manifest.json') -Raw | ConvertFrom-Json
$proofHome = Join-Path $taskRoot ('artifacts/qualification/service-process-' + [DateTime]::UtcNow.Ticks)
[void][IO.Directory]::CreateDirectory($proofHome)
$installation = [Guid]::NewGuid().ToString()
$server = $null
function New-TaskProcess([string[]]$Arguments) {
    $info = [Diagnostics.ProcessStartInfo]::new($binary)
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardInput = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $info.ArgumentList.Add($argument) }
    return [Diagnostics.Process]::Start($info)
}
function Invoke-Service([hashtable]$Request, [switch]$AllowAbsent) {
    $client = New-TaskProcess @('local-services-call', $proofHome, $installation, '-')
    $output = $client.StandardOutput.ReadToEndAsync()
    $errors = $client.StandardError.ReadToEndAsync()
    $client.StandardInput.Write(($Request | ConvertTo-Json -Depth 12 -Compress))
    $client.StandardInput.Close()
    if (!$client.WaitForExit(45000)) { $client.Kill(); throw 'Owned diagnostic IPC client timed out.' }
    $stdout = $output.GetAwaiter().GetResult()
    $stderr = $errors.GetAwaiter().GetResult()
    if ($AllowAbsent -and $client.ExitCode -eq 126) { return $null }
    if ($client.ExitCode -ne 0) { throw "IPC client exit $($client.ExitCode): $stderr" }
    $response = $stdout | ConvertFrom-Json
    if ($response.error) { throw $response.error }
    return $response
}
function Start-ServiceProof {
    $script:server = New-TaskProcess @('local-services-serve', $proofHome, $installation, $DaemonId, $asset, $manifest.binarySha256)
    $script:serverErrors = $script:server.StandardError.ReadToEndAsync()
    $script:serverOutput = $script:server.StandardOutput.ReadToEndAsync()
    $script:server.StandardInput.Close()
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        if ($script:server.HasExited) { throw ('Service exited: ' + $script:serverErrors.GetAwaiter().GetResult()) }
        $response = Invoke-Service @{version=1; operation='snapshot'} -AllowAbsent
        if ($null -ne $response) { return $response }
        Start-Sleep -Milliseconds 100
    }
    throw 'Service did not expose its private pipe.'
}
function Stop-ServiceProof {
    [void](Invoke-Service @{version=1; operation='stop-process'})
    if (!$script:server.WaitForExit(45000)) { throw 'Service cleanup did not finish.' }
    if ($script:server.ExitCode -ne 0) { throw ('Service cleanup: ' + $script:serverErrors.GetAwaiter().GetResult()) }
}
try {
    $first = Start-ServiceProof
    $expectedBinary = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($first.state.binarySha256 -ne $expectedBinary -or $first.state.toolsSha256 -ne $manifest.binarySha256 -or $first.state.daemonId -ne $DaemonId -or $first.state.installation -ne $installation) { throw 'Running component identity differs.' }
    $preferences = @(1..360 | ForEach-Object { @{selector=@{containerId=('{0:x64}' -f $_)}; disabled=$true} })
    $settings = @{enabled=$false; ports=@{http=18080; https=18443}; preferences=$preferences}
    $request = @{version=1; operation='configure'; revision=0; settings=$settings}
    $requestBytes = [Text.Encoding]::UTF8.GetByteCount(($request | ConvertTo-Json -Depth 12 -Compress))
    if ($requestBytes -le 32767) { throw 'Fixture does not exercise input beyond the Windows command-line budget.' }
    $configured = Invoke-Service $request
    if ($configured.state.revision -ne 1 -or @($configured.state.settings.preferences).Count -ne 360) { throw 'Complete reviewed settings were not persisted.' }
    Stop-ServiceProof
    $second = Start-ServiceProof
    if ($second.state.revision -ne 1 -or $second.state.settings.ports.http -ne 18080 -or @($second.state.settings.preferences).Count -ne 360 -or $second.state.authority) { throw 'Restart did not preserve disabled intent exactly.' }
    Stop-ServiceProof
    $authorityExists = Test-Path -LiteralPath (Join-Path $proofHome '.rdorbwin/local-services/authority-v1.json')
    if ($authorityExists) { throw 'Disabled setup generated an authority.' }
    $proof = @{passed=$true; recordedAt=[DateTime]::UtcNow.ToString('o'); home=$proofHome; installation=$installation; daemonId=$DaemonId; binarySha256=$expectedBinary; toolsSha256=$manifest.binarySha256; processRestarted=$true; rendererRequired=$false; persistedRevision=1; persistedPreferences=360; stdinRequestBytes=$requestBytes; authorityGenerated=$false; trustStoreChanged=$false}
    $proof | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $proofHome 'proof.json') -Encoding utf8
    $proof | ConvertTo-Json -Depth 8
} finally {
    if ($null -ne $server -and !$server.HasExited) {
        try { Stop-ServiceProof } catch { Write-Warning "Owned proof service PID $($server.Id) still requires cleanup: $_" }
    }
}
