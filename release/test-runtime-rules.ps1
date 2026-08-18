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
                                somebody else chose: no. In this package's own
                                managed game copy: always - this installer put
                                it there itself, one run earlier. Answering that
                                wrong is what left a game folder holding 27 files
                                of BepInEx and no game, which the next install
                                refused to overwrite (INS-RUNTIME).

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

Write-Host ""
if ($script:failures -gt 0) {
    Write-Host ("!! {0} of {1} checks failed" -f $script:failures, $script:checks)
    exit 1
}
Write-Host ("    {0} checks pass" -f $script:checks)
exit 0
