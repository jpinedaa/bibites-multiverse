<#
.SYNOPSIS
    Show the Bibites Multiverse installer window.

.DESCRIPTION
    The included portable game is the default. A user can select an existing
    game instead. The installer searches the Steam and itch.io locations first.
#>
[CmdletBinding()]
param([switch]$Probe)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $Here 'Find-BibitesGame.ps1')

$hasBundledGame = Test-Path -LiteralPath (Join-Path $Here 'game-payload.json') -PathType Leaf
$foundGame = Find-BibitesGameDirectory
$defaultRuntime = if ($hasBundledGame) { 'bundled' } else { 'external' }

if ($Probe) {
    [ordered]@{
        hasBundledGame = $hasBundledGame
        foundGame = $foundGame
        defaultRuntime = $defaultRuntime
    } | ConvertTo-Json
    exit 0
}

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Install Bibites Multiverse 0.2.1'
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
$startAfter.Text = 'Start The Bibites and connect after installation'
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
    $installer = Join-Path $Here 'Install-BibitesMultiverse.ps1'
    $engine = (Get-Process -Id $PID).Path
    $log = Join-Path $env:TEMP ('bibites-multiverse-install-' + [guid]::NewGuid().ToString('N') + '.log')
    $arguments = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'RemoteSigned', '-File', $installer,
                   '-RuntimeSelection', $runtime)
    if ($external.Checked) { $arguments += @('-GameDir', $pathBox.Text) }
    if ($startAfter.Checked) { $arguments += '-StartAfterInstall' }

    $quoted = @($arguments | ForEach-Object { '"' + ([string]$_).Replace('"', '\"') + '"' }) -join ' '
    $form.UseWaitCursor = $true
    $form.Controls | ForEach-Object { $_.Enabled = $false }
    $gameStatus.Text = 'Installing. Keep this window open.'
    $gameStatus.ForeColor = [System.Drawing.Color]::Navy
    [System.Windows.Forms.Application]::DoEvents()

    try {
        $process = Start-Process -FilePath $engine -ArgumentList $quoted -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput $log -RedirectStandardError ($log + '.err')
        if ($process.ExitCode -ne 0) {
            $detail = ''
            if (Test-Path -LiteralPath $log) {
                $detail = (Get-Content -LiteralPath $log -Tail 18) -join [Environment]::NewLine
            }
            if (Test-Path -LiteralPath ($log + '.err')) {
                $detail += [Environment]::NewLine
                $detail += (Get-Content -LiteralPath ($log + '.err') -Tail 12) -join [Environment]::NewLine
            }
            [void][System.Windows.Forms.MessageBox]::Show(
                $form,
                ('Installation stopped.' + [Environment]::NewLine + [Environment]::NewLine + $detail),
                'Bibites Multiverse was not installed',
                [System.Windows.Forms.MessageBoxButtons]::OK,
                [System.Windows.Forms.MessageBoxIcon]::Error)
            $form.UseWaitCursor = $false
            $form.Controls | ForEach-Object { $_.Enabled = $true }
            Update-GameStatus
            return
        }
        $message = if ($startAfter.Checked) {
            'Installation is complete. The game and its Multiverse connection started.'
        } else {
            'Installation is complete. Run Start-Multiverse.ps1 when you want to connect.'
        }
        [void][System.Windows.Forms.MessageBox]::Show(
            $form, $message, 'Bibites Multiverse is ready',
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Information)
        $form.DialogResult = [System.Windows.Forms.DialogResult]::OK
        $form.Close()
    } finally {
        Remove-Item -LiteralPath $log, ($log + '.err') -Force -ErrorAction SilentlyContinue
    }
})

Update-GameStatus
$result = $form.ShowDialog()
$form.Dispose()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) { exit 0 }
exit 1
