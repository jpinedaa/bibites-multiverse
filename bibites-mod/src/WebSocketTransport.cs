using System;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Net.WebSockets;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace BibitesMultiverse
{
    internal enum TransportEventKind
    {
        /// <summary>A line the socket thread wants logged. Logging is deferred so no BepInEx call crosses a thread.</summary>
        Log,
        Connected,
        Frame,
        Disconnected
    }

    internal enum TransportLogLevel
    {
        Info,
        Warning,
        Error
    }

    internal class TransportEvent
    {
        internal TransportEventKind kind;
        internal string text;
        internal TransportLogLevel level;
        internal int closeCode;

        /// <summary>Which connection this event belongs to. See <see cref="WebSocketTransport.Generation"/>.</summary>
        internal int generation;
    }

    /// <summary>
    /// The Contract A client transport (§2, §6.2). Everything socket-shaped runs on a background
    /// task; the game thread only ever touches two concurrent queues.
    ///
    /// Transport choice: the BCL <see cref="ClientWebSocket"/>. The game ships Mono's System.dll,
    /// whose ClientWebSocket delegates to WebSocketHandle -> WebSocket.CreateClientWebSocket ->
    /// ManagedWebSocket.CreateFromConnectedStream — the fully managed .NET implementation, not a
    /// PlatformNotSupported stub. A hand-rolled RFC 6455 client would only be needed if that type
    /// were missing or stubbed; <see cref="ProbeAvailability"/> reports which case this build is in
    /// before the first connect, so the answer is in the log rather than in a crash.
    /// </summary>
    internal class WebSocketTransport
    {
        private readonly string url;
        private readonly ConcurrentQueue<TransportEvent> inbound = new ConcurrentQueue<TransportEvent>();
        private readonly ConcurrentQueue<string> outbound = new ConcurrentQueue<string>();
        private readonly SemaphoreSlim outboundSignal = new SemaphoreSlim(0);
        private readonly Random random = new Random();

        private CancellationTokenSource cancellation;
        private Task worker;
        private volatile bool halted;
        private volatile bool connected;
        private int generation;

        /// <summary>Bound on the send queue, so an unread socket cannot grow memory without limit.</summary>
        private const int MaxQueuedFrames = 256;

        /// <summary>
        /// How long a session must stay up before it counts as healthy and the backoff ladder starts
        /// again from its lowest rung (contract-a.md §13, amendment A8).
        ///
        /// Resetting on the bare TCP connect instead is what turns a repeated close-on-handshake —
        /// close 4003 for an unusable CONFIG_UPDATE is the one that matters — into a zero-delay
        /// redial loop, because the connect always succeeds and the ladder never climbs.
        /// </summary>
        private const int StableSessionMs = 5000;

        internal WebSocketTransport(string url)
        {
            this.url = url;
        }

        internal bool IsConnected => connected;

        /// <summary>
        /// Counts successful connects. It is what lets the client tell "the socket is up" from "the
        /// socket I handshook on is up": the <c>Connected</c> event is drained on the game thread one
        /// or more frames after the socket actually opened, and in that window a heartbeat aimed at
        /// the previous session would otherwise become the first frame of the new one — which the
        /// sidecar answers with close 4003, because CONFIG_UPDATE must come first (§5.1).
        /// </summary>
        internal int Generation => Volatile.Read(ref generation);

        /// <summary>True once a close code told us not to reconnect until the mod is restarted (§6.2).</summary>
        internal bool IsHalted => halted;

        /// <summary>
        /// Begin dialling. <paramref name="initialAttempt"/> above 0 makes the first connect wait out
        /// the backoff, which is what a reconnect after a fault has to do (§6.2).
        /// </summary>
        internal void Start(int initialAttempt = 0)
        {
            if (cancellation != null)
            {
                return;
            }

            halted = false;
            cancellation = new CancellationTokenSource();
            CancellationToken token = cancellation.Token;

            // Chain onto a previous run that is still unwinding, so two dial loops can never share
            // the two queues.
            Task previous = worker;
            worker = (previous == null || previous.IsCompleted)
                ? Task.Run(() => RunAsync(token, initialAttempt))
                : previous.ContinueWith(_ => RunAsync(token, initialAttempt), TaskScheduler.Default).Unwrap();
        }

        /// <summary>
        /// Stop dialling and close any live connection with <paramref name="closeCode"/>. Never blocks
        /// the caller: the close handshake finishes on the background task.
        /// </summary>
        internal void Stop(int closeCode, string reason)
        {
            if (cancellation == null)
            {
                return;
            }

            pendingCloseCode = closeCode;
            pendingCloseReason = reason;
            try
            {
                cancellation.Cancel();
            }
            catch (Exception)
            {
                // A disposed source means the worker already finished.
            }

            cancellation = null;
            connected = false;
        }

        /// <summary>Refuse to reconnect until the mod is restarted or reconfigured (close 4000/4001/4002/4006).</summary>
        internal void Halt()
        {
            halted = true;
        }

        internal void Send(string frame)
        {
            if (string.IsNullOrEmpty(frame))
            {
                return;
            }

            if (outbound.Count >= MaxQueuedFrames)
            {
                Report(TransportLogLevel.Warning, $"outbound queue is full ({MaxQueuedFrames}) — dropping a frame.");
                return;
            }

            outbound.Enqueue(frame);
            try
            {
                outboundSignal.Release();
            }
            catch (SemaphoreFullException)
            {
                // The send loop will pick the frame up on its next pass anyway.
            }
        }

        internal bool TryDequeue(out TransportEvent transportEvent)
        {
            return inbound.TryDequeue(out transportEvent);
        }

        /// <summary>
        /// Answer the "does the BCL WebSocket exist in this Mono?" question once, at startup, so a
        /// missing type is a log line rather than a TypeLoadException in the middle of a migration.
        /// </summary>
        internal static string ProbeAvailability()
        {
            try
            {
                using (ClientWebSocket probe = new ClientWebSocket())
                {
                    return "System.Net.WebSockets.ClientWebSocket available (state=" + probe.State + ")";
                }
            }
            catch (Exception e)
            {
                return "System.Net.WebSockets.ClientWebSocket UNAVAILABLE: " + e.GetType().Name + ": " + e.Message;
            }
        }

        // ---- background task ----------------------------------------------------------------

        private int pendingCloseCode = ContractA.CloseNormal;
        private string pendingCloseReason = "world unloaded";

        private async Task RunAsync(CancellationToken token, int initialAttempt)
        {
            int attempt = initialAttempt;
            Uri uri;
            try
            {
                uri = new Uri(url);
            }
            catch (Exception e)
            {
                Report(TransportLogLevel.Error, $"'{url}' is not a usable WebSocket URL: {e.Message}");
                return;
            }

            while (!token.IsCancellationRequested && !halted)
            {
                if (attempt > 0)
                {
                    int delay = FullJitterDelayMs(attempt);
                    try
                    {
                        await Task.Delay(delay, token).ConfigureAwait(false);
                    }
                    catch (OperationCanceledException)
                    {
                        break;
                    }
                }

                attempt++;
                int closeCode = ContractA.CloseNormal;
                string closeReason = "closed";
                Stopwatch session = new Stopwatch();

                ClientWebSocket socket = null;
                try
                {
                    socket = new ClientWebSocket();
                    await socket.ConnectAsync(uri, token).ConfigureAwait(false);

                    // Drain **after** the connect, not before it. Anything still queued belongs to the
                    // session that just died — including a heartbeat the game thread produced while it
                    // had not yet drained the Disconnected event — and §5.1 says the first frame on a
                    // new connection is CONFIG_UPDATE.
                    DrainOutbound();

                    Report(TransportLogLevel.Info, $"connected to {url} on backoff rung {attempt}.");
                    session.Start();
                    connected = true;
                    int thisGeneration = Interlocked.Increment(ref generation);
                    inbound.Enqueue(new TransportEvent { kind = TransportEventKind.Connected, generation = thisGeneration });

                    await PumpAsync(socket, token).ConfigureAwait(false);

                    closeCode = (socket.CloseStatus.HasValue ? (int)socket.CloseStatus.Value : ContractA.CloseNormal);
                    closeReason = socket.CloseStatusDescription ?? "closed";
                }
                catch (OperationCanceledException)
                {
                    closeCode = pendingCloseCode;
                    closeReason = pendingCloseReason;
                }
                catch (Exception e)
                {
                    closeCode = ContractA.CloseNormal;
                    closeReason = e.GetType().Name + ": " + Flatten(e);
                    if (connected)
                    {
                        Report(TransportLogLevel.Warning, $"connection to {url} dropped: {closeReason}");
                    }
                    else
                    {
                        // Exactly one line per failed attempt — a dead sidecar must not flood the log.
                        int next = FullJitterDelayMs(attempt);
                        Report(
                            TransportLogLevel.Info,
                            $"connect attempt {attempt} to {url} failed ({closeReason}) — retrying in about {next} ms.");
                    }
                }
                finally
                {
                    bool wasConnected = connected;
                    connected = false;
                    session.Stop();
                    await CloseQuietlyAsync(socket, wasConnected).ConfigureAwait(false);
                    socket?.Dispose();

                    if (wasConnected)
                    {
                        inbound.Enqueue(new TransportEvent
                        {
                            kind = TransportEventKind.Disconnected,
                            closeCode = closeCode,
                            text = closeReason
                        });
                    }

                    // §6.2 + §13 amendment A8: the ladder resets only after a session that stayed
                    // up, and it resets to rung 1 rather than to "dial immediately", so every
                    // reconnect still waits out a backoff. A session that died on the handshake
                    // leaves the ladder climbing to the 30 s ceiling instead of spinning.
                    if (wasConnected && session.ElapsedMilliseconds >= StableSessionMs)
                    {
                        attempt = 1;
                    }
                }
            }
        }

        /// <summary>
        /// Run the two socket loops for one connection and **end both of them together**.
        ///
        /// The send loop parks on <see cref="outboundSignal"/>, which is shared by every connection.
        /// Waiting only on the first loop to finish therefore leaves the other one alive and still
        /// queued on that semaphore: the next connection's <c>CONFIG_UPDATE</c> wakes the orphan
        /// first, the orphan dequeues the frame, its send fails on the dead socket, and the frame is
        /// gone. The sidecar then sees a HEARTBEAT as the first frame and closes 4003, forever, one
        /// stolen frame per leaked loop. That is a permanent inability to reconnect, and it is what
        /// the M2 kill test hit the first time a sidecar was restarted under a live mod.
        ///
        /// So: cancel the session, wait for both loops, then re-await the one that failed so the
        /// caller still sees the real cause rather than the cancellation.
        /// </summary>
        private async Task PumpAsync(ClientWebSocket socket, CancellationToken token)
        {
            using (CancellationTokenSource session = CancellationTokenSource.CreateLinkedTokenSource(token))
            {
                Task receive = ReceiveLoopAsync(socket, session.Token);
                Task send = SendLoopAsync(socket, session.Token);
                Task finished = await Task.WhenAny(receive, send).ConfigureAwait(false);

                session.Cancel();
                try
                {
                    await Task.WhenAll(receive, send).ConfigureAwait(false);
                }
                catch (Exception)
                {
                    // The loser always ends in a cancellation, and the winner's own failure is
                    // re-thrown below. Neither is worth reporting twice.
                }

                // Surface the failure of whichever loop stopped first.
                await finished.ConfigureAwait(false);
            }
        }

        private async Task ReceiveLoopAsync(ClientWebSocket socket, CancellationToken token)
        {
            byte[] buffer = new byte[16384];
            ArraySegment<byte> segment = new ArraySegment<byte>(buffer);
            using (MemoryStream assembled = new MemoryStream())
            {
                while (!token.IsCancellationRequested && socket.State == WebSocketState.Open)
                {
                    WebSocketReceiveResult result = await socket.ReceiveAsync(segment, token).ConfigureAwait(false);
                    if (result.MessageType == WebSocketMessageType.Close)
                    {
                        return;
                    }

                    assembled.Write(buffer, 0, result.Count);
                    if (assembled.Length > ContractA.MaxFrameBytes)
                    {
                        Report(TransportLogLevel.Error, $"an inbound frame passed maxFrameBytes ({ContractA.MaxFrameBytes}) — closing 1009.");
                        await CloseAsync(socket, ContractA.CloseTooBig, "frame over maxFrameBytes").ConfigureAwait(false);
                        return;
                    }

                    if (!result.EndOfMessage)
                    {
                        continue;
                    }

                    string text = Encoding.UTF8.GetString(assembled.GetBuffer(), 0, (int)assembled.Length);
                    assembled.SetLength(0);
                    inbound.Enqueue(new TransportEvent { kind = TransportEventKind.Frame, text = text });
                }
            }
        }

        private async Task SendLoopAsync(ClientWebSocket socket, CancellationToken token)
        {
            while (!token.IsCancellationRequested && socket.State == WebSocketState.Open)
            {
                await outboundSignal.WaitAsync(token).ConfigureAwait(false);
                while (outbound.TryDequeue(out string frame))
                {
                    byte[] bytes = Encoding.UTF8.GetBytes(frame);
                    if (bytes.Length > ContractA.MaxFrameBytes)
                    {
                        Report(TransportLogLevel.Error, $"refusing to send a {bytes.Length} byte frame — maxFrameBytes is {ContractA.MaxFrameBytes}.");
                        continue;
                    }

                    await socket.SendAsync(new ArraySegment<byte>(bytes), WebSocketMessageType.Text, true, token).ConfigureAwait(false);
                }
            }
        }

        private async Task CloseQuietlyAsync(ClientWebSocket socket, bool wasConnected)
        {
            if (socket == null || !wasConnected)
            {
                return;
            }

            if (socket.State != WebSocketState.Open && socket.State != WebSocketState.CloseReceived)
            {
                return;
            }

            await CloseAsync(socket, pendingCloseCode, pendingCloseReason).ConfigureAwait(false);
        }

        private static async Task CloseAsync(ClientWebSocket socket, int code, string reason)
        {
            try
            {
                using (CancellationTokenSource timeout = new CancellationTokenSource(TimeSpan.FromSeconds(3)))
                {
                    await socket.CloseAsync((WebSocketCloseStatus)code, reason ?? string.Empty, timeout.Token).ConfigureAwait(false);
                }
            }
            catch (Exception)
            {
                // A peer that is already gone cannot complete a close handshake. Nothing to do.
            }
        }

        private void DrainOutbound()
        {
            // A new connection starts with CONFIG_UPDATE (§5.1). Anything queued against the previous
            // connection must not be allowed to jump in front of the handshake.
            while (outbound.TryDequeue(out string _))
            {
            }

            while (outboundSignal.CurrentCount > 0)
            {
                try
                {
                    outboundSignal.Wait(0);
                }
                catch (Exception)
                {
                    break;
                }
            }
        }

        /// <summary>§6.2 — delay = random(0, min(max, min * 2^n)).</summary>
        private int FullJitterDelayMs(int attempt)
        {
            int exponent = Math.Min(attempt - 1, 30);
            long ceiling = ContractA.ReconnectBackoffMinMs * (1L << exponent);
            if (ceiling > ContractA.ReconnectBackoffMaxMs || ceiling <= 0)
            {
                ceiling = ContractA.ReconnectBackoffMaxMs;
            }

            lock (random)
            {
                return random.Next(0, (int)ceiling + 1);
            }
        }

        private void Report(TransportLogLevel level, string message)
        {
            inbound.Enqueue(new TransportEvent
            {
                kind = TransportEventKind.Log,
                level = level,
                text = message
            });
        }

        private static string Flatten(Exception e)
        {
            Exception current = e;
            string message = current.Message;
            while (current.InnerException != null)
            {
                current = current.InnerException;
                message = message + " <- " + current.Message;
            }

            // Mono pads some socket error messages out of a fixed native buffer, so they carry a long
            // run of blanks and NULs. One tidy line per attempt.
            StringBuilder tidy = new StringBuilder(message.Length);
            bool lastWasSpace = false;
            foreach (char c in message)
            {
                bool isSpace = char.IsWhiteSpace(c) || char.IsControl(c) || c == '\0';
                if (isSpace)
                {
                    if (!lastWasSpace)
                    {
                        tidy.Append(' ');
                    }
                }
                else
                {
                    tidy.Append(c);
                }

                lastWasSpace = isSpace;
            }

            return tidy.ToString().Trim();
        }

        internal static string DescribeCloseCode(int code)
        {
            switch (code)
            {
                case ContractA.CloseNormal: return "1000 NORMAL";
                case ContractA.CloseTooBig: return "1009 TOO_BIG";
                case ContractA.CloseProtocolUnsupported: return "4000 PROTOCOL_UNSUPPORTED";
                case ContractA.CloseSectorMismatch: return "4001 SECTOR_MISMATCH";
                case ContractA.CloseGameVersionUnsupported: return "4002 GAME_VERSION_UNSUPPORTED";
                case ContractA.CloseMalformedFrame: return "4003 MALFORMED_FRAME";
                case ContractA.CloseHeartbeatTimeout: return "4004 HEARTBEAT_TIMEOUT";
                case ContractA.CloseShuttingDown: return "4005 SHUTTING_DOWN";
                case ContractA.CloseReplaced: return "4006 REPLACED";
                default: return code.ToString(CultureInfo.InvariantCulture);
            }
        }
    }
}
