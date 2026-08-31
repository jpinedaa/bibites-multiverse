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
    # Dot-source this file with -DefineOnly to load the functions above the
    # `if ($DefineOnly) { return }` line - the install wait, and the two published
    # names' reader, bounds and quoting - without opening a window, writing the
    # setup log or touching anything else. It is how release/test-installer-wait.ps1
    # calls the real wait and how release/test-install-uninstall.ps1 tests what
    # this window puts in its boxes and hands to the installer, against the real
    # code rather than a copy of it. Nothing that installs ever passes it.
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

# ------------------------------------------- the two names, and what this computer already publishes
#
# THE BOXES BELOW ARE FILLED IN FROM THE INSTALLATION THAT IS ALREADY HERE, and
# only a computer with none of one gets the Windows account name offered.
#
# WHY THAT IS NOT A DETAIL. This window always passes -Keeper and -WorldName, and
# a named flag beats the stored answer in the installer below (Get-PreviousSetting's
# first rule). So a window that filled its boxes from $env:UserName was not
# SUGGESTING a name on an upgrade - it was OVERWRITING one. Somebody who had been
# on the map as "Nightjar" for a year, or who had deliberately published nothing,
# double-clicked the newest release and became their Windows account name, with
# nothing on screen to say it had happened.
#
# So this reads what the installer would read, the same two files in the same
# order it reads them in (Install-BibitesMultiverse.ps1, Get-PreviousInstall and
# Find-PreviousSetting): the launcher's profile first, because that is what this
# installation is running now and what its own edits go into, and the install
# record behind it. AN EMPTY STORED VALUE IS AN ANSWER - somebody's decline - and
# it fills the box with nothing, which is this window's way of saying "publish
# none". It is not the same as a missing key, which is a question this computer
# has never been asked.
#
# IT READS NOTHING ELSE AND IT NEVER FAILS. A file that is missing, unreadable or
# describes another folder leaves both boxes on the fresh-install suggestion,
# which is where this window was before any of it existed.
$MaxPublicNameBytes = 64

function Read-SetupJson {
    param([string]$Path)
    if (-not $Path -or -not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    try { return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json) } catch { return $null }
}

function Test-SameFolder {
    param([string]$A, [string]$B)
    if (-not $A -or -not $B) { return $false }
    # Windows file names are case-insensitive, so this comparison is too.
    return ($A.TrimEnd('\', '/') -eq $B.TrimEnd('\', '/'))
}

function Get-JsonMember {
    param($Object, [string]$Name)
    if ($null -eq $Object) { return $null }
    if ($Object.PSObject.Properties.Match($Name).Count -eq 0) { return $null }
    return $Object.$Name
}

function Get-PreviousPublicNames {
    param([string]$DataRoot, [string]$ProgramRoot)
    $answer = [pscustomobject]@{
        Present           = $false
        Keeper            = ''
        KeeperAnswered    = $false
        WorldName         = ''
        WorldNameAnswered = $false
    }
    if (-not $DataRoot) { return $answer }

    # A record with no dataRoot of its own is one an older release wrote, and it
    # belongs to the folder it is in. One that names a different folder does not.
    $record = Read-SetupJson (Join-Path $DataRoot 'install-record.json')
    $recordRoot = [string](Get-JsonMember $record 'dataRoot')
    if ($record -and ((-not $recordRoot) -or (Test-SameFolder $recordRoot $DataRoot))) {
        $answer.Present = $true
    } else {
        $record = $null
    }
    $profileData = $null
    if ($ProgramRoot) {
        $profileData = Read-SetupJson (Join-Path $ProgramRoot 'profiles\default.json')
        if ($profileData -and (Test-SameFolder ([string](Get-JsonMember $profileData 'dataRoot')) $DataRoot)) {
            $answer.Present = $true
        } else {
            $profileData = $null
        }
    }

    $settings = Get-JsonMember $record 'settings'
    foreach ($field in @('keeper', 'worldName')) {
        $value = $null
        $found = $false
        if ($profileData -and $profileData.PSObject.Properties.Match($field).Count -gt 0) {
            $value = $profileData.$field
            $found = $true
        } elseif ($settings -and $settings.PSObject.Properties.Match($field).Count -gt 0) {
            $value = $settings.$field
            $found = $true
        }
        if (-not $found) { continue }
        if ($field -eq 'keeper') {
            $answer.Keeper = "$value"
            $answer.KeeperAnswered = $true
        } else {
            $answer.WorldName = "$value"
            $answer.WorldNameAnswered = $true
        }
    }
    return $answer
}

function Get-PublicNameProblem {
    # The same two bounds Install-BibitesMultiverse.ps1's Resolve-PublicName
    # applies, in the same words, so that a name this window accepts is a name
    # the installer accepts. See the button handler for why they are checked HERE
    # rather than left to the child process.
    param([string]$Value)
    if ([System.Text.Encoding]::UTF8.GetByteCount($Value) -gt $MaxPublicNameBytes) {
        return ("That is longer than $MaxPublicNameBytes bytes, which is the most the map carries " +
                "(an accented or non-Latin letter is more than one byte).")
    }
    foreach ($ch in $Value.ToCharArray()) {
        if ([char]::IsControl($ch)) {
            return 'That holds a control character, which no map, log or web page can show.'
        }
    }
    return ''
}

function Get-SuggestedKeeperName {
    # OFFERED, NEVER TAKEN, and only on a computer that has never answered. The
    # account name is reduced to the part a person would recognise - 'CORP\alice'
    # is how a machine spells an account, not a handle - and an account name this
    # window would then refuse is not offered at all.
    $name = "$env:UserName".Trim()
    $cut = $name.LastIndexOfAny([char[]]@('\', '/'))
    if ($cut -ge 0) { $name = $name.Substring($cut + 1) }
    if (-not $name -or (Get-PublicNameProblem $name)) { return '' }
    return $name
}

function Get-PublishedNameArgument {
    # An empty box publishes nothing, and it is passed as '-' rather than as an
    # empty string: an empty argument is the one thing Windows argument parsing
    # can quietly drop on the way to a child process, and a dropped argument here
    # would leave the installer thinking nobody had answered.
    param([string]$Value)
    $trimmed = "$Value".Trim()
    if (-not $trimmed) { return '-' }
    return $trimmed
}

function ConvertTo-NativeArgument {
    # ONE ARGUMENT, QUOTED THE WAY WINDOWS TAKES IT APART AGAIN.
    #
    # There is no argument array underneath any of this: a Windows process is
    # handed ONE string and splits it itself, by CommandLineToArgvW's rules. A
    # quoted run may hold spaces; a backslash is an ordinary character EXCEPT in
    # the run immediately before a double quote, where the run is halved and the
    # quote it precedes is escaped.
    #
    # WHICH IS WHY WRAPPING A VALUE IN QUOTES IS NOT ENOUGH. This wrapped each
    # argument and escaped the quotes inside it, and that is right until a value
    # ENDS IN A BACKSLASH - a game folder somebody typed with a trailing slash, a
    # keeper handle that ends in one. That backslash then escaped the closing
    # quote this window had just added, and everything after it, up to the next
    # quote, arrived as part of the same argument: -GameDir swallowed
    # -StartAfterInstall, or a name swallowed the flag behind it.
    #
    # The run before a quote - the closing one included - is doubled here, which
    # is the whole of the fix and the whole of the rule.
    param([string]$Value)
    $quoted = New-Object System.Text.StringBuilder
    [void]$quoted.Append('"')
    $slashes = 0
    foreach ($ch in "$Value".ToCharArray()) {
        if ($ch -eq '\') { $slashes++; continue }
        if ($ch -eq '"') {
            [void]$quoted.Append('\' * (2 * $slashes + 1))
            [void]$quoted.Append('"')
            $slashes = 0
            continue
        }
        if ($slashes -gt 0) { [void]$quoted.Append('\' * $slashes); $slashes = 0 }
        [void]$quoted.Append($ch)
    }
    [void]$quoted.Append('\' * (2 * $slashes))
    [void]$quoted.Append('"')
    return $quoted.ToString()
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

# Read after the probe, which opens no window and installs nothing: what is
# already here only matters to the two boxes this window is about to fill in.
# The data root is the installer's own default, because this window never passes
# -DataRoot, and the application folder is the one it would install into.
$previousDataRoot = ''
if ($env:LOCALAPPDATA) { $previousDataRoot = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse' }
$previousNames = Get-PreviousPublicNames `
    -DataRoot    $previousDataRoot `
    -ProgramRoot $(if ($InstallRoot) { $InstallRoot } else { $Here })
Write-SetupLog ("previousInstall={0} keeperAnswered={1} worldNameAnswered={2}" -f `
    $previousNames.Present, $previousNames.KeeperAnswered, $previousNames.WorldNameAnswered)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Install Bibites Multiverse 0.3.10'
$form.StartPosition = 'CenterScreen'
$form.ClientSize = New-Object System.Drawing.Size(650, 576)
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

# THE TWO NAMES THIS WORLD IS PUBLISHED UNDER, on the screen that installs it.
#
# They are filled in and they are EDITABLE, and WHAT THEY ARE FILLED IN WITH
# depends on whether this computer has answered before:
#
#   an upgrade    the names this installation already publishes, read above. An
#                 empty box is a decline somebody made and it stays empty.
#   a fresh one   the Windows account name, offered and never taken, with
#                 "<that name>'s world" beside it.
#
# Either way nothing is derived behind the person's back and nothing is taken
# without being shown first, which is the whole rule these two fields carry
# (contract-b-m4.md §33, B49) - so they are on the form, above the button that
# publishes them, rather than in a setting somebody would have to go looking for
# afterwards.
$namesTitle = New-Object System.Windows.Forms.Label
$namesTitle.Text = 'How your world is shown to everyone else'
$namesTitle.Font = New-Object System.Drawing.Font('Segoe UI Semibold', 10)
$namesTitle.AutoSize = $true
$namesTitle.Location = New-Object System.Drawing.Point(30, 284)
$form.Controls.Add($namesTitle)

$keeperLabel = New-Object System.Windows.Forms.Label
$keeperLabel.Text = 'Your name on the map'
$keeperLabel.AutoSize = $true
$keeperLabel.Location = New-Object System.Drawing.Point(30, 318)
$form.Controls.Add($keeperLabel)

$keeperBox = New-Object System.Windows.Forms.TextBox
$keeperBox.Location = New-Object System.Drawing.Point(230, 315)
$keeperBox.Size = New-Object System.Drawing.Size(290, 27)
$keeperBox.Text = if ($previousNames.KeeperAnswered) {
    $previousNames.Keeper
} else {
    Get-SuggestedKeeperName
}
$form.Controls.Add($keeperBox)

$worldNameLabel = New-Object System.Windows.Forms.Label
$worldNameLabel.Text = "This world's name"
$worldNameLabel.AutoSize = $true
$worldNameLabel.Location = New-Object System.Drawing.Point(30, 352)
$form.Controls.Add($worldNameLabel)

$worldNameBox = New-Object System.Windows.Forms.TextBox
$worldNameBox.Location = New-Object System.Drawing.Point(230, 349)
$worldNameBox.Size = New-Object System.Drawing.Size(290, 27)
$worldNameBox.Text = if ($previousNames.WorldNameAnswered) {
    $previousNames.WorldName
} elseif ($keeperBox.Text -and -not (Get-PublicNameProblem "$($keeperBox.Text)'s world")) {
    "$($keeperBox.Text)'s world"
} else {
    ''
}
$form.Controls.Add($worldNameBox)

$namesNote = New-Object System.Windows.Forms.Label
# TWO LINES, EITHER WAY. There is room for two above the checkbox below and no
# more, so the upgrade wording says where the values came from and keeps the one
# instruction that is not obvious: an emptied box publishes nothing.
$namesNote.Text = if ($previousNames.Present) {
    'These are the names this installation already publishes, and everyone on the' +
    [Environment]::NewLine +
    'map sees them exactly as typed. Empty a box to publish nothing in its place.'
} else {
    'Both are published publicly with your world: everyone on the map sees them,' +
    [Environment]::NewLine +
    'exactly as typed. Empty a box to publish nothing in its place.'
}
$namesNote.AutoSize = $true
$namesNote.ForeColor = [System.Drawing.Color]::DimGray
$namesNote.Location = New-Object System.Drawing.Point(30, 384)
$form.Controls.Add($namesNote)

$startAfter = New-Object System.Windows.Forms.CheckBox
$startAfter.Text = 'After installation, start The Bibites, connect, and open the launcher'
$startAfter.AutoSize = $true
$startAfter.Checked = $true
$startAfter.Location = New-Object System.Drawing.Point(30, 436)
$form.Controls.Add($startAfter)

$connectionNote = New-Object System.Windows.Forms.Label
$connectionNote.Text = 'The map gives this installation a unique identity. No join string is shared with another player.'
$connectionNote.AutoSize = $true
$connectionNote.ForeColor = [System.Drawing.Color]::DimGray
$connectionNote.Location = New-Object System.Drawing.Point(50, 464)
$form.Controls.Add($connectionNote)

$cancel = New-Object System.Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$cancel.Location = New-Object System.Drawing.Point(430, 518)
$cancel.Size = New-Object System.Drawing.Size(90, 34)
$form.Controls.Add($cancel)

$install = New-Object System.Windows.Forms.Button
$install.Text = 'Install'
$install.Location = New-Object System.Drawing.Point(530, 518)
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

    # THE TWO NAMES ARE CHECKED HERE, ON THE FORM, BEFORE ANYTHING STARTS.
    #
    # The installer below refuses a name it cannot carry, and it refuses it at
    # step 8 - after step 6 has enrolled this world with the map and step 7 has
    # imported a certificate. In a hidden console with its output redirected that
    # refusal is a red line in a log file nobody is reading, and what this window
    # would show is "Installation stopped" over a machine that is now half
    # changed: an identity taken on the map, a mod inside the game, no launcher
    # and no way back but the uninstaller.
    #
    # A box is a keyboard, and every other keyboard in this project gets told and
    # asked again. So this one does too, before the child process exists.
    foreach ($field in @(
        @{ Box = $keeperBox;    What = 'Your name on the map' },
        @{ Box = $worldNameBox; What = "This world's name" })) {
        $typed = "$($field.Box.Text)".Trim()
        # An empty box, and the '-' somebody may type into one, are both the
        # decline. There is nothing to bound about publishing nothing.
        if (-not $typed -or $typed -eq '-') { continue }
        $problem = Get-PublicNameProblem $typed
        if (-not $problem) { continue }
        Write-SetupLog "refused $($field.What): $problem"
        [void][System.Windows.Forms.MessageBox]::Show(
            $form,
            ($problem + [Environment]::NewLine + [Environment]::NewLine +
             'Type another one, or empty the box to publish nothing in its place.'),
            ("$($field.What) cannot be published"),
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Warning)
        [void]$field.Box.Focus()
        $field.Box.SelectAll()
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
    # -Unattended: this window is the keyboard, and the process it starts has a
    # hidden console nobody can type into. Every question the installer would ask
    # is answered here or not asked at all.
    $arguments = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'RemoteSigned', '-File', $installer,
                   '-RuntimeSelection', $runtime, '-Unattended')
    if ($InstallRoot) { $arguments += @('-InstallRoot', $InstallRoot) }
    if ($external.Checked) { $arguments += @('-GameDir', $pathBox.Text) }
    if ($startAfter.Checked) { $arguments += '-StartAfterInstall' }
    # BOTH ARE ALWAYS PASSED, and an emptied box is passed as '-' - the same
    # character every prompt in this project takes as "publish none".
    #
    # ALWAYS is the load-bearing word. The installer below asks at the keyboard
    # for a name it was not given, and the process this window starts has a
    # hidden console with its output redirected: a question asked there would
    # wait forever, on a screen that says "Installing. Keep this window open."
    # An answer given here is an answer it never has to ask for.
    #
    # WHAT IS PASSED IS WHAT IS IN THE BOX, which on an upgrade is what this
    # installation already publishes unless somebody edited it on this screen.
    # These flags beat the stored value in the installer, so filling the boxes
    # from the install above is what keeps that from being a silent rename.
    $arguments += @('-Keeper',    (Get-PublishedNameArgument $keeperBox.Text))
    $arguments += @('-WorldName', (Get-PublishedNameArgument $worldNameBox.Text))

    $quoted = @($arguments | ForEach-Object { ConvertTo-NativeArgument $_ }) -join ' '
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
