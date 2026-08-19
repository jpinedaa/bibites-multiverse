<#
.SYNOPSIS
    Prove the two rules that decide what the installer and the uninstall may
    delete, and whose BepInEx a game folder holds. No game, no install, nothing
    written anywhere.

.DESCRIPTION
    release/test-install-uninstall.ps1 needs a real Windows game and a machine to
    install it on. These two rules need neither: they are pure functions over a
    path, and they are the guard on a RECURSIVE DELETE and the answer to the
    question that made the install-uninstall-install cycle fail.

      Test-ManagedRuntimePath   is this path exactly <data root>\runtimes\<sha> -
                                the one folder the complete edition owns and may
                                remove whole? Both scripts carry it, so both
                                copies are driven here: a rule the two only
                                approximated in the same way is how they would
                                come to disagree about a delete.
      Test-InstallerOwnedBepInEx  is the mod framework in this game folder this
                                install's to record and to remove? In a game
                                somebody else chose: no - unless the PREVIOUS
                                install's own record says this package put it
                                there, which is the upgrade case. In this
                                package's own managed game copy: always - this
                                installer put it there itself, one run earlier.
                                Answering that wrong is what left a game folder
                                holding 27 files of BepInEx and no game, which
                                the next install refused to overwrite
                                (INS-RUNTIME) - and, in the other edition, what
                                left a whole framework in somebody's own Steam
                                folder after an upgrade and an uninstall.
      Get-ProtectedRoot         which of the paths a launcher profile may never
                                name as a world's data root still count once the
                                managed game copy is being reclaimed by the run
                                asking. Protecting a folder this same uninstall
                                removes made every complete-edition data root
                                fail Test-SafeDataRoot, so every profile was
                                kept, the profiles directory stayed, and the
                                application directory could never be emptied.

    The functions are lifted out of the scripts themselves rather than copied
    here, so a rule that changes in one and not in the other fails this.

    It runs on Windows PowerShell 5.1, on PowerShell 7, and on Linux - the paths
    it builds use whatever separator the platform running it uses, which is the
    point: the rule is about the shape of a path, not about a separator.

.EXAMPLE
    pwsh -NoProfile -File release/test-runtime-rules.ps1
#>
[CmdletBinding()]
param([string]$RepoRoot = '')

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

if (-not $RepoRoot) { $RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path) }
$installer   = Join-Path $RepoRoot 'release/kit/Install-BibitesMultiverse.ps1'
$uninstaller = Join-Path $RepoRoot 'release/kit/Uninstall-BibitesMultiverse.ps1'
foreach ($p in @($installer, $uninstaller)) {
    if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { throw "not here: $p" }
}

$script:failures = 0
$script:checks   = 0
function Check {
    param([string]$What, [bool]$Ok)
    $script:checks++
    if ($Ok) {
        Write-Host ("  PASS  {0}" -f $What)
    } else {
        $script:failures++
        Write-Host ("  FAIL  {0}" -f $What)
    }
}
function Scenario { param([string]$Name) Write-Host ""; Write-Host "==== $Name" }

# The function as the script itself has it, brace to brace at column zero. It is
# dot-sourced at THIS scope, so the definitions land beside these tests.
function Get-ScriptFunction {
    param([string]$File, [string]$Name, [string]$As = '')
    $text = Get-Content -LiteralPath $File -Raw
    $match = [regex]::Match($text, "(?s)function $Name \{.*?\r?\n\}")
    if (-not $match.Success) { throw "no function $Name in $File" }
    $body = $match.Value
    if ($As) { $body = $body -replace "function $Name", "function $As" }
    return [scriptblock]::Create($body)
}

. (Get-ScriptFunction $installer 'Get-FullPath')
. (Get-ScriptFunction $installer 'Test-ManagedRuntimePath')
. (Get-ScriptFunction $installer 'Test-InstallerOwnedBepInEx')
. (Get-ScriptFunction $uninstaller 'Test-ManagedRuntimePath' 'Test-ManagedRuntimePathUninstall')

$root     = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), 'bibites-multiverse-rules')
$sha      = 'A' * 64
$other    = 'B' * 64
$runtimes = [System.IO.Path]::Combine($root, 'runtimes')
$managed  = [System.IO.Path]::Combine($runtimes, $sha)
$chosen   = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), 'Steam', 'The Bibites')

Scenario "the installer's rule, against the payload it is installing"
Check "accepts <data root>\runtimes\<payload sha>" (Test-ManagedRuntimePath $managed $root $sha)
Check "accepts it with a trailing separator" `
    (Test-ManagedRuntimePath ($managed + [System.IO.Path]::DirectorySeparatorChar) $root $sha)
Check "accepts a lower-case name against an upper-case hash" `
    (Test-ManagedRuntimePath ([System.IO.Path]::Combine($runtimes, $sha.ToLowerInvariant())) $root $sha)
Check "refuses the data root itself" (-not (Test-ManagedRuntimePath $root $root $sha))
Check "refuses the journal's directory" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($root, 'data')) $root $sha))
Check "refuses the logs directory" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($root, 'logs')) $root $sha))
Check "refuses the runtimes directory itself" (-not (Test-ManagedRuntimePath $runtimes $root $sha))
Check "refuses another runtime, which this run is not installing" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($runtimes, $other)) $root $sha))
Check "refuses a directory inside the runtime" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($managed, 'BepInEx')) $root $sha))
Check "refuses a name that is not a sha256" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($runtimes, 'game')) $root $sha))
Check "refuses a path with .. in it" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($runtimes, $sha, '..', $sha)) $root $sha))
Check "refuses .. that climbs out of the data root" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($runtimes, '..', '..', 'etc')) $root $sha))
Check "refuses a sibling directory whose name only starts the same way" `
    (-not (Test-ManagedRuntimePath ([System.IO.Path]::Combine($root, 'runtimes-old', $sha)) $root $sha))
Check "refuses a runtime under another data root" `
    (-not (Test-ManagedRuntimePath $managed ([System.IO.Path]::Combine($root, 'elsewhere')) $sha))
Check "refuses a game folder somebody chose" (-not (Test-ManagedRuntimePath $chosen $root $sha))
Check "refuses an empty path" (-not (Test-ManagedRuntimePath '' $root $sha))
Check "refuses an empty data root" (-not (Test-ManagedRuntimePath $managed '' $sha))
Check "refuses a sha that is not a sha" (-not (Test-ManagedRuntimePath $managed $root 'nope'))

Scenario "the uninstall's copy of the same rule, with no payload to name"
Check "accepts <data root>\runtimes\<sha>" (Test-ManagedRuntimePathUninstall $managed $root)
Check "accepts any runtime under that data root, because it removes what the record names" `
    (Test-ManagedRuntimePathUninstall ([System.IO.Path]::Combine($runtimes, $other)) $root)
Check "refuses the data root itself" (-not (Test-ManagedRuntimePathUninstall $root $root))
Check "refuses the journal's directory" `
    (-not (Test-ManagedRuntimePathUninstall ([System.IO.Path]::Combine($root, 'data')) $root))
Check "refuses the runtimes directory itself" (-not (Test-ManagedRuntimePathUninstall $runtimes $root))
Check "refuses a name that is not a sha256" `
    (-not (Test-ManagedRuntimePathUninstall ([System.IO.Path]::Combine($runtimes, 'game')) $root))
Check "refuses a path with .. in it" `
    (-not (Test-ManagedRuntimePathUninstall ([System.IO.Path]::Combine($runtimes, $sha, '..', $sha)) $root))
Check "refuses a game folder somebody chose" (-not (Test-ManagedRuntimePathUninstall $chosen $root))

Scenario "whose BepInEx is this"
Check "one that is not there yet is this install's, in a chosen game folder" `
    (Test-InstallerOwnedBepInEx -Present $false -RuntimeMode 'external' -GameDir $chosen -DataRoot $root)
Check "one that is not there yet is this install's, in the managed runtime" `
    (Test-InstallerOwnedBepInEx -Present $false -RuntimeMode 'bundled' -GameDir $managed -DataRoot $root)
Check "one already in a game folder somebody chose is NOT this install's" `
    (-not (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'external' -GameDir $chosen -DataRoot $root))
Check "one already in this package's own managed game copy IS this install's" `
    (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'bundled' -GameDir $managed -DataRoot $root)
Check "an existing-game run pointed at that same path is still not ours" `
    (-not (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'external' -GameDir $managed -DataRoot $root))
Check "a portable run whose game folder is not the managed runtime is not ours" `
    (-not (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'bundled' -GameDir $chosen -DataRoot $root))
Check "a portable run pointed at the data root itself is not ours" `
    (-not (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'bundled' -GameDir $root -DataRoot $root))

# THE UPGRADE. A framework this package unpacked into somebody's own game folder
# on install one is still this install's on install two: the previous install
# record says so, and it is the only evidence that exists in a folder this
# installer does not own. Without it the second install writes an empty file list
# and the uninstall leaves the whole framework - its log, its config and its
# cache - in a game folder it promised to put back exactly as it found it.
Check "one a previous install of this package put in a chosen game folder IS ours" `
    (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'external' -GameDir $chosen -DataRoot $root `
        -PreviouslyOurs $true)
Check "and a stranger's is still a stranger's when no record claims it" `
    (-not (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'external' -GameDir $chosen -DataRoot $root `
        -PreviouslyOurs $false))
Check "the managed game copy needs no record to be ours" `
    (Test-InstallerOwnedBepInEx -Present $true -RuntimeMode 'bundled' -GameDir $managed -DataRoot $root `
        -PreviouslyOurs $false)

# ---------------------------------------------------------------------------
# A DATA ROOT IS SAFE TO ACT ON, OR IT IS NOT. Test-SafeDataRoot answers that for
# a path a launcher profile names, and it refuses any data root that CONTAINS a
# protected path - the game folder among them. The complete edition's game folder
# is inside the data root by design, so the two rules have to be read together;
# they are driven together here for the same reason.
& {
    . (Get-ScriptFunction $uninstaller 'Test-ManagedRuntimePath')
    . (Get-ScriptFunction $uninstaller 'Get-ProtectedRoot')
    . (Get-ScriptFunction $uninstaller 'Test-SafeDataRoot')

    $user     = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), 'someone')
    $data     = [System.IO.Path]::Combine($user, 'BibitesMultiverse')
    $data2    = [System.IO.Path]::Combine($user, 'BibitesMultiverse-2')
    $runtime  = [System.IO.Path]::Combine($data, 'runtimes', ('A' * 64))
    $program  = [System.IO.Path]::Combine($user, 'Programs', 'Bibites Multiverse')
    $profiles = [System.IO.Path]::Combine($program, 'profiles')
    $steam    = [System.IO.Path]::Combine($user, 'Steam', 'The Bibites')
    $inRoot   = [System.IO.Path]::Combine($data, 'game')

    $complete = @(Get-ProtectedRoot @($runtime, $program, $profiles, $user) $data)
    $addOn    = @(Get-ProtectedRoot @($steam, $program, $profiles, $user) $data)
    $inside   = @(Get-ProtectedRoot @($inRoot, $program, $profiles, $user) $data)

    Scenario "which protected paths survive Get-ProtectedRoot"
    Check "the managed game copy this run reclaims is dropped" ($complete.Count -eq 3)
    Check "and nothing else is" `
        ($complete -contains $program -and $complete -contains $profiles -and $complete -contains $user)
    Check "a game folder somebody chose is kept, wherever it is" ($addOn.Count -eq 4)
    Check "including one that happens to sit inside the data root" ($inside.Count -eq 4)
    Check "a runtime under a DIFFERENT data root is not this run's to drop" `
        ((@(Get-ProtectedRoot @($runtime) $data2)).Count -eq 1)
    Check "an empty entry is dropped, as it always was" `
        ((@(Get-ProtectedRoot @('', $program) $data)).Count -eq 1)

    Scenario "which data roots a profile may name"
    Check "the complete edition's own data root is one this script may act on" `
        (Test-SafeDataRoot $data $complete)
    # The rule this replaced, kept as a check because it is the whole bug: the
    # game folder read raw made the data root that CONTAINS it refuse for ever.
    Check "and it was not, while the game copy inside it was still protected" `
        (-not (Test-SafeDataRoot $data @($runtime, $program, $profiles, $user)))
    Check "an add-on install's data root is safe too" (Test-SafeDataRoot $data $addOn)
    Check "a data root holding a game somebody chose is still refused" `
        (-not (Test-SafeDataRoot $data $inside))
    Check "a second world's own data root is safe" (Test-SafeDataRoot $data2 $complete)
    Check "the application directory is never a data root" (-not (Test-SafeDataRoot $program $complete))
    Check "nor the profiles directory" (-not (Test-SafeDataRoot $profiles $complete))
    Check "nor the user's own profile folder, which holds both" (-not (Test-SafeDataRoot $user $complete))
    Check "nor a relative path" (-not (Test-SafeDataRoot 'BibitesMultiverse' $complete))
    Check "nor a drive root" (-not (Test-SafeDataRoot ([System.IO.Path]::GetPathRoot($data)) $complete))
}

Write-Host ""
if ($script:failures -gt 0) {
    Write-Host ("!! {0} of {1} checks failed" -f $script:failures, $script:checks)
    exit 1
}
Write-Host ("    {0} checks pass" -f $script:checks)
exit 0
