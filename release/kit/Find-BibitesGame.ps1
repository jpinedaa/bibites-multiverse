Set-StrictMode -Version 2.0

function Test-BibitesGameDirectory {
    param([string]$Path)
    if (-not $Path) { return $false }
    return (Test-Path -LiteralPath (Join-Path $Path 'The Bibites.exe') -PathType Leaf)
}

function Get-BibitesSteamRoots {
    $roots = New-Object System.Collections.ArrayList

    foreach ($key in @('HKCU:\Software\Valve\Steam', 'HKLM:\SOFTWARE\WOW6432Node\Valve\Steam',
                       'HKLM:\SOFTWARE\Valve\Steam')) {
        try {
            $item = Get-ItemProperty -Path $key -ErrorAction Stop
        } catch {
            continue
        }
        foreach ($name in @('InstallPath', 'SteamPath')) {
            $value = $null
            if ($item.PSObject.Properties.Match($name).Count -gt 0) { $value = $item.$name }
            if ($value) { [void]$roots.Add(([string]$value).Replace('/', '\')) }
        }
    }

    foreach ($guess in @("${env:ProgramFiles(x86)}\Steam", "$env:ProgramFiles\Steam",
                         "$env:SystemDrive\Steam", "$env:SystemDrive\SteamLibrary")) {
        if ($guess) { [void]$roots.Add($guess) }
    }

    $extra = New-Object System.Collections.ArrayList
    foreach ($root in @($roots)) {
        $vdf = Join-Path $root 'steamapps\libraryfolders.vdf'
        if (-not (Test-Path -LiteralPath $vdf)) { continue }
        foreach ($line in (Get-Content -LiteralPath $vdf)) {
            $match = [regex]::Match($line, '"path"\s+"(.+?)"')
            if ($match.Success) { [void]$extra.Add($match.Groups[1].Value.Replace('\\', '\')) }
        }
    }
    foreach ($root in $extra) { [void]$roots.Add($root) }

    return @($roots | Where-Object { $_ } | Select-Object -Unique)
}

function Find-BibitesGameDirectory {
    $candidates = New-Object System.Collections.ArrayList
    foreach ($root in (Get-BibitesSteamRoots)) {
        [void]$candidates.Add((Join-Path $root 'steamapps\common\The Bibites'))
    }

    if ($env:APPDATA) {
        [void]$candidates.Add((Join-Path $env:APPDATA 'itch\apps\the-bibites'))
        [void]$candidates.Add((Join-Path $env:APPDATA 'itch\apps\The Bibites'))
    }
    foreach ($code in 67..90) {
        $letter = [char]$code
        $drive = $letter + ':\'
        if (-not (Test-Path -LiteralPath $drive -PathType Container)) { continue }
        [void]$candidates.Add($letter + ':\SteamLibrary\steamapps\common\The Bibites')
        [void]$candidates.Add($letter + ':\Games\The Bibites')
        [void]$candidates.Add($letter + ':\The Bibites')
    }

    foreach ($candidate in @($candidates | Select-Object -Unique)) {
        if (Test-BibitesGameDirectory $candidate) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    return ''
}
