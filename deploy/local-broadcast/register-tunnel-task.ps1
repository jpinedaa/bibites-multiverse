#requires -Version 5
<#
  Register (idempotently) the BibitesBroadcastTunnel scheduled task, which runs
  run-tunnel.ps1 at logon and restarts it on failure. This is the native Windows
  replacement for the retired WSL systemd tunnel: it keeps the SSM port-forward
  on 127.0.0.1:<LocalPort> that OBS publishes to, with no dependency on the
  Windows<->WSL localhost-forwarding relay.
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$ScriptPath,   # full Windows path to run-tunnel.ps1
  [string]$AwsProfile = 'bibites-multiverse',
  [string]$Region     = 'us-east-1',
  [string]$AccountId  = '663615031964',
  [string]$Stack      = 'bibites-cloud-worlds',
  [Parameter(Mandatory)][string]$OriginIp,
  [int]   $LocalPort  = 1935,
  [string]$TaskName   = 'BibitesBroadcastTunnel'
)
$ErrorActionPreference = 'Stop'
$user = "$env:USERDOMAIN\$env:USERNAME"
$arg = @(
  '-NoProfile','-WindowStyle','Hidden','-ExecutionPolicy','Bypass',
  '-File', ('"{0}"' -f $ScriptPath),
  '-AwsProfile', $AwsProfile, '-Region', $Region, '-AccountId', $AccountId,
  '-Stack', $Stack, '-OriginIp', $OriginIp, '-LocalPort', $LocalPort
) -join ' '
$action    = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $arg
$trigger   = New-ScheduledTaskTrigger -AtLogOn -User $user
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
$settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
               -StartWhenAvailable -MultipleInstances IgnoreNew `
               -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
               -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger `
  -Principal $principal -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName
Write-Host "registered and started scheduled task $TaskName"
