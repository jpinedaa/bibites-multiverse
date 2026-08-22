<#
.SYNOPSIS
    Prove that the graphical installer waits for the installer it started, and
    not for the world that installer leaves running behind it.

.DESCRIPTION
    The regression this file exists for: the GUI ran the real installer with
    `Start-Process -Wait`, Windows PowerShell implements that switch with a job
    object, and a job object is not empty until the CHILD AND EVERY DESCENDANT
    has ended. A default install ends by starting a world - the sidecar and the
    game, both meant to outlive the installer - so the wait never returned. The
    setup window sat on "Installing. Keep this window open." with the install
    already complete, the setup that wrapped it never wrote the uninstaller, the
    shortcuts or the Uninstall registry key, and the unpacked payload stayed in
    %TEMP%.

    So this test does exactly what an install does: it runs a stub installer
    that leaves ONE long-lived grandchild behind and exits at once, and requires
    the wait to come back while that grandchild is still running.

    It runs the GUI's own wait, loaded from the shipped script with -DefineOnly.
    Nothing here opens a window, reads a game, writes to the application
    directory or to the real data root, touches a trust store, or dials the
    network: the stub, its log and its grandchild's process id live in one
    throwaway folder under the temp directory, and the grandchild is stopped
    before this script returns.

    Windows PowerShell 5.1 is the engine that matters - it is the one the setup
    uses - and this file is written to run there:

      powershell.exe -NoProfile -ExecutionPolicy Bypass -File release\test-installer-wait.ps1

.PARAMETER KitDir
    The folder holding Install-BibitesMultiverse-Gui.ps1. Defaults to the `kit`
    folder beside this script, and falls back to this script's own directory so
    it also runs from inside a staged package.

.PARAMETER GrandchildSeconds
    How long the grandchild the stub leaves behind stays alive. The wait has to
    return well inside it.

.PARAMETER MaxWaitSeconds
    The longest the wait may take and still be a wait on the process alone.

.PARAMETER KeepSandbox
    Keep the temp folder for inspection. The grandchild is stopped either way.
#>
[CmdletBinding()]
param(
    [string]$KitDir = '',
    [int]$GrandchildSeconds = 30,
    [int]$MaxWaitSeconds = 15,
    [switch]$KeepSandbox
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

if (-not $KitDir) {
    $KitDir = Join-Path $PSScriptRoot 'kit'
    if (-not (Test-Path -LiteralPath (Join-Path $KitDir 'Install-BibitesMultiverse-Gui.ps1'))) {
        $KitDir = $PSScriptRoot
    }
}
$KitDir = (Resolve-Path -LiteralPath $KitDir).Path
$gui = Join-Path $KitDir 'Install-BibitesMultiverse-Gui.ps1'
if (-not (Test-Path -LiteralPath $gui -PathType Leaf)) { throw "not in -KitDir: $gui" }

$script:failures = 0
$script:checks   = 0

function Check {
    param([string]$What, [bool]$Ok, [string]$Detail = '')
    $script:checks++
    if ($Ok) {
        Write-Host ("  PASS  {0}" -f $What) -ForegroundColor Green
    } else {
        $script:failures++
        Write-Host ("  FAIL  {0}" -f $What) -ForegroundColor Red
        if ($Detail) { Write-Host ("        {0}" -f $Detail) -ForegroundColor Red }
    }
}

function Scenario { param([string]$Name) Write-Host ""; Write-Host "==== $Name" -ForegroundColor Cyan }

# The setup's diagnostic log belongs to whatever setup ran last, and a test that
# truncated it would destroy the evidence of the failure somebody is reading it
# for. -DefineOnly has to return above the line that writes it; this is how the
# test says so.
$setupLog = Join-Path $env:TEMP 'bibites-multiverse-setup.log'
$setupLogBefore = if (Test-Path -LiteralPath $setupLog) {
    $item = Get-Item -LiteralPath $setupLog
    "$($item.Length)/$($item.LastWriteTimeUtc.Ticks)"
} else { '<absent>' }

. $gui -DefineOnly

$setupLogAfter = if (Test-Path -LiteralPath $setupLog) {
    $item = Get-Item -LiteralPath $setupLog
    "$($item.Length)/$($item.LastWriteTimeUtc.Ticks)"
} else { '<absent>' }

Scenario "the GUI's wait, loaded from the shipped script"
Check "the shipped GUI defines the wait as a function this test can call" `
    ([bool](Get-Command -Name Invoke-BibitesInstaller -CommandType Function -ErrorAction SilentlyContinue))
Check "loading it with -DefineOnly leaves the setup's own diagnostic log alone" `
    ($setupLogBefore -eq $setupLogAfter) "$setupLogBefore -> $setupLogAfter"
if ($script:failures -gt 0) {
    Write-Host ""
    Write-Host "!! the GUI did not load; nothing below could run" -ForegroundColor Red
    exit 1
}

$sandbox = Join-Path $env:TEMP ("bibites-multiverse-waittest-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Force -Path $sandbox | Out-Null
Write-Host "sandbox: $sandbox"
Write-Host "kit    : $KitDir"

# Stands in for Install-BibitesMultiverse.ps1: it writes to both streams, starts
# one process that outlives it - the stub's answer to the sidecar and the game -
# and exits immediately with the code it was given.
$stub = Join-Path $sandbox 'stub-installer.ps1'
@'
param(
    [Parameter(Mandatory = $true)][string]$PidFile,
    [int]$GrandchildSeconds = 30,
    [int]$ExitCode = 0
)
Write-Output 'stub-installer stdout marker'
[Console]::Error.WriteLine('stub-installer stderr marker')
$engine = (Get-Process -Id $PID).Path
$grandchild = Start-Process -FilePath $engine -WindowStyle Hidden -PassThru `
    -ArgumentList "-NoLogo -NoProfile -Command Start-Sleep -Seconds $GrandchildSeconds"
Set-Content -LiteralPath $PidFile -Value $grandchild.Id -Encoding ASCII
exit $ExitCode
'@ | Set-Content -LiteralPath $stub -Encoding UTF8

$engine = (Get-Process -Id $PID).Path
$script:grandchildren = New-Object System.Collections.ArrayList

# The engine, the quoting and the redirection are the GUI's own, so what runs
# here is the shape a real install runs.
function Invoke-Stub {
    param([int]$ExitCode, [string]$Name)
    $pidFile = Join-Path $sandbox "$Name.pid"
    $log     = Join-Path $sandbox "$Name.log"
    Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
    $arguments = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'RemoteSigned', '-File', $stub,
                   '-PidFile', $pidFile,
                   '-GrandchildSeconds', $GrandchildSeconds,
                   '-ExitCode', $ExitCode)
    $quoted = @($arguments | ForEach-Object { '"' + ([string]$_).Replace('"', '\"') + '"' }) -join ' '

    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $code = Invoke-BibitesInstaller -Engine $engine -Arguments $quoted `
        -StandardOutput $log -StandardError ($log + '.err')
    $watch.Stop()

    $grandchildPid = 0
    if (Test-Path -LiteralPath $pidFile) {
        [void][int]::TryParse([string](Get-Content -LiteralPath $pidFile | Select-Object -First 1), [ref]$grandchildPid)
    }
    if ($grandchildPid) { [void]$script:grandchildren.Add($grandchildPid) }
    return [pscustomobject]@{
        Code          = $code
        Seconds       = $watch.Elapsed.TotalSeconds
        GrandchildPid = $grandchildPid
        # Read before anything stops it: whether it was still running at the
        # moment the wait returned is the whole point of the measurement.
        GrandchildRan = [bool]($grandchildPid -and (Get-Process -Id $grandchildPid -ErrorAction SilentlyContinue))
        Log           = $log
    }
}

try {
    Scenario "an installer that exits at once, leaving a world running"
    $run = Invoke-Stub -ExitCode 0 -Name 'success'
    Write-Host ("  the wait returned after {0:N2}s; the grandchild sleeps {1}s" -f $run.Seconds, $GrandchildSeconds)
    Check "the stub left a grandchild behind" ($run.GrandchildPid -ne 0)
    Check "that grandchild was still running when the wait returned" $run.GrandchildRan `
        'it ended early, so the measurement below proves nothing'
    Check ("the wait returned inside {0}s rather than following the grandchild to {1}s" -f $MaxWaitSeconds, $GrandchildSeconds) `
        ($run.Seconds -lt $MaxWaitSeconds) ("it took {0:N2}s" -f $run.Seconds)
    Check "the installer's exit code is 0 and not `$null" `
        ($null -ne $run.Code -and $run.Code -eq 0) ("it was [{0}]" -f $run.Code)
    Check "the redirected standard output was complete when the wait returned" `
        ((Test-Path -LiteralPath $run.Log) -and
         ((Get-Content -Raw -LiteralPath $run.Log) -match 'stub-installer stdout marker'))
    Check "the redirected standard error was complete when the wait returned" `
        ((Test-Path -LiteralPath ($run.Log + '.err')) -and
         ((Get-Content -Raw -LiteralPath ($run.Log + '.err')) -match 'stub-installer stderr marker'))

    # A refusal has to arrive as itself. This is the check that fails when the
    # process handle is not kept: ExitCode reads as $null, and the window would
    # call every successful install a failed one.
    Scenario "an installer that refuses, leaving a world running"
    $refusal = Invoke-Stub -ExitCode 3 -Name 'refusal'
    Write-Host ("  the wait returned after {0:N2}s" -f $refusal.Seconds)
    Check "the wait returned inside $MaxWaitSeconds s here too" `
        ($refusal.Seconds -lt $MaxWaitSeconds) ("it took {0:N2}s" -f $refusal.Seconds)
    Check "the refusal's own exit code came back, not `$null and not 0" `
        ($null -ne $refusal.Code -and $refusal.Code -eq 3) ("it was [{0}]" -f $refusal.Code)

    # Belt and braces: the source itself, so a wait reintroduced anywhere in that
    # file is caught even if it is not the call this test drives.
    Scenario "the shipped GUI's source"
    $guiCode = ((Get-Content -LiteralPath $gui) | Where-Object { $_ -notmatch '^\s*#' }) -join "`n"
    Check "the GUI never starts a process with -Wait" `
        (-not ($guiCode -match 'Start-Process[^\r\n]*\s-Wait\b'))
    Check "the GUI waits on the process object it started" ($guiCode -match '\.WaitForExit\(\)')
    Check "the GUI keeps the handle its exit code is read through" ($guiCode -match '\.Handle')
    Check "the default finish option says that it opens the launcher" `
        ($guiCode -match 'After installation, start The Bibites, connect, and open the launcher')
    Check "the GUI locates the launcher in the installed application directory" `
        ($guiCode -match '\$launcherPath\s*=\s*Join-Path\s+\$programRoot\s+''BibitesMultiverseLauncher\.exe''')
    Check "the GUI opens the launcher only when start-after-install was selected" `
        ($guiCode -match '(?s)if \(\$startAfter\.Checked\) \{.{0,1200}Start-Process -FilePath \$launcherPath -WorkingDirectory \$programRoot')
    Check "a launcher-window failure does not report the completed install as failed" `
        ($guiCode -match '(?s)catch \{\s*\$launcherFailure = \$_\.Exception\.Message.{0,800}The launcher could not open')
} finally {
    foreach ($grandchildPid in $script:grandchildren) {
        Get-Process -Id $grandchildPid -ErrorAction SilentlyContinue |
            Stop-Process -Force -ErrorAction SilentlyContinue
    }
    if ($KeepSandbox) {
        Write-Host ""
        Write-Host "sandbox kept: $sandbox"
    } else {
        Remove-Item -LiteralPath $sandbox -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Host ""
if ($script:failures -eq 0) {
    Write-Host ("ALL {0} CHECKS PASSED" -f $script:checks) -ForegroundColor Green
    exit 0
}
Write-Host ("{0} of {1} CHECKS FAILED" -f $script:failures, $script:checks) -ForegroundColor Red
exit 1
