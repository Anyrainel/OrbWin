$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/../completions/OrbWin.ps1"
$cases = @(
    @{ Input = 'orbwin-cli st'; Expected = 'status' },
    @{ Input = 'orbwin-cli config '; Expected = 'get' },
    @{ Input = 'orbwin-cli config get '; Expected = 'running' },
    @{ Input = 'orbwin-cli config set kub'; Expected = 'kubernetes.enabled=true' },
    @{ Input = 'orbwin-cli docker '; Expected = 'compose' },
    @{ Input = 'orbwin-cli clients '; Expected = 'recover' },
    @{ Input = 'orbwin-cli clients recover --rest'; Expected = '--restore-original' },
    @{ Input = 'orbwin-cli clients recover --apply'; Expected = '--apply-reviewed' },
    @{ Input = 'orbwin-cli config set kub trailing'; Cursor = 25; Expected = 'kubernetes.enabled=true' },
    @{ Input = 'orbwin-cli config get "kubernetes.enabled"'; Expected = 'kubernetes.enabled' },
    @{ Input = "orbwin-cli 'config' 'get' run"; Expected = 'running' },
    @{ Input = 'orbwin-cli.exe CONFIG s'; Expected = 'set' },
    @{ Input = "& '$([System.IO.Path]::GetFullPath("$PSScriptRoot/../artifacts/cli/orbwin-cli.exe"))' config s"; Expected = 'set' }
)
foreach ($case in $cases) {
    $cursor = if ($case.ContainsKey('Cursor')) { $case.Cursor } else { $case.Input.Length }
    $result = TabExpansion2 $case.Input $cursor
    if ($case.Expected -notin $result.CompletionMatches.CompletionText) {
        throw "Completion missing $($case.Expected) for '$($case.Input)'"
    }
}
foreach ($inputText in @('orbwin-cli config get running ', 'orbwin-cli config set running=true ', 'orbwin-cli docker ps ', 'orbwin-cli status ', 'orbwin-cli $untrusted ')) {
    $result = TabExpansion2 $inputText $inputText.Length
    if (@($result.CompletionMatches | Where-Object { $_.CompletionText -in @('running','get','status','compose','running=true') }).Count) {
        throw "Unexpected static suggestion after complete/unsupported argument: '$inputText'"
    }
}
$proofDirectory = Join-Path $PSScriptRoot "../artifacts/cli/completion-$([guid]::NewGuid().ToString())"
New-Item -ItemType Directory -Path $proofDirectory | Out-Null
[ordered]@{
    schema = 1
    success = $true
    powershellVersion = $PSVersionTable.PSVersion.ToString()
    completionSha256 = (Get-FileHash "$PSScriptRoot/../completions/OrbWin.ps1" -Algorithm SHA256).Hash.ToLowerInvariant()
    positiveCases = $cases.Count
    negativeCases = 5
    scope = 'Real TabExpansion2; cursor-in-middle, quoted arguments, absolute executable path, finished commands and unevaluated expression refusal. No engine command or profile/configuration writes.'
} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $proofDirectory 'proof.json') -Encoding utf8
Write-Output "$($cases.Count) positive and five negative completion cases passed; no profile or persistent configuration was edited."
Write-Output ([System.IO.Path]::GetFullPath((Join-Path $proofDirectory 'proof.json')))
