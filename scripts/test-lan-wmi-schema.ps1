# Read-only LAN provider metadata qualification. Never enumerates rule instances
# or invokes provider methods. This does not prove create/atomicity support.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$taskClass = Get-CimClass -Namespace root/StandardCimv2 -ClassName MSFT_NetFirewallRule -OperationTimeoutSec 5 -ErrorAction Stop
if ($taskClass.CimClassProperties.Count -gt 128 -or $taskClass.CimClassMethods.Count -gt 32) { throw 'Provider schema exceeds metadata budget' }
$taskSystem = [Environment]::GetFolderPath([Environment+SpecialFolder]::System)
$taskCDXMLPath = Join-Path $taskSystem 'WindowsPowerShell/v1.0/Modules/NetSecurity/NetFirewallRule.cmdletDefinition.cdxml'
$taskItem = Get-Item -LiteralPath $taskCDXMLPath
if ($taskItem.Length -gt 262144) { throw 'Installed Microsoft CDXML exceeds metadata budget' }
$taskCDXML = [xml](Get-Content -LiteralPath $taskCDXMLPath -Raw)
$taskNS = New-Object System.Xml.XmlNamespaceManager($taskCDXML.NameTable)
$taskNS.AddNamespace('c', 'http://schemas.microsoft.com/cmdlets-over-objects/2009/11')
$taskNew = $taskCDXML.SelectSingleNode('//c:StaticCmdlets/c:Cmdlet[c:CmdletMetadata/@Verb="New"]/c:Method', $taskNS)
if ($null -eq $taskNew -or $taskNew.MethodName -ne 'cim:CreateInstance') { throw 'Installed Microsoft provider creation contract changed' }
$taskParameters = @($taskNew.Parameters.Parameter)
if ($taskParameters.Count -gt 64) { throw 'Provider creation parameters exceed budget' }
$taskResult = [ordered]@{
    version = 1
    readAt = [DateTimeOffset]::UtcNow.ToString('o')
    source = 'local-cim-class-and-installed-microsoft-cdxml'
    className = $taskClass.CimClassName
    provider = [string]$taskClass.CimClassQualifiers['provider'].Value
    properties = @($taskClass.CimClassProperties | ForEach-Object { [ordered]@{name=$_.Name;type=[string]$_.CimType;flags=[string]$_.Flags} })
    methods = @($taskClass.CimClassMethods | ForEach-Object { [ordered]@{name=$_.Name;returnType=[string]$_.ReturnType} })
    installedCreateMethod = $taskNew.MethodName
    installedCreateParameters = @($taskParameters | ForEach-Object { [ordered]@{name=$_.ParameterName;type=$_.Type.PSType} })
    installedCDXMLSHA256 = (Get-FileHash -LiteralPath $taskCDXMLPath -Algorithm SHA256).Hash.ToLowerInvariant()
    activationAvailable = $false
    atomicCreateQualified = $false
}
$taskJSON = ConvertTo-Json -InputObject $taskResult -Depth 8 -Compress
if ([Text.Encoding]::UTF8.GetByteCount($taskJSON) -gt 65536) { throw 'Provider schema output exceeds 64KiB' }
Write-Output $taskJSON
