# Opt-in for this PowerShell session only:
# . 'C:\path\to\OrbWin\completions\OrbWin.ps1'
# This does not edit a profile, PATH, Docker config, or any file.
Register-ArgumentCompleter -Native -CommandName orbwin-cli, orbwin-cli.exe -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    # Use the cursor's AST position, not all tokens on the line. Completion can
    # happen before existing arguments; never evaluate expressions to find words.
    $elements = @($commandAst.CommandElements)
    $argumentIndex = 0
    foreach ($element in $elements) {
        if ($element.Extent.EndOffset -lt $cursorPosition -or
            ([string]::IsNullOrEmpty($wordToComplete) -and $element.Extent.EndOffset -le $cursorPosition)) {
            $argumentIndex++
        } else { break }
    }
    if ($argumentIndex -lt 1) { return }
    $words = @()
    foreach ($element in $elements | Select-Object -First $argumentIndex) {
        if ($element -isnot [System.Management.Automation.Language.StringConstantExpressionAst]) { return }
        $words += $element.Value
    }
    $choices = @()
    if ($argumentIndex -eq 1) {
        $choices = @('status','start','stop','logs','snapshot','docker','config','clients','top','feedback','update','help')
    } elseif ($words[1] -eq 'clients') {
        if ($argumentIndex -eq 2) { $choices = @('recover') }
        elseif ($argumentIndex -eq 3 -and $words[2] -eq 'recover') { $choices = @('--apply-reviewed','--restore-original') }
    } elseif ($words[1] -eq 'config') {
        if ($argumentIndex -eq 2) { $choices = @('get','set') }
        elseif ($argumentIndex -eq 3 -and $words[2] -eq 'get') { $choices = @('running','kubernetes.enabled') }
        elseif ($argumentIndex -eq 3 -and $words[2] -eq 'set') { $choices = @('running=true','running=false','kubernetes.enabled=true','kubernetes.enabled=false') }
    } elseif ($words[1] -eq 'docker' -and $argumentIndex -eq 2) {
        $choices = @('ps','images','volume','network','compose','logs','exec','run','build','pull','push','stats','version','info')
    }
    $prefix = $wordToComplete
    if ($prefix.Length -gt 0 -and ($prefix[0] -eq "'" -or $prefix[0] -eq '"')) {
        $quote = $prefix[0]
        $prefix = $prefix.Substring(1)
        if ($prefix.Length -gt 0 -and $prefix[$prefix.Length - 1] -eq $quote) { $prefix = $prefix.Substring(0, $prefix.Length - 1) }
    }
    $choices | Where-Object { $_.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
