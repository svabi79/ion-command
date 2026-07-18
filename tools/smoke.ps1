param([int]$RequiredRadioLinks = 100)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$collectorRoot = Join-Path $repositoryRoot 'collector'
$dataDirectory = Join-Path $repositoryRoot 'data\smoke'
New-Item -ItemType Directory -Path $dataDirectory -Force | Out-Null
& (Join-Path $PSScriptRoot 'build.ps1')
$binary = Join-Path $collectorRoot 'bin\ion-collector.exe'
$process = Start-Process -FilePath $binary -WorkingDirectory $collectorRoot -ArgumentList @('-config','configs/development.json') -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $dataDirectory 'collector-stdout.log') -RedirectStandardError (Join-Path $dataDirectory 'collector-stderr.log')
try {
    $ready = $false
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        if ($process.HasExited) { throw "Collector exited early with code $($process.ExitCode)" }
        try { $health = Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/health' -TimeoutSec 1; $ready = $true; break } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $ready -or $health.status -ne 'ok') { throw 'Collector did not become healthy' }

    $socket = [System.Net.WebSockets.ClientWebSocket]::new()
    $timeout = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(15))
    $null = $socket.ConnectAsync([Uri]'ws://127.0.0.1:7810/ws/live', $timeout.Token).GetAwaiter().GetResult()
    $radioLinks = 0
    while ($radioLinks -lt $RequiredRadioLinks) {
        $buffer = [byte[]]::new(65536)
        $result = $socket.ReceiveAsync([ArraySegment[byte]]::new($buffer), $timeout.Token).GetAwaiter().GetResult()
        if ($result.MessageType -ne [System.Net.WebSockets.WebSocketMessageType]::Text) { continue }
        $event = [Text.Encoding]::UTF8.GetString($buffer, 0, $result.Count) | ConvertFrom-Json
        if ($event.semanticType -eq 'radio.reception') { $radioLinks++ }
    }
    $socket.Dispose()
    Start-Sleep -Milliseconds 1200

    $stats = Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/stats'
    $sources = Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/sources'
    $range = Invoke-RestMethod -Uri 'http://127.0.0.1:7810/api/replay/range'
    if ($stats.invalidEvents -ne 0) { throw "Collector reported $($stats.invalidEvents) invalid events" }
    if (($sources | Where-Object state -ne 'active').Count -ne 0) { throw 'At least one source is not active' }
    if (-not $range.available) { throw 'Recording replay range is not available' }

    $replaySocket = [System.Net.WebSockets.ClientWebSocket]::new()
    $replayTimeout = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(5))
    $null = $replaySocket.ConnectAsync([Uri]'ws://127.0.0.1:7810/ws/replay?speed=1000', $replayTimeout.Token).GetAwaiter().GetResult()
    $replayBuffer = [byte[]]::new(65536)
    $replayResult = $replaySocket.ReceiveAsync([ArraySegment[byte]]::new($replayBuffer), $replayTimeout.Token).GetAwaiter().GetResult()
    $replayed = [Text.Encoding]::UTF8.GetString($replayBuffer, 0, $replayResult.Count) | ConvertFrom-Json
    $replaySocket.Dispose()

    [PSCustomObject]@{
        Health = $health.status
        RadioLinksObserved = $radioLinks
        CanonicalPublished = $stats.canonicalPublished
        InvalidEvents = $stats.invalidEvents
        DroppedClientMessages = $stats.droppedClientMessages
        SourcesActive = ($sources | Where-Object state -eq 'active').Count
        ReplayFiles = $range.fileCount
        ReplayedMessageId = $replayed.messageId
    } | Format-List
} finally {
    if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force }
}

