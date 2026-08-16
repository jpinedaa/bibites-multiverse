# Start and stop the RTMP publish from the service's viewer-presence signal.
#
# WHY THIS EXISTS. The publisher costs approximately 780 GB of inbound transfer
# each month whether or not one person is watching, which is a quarter of the
# service's whole transfer allowance spent on an empty room. The service host
# cannot start or stop this publisher: the publisher is OBS on a Windows machine
# that the host holds no credential for, so MediaMTX's own `runOnDemand` cannot
# reach it. The host publishes the audience instead, and this script reads it.
#
#   GET https://<service-domain>/api/viewers
#   {"watching":bool,"hlsSessions":int,"lastViewerRequestAgeSec":int|null,
#    "asOf":"<UTC ISO 8601>"}
#
# Read `docs/live-broadcast.md`, "Publish on demand", for the endpoint contract.
# This script owns only the publisher half of it. Its own poll is not an
# audience: the service counts requests for `/watch` and `/stream/`, and this
# watcher reads neither, so it cannot hold itself up.
#
# WHY obs-websocket AND NOT AN OBS RESTART. OBS keeps running while nobody
# watches. Only its stream output stops, so the scene keeps its game-capture
# hook and the next start is one request instead of an application launch. The
# other reason is safety: `run-windows.ps1` treats an OBS exit as a fault of the
# whole trio and restarts the game with it, so stopping OBS for an empty room
# would reload the world every time the audience left.
#
# WHY A FAILED READING NEVER CHANGES THE STREAM. An unreachable endpoint, an
# HTTP error, an unparsable document and a frozen `asOf` all read as "unknown",
# and unknown holds the current state. The alternative -- reading a fault as an
# empty room -- takes a live broadcast off the air because a status timer
# stopped.

[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'Low')]
param(
    # The service's viewer-presence document. `run-windows.ps1` derives this from
    # the configured relay address so the watcher follows the deployed topology.
    [string]$PresenceUrl = 'https://bibitesmultiverse.com/api/viewers',
    # The portable OBS configuration root, `...\obs\config\obs-studio`. The
    # obs-websocket port and password are read from its plugin configuration, so
    # no secret is ever passed on a command line.
    [string]$ObsConfigRoot = '',
    [string]$ObsWebSocketHost = '127.0.0.1',
    # Zero means "read the port from the obs-websocket configuration".
    [int]$ObsWebSocketPort = 0,
    [int]$PollSeconds = 10,
    # A stop needs this many seconds of unbroken "nobody is watching". One
    # reading of `watching` cancels it, so a viewer who reloads a page does not
    # take the stream down.
    [int]$IdleStopSeconds = 180,
    # A presence document older than this is a stopped status timer, not an
    # answer. Read it as unknown.
    [int]$StaleAfterSeconds = 120,
    # How long an unbroken run of unknown readings may last before the log gets
    # an alert line, and how often that line repeats.
    [int]$UnknownAlertSeconds = 300,
    [int]$HttpTimeoutSeconds = 5,
    [int]$ObsTimeoutSeconds = 5,
    [string]$LogFile = '',
    # The process whose death ends this watcher. It is the `run-windows.ps1`
    # runner, so the watcher can never outlive the trio it belongs to.
    [int]$SupervisorProcessId = 0,
    # One cycle and exit. For tests.
    [switch]$Once,
    # The simulated starting stream state under -WhatIf, where no OBS is reached.
    [switch]$AssumeStreaming,
    # Pure checks with no network and no OBS. For tests.
    [switch]$SelfTest
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# PowerShell 5.1 negotiates TLS 1.0 by default, which the service refuses.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$script:LogPath = $LogFile
$script:LogSizeLimit = 4MB

function Write-WatcherLine {
    param([string]$Level, [string]$Message)

    $line = '{0} {1} {2}' -f [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'), $Level, $Message
    Write-Host $line
    if (-not $script:LogPath) { return }
    try {
        $existing = Get-Item -LiteralPath $script:LogPath -ErrorAction SilentlyContinue
        if ($existing -and $existing.Length -gt $script:LogSizeLimit) {
            Move-Item -LiteralPath $script:LogPath -Destination ($script:LogPath + '.1') -Force
        }
        Add-Content -LiteralPath $script:LogPath -Value $line -Encoding UTF8
    } catch {
        # A log that cannot be written must not stop the publisher from being
        # started or stopped correctly.
    }
}

# ------------------------------------------------------------------ presence

function Get-ViewerPresence {
    param([string]$Url, [int]$TimeoutSeconds, [int]$StaleSeconds)

    $reading = @{ Known = $false; Watching = $false; Reason = ''; Detail = '' }
    $content = ''
    try {
        $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec $TimeoutSeconds
        if ([int]$response.StatusCode -ne 200) {
            $reading.Reason = 'the presence endpoint answered HTTP {0}' -f [int]$response.StatusCode
            return $reading
        }
        $content = [string]$response.Content
    } catch [System.Net.WebException] {
        $status = 'no response'
        if ($_.Exception.Response) { $status = 'HTTP {0}' -f [int]$_.Exception.Response.StatusCode }
        $reading.Reason = 'the presence endpoint could not be read ({0}): {1}' -f $status, $_.Exception.Message
        return $reading
    } catch {
        $reading.Reason = 'the presence request failed: {0}' -f $_.Exception.Message
        return $reading
    }

    $document = $null
    try {
        $document = $content | ConvertFrom-Json
    } catch {
        $reading.Reason = 'the presence document is not JSON'
        return $reading
    }
    if ($null -eq $document -or $null -eq $document.watching -or $document.watching -isnot [bool]) {
        $reading.Reason = 'the presence document has no boolean "watching" field'
        return $reading
    }

    # A frozen document is a stopped status timer. Reading it as an answer would
    # hold a stale audience, or take the stream down for an audience the timer
    # never got to record.
    $asOf = [string]$document.asOf
    if ($asOf) {
        $age = Get-DocumentAge -Stamp $asOf
        if ($null -eq $age) {
            $reading.Reason = 'the presence document has an unreadable asOf value: {0}' -f $asOf
            return $reading
        }
        if ($age -gt $StaleSeconds) {
            $reading.Reason = 'the presence document is {0} seconds old, over the {1} second limit' -f `
                [int]$age, $StaleSeconds
            return $reading
        }
    }

    $reading.Known = $true
    $reading.Watching = [bool]$document.watching
    $reading.Detail = 'watching={0} hlsSessions={1} lastViewerRequestAgeSec={2} asOf={3}' -f `
        $document.watching, $document.hlsSessions, $document.lastViewerRequestAgeSec, $asOf
    return $reading
}

function Get-DocumentAge {
    param([string]$Stamp)

    $parsed = [DateTime]::MinValue
    $styles = [System.Globalization.DateTimeStyles]::AdjustToUniversal -bor `
              [System.Globalization.DateTimeStyles]::AssumeUniversal
    if (-not [DateTime]::TryParse($Stamp, [System.Globalization.CultureInfo]::InvariantCulture,
                                  $styles, [ref]$parsed)) {
        return $null
    }
    $age = ([DateTime]::UtcNow - $parsed).TotalSeconds
    # A clock step can put the reading in the future. Zero is the honest reading
    # of that, and the same forgiveness the service gives its own access log.
    if ($age -lt 0) { return 0.0 }
    return $age
}

# --------------------------------------------------------------- obs-websocket

function Get-ObsWebSocketConfig {
    param([string]$ConfigRoot, [int]$PortOverride)

    $settings = @{ Port = $PortOverride; Password = '' }
    if (-not $ConfigRoot) {
        if ($settings.Port -le 0) { throw 'no obs-websocket configuration root and no port was given' }
        return $settings
    }
    $path = Join-Path $ConfigRoot 'plugin_config\obs-websocket\config.json'
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "there is no obs-websocket configuration at $path"
    }
    $document = (Get-Content -LiteralPath $path -Raw) | ConvertFrom-Json
    if (-not $document.server_enabled) {
        throw "obs-websocket is disabled in $path, and OBS reads that file only at start"
    }
    if ($settings.Port -le 0) { $settings.Port = [int]$document.server_port }
    if ($document.auth_required) { $settings.Password = [string]$document.server_password }
    if ($settings.Port -le 0) { throw "obs-websocket has no server port in $path" }
    return $settings
}

function Get-ObsAuthentication {
    param([string]$Password, [string]$Salt, [string]$Challenge)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $secret = [Convert]::ToBase64String(
            $sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Password + $Salt)))
        return [Convert]::ToBase64String(
            $sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($secret + $Challenge)))
    } finally {
        $sha.Dispose()
    }
}

function Receive-ObsMessage {
    param($Socket, [int]$TimeoutSeconds)

    $source = New-Object System.Threading.CancellationTokenSource -ArgumentList ($TimeoutSeconds * 1000)
    try {
        $builder = New-Object System.Text.StringBuilder
        $buffer = New-Object byte[] 65536
        do {
            $segment = New-Object 'System.ArraySegment[byte]' -ArgumentList $buffer, 0, $buffer.Length
            $result = $Socket.ReceiveAsync($segment, $source.Token).GetAwaiter().GetResult()
            if ($result.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) {
                throw 'obs-websocket closed the connection'
            }
            [void]$builder.Append([Text.Encoding]::UTF8.GetString($buffer, 0, $result.Count))
        } while (-not $result.EndOfMessage)
        return ($builder.ToString() | ConvertFrom-Json)
    } finally {
        $source.Dispose()
    }
}

function Send-ObsMessage {
    param($Socket, $Message, [int]$TimeoutSeconds)

    $source = New-Object System.Threading.CancellationTokenSource -ArgumentList ($TimeoutSeconds * 1000)
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes((ConvertTo-Json -InputObject $Message -Depth 6 -Compress))
        $segment = New-Object 'System.ArraySegment[byte]' -ArgumentList $bytes, 0, $bytes.Length
        $Socket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true,
            $source.Token).GetAwaiter().GetResult() | Out-Null
    } finally {
        $source.Dispose()
    }
}

function Close-ObsWebSocket {
    param($Socket)

    if (-not $Socket) { return }
    try {
        if ($Socket.State -eq [System.Net.WebSockets.WebSocketState]::Open) {
            $source = New-Object System.Threading.CancellationTokenSource -ArgumentList 2000
            $Socket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure,
                'done', $source.Token).GetAwaiter().GetResult() | Out-Null
            $source.Dispose()
        }
    } catch {
        # A connection that will not close politely is dropped instead.
    } finally {
        $Socket.Dispose()
    }
}

function Connect-ObsWebSocket {
    param([string]$Address, [int]$Port, [string]$Password, [int]$TimeoutSeconds)

    $socket = New-Object System.Net.WebSockets.ClientWebSocket
    $socket.Options.AddSubProtocol('obswebsocket.json')
    $source = New-Object System.Threading.CancellationTokenSource -ArgumentList ($TimeoutSeconds * 1000)
    try {
        $socket.ConnectAsync([Uri]"ws://${Address}:$Port", $source.Token).GetAwaiter().GetResult()
    } catch {
        $socket.Dispose()
        throw
    } finally {
        $source.Dispose()
    }

    try {
        $hello = Receive-ObsMessage -Socket $socket -TimeoutSeconds $TimeoutSeconds
        if ([int]$hello.op -ne 0) { throw "obs-websocket did not open with Hello (op $($hello.op))" }

        # eventSubscriptions 0 keeps this a request-and-answer connection, so no
        # event ever arrives between a request and its response.
        $identify = @{ op = 1; d = @{ rpcVersion = 1; eventSubscriptions = 0 } }
        if ($hello.d.authentication) {
            if (-not $Password) {
                throw 'obs-websocket asked for authentication and no password was found'
            }
            $identify.d.authentication = Get-ObsAuthentication -Password $Password `
                -Salt ([string]$hello.d.authentication.salt) `
                -Challenge ([string]$hello.d.authentication.challenge)
        }
        Send-ObsMessage -Socket $socket -Message $identify -TimeoutSeconds $TimeoutSeconds
        $identified = Receive-ObsMessage -Socket $socket -TimeoutSeconds $TimeoutSeconds
        if ([int]$identified.op -ne 2) {
            throw "obs-websocket refused this connection (op $($identified.op))"
        }
        return $socket
    } catch {
        Close-ObsWebSocket -Socket $socket
        throw
    }
}

function Invoke-ObsRequest {
    param($Socket, [string]$RequestType, [int]$TimeoutSeconds)

    $requestId = [guid]::NewGuid().ToString('N')
    Send-ObsMessage -Socket $Socket -TimeoutSeconds $TimeoutSeconds -Message @{
        op = 6
        d  = @{ requestType = $RequestType; requestId = $requestId }
    }
    for ($attempt = 0; $attempt -lt 8; $attempt++) {
        $message = Receive-ObsMessage -Socket $Socket -TimeoutSeconds $TimeoutSeconds
        if ([int]$message.op -ne 7) { continue }
        if ([string]$message.d.requestId -ne $requestId) { continue }
        if (-not $message.d.requestStatus.result) {
            throw "$RequestType failed with obs-websocket code $($message.d.requestStatus.code)"
        }
        return $message.d.responseData
    }
    throw "obs-websocket never answered $RequestType"
}

# ------------------------------------------------------------------- the loop

function Set-StreamState {
    param([bool]$Wanted, [string]$Because)

    if ($WhatIfPreference) {
        # -WhatIf reaches no OBS at all. It carries a simulated stream state, so
        # the start and stop decisions are still exercised from end to end.
        if ($script:SimulatedStreaming -eq $Wanted) { return }
        $simulated = if ($Wanted) { 'start' } else { 'stop' }
        Write-WatcherLine 'WHATIF' "would $simulated the stream because $Because"
        $script:SimulatedStreaming = $Wanted
        return
    }

    $socket = $null
    try {
        $socket = Connect-ObsWebSocket -Address $ObsWebSocketHost -Port $script:ObsPort `
            -Password $script:ObsPassword -TimeoutSeconds $ObsTimeoutSeconds
        $status = Invoke-ObsRequest -Socket $socket -RequestType 'GetStreamStatus' `
            -TimeoutSeconds $ObsTimeoutSeconds
        $active = [bool]$status.outputActive
        $script:LastKnownStreaming = $active
        if ($active -eq $Wanted) { return }

        $action = if ($Wanted) { 'start' } else { 'stop' }
        $requestType = if ($Wanted) { 'StartStream' } else { 'StopStream' }
        if ($PSCmdlet.ShouldProcess('the OBS stream output', $action)) {
            Invoke-ObsRequest -Socket $socket -RequestType $requestType `
                -TimeoutSeconds $ObsTimeoutSeconds | Out-Null
            $script:LastKnownStreaming = $Wanted
            Write-WatcherLine 'INFO' "sent $requestType because $Because"
        }
    } catch {
        # OBS being unreachable changes nothing about the audience. The idle
        # timer keeps running, so the decision is applied by the next cycle that
        # reaches OBS.
        Write-WatcherLine 'WARN' "obs-websocket did not answer: $($_.Exception.Message)"
    } finally {
        Close-ObsWebSocket -Socket $socket
    }
}

if ($SelfTest) {
    # No published example vector is trusted here. `test-watch-viewers.sh`
    # recomputes this digest independently, so two implementations of the
    # obs-websocket handshake have to agree before the test passes.
    $digest = Get-ObsAuthentication -Password 'supersecretpassword' `
        -Salt 'lM1GncleQOaCu9lT1yeUZhFYnqhsLLP1G5lAGo3ixaI=' `
        -Challenge '+IxH4CnCiqpX1rM9scsNynZzbOe4KhDeYcTNS3PDaeY='
    Write-Host "authDigest=$digest"
    Write-Host ('staleAge={0}' -f [int](Get-DocumentAge -Stamp '2000-01-01T00:00:00Z'))
    Write-Host ('futureAge={0}' -f [int](Get-DocumentAge -Stamp '2999-01-01T00:00:00Z'))
    Write-Host ('unreadableAge={0}' -f (Get-DocumentAge -Stamp 'not a time'))
    Write-Host 'selfTest=PASS'
    exit 0
}

if ($WhatIfPreference) {
    $script:ObsPort = 0
    $script:ObsPassword = ''
} else {
    $settings = Get-ObsWebSocketConfig -ConfigRoot $ObsConfigRoot -PortOverride $ObsWebSocketPort
    $script:ObsPort = [int]$settings.Port
    $script:ObsPassword = [string]$settings.Password
}

$script:SimulatedStreaming = [bool]$AssumeStreaming
$script:LastKnownStreaming = $null
$idleSince = $null
$unknownSince = $null
$lastAlert = $null
$lastHeartbeat = [DateTime]::UtcNow

Write-WatcherLine 'INFO' ('watcher started presence={0} poll={1}s idleStop={2}s obsPort={3} whatIf={4} supervisor={5}' -f `
    $PresenceUrl, $PollSeconds, $IdleStopSeconds, $script:ObsPort, [bool]$WhatIfPreference, $SupervisorProcessId)

while ($true) {
    if ($SupervisorProcessId -gt 0) {
        if (-not (Get-Process -Id $SupervisorProcessId -ErrorAction SilentlyContinue)) {
            # The trio's runner is gone. Leave the stream exactly as it is: a
            # publisher outliving its supervisor is a fault for `bin/stop` to
            # resolve, not a reason to take a live broadcast off the air.
            Write-WatcherLine 'INFO' 'the supervisor process is gone, so the watcher is stopping'
            break
        }
    }

    $now = [DateTime]::UtcNow
    $presence = Get-ViewerPresence -Url $PresenceUrl -TimeoutSeconds $HttpTimeoutSeconds `
        -StaleSeconds $StaleAfterSeconds

    if (-not $presence.Known) {
        if ($null -eq $unknownSince) {
            $unknownSince = $now
            Write-WatcherLine 'WARN' ('presence unknown, holding the current state: {0}' -f $presence.Reason)
        }
        $unknownFor = ($now - $unknownSince).TotalSeconds
        $alertDue = ($null -eq $lastAlert) -or (($now - $lastAlert).TotalSeconds -ge $UnknownAlertSeconds)
        if ($unknownFor -ge $UnknownAlertSeconds -and $alertDue) {
            Write-WatcherLine 'ALERT' ('the presence signal has been unreadable for {0} seconds: {1}' -f `
                [int]$unknownFor, $presence.Reason)
            $lastAlert = $now
        }
    } else {
        if ($null -ne $unknownSince) {
            Write-WatcherLine 'INFO' ('presence readable again after {0} seconds' -f `
                [int](($now - $unknownSince).TotalSeconds))
            $unknownSince = $null
            $lastAlert = $null
        }

        if ($presence.Watching) {
            if ($null -ne $idleSince) {
                Write-WatcherLine 'INFO' 'somebody is watching, so the idle timer is cancelled'
                $idleSince = $null
            }
            Set-StreamState -Wanted $true -Because ('somebody is watching ({0})' -f $presence.Detail)
        } else {
            if ($null -eq $idleSince) {
                $idleSince = $now
                Write-WatcherLine 'INFO' ('nobody is watching, so the stream stops in {0} seconds unless somebody arrives' -f `
                    $IdleStopSeconds)
            }
            $idleFor = ($now - $idleSince).TotalSeconds
            if ($idleFor -ge $IdleStopSeconds) {
                Set-StreamState -Wanted $false -Because ('nobody watched for {0} seconds' -f [int]$idleFor)
            }
        }
    }

    if (($now - $lastHeartbeat).TotalSeconds -ge 300) {
        $observed = if ($WhatIfPreference) { $script:SimulatedStreaming } else { $script:LastKnownStreaming }
        Write-WatcherLine 'INFO' ('heartbeat streaming={0} presence={1}{2}' -f `
            $observed, $presence.Detail, $presence.Reason)
        $lastHeartbeat = $now
    }

    if ($Once) { break }
    Start-Sleep -Seconds $PollSeconds
}
