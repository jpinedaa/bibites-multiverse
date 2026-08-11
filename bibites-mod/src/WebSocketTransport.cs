using System;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Net;
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
    /// before the first connect, so the answer is in the log rather than in a crash. §11.2 names
    /// that type and its <c>Options.SetRequestHeader</c> as the way A47's bearer token rides the
    /// upgrade, which is the second reason not to hand-roll one.
    ///
    /// **The dial is authenticated (§21, A47).** Every connect attempt carries
    /// <c>Authorization: Bearer &lt;token&gt;</c>, resolved on this thread, at dial time, on every
    /// dial (<see cref="ContractAToken"/>). A refusal is HTTP **401** and no WebSocket — deliberately
    /// not a close code, because §2.1's codes are statements made inside a session and a session the
    /// token did not open never started. So the refusal arrives here as a failed
    /// <see cref="ClientWebSocket.ConnectAsync"/> rather than as a <c>Disconnected</c> event, it
    /// retries on §6.2's ordinary ladder, and after
    /// <see cref="ContractA.AuthFailuresBeforeCeiling"/> consecutive refusals the ladder holds at
    /// <see cref="ContractA.ReconnectBackoffMaxMs"/> and one line names the remedy and who must act.
    /// </summary>
    internal class WebSocketTransport
    {
        private readonly string url;

        /// <summary>
        /// Received events waiting for the game thread to drain them. It carries Log / Connected /
        /// Disconnected / Frame events in arrival order.
        ///
        /// C4 — the Frame backlog here is bounded in practice, which is why neither this queue nor the
        /// mod's pendingMutations list carries a cap of its own. The sidecar caps its un-ACKed inbound
        /// deliveries at 64 and paces their release (contract-a §7.5, §15 A20, and the parallel Go-side
        /// fix), so it cannot pile MIGRATE_IN frames here faster than the mod ACKs them back. The one
        /// residual way this queue could grow unbounded is a misbehaving sidecar flooding a frame type that
        /// carries no ACK gate — EDGE_STATUS is the only inbound example — which is why the count below is
        /// exposed but deliberately not turned into a silent drop: a dropped control frame would be a worse
        /// failure than the memory it saves.
        /// </summary>
        private readonly ConcurrentQueue<TransportEvent> inbound = new ConcurrentQueue<TransportEvent>();

        /// <summary>
        /// C1/C4 — the number of Frame events currently in <see cref="inbound"/>, maintained with
        /// Interlocked because the receive loop increments it and the game thread decrements it in
        /// <see cref="TryDequeue"/>. The client reads it through <see cref="QueuedFrameCount"/> to keep
        /// §5.2 <c>pendingIn</c> honest once DrainTransport parses only a few frames per Update.
        /// </summary>
        private int queuedFrames;

        private readonly ConcurrentQueue<string> outbound = new ConcurrentQueue<string>();
        private readonly SemaphoreSlim outboundSignal = new SemaphoreSlim(0);
        private readonly Random random = new Random();

        private CancellationTokenSource cancellation;
        private Task worker;
        private volatile bool halted;
        private volatile bool connected;
        private int generation;

        /// <summary>
        /// §21 A47 — consecutive HTTP 401s on the upgrade. Reset by any dial that opened a socket and
        /// by any failure that was not an authentication refusal, because A47 counts *consecutive*
        /// ones. Socket-thread only.
        /// </summary>
        private int authFailures;

        /// <summary>
        /// §21 A47 — the ceiling line is logged **once** per run of refusals, not once per attempt. A
        /// misconfigured install must be a quiet, diagnosable loop rather than a wall of text.
        /// </summary>
        private bool authCeilingLogged;

        /// <summary>
        /// The last <see cref="ContractAToken.Resolution.Summary"/> reported, so the token's source is
        /// stated when it changes and not on every rung of the ladder. **Never the value.**
        /// </summary>
        private string lastTokenSummary;

        /// <summary>The remedy sentence that belongs to the resolution the current dial used.</summary>
        private string lastTokenRemedy = string.Empty;

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
        private const int StableSessionMs = ContractA.StableSessionMs;

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

        /// <summary>
        /// Refuse to reconnect until the mod is restarted or reconfigured
        /// (close 4000/4001/4002/4006/4007 — §6.2, amended §21 A50).
        /// </summary>
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
            if (!inbound.TryDequeue(out transportEvent))
            {
                return false;
            }

            if (transportEvent.kind == TransportEventKind.Frame)
            {
                Interlocked.Decrement(ref queuedFrames);
            }

            return true;
        }

        /// <summary>
        /// §5.2 <c>pendingIn</c> — received frames not yet drained by the game thread. The client adds this
        /// to its own not-yet-applied count so a deferred DrainTransport backlog (C1) is reported honestly.
        /// </summary>
        internal int QueuedFrameCount => Math.Max(0, Volatile.Read(ref queuedFrames));

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
                    Authenticate(socket);
                    await socket.ConnectAsync(uri, token).ConfigureAwait(false);

                    // Drain **after** the connect, not before it. Anything still queued belongs to the
                    // session that just died — including a heartbeat the game thread produced while it
                    // had not yet drained the Disconnected event — and §5.1 says the first frame on a
                    // new connection is CONFIG_UPDATE.
                    DrainOutbound();

                    // §21 A47 — the sidecar verified the token before the upgrade completed, so an
                    // open socket **is** the acceptance. The run of consecutive 401s ends here.
                    authFailures = 0;
                    authCeilingLogged = false;

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
                    else if (IsUpgradeRefused(e))
                    {
                        NoteAuthenticationRefusal(attempt);
                    }
                    else
                    {
                        // Not an authentication refusal, so the run of consecutive 401s is broken
                        // (§21, A47 counts consecutive ones) and the ceiling line may be earned again.
                        authFailures = 0;
                        authCeilingLogged = false;

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
                    Interlocked.Increment(ref queuedFrames);
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

        /// <summary>
        /// §21 A47 — put the bearer token on this dial. Resolved here, on the socket thread, once per
        /// connect attempt (§11.2): it is a file read, so it belongs off the main thread with the
        /// connect, and reading it per dial is what makes A47's rotation cost one reconnect instead of
        /// a game restart.
        ///
        /// A source that yields nothing leaves the header off and lets the sidecar's 401 be the
        /// answer — which is also how a rig running the sidecar's own insecure no-token switch
        /// connects. It is **not** a retry without the header: a mod that has no token has never had
        /// one to drop, and nothing here invents, derives or hunts for a value that a refusal would
        /// accept.
        /// </summary>
        private void Authenticate(ClientWebSocket socket)
        {
            ContractAToken.Resolution resolution = ContractAToken.Resolve();
            lastTokenRemedy = resolution.Remedy;

            string summary = resolution.Summary;
            if (!string.Equals(summary, lastTokenSummary, StringComparison.Ordinal))
            {
                lastTokenSummary = summary;
                Report(
                    resolution.Value != null ? TransportLogLevel.Info : TransportLogLevel.Warning,
                    summary);
            }

            if (resolution.Value == null)
            {
                return;
            }

            try
            {
                socket.Options.SetRequestHeader(
                    ContractAToken.HeaderName, ContractAToken.HeaderValue(resolution.Value));
            }
            catch (Exception e)
            {
                // A value with a control character in it cannot become a header. Report the failure
                // and the source, never the value; the dial goes ahead bare and the sidecar's 401
                // puts the same conclusion in the sidecar's log too.
                Report(
                    TransportLogLevel.Error,
                    $"contract A: the bearer token from {resolution.Source} cannot be sent as an " +
                    $"{ContractAToken.HeaderName} header ({e.GetType().Name}: {e.Message}). The token file holds " +
                    "one line and nothing else.");
            }
        }

        /// <summary>
        /// §21 A47 — count one HTTP 401 on the upgrade, and at
        /// <see cref="ContractA.AuthFailuresBeforeCeiling"/> consecutive ones log **once**, naming the
        /// remedy and who must act: the person at this machine, because the file is on it.
        ///
        /// Nothing about custody moves while this fails, and the line says so on purpose. A mod that
        /// cannot authenticate is a mod with a closed export set (§5.4's fail-safe) and a sidecar
        /// holding its journal (§13, A1). The failure costs migrations that have not happened yet, and
        /// costs no organism that has.
        /// </summary>
        private void NoteAuthenticationRefusal(int attempt)
        {
            authFailures++;
            int next = FullJitterDelayMs(attempt);

            // One line per attempt, the same budget the ordinary failure path keeps.
            Report(
                TransportLogLevel.Warning,
                $"contract A: the upgrade to {url} was refused before the WebSocket existed — HTTP 401, the " +
                $"bearer token was not accepted ({authFailures} of {ContractA.AuthFailuresBeforeCeiling}). " +
                $"Retrying in about {next} ms, with the header and never without it.");

            if (authFailures < ContractA.AuthFailuresBeforeCeiling || authCeilingLogged)
            {
                return;
            }

            authCeilingLogged = true;
            Report(
                TransportLogLevel.Error,
                $"contract A: 401 from {url} — token rejected. Remedy: {lastTokenRemedy}. Backoff pinned at " +
                $"{ContractA.ReconnectBackoffMaxMs} ms after {ContractA.AuthFailuresBeforeCeiling} attempts; no " +
                "organism is at risk — every edge is closed and the sidecar is holding its journal. This mod will " +
                "not retry without the header, mint a token, or dial another port looking for a sidecar that " +
                "would take it.");
        }

        /// <summary>
        /// §21 A47 — did the sidecar refuse this upgrade rather than fail to answer it?
        ///
        /// A 401 is an HTTP status on a request that never became a session, so it never reaches
        /// <c>DescribeCloseCode</c>: it surfaces as a failed <c>ConnectAsync</c>, and the mod has to
        /// tell it apart from "no sidecar is listening". Two shapes are read, because the game's Mono
        /// is the one that decides which one arrives:
        ///
        /// * a <see cref="WebException"/> anywhere in the chain carrying an <see cref="HttpWebResponse"/>
        ///   with status 401 — the exact answer, available from the <c>HttpWebRequest</c>-based
        ///   ClientWebSocket some Mono builds ship;
        /// * a <see cref="WebSocketException"/> with **no inner exception**, which is what the build
        ///   this mod is compiled against throws. Its <c>WebSocketHandle</c> writes the upgrade over a
        ///   raw socket and throws a bare <c>WebSocketException</c> when the status line is not
        ///   <c>HTTP/1.1 101</c>, without putting the code in the message; every failure *before* the
        ///   response — DNS, connect, cancellation — is wrapped, so it arrives with an inner exception
        ///   instead. An inner-null one therefore means the server answered and the answer was not a
        ///   101, and on <c>/contract-a/v2</c> the sidecar's only such answer is A47's 401. The two
        ///   error codes that mean "this is not HTTP at all" are excluded so a squatter on the port
        ///   does not read as a rejected token.
        /// </summary>
        private static bool IsUpgradeRefused(Exception e)
        {
            for (Exception current = e; current != null; current = current.InnerException)
            {
                if (current is WebException webException
                    && webException.Response is HttpWebResponse response
                    && response.StatusCode == HttpStatusCode.Unauthorized)
                {
                    return true;
                }
            }

            return e is WebSocketException socketException
                && socketException.InnerException == null
                && socketException.WebSocketErrorCode != WebSocketError.HeaderError
                && socketException.WebSocketErrorCode != WebSocketError.UnsupportedProtocol;
        }

        /// <summary>
        /// §6.2 — delay = random(0, min(max, min * 2^n)).
        ///
        /// §21 A47 pins the rung: after <see cref="ContractA.AuthFailuresBeforeCeiling"/> consecutive
        /// 401s the ladder **holds at** <see cref="ContractA.ReconnectBackoffMaxMs"/>. The jitter stays
        /// — A47 says the refusal retries on §6.2's *ordinary* ladder and names only the ceiling it
        /// holds at, and full jitter is what that ladder is. In practice the climb reaches the same
        /// ceiling by rung 6 anyway, because §13 A8 resets the ladder only after a session that stayed
        /// up and a refused upgrade never opens one; the pin is here so the rule is enforced rather
        /// than inferred from the arithmetic.
        /// </summary>
        private int FullJitterDelayMs(int attempt)
        {
            long ceiling;
            if (authFailures >= ContractA.AuthFailuresBeforeCeiling)
            {
                ceiling = ContractA.ReconnectBackoffMaxMs;
            }
            else
            {
                int exponent = Math.Min(attempt - 1, 30);
                ceiling = ContractA.ReconnectBackoffMinMs * (1L << exponent);
                if (ceiling > ContractA.ReconnectBackoffMaxMs || ceiling <= 0)
                {
                    ceiling = ContractA.ReconnectBackoffMaxMs;
                }
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
                case ContractA.CloseSlotMismatch: return "4001 SLOT_MISMATCH";
                case ContractA.CloseGameVersionUnsupported: return "4002 GAME_VERSION_UNSUPPORTED";
                case ContractA.CloseMalformedFrame: return "4003 MALFORMED_FRAME";
                case ContractA.CloseHeartbeatTimeout: return "4004 HEARTBEAT_TIMEOUT";
                case ContractA.CloseShuttingDown: return "4005 SHUTTING_DOWN";
                case ContractA.CloseReplaced: return "4006 REPLACED";
                case ContractA.CloseExportEdgesUnusable: return "4007 EXPORT_EDGES_UNUSABLE";
                default: return code.ToString(CultureInfo.InvariantCulture);
            }
        }
    }
}
