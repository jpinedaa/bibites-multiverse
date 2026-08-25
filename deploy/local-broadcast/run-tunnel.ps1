#requires -Version 5
<#
  Bibites Multiverse - native Windows RTMP tunnel.

  Replaces the former WSL systemd tunnel (which depended on the Windows<->WSL
  localhost-forwarding relay, wslrelay.exe; that relay wedged on 2026-08-24 and
  could only be cleared by resetting the WSL utility VM). Running the SSM
  port-forward natively on Windows removes WSL from the video path entirely:
  OBS -> 127.0.0.1:<LocalPort> -> session-manager-plugin -> SSM -> stream origin.

  Loops forever: verify account, resolve the cloud-world host from the
  CloudFormation stack (its instance id changes when the Spot host is replaced),
  then hold the port-forward. On any exit it logs and retries after RetrySec.

  NOTE: --parameters uses AWS CLI shorthand (host=...,portNumber=...), NOT JSON:
  PowerShell strips embedded double quotes from native-command arguments, which
  corrupts JSON. Shorthand carries no quotes and survives intact.
#>
[CmdletBinding()]
param(
  [string]$AwsProfile   = 'bibites-multiverse',
  [string]$Region       = 'us-east-1',
  [string]$AccountId    = '663615031964',
  [string]$Stack        = 'bibites-cloud-worlds',
  [string]$OriginIp     = '172.26.12.110',
  [int]   $LocalPort    = 1935,
  [int]   $RetrySec     = 5,
  [string]$LogFile      = "$env:LOCALAPPDATA\BibitesMultiverse\broadcast\logs\tunnel.log"
)

$ErrorActionPreference = 'Stop'
# aws shells out to session-manager-plugin by name; make sure it is on PATH even
# when this process was started before the installer refreshed the machine PATH.
$env:PATH = "$env:ProgramFiles\Amazon\SessionManagerPlugin\bin;$env:PATH"
$aws = Join-Path $env:ProgramFiles 'Amazon\AWSCLIV2\aws.exe'
if (-not (Test-Path $aws)) { $aws = 'aws' }

function Write-Log([string]$level,[string]$msg) {
  $line = ('{0} {1} {2}' -f (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'), $level, $msg)
  try {
    $dir = Split-Path -Parent $LogFile
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    if ((Test-Path $LogFile) -and ((Get-Item $LogFile).Length -gt 4MB)) { Move-Item -Force $LogFile ($LogFile + '.1') }
    Add-Content -LiteralPath $LogFile -Value $line -Encoding UTF8
  } catch {}
  Write-Host $line
}

Write-Log 'INFO' "native tunnel starting (profile=$AwsProfile stack=$Stack origin=$OriginIp local=$LocalPort)"
while ($true) {
  try {
    $acct = & $aws sts get-caller-identity --profile $AwsProfile --region $Region --query Account --output text 2>&1
    if ($LASTEXITCODE -ne 0) { throw "sts get-caller-identity failed: $acct" }
    if ("$acct".Trim() -ne $AccountId) { throw "wrong AWS account: $acct (expected $AccountId)" }

    $instance = & $aws cloudformation describe-stacks --stack-name $Stack --profile $AwsProfile --region $Region `
      --query "Stacks[0].Outputs[?OutputKey=='InstanceId'].OutputValue" --output text 2>&1
    if ($LASTEXITCODE -ne 0) { throw "describe-stacks failed: $instance" }
    $instance = "$instance".Trim()
    if (-not $instance -or $instance -eq 'None') { throw 'cloud host instance not found in stack outputs' }

    $params = "host=$OriginIp,portNumber=1935,localPortNumber=$LocalPort"
    Write-Log 'INFO' "opening port-forward to $instance ($OriginIp`:1935 -> 127.0.0.1:$LocalPort)"
    & $aws ssm start-session --target $instance --document-name AWS-StartPortForwardingSessionToRemoteHost `
      --parameters $params --profile $AwsProfile --region $Region
    Write-Log 'WARN' "port-forward session ended (exit $LASTEXITCODE); retrying in ${RetrySec}s"
  } catch {
    Write-Log 'WARN' ("tunnel cycle failed: " + $_.Exception.Message + "; retrying in ${RetrySec}s")
  }
  Start-Sleep -Seconds $RetrySec
}
