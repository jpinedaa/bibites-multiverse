<#
.SYNOPSIS
  Parse PowerShell files and fail on any syntax error.

.DESCRIPTION
  This repository ships PowerShell that a player runs on Windows, but it is
  written on a Linux box that has no pwsh. Until this check existed, a syntax
  error in an installer script was found by the first player who ran it.

  The parser is the same one PowerShell itself uses, so a file that passes here
  is a file PowerShell will accept. It only reads: nothing is executed, so this
  is safe to run over an installer.

  Arguments may be literal paths or wildcards; wildcards are expanded here
  because PowerShell does not glob the arguments of a -File invocation.

.EXAMPLE
  pwsh -NoProfile -File release/pscheck.ps1 release/kit/*.ps1 farend/setup-farend.ps1

.OUTPUTS
  One "<file>:<line>:<column>: <message>" line per parse error. Exit code 0 when
  every file parses, 1 when any file does not, 2 when the arguments name no file.
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$Path
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not $Path -or $Path.Count -eq 0) {
    Write-Error 'pscheck: no paths given'
    exit 2
}

$files = New-Object System.Collections.Generic.List[string]
$missing = New-Object System.Collections.Generic.List[string]

foreach ($pattern in $Path) {
    $resolved = @(Resolve-Path -Path $pattern -ErrorAction SilentlyContinue)
    if ($resolved.Count -eq 0) {
        $missing.Add($pattern)
        continue
    }
    foreach ($item in $resolved) {
        $full = $item.ProviderPath
        if (Test-Path -LiteralPath $full -PathType Leaf) {
            if (-not $files.Contains($full)) { $files.Add($full) }
        }
    }
}

if ($missing.Count -gt 0) {
    foreach ($m in $missing) {
        Write-Host "!! pscheck: nothing matches $m"
    }
    exit 2
}

if ($files.Count -eq 0) {
    Write-Host '!! pscheck: the arguments matched no file'
    exit 2
}

$root = (Get-Location).ProviderPath
$annotate = ($env:GITHUB_ACTIONS -eq 'true')
$errorCount = 0

foreach ($file in ($files | Sort-Object)) {
    $shown = $file
    if ($shown.StartsWith($root)) {
        $shown = $shown.Substring($root.Length).TrimStart('\', '/')
    }

    $tokens = $null
    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($file, [ref]$tokens, [ref]$errors)

    if ($errors -and $errors.Count -gt 0) {
        foreach ($e in $errors) {
            $line = $e.Extent.StartLineNumber
            $col = $e.Extent.StartColumnNumber
            $message = $e.Message -replace '\r?\n', ' '
            Write-Host "${shown}:${line}:${col}: $message"
            if ($annotate) {
                Write-Host "::error file=$shown,line=$line,col=$col::$message"
            }
            $errorCount++
        }
    } else {
        Write-Host "    ok  $shown"
    }
}

if ($errorCount -gt 0) {
    Write-Host ''
    Write-Host "!! pscheck: $errorCount parse error(s) in $($files.Count) file(s)"
    exit 1
}

Write-Host ''
Write-Host "    pscheck: $($files.Count) PowerShell file(s) parse"
exit 0
