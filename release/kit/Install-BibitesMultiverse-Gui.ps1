<#
.SYNOPSIS
    Show the Bibites Multiverse installer window.

.DESCRIPTION
    The included portable game is the default. A user can select an existing
    game instead. The installer searches the Steam and itch.io locations first.
    The default completion option starts the connected game and opens the
    installed launcher window.
#>
[CmdletBinding()]
param(
    [switch]$Probe,
    [string]$InstallRoot = '',
    # Dot-source this file with -DefineOnly to load the function below without
    # opening a window, writing the setup log or touching anything else. It is
    # how release/test-installer-wait.ps1 calls the real wait; nothing that
    # installs ever passes it.
    [switch]$DefineOnly
)

# ------------------------------------------------------------ the install wait
# WHY THIS IS NOT `Start-Process -Wait`. Windows PowerShell implements that
# switch with a JOB OBJECT, and it waits until the job is EMPTY - the child AND
# every descendant the child leaves running. The last thing a default install
# does is start a world: Install-BibitesMultiverse.ps1 runs the Start-Multiverse
# script it just generated, which launches multiverse-sidecar.exe and
# The Bibites.exe and returns. Those two outlive the installer on purpose, so
# the job never emptied and -Wait never came back. This window sat on
# "Installing. Keep this window open." with the install already finished, and
# because this script never exited, the setup around it never reached the steps
# that come after it: no uninstaller, no shortcuts, no Uninstall registry key,
# and the unpacked payload left behind in %TEMP%.
#
# Waiting on the process object waits for THAT process and nothing under it.
# This is not the case Install-BibitesMultiverse.ps1's stop script refuses
# WaitForExit for: what makes WaitForExit lie there is a handle the script
# cannot open for a process it did not start, and this is the handle
# Start-Process has just handed back.
#
# READING .Handle IS LOAD-BEARING. With redirection, Start-Process -PassThru
# returns a process object that holds no handle of its own, and $process.ExitCode
# then reads as $null - which this window would show as a failed install after
# every successful one. Touching .Handle once, while the process is certainly
# alive, caches the handle the exit code is later read through. `-Wait` used to
# do that for us.
function Invoke-BibitesInstaller {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Engine,
        [Parameter(Mandatory = $true)][string]$Arguments,
        [Parameter(Mandatory = $true)][string]$StandardOutput,
        [Parameter(Mandatory = $true)][string]$StandardError
    )
    $process = Start-Process -FilePath $Engine -ArgumentList $Arguments -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput $StandardOutput -RedirectStandardError $StandardError
    if (-not $process) { throw 'the installer process did not start' }
    try { $null = $process.Handle } catch { }
    # No timeout: this returns when the process has ended and its redirected
    # output has been written, which is what both dialogs below read.
    $process.WaitForExit()
    $code = $process.ExitCode
    if ($null -eq $code) {
        throw 'the installer finished and its exit code could not be read'
    }
    return $code
}

if ($DefineOnly) { return }

# ---------------------------------------------------------------- diagnostic log
# Written before strict error handling so any startup failure is captured in a
# file the NSIS wrapper can reference. Each run truncates the previous log.
$SetupLog = Join-Path $env:TEMP 'bibites-multiverse-setup.log'
try { Set-Content -LiteralPath $SetupLog -Value '' -ErrorAction Stop } catch { $SetupLog = '' }
function Write-SetupLog {
    param([string]$Message)
    if (-not $SetupLog) { return }
    try { Add-Content -LiteralPath $SetupLog -Value "$(Get-Date -Format 'HH:mm:ss') $Message" } catch { }
}
Write-SetupLog "gui-installer start"
Write-SetupLog "ps=$($PSVersionTable.PSVersion) lang=$($ExecutionContext.SessionState.LanguageMode)"
Write-SetupLog "script=$($MyInvocation.MyCommand.Path)"
Write-SetupLog "cwd=$PWD"
Write-SetupLog "temp=$env:TEMP"
Write-SetupLog "args: Probe=$Probe InstallRoot='$InstallRoot'"

trap {
    Write-SetupLog "FATAL $($_.Exception.GetType().FullName): $($_.Exception.Message)"
    Write-SetupLog "  at $($_.InvocationInfo.PositionMessage)"
    exit 1
}

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $Here 'Find-BibitesGame.ps1')

# Install-BibitesMultiverse.ps1's dedicated exit code for "something of this
# installation is running" - its step 0, which asks before it writes anything.
# It is the one refusal this window can name in its own title, because it is the
# one whose remedy is a thing to do on this screen rather than a thing to read.
$ExitBusy = 3

$hasBundledGame = Test-Path -LiteralPath (Join-Path $Here 'game-payload.json') -PathType Leaf
$foundGame = Find-BibitesGameDirectory
$defaultRuntime = if ($hasBundledGame) { 'bundled' } else { 'external' }
Write-SetupLog "hasBundledGame=$hasBundledGame defaultRuntime=$defaultRuntime foundGame=$foundGame"

if ($Probe) {
    $manifestMatches = $true
    $manifestFiles = 0
    $manifestPath = Join-Path $Here 'MANIFEST.sha256'
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        $manifestMatches = $false
    } else {
        foreach ($line in (Get-Content -LiteralPath $manifestPath)) {
            $text = $line.Trim()
            if (-not $text -or $text.StartsWith('#')) { continue }
            $match = [regex]::Match($text, '^([0-9A-Fa-f]{64})\s+\*?(.+)$')
            if (-not $match.Success) {
                $manifestMatches = $false
                break
            }
            $file = Join-Path $Here $match.Groups[2].Value.Trim()
            if (-not (Test-Path -LiteralPath $file -PathType Leaf) -or
                (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ne $match.Groups[1].Value) {
                $manifestMatches = $false
                break
            }
            $manifestFiles++
        }
    }
    if ($manifestFiles -eq 0) { $manifestMatches = $false }
    [ordered]@{
        hasBundledGame = $hasBundledGame
        foundGame = $foundGame
        defaultRuntime = $defaultRuntime
        manifestMatches = $manifestMatches
        manifestFiles = $manifestFiles
    } | ConvertTo-Json
    if (-not $manifestMatches) { exit 1 }
    exit 0
}

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Install Bibites Multiverse 0.3.4'
$form.StartPosition = 'CenterScreen'
$form.ClientSize = New-Object System.Drawing.Size(650, 440)
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.Font = New-Object System.Drawing.Font('Segoe UI', 10)

$title = New-Object System.Windows.Forms.Label
$title.Text = 'Choose the game that this world will use'
$title.Font = New-Object System.Drawing.Font('Segoe UI Semibold', 16)
$title.AutoSize = $true
$title.Location = New-Object System.Drawing.Point(24, 20)
$form.Controls.Add($title)

$intro = New-Object System.Windows.Forms.Label
$intro.Text = 'The installer configures the game and connects it to bibitesmultiverse.com.'
$intro.AutoSize = $true
$intro.Location = New-Object System.Drawing.Point(27, 58)
$form.Controls.Add($intro)

$bundled = New-Object System.Windows.Forms.RadioButton
$bundled.Text = 'Use the included portable game (recommended)'
$bundled.AutoSize = $true
$bundled.Location = New-Object System.Drawing.Point(30, 105)
$bundled.Enabled = $hasBundledGame
$bundled.Checked = $hasBundledGame
$form.Controls.Add($bundled)

$bundledNote = New-Object System.Windows.Forms.Label
$bundledNote.Text = if ($hasBundledGame) {
    'The installer makes a managed game copy. It does not change another installation.'
} else {
    'This package does not contain a portable game.'
}
$bundledNote.AutoSize = $true
$bundledNote.ForeColor = [System.Drawing.Color]::DimGray
$bundledNote.Location = New-Object System.Drawing.Point(50, 132)
$form.Controls.Add($bundledNote)

$external = New-Object System.Windows.Forms.RadioButton
$external.Text = 'Use a game that is already installed'
$external.AutoSize = $true
$external.Location = New-Object System.Drawing.Point(30, 178)
$external.Checked = -not $hasBundledGame
$form.Controls.Add($external)

$pathBox = New-Object System.Windows.Forms.TextBox
$pathBox.Location = New-Object System.Drawing.Point(50, 210)
$pathBox.Size = New-Object System.Drawing.Size(470, 27)
$pathBox.Text = $foundGame
$form.Controls.Add($pathBox)

$browse = New-Object System.Windows.Forms.Button
$browse.Text = 'Browse...'
$browse.Location = New-Object System.Drawing.Point(530, 208)
$browse.Size = New-Object System.Drawing.Size(90, 30)
$form.Controls.Add($browse)

$gameStatus = New-Object System.Windows.Forms.Label
$gameStatus.AutoSize = $true
$gameStatus.Location = New-Object System.Drawing.Point(50, 246)
$form.Controls.Add($gameStatus)

function Update-GameStatus {
    $useExternal = $external.Checked
    $pathBox.Enabled = $useExternal
    $browse.Enabled = $useExternal
    if (-not $useExternal) {
        $gameStatus.Text = 'The included portable game is selected.'
        $gameStatus.ForeColor = [System.Drawing.Color]::DarkGreen
    } elseif (Test-BibitesGameDirectory $pathBox.Text) {
        $gameStatus.Text = 'Game found.'
        $gameStatus.ForeColor = [System.Drawing.Color]::DarkGreen
    } else {
        $gameStatus.Text = 'Game not found. Select the folder that contains The Bibites.exe.'
        $gameStatus.ForeColor = [System.Drawing.Color]::Firebrick
    }
}

$bundled.Add_CheckedChanged({ Update-GameStatus })
$external.Add_CheckedChanged({ Update-GameStatus })
$pathBox.Add_TextChanged({ Update-GameStatus })
$browse.Add_Click({
    $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
    $dialog.Description = 'Select the folder that contains The Bibites.exe.'
    $dialog.ShowNewFolderButton = $false
    if ($pathBox.Text -and (Test-Path -LiteralPath $pathBox.Text -PathType Container)) {
        $dialog.SelectedPath = $pathBox.Text
    }
    if ($dialog.ShowDialog($form) -eq [System.Windows.Forms.DialogResult]::OK) {
        $pathBox.Text = $dialog.SelectedPath
    }
    $dialog.Dispose()
})

$startAfter = New-Object System.Windows.Forms.CheckBox
$startAfter.Text = 'After installation, start The Bibites, connect, and open the launcher'
$startAfter.AutoSize = $true
$startAfter.Checked = $true
$startAfter.Location = New-Object System.Drawing.Point(30, 300)
$form.Controls.Add($startAfter)

$connectionNote = New-Object System.Windows.Forms.Label
$connectionNote.Text = 'The map gives this installation a unique identity. No join string is shared with another player.'
$connectionNote.AutoSize = $true
$connectionNote.ForeColor = [System.Drawing.Color]::DimGray
$connectionNote.Location = New-Object System.Drawing.Point(50, 328)
$form.Controls.Add($connectionNote)

$cancel = New-Object System.Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$cancel.Location = New-Object System.Drawing.Point(430, 382)
$cancel.Size = New-Object System.Drawing.Size(90, 34)
$form.Controls.Add($cancel)

$install = New-Object System.Windows.Forms.Button
$install.Text = 'Install'
$install.Location = New-Object System.Drawing.Point(530, 382)
$install.Size = New-Object System.Drawing.Size(90, 34)
$form.Controls.Add($install)
$form.AcceptButton = $install
$form.CancelButton = $cancel

$install.Add_Click({
    Write-SetupLog "install button clicked runtime=$($bundled.Checked.ToString()) startAfter=$($startAfter.Checked)"

    if ($external.Checked -and -not (Test-BibitesGameDirectory $pathBox.Text)) {
        [void][System.Windows.Forms.MessageBox]::Show(
            $form,
            'Select the folder that contains The Bibites.exe.',
            'Game not found',
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Warning)
        return
    }

    $runtime = if ($bundled.Checked) { 'bundled' } else { 'external' }
    Write-SetupLog "runtime=$runtime installFromExternal=$($external.Checked)"
    $installer = Join-Path $Here 'Install-BibitesMultiverse.ps1'
    $engine = (Get-Process -Id $PID).Path
    Write-SetupLog "engine=$engine installer=$installer startAfter=$($startAfter.Checked)"
    # The installer's own words, kept. This window is the only place a graphical
    # install ever sees them, and some of what step 6 says - a world adopted, a
    # world left dark, a refusal with numbered choices - has to outlive the
    # dialog. The data root is the installer's default here, because this GUI
    # never passes -DataRoot.
    $logStamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
    $logDir = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse\logs'
    try {
        New-Item -ItemType Directory -Force -Path $logDir -ErrorAction Stop | Out-Null
    } catch {
        $logDir = $env:TEMP
    }
    $log = Join-Path $logDir ('install-' + $logStamp + '.log')
    Write-SetupLog "installLog=$log"
    $arguments = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'RemoteSigned', '-File', $installer,
                   '-RuntimeSelection', $runtime)
    if ($InstallRoot) { $arguments += @('-InstallRoot', $InstallRoot) }
    if ($external.Checked) { $arguments += @('-GameDir', $pathBox.Text) }
    if ($startAfter.Checked) { $arguments += '-StartAfterInstall' }

    $quoted = @($arguments | ForEach-Object { '"' + ([string]$_).Replace('"', '\"') + '"' }) -join ' '
    $form.UseWaitCursor = $true
    $form.Controls | ForEach-Object { $_.Enabled = $false }
    $gameStatus.Text = 'Installing. Keep this window open.'
    $gameStatus.ForeColor = [System.Drawing.Color]::Navy
    [System.Windows.Forms.Application]::DoEvents()

    try {
        $exitCode = Invoke-BibitesInstaller -Engine $engine -Arguments $quoted `
            -StandardOutput $log -StandardError ($log + '.err')
        Write-SetupLog "installer exit=$exitCode"
        if ($exitCode -ne 0) {
            # Enough lines to carry a whole refusal, not only its last word: a
            # STOP that prints numbered ways on is useless cut off above the
            # numbers, and this dialog is all a GUI install ever shows. Step 0's
            # refusal is longer than the rest - it lists the programs to close
            # AND the three ways to close them - and cut off at the top it hides
            # the list, so it gets a tail of its own.
            $tail = 30
            $caption = 'Bibites Multiverse was not installed'
            $headline = 'Installation stopped.'
            if ($exitCode -eq $ExitBusy) {
                $tail = 60
                $caption = 'Close Bibites Multiverse first'
                # THE HEADLINE DOES NOT SAY WHAT WAS CHANGED, because this
                # dialog cannot know. The same exit code carries two refusals:
                # step 0's, which happens before anything at all is written, and
                # the one a program STARTED DURING the install raises at step 5
                # or step 9, by which time the mod and this world's identity are
                # already in place. The installer's own words say which of the
                # two happened, in the text below, and a headline that asserted
                # "nothing was changed" over the second of them was simply wrong
                # on the one screen a graphical install ever shows.
                $headline = 'Part of this installation is running, and Windows will not let' +
                            [Environment]::NewLine +
                            'a setup replace a program while it runs. What this run did and' +
                            [Environment]::NewLine +
                            'did not change is below.'
            }
            $detail = ''
            if (Test-Path -LiteralPath $log) {
                $detail = (Get-Content -LiteralPath $log -Tail $tail) -join [Environment]::NewLine
            }
            if (Test-Path -LiteralPath ($log + '.err')) {
                $detail += [Environment]::NewLine
                $detail += (Get-Content -LiteralPath ($log + '.err') -Tail 12) -join [Environment]::NewLine
            }
            [void][System.Windows.Forms.MessageBox]::Show(
                $form,
                ($headline + [Environment]::NewLine + [Environment]::NewLine + $detail +
                 [Environment]::NewLine + [Environment]::NewLine + 'The whole of it is in:' +
                 [Environment]::NewLine + $log),
                $caption,
                [System.Windows.Forms.MessageBoxButtons]::OK,
                [System.Windows.Forms.MessageBoxIcon]::Error)
            $form.UseWaitCursor = $false
            $form.Controls | ForEach-Object { $_.Enabled = $true }
            Update-GameStatus
            return
        }
        # Starting the world belongs to the core installer above. Opening the
        # window belongs here: -StartAfterInstall is also a supported advanced
        # script option, and a command-line install must not unexpectedly open
        # a graphical application. Both the setup and the advanced ZIP put the
        # launcher in $InstallRoot (or beside this GUI when no root was given).
        # A launch failure does not turn a completed install and a running world
        # into a failed install. Say what happened and leave the icon/executable
        # available for another try.
        $launcherOpened = $false
        $launcherFailure = ''
        $programRoot = if ($InstallRoot) { $InstallRoot } else { $Here }
        $launcherPath = Join-Path $programRoot 'BibitesMultiverseLauncher.exe'
        if ($startAfter.Checked) {
            try {
                if (-not (Test-Path -LiteralPath $launcherPath -PathType Leaf)) {
                    throw "the installed launcher was not found at $launcherPath"
                }
                Start-Process -FilePath $launcherPath -WorkingDirectory $programRoot | Out-Null
                $launcherOpened = $true
                Write-SetupLog "opened launcher=$launcherPath"
            } catch {
                $launcherFailure = $_.Exception.Message
                Write-SetupLog "launcher open failed: $launcherFailure"
            }
        }

        $message = if ($startAfter.Checked -and $launcherOpened) {
            'Installation is complete. The game and its Multiverse connection started, and the launcher is open.'
        } elseif ($startAfter.Checked) {
            'Installation is complete, and the game and its Multiverse connection started.' +
            [Environment]::NewLine + [Environment]::NewLine +
            'The launcher could not open. You can open it later from:' +
            [Environment]::NewLine + $launcherPath +
            [Environment]::NewLine + [Environment]::NewLine + $launcherFailure
        } else {
            'Installation is complete. Use the Bibites Multiverse icon when you want to connect.'
        }
        # What the installer said is worth reading even when it succeeded: it names
        # this world's identity, and whether it took a new one.
        $message += [Environment]::NewLine + [Environment]::NewLine + 'What it did, in its own words:' +
                    [Environment]::NewLine + $log
        [void][System.Windows.Forms.MessageBox]::Show(
            $form, $message, 'Bibites Multiverse is ready',
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Information)
        $form.DialogResult = [System.Windows.Forms.DialogResult]::OK
        $form.Close()
    } finally {
        # The log is NOT deleted: it is this install's only durable account of what
        # happened, and both dialogs name its path. The empty error file is.
        if ((Test-Path -LiteralPath ($log + '.err')) -and
            -not (Get-Item -LiteralPath ($log + '.err')).Length) {
            Remove-Item -LiteralPath ($log + '.err') -Force -ErrorAction SilentlyContinue
        }
    }
})

Update-GameStatus
$result = $form.ShowDialog()
$form.Dispose()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) { exit 0 }
exit 2
