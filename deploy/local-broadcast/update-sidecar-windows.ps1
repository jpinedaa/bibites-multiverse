[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet(
        'AssertRoot',
        'AssertNoReparse',
        'FileMetadata',
        'PathAcl',
        'GetAcl',
        'SetAcl',
        'ProtectDirectory',
        'AtomicReplace',
        'ProcessCounts',
        'ValidateInstall'
    )]
    [string]$Operation,
    [string]$Path = '',
    [string]$Root = '',
    [string]$Source = '',
    [string]$Destination = '',
    [string]$AclBase64 = '',
    [string]$ExpectedPeerId = ''
)

$ErrorActionPreference = 'Stop'

function Get-NormalPath([string]$Value) {
    if (-not $Value) { throw 'A required Windows path is empty' }
    $full = [System.IO.Path]::GetFullPath($Value)
    $pathRoot = [System.IO.Path]::GetPathRoot($full)
    if ($full.Length -gt $pathRoot.Length) { return $full.TrimEnd('\') }
    return $full
}

function Assert-SamePath([string]$Actual, [string]$Expected, [string]$Name) {
    $actualPath = Get-NormalPath $Actual
    $expectedPath = Get-NormalPath $Expected
    if (-not $actualPath.Equals($expectedPath, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "$Name is $actualPath, not $expectedPath"
    }
}

function Assert-NoReparsePath([string]$Value) {
    $current = Get-NormalPath $Value
    $pathRoot = [System.IO.Path]::GetPathRoot($current)
    while ($current) {
        $item = Get-Item -LiteralPath $current -Force
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "A Windows reparse point is not permitted: $current"
        }
        if ($current.Equals($pathRoot, [System.StringComparison]::OrdinalIgnoreCase)) { break }
        $parent = Split-Path -Parent $current
        if (-not $parent -or $parent -eq $current) { break }
        $current = Get-NormalPath $parent
    }
}

function Get-PrivateProcessCounts([string]$BroadcastRoot) {
    $gamePath = Join-Path $BroadcastRoot 'game\The Bibites.exe'
    $sidecarPath = Join-Path $BroadcastRoot 'multiverse-sidecar.exe'
    $obsPath = Join-Path $BroadcastRoot 'obs\bin\64bit\obs64.exe'
    $runnerPath = Join-Path $BroadcastRoot 'run-windows.ps1'
    $watcherPath = Join-Path $BroadcastRoot 'watch-viewers.ps1'

    $counts = [ordered]@{ game = 0; sidecar = 0; obs = 0; runner = 0; watcher = 0 }
    foreach ($process in @(Get-Process -ErrorAction SilentlyContinue)) {
        $processPath = ''
        try { $processPath = [string]$process.Path } catch {}
        if ($processPath.Equals($gamePath, [System.StringComparison]::OrdinalIgnoreCase)) {
            $counts.game++
        }
        if ($processPath.Equals($sidecarPath, [System.StringComparison]::OrdinalIgnoreCase)) {
            $counts.sidecar++
        }
        if ($processPath.Equals($obsPath, [System.StringComparison]::OrdinalIgnoreCase)) {
            $counts.obs++
        }
    }

    foreach ($process in @(Get-CimInstance -ClassName Win32_Process)) {
        $commandLine = [string]$process.CommandLine
        if ($commandLine.IndexOf($runnerPath, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) {
            $counts.runner++
        }
        if ($commandLine.IndexOf($watcherPath, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) {
            $counts.watcher++
        }
    }
    return $counts
}

switch ($Operation) {
    'AssertRoot' {
        $expected = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) `
            'BibitesMultiverse\broadcast'
        Assert-SamePath -Actual $Root -Expected $expected -Name 'The broadcast root'
        Assert-NoReparsePath $Root
    }
    'AssertNoReparse' {
        Assert-NoReparsePath $Path
    }
    'FileMetadata' {
        Assert-NoReparsePath $Path
        $item = Get-Item -LiteralPath $Path -Force
        if ($item.PSIsContainer) { throw "The static path is not a file: $Path" }
        $hash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
        $sddl = (Get-Acl -LiteralPath $Path).Sddl
        $acl = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sddl))
        Write-Output "$hash`t$acl"
    }
    'PathAcl' {
        Assert-NoReparsePath $Path
        $null = Get-Item -LiteralPath $Path -Force
        $sddl = (Get-Acl -LiteralPath $Path).Sddl
        Write-Output ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sddl)))
    }
    'GetAcl' {
        Assert-NoReparsePath $Path
        $sddl = (Get-Acl -LiteralPath $Path).Sddl
        Write-Output ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sddl)))
    }
    'SetAcl' {
        Assert-NoReparsePath $Path
        if (-not $AclBase64) { throw 'The saved ACL is empty' }
        $sddl = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($AclBase64))
        $security = New-Object System.Security.AccessControl.FileSecurity
        $security.SetSecurityDescriptorSddlForm(
            $sddl,
            [System.Security.AccessControl.AccessControlSections]::All
        )
        [System.IO.File]::SetAccessControl($Path, $security)
        $actual = (Get-Acl -LiteralPath $Path).Sddl
        if ($actual -ne $sddl) { throw "Windows did not restore the exact ACL on $Path" }
    }
    'ProtectDirectory' {
        Assert-NoReparsePath $Path
        $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
        foreach ($item in @(
            Get-Item -LiteralPath $Path -Force
            Get-ChildItem -LiteralPath $Path -Force -Recurse
        )) {
            if ($item.PSIsContainer) {
                $security = New-Object System.Security.AccessControl.DirectorySecurity
                $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
                    $sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'
                )
            } else {
                $security = New-Object System.Security.AccessControl.FileSecurity
                $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
                    $sid, 'FullControl', 'Allow'
                )
            }
            $security.SetOwner($sid)
            $security.SetAccessRuleProtection($true, $false)
            $security.AddAccessRule($rule)
            $item.SetAccessControl($security)

            $actual = $item.GetAccessControl(
                [System.Security.AccessControl.AccessControlSections]::Access
            )
            $rules = @($actual.GetAccessRules(
                $true, $true, [System.Security.Principal.SecurityIdentifier]
            ))
            $other = @($rules | Where-Object {
                $_.AccessControlType -ne 'Allow' -or $_.IdentityReference -ne $sid
            })
            if (-not $actual.AreAccessRulesProtected -or $rules.Count -ne 1 -or $other.Count -ne 0) {
                throw "Windows did not protect $($item.FullName) for the current user"
            }
        }
    }
    'AtomicReplace' {
        Assert-NoReparsePath $Source
        Assert-NoReparsePath $Destination
        $sourceRoot = [System.IO.Path]::GetPathRoot((Get-NormalPath $Source))
        $destinationRoot = [System.IO.Path]::GetPathRoot((Get-NormalPath $Destination))
        if (-not $sourceRoot.Equals(
            $destinationRoot,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw 'The staged executable and installed executable are on different Windows volumes'
        }
        $backupPath = Join-Path ([System.IO.Path]::GetDirectoryName($Source)) (
            '.atomic-replace-backup.' + [guid]::NewGuid().ToString('N')
        )
        [System.IO.File]::Replace($Source, $Destination, $backupPath, $true)
        Remove-Item -LiteralPath $backupPath -Force
        if (Test-Path -LiteralPath $backupPath) {
            throw 'The temporary atomic-replacement backup remains'
        }
    }
    'ProcessCounts' {
        Assert-NoReparsePath $Root
        Get-PrivateProcessCounts $Root | ConvertTo-Json -Compress
    }
    'ValidateInstall' {
        Assert-NoReparsePath $Root
        $configPath = Join-Path $Root 'config.env'
        Assert-NoReparsePath $configPath
        $config = @{}
        foreach ($line in Get-Content -LiteralPath $configPath) {
            if (-not $line -or $line.StartsWith('#')) { continue }
            $pair = $line.Split('=', 2)
            if ($pair.Count -ne 2) { throw "The installed configuration has an invalid line: $configPath" }
            $config[$pair[0]] = $pair[1]
        }
        foreach ($name in @('GameDir', 'Obs', 'SidecarExe', 'DataRoot', 'PeerId', 'ExportEdges')) {
            if (-not $config.ContainsKey($name) -or -not $config[$name]) {
                throw "The installed configuration has no $name value: $configPath"
            }
        }
        Assert-SamePath $config.GameDir (Join-Path $Root 'game') 'GameDir'
        Assert-SamePath $config.Obs (Join-Path $Root 'obs\bin\64bit\obs64.exe') 'Obs'
        Assert-SamePath $config.SidecarExe (Join-Path $Root 'multiverse-sidecar.exe') 'SidecarExe'
        Assert-SamePath $config.DataRoot (Join-Path $Root 'multiverse') 'DataRoot'
        if ($config.PeerId -ne $ExpectedPeerId) {
            throw 'PeerId does not match the expected broadcast identity'
        }
        $edges = @($config.ExportEdges.Split(',') | ForEach-Object { $_.Trim() } | Sort-Object)
        if (($edges -join ',') -ne 'E,N,S,W') {
            throw 'ExportEdges does not contain E, N, W, and S exactly once'
        }
    }
}
