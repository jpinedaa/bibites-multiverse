using System;
using System.Globalization;
using Newtonsoft.Json.Linq;

namespace BibitesMultiverse
{
    /// <summary>One of the four map edges. <see cref="Edge.None"/> means "not configured".</summary>
    internal enum Edge
    {
        None,
        N,
        S,
        E,
        W
    }

    /// <summary>
    /// The wire vocabulary of contracts/contract-a.md, version contract-a/1.1: the envelope, the nine
    /// message types, the close codes, the NACK taxonomy and the mod-owned tunables of §10.
    ///
    /// Nothing here touches Unity, so it is safe to call from the socket thread.
    /// </summary>
    internal static class ContractA
    {
        /// <summary>
        /// §3, §14 A16 — the identifier carries a major **and** a minor. This release is 1.1: A11
        /// added <c>exportEdge</c>, A12 added <c>parents</c>, A14 retired <c>sector</c> for
        /// <c>ringSlot</c>. Every one of those is additive or narrowing, so the major stays at 1 and
        /// a contract-a/1 peer is still compatible.
        /// </summary>
        internal const string Protocol = "contract-a/1.1";

        internal const string ProtocolName = "contract-a";
        internal const int ProtocolMajor = 1;
        internal const int ProtocolMinor = 1;

        /// <summary>§3.1 — the path stays major-scoped and serves every contract-a/1.x.</summary>
        internal const string UrlPath = "/contract-a/v1";
        internal const int DefaultPort = 8787;

        // ---- §5, the nine message types -------------------------------------------------
        internal const string TypeConfigUpdate = "CONFIG_UPDATE";
        internal const string TypeHeartbeat = "HEARTBEAT";
        internal const string TypeMigrateOut = "MIGRATE_OUT";
        internal const string TypeMigrateInAck = "MIGRATE_IN_ACK";
        internal const string TypeMigrateInNack = "MIGRATE_IN_NACK";
        internal const string TypeEdgeStatus = "EDGE_STATUS";
        internal const string TypeMigrateIn = "MIGRATE_IN";
        internal const string TypeMigrateOutAck = "MIGRATE_OUT_ACK";
        internal const string TypeMigrateOutNack = "MIGRATE_OUT_NACK";

        // ---- §2.1, close codes ------------------------------------------------------------
        internal const int CloseNormal = 1000;
        internal const int CloseTooBig = 1009;
        internal const int CloseProtocolUnsupported = 4000;

        /// <summary>4001. Named SECTOR_MISMATCH in M2; the code and the behaviour are unchanged (§14, A14).</summary>
        internal const int CloseSlotMismatch = 4001;
        internal const int CloseGameVersionUnsupported = 4002;
        internal const int CloseMalformedFrame = 4003;
        internal const int CloseHeartbeatTimeout = 4004;
        internal const int CloseShuttingDown = 4005;
        internal const int CloseReplaced = 4006;

        // ---- §10, the tunables this side owns ---------------------------------------------
        internal const int HeartbeatIntervalMs = 1000;
        internal const int MigrateOutTimeoutMs = 5000;
        internal const int ReconnectBackoffMinMs = 1000;
        internal const int ReconnectBackoffMaxMs = 30000;
        internal const int StableSessionMs = 5000;
        internal const int SimNotReadyGraceMs = 2000;
        internal const int MaxFrameBytes = 8388608;
        internal const int MaxPayloadBytes = 4194304;

        /// <summary>§10, §14 A12 — a bibite has at most two parents.</summary>
        internal const int MaxParentBlobs = 2;

        /// <summary>
        /// §10, §14 A12 — envelope, escaping and JSON overhead the mod reserves under
        /// <see cref="MaxFrameBytes"/> when it decides whether a parent blob still fits.
        /// </summary>
        internal const int FrameHeadroomBytes = 65536;

        /// <summary>§10, §13 A10 — relative tolerance for every comparison of two S values.</summary>
        internal const double SimSizeEpsilon = 1e-6;

        internal const float MigrationCooldownSeconds = 5f;
        internal const float EntryImmunitySeconds = 5f;

        // ---- §5.4, the EDGE_STATUS reasons this side reads ----------------------------------
        internal const string ReasonPeerLive = "peer_live";
        internal const string ReasonNoPeer = "no_peer";

        /// <summary>Added by §14 A11: the east neighbour's sidecar is live but has no mod.</summary>
        internal const string ReasonPeerModAbsent = "peer_mod_absent";
        internal const string ReasonPeerIncompatible = "peer_incompatible";
        internal const string ReasonPeerUnreachable = "peer_unreachable";
        internal const string ReasonPeerOverloaded = "peer_overloaded";
        internal const string ReasonAdminClosed = "admin_closed";
        internal const string ReasonSimSizeMismatch = "sim_size_mismatch";

        // ---- §9.2, the codes this side emits ------------------------------------------------
        internal const string NackSimNotReady = "SIM_NOT_READY";
        internal const string NackSimOverloaded = "SIM_OVERLOADED";
        internal const string NackEdgeClosed = "EDGE_CLOSED";
        internal const string NackDeserializeFailed = "DESERIALIZE_FAILED";
        internal const string NackRelinkFailed = "RELINK_FAILED";
        internal const string NackVersionUnsupported = "VERSION_UNSUPPORTED";
        internal const string NackKindUnsupported = "KIND_UNSUPPORTED";
        internal const string NackMalformedMessage = "MALFORMED_MESSAGE";
        internal const string NackShuttingDown = "SHUTTING_DOWN";

        internal const string ClassTransient = "transient";
        internal const string ClassPermanent = "permanent";

        internal const string KindBibite = "bibite";

        /// <summary>§3 — wrap a body in the envelope every frame shares.</summary>
        internal static JObject Envelope(string type, JObject data)
        {
            return new JObject
            {
                ["protocol"] = Protocol,
                ["type"] = type,
                ["messageId"] = NewUuid(),
                ["sentAt"] = UnixMilliseconds(),
                ["data"] = data ?? new JObject()
            };
        }

        internal static string NewUuid()
        {
            return Guid.NewGuid().ToString("D", CultureInfo.InvariantCulture).ToLowerInvariant();
        }

        internal static long UnixMilliseconds()
        {
            return (long)(DateTime.UtcNow - new DateTime(1970, 1, 1, 0, 0, 0, DateTimeKind.Utc)).TotalMilliseconds;
        }

        /// <summary>
        /// §3.2 — a frame is malformed when it is not an object or when any of the five envelope
        /// fields is missing or has the wrong JSON type. Never guess a field.
        /// </summary>
        internal static bool TryReadEnvelope(string text, out string type, out JObject data, out string problem, out int closeCode)
        {
            type = null;
            data = null;
            problem = null;
            closeCode = CloseMalformedFrame;

            JToken token;
            try
            {
                token = JToken.Parse(text);
            }
            catch (Exception e)
            {
                problem = "not valid JSON: " + e.Message;
                return false;
            }

            if (token.Type != JTokenType.Object)
            {
                problem = "the frame is not a JSON object";
                return false;
            }

            JObject frame = (JObject)token;
            if (!(frame["protocol"] is JValue protocolValue) || protocolValue.Type != JTokenType.String)
            {
                problem = "'protocol' is missing or not a string";
                return false;
            }

            if (!(frame["type"] is JValue typeValue) || typeValue.Type != JTokenType.String)
            {
                problem = "'type' is missing or not a string";
                return false;
            }

            if (!(frame["messageId"] is JValue idValue) || idValue.Type != JTokenType.String)
            {
                problem = "'messageId' is missing or not a string";
                return false;
            }

            JToken sentAt = frame["sentAt"];
            if (sentAt == null || (sentAt.Type != JTokenType.Integer && sentAt.Type != JTokenType.Float))
            {
                problem = "'sentAt' is missing or not a number";
                return false;
            }

            if (!(frame["data"] is JObject body))
            {
                problem = "'data' is missing or not an object";
                return false;
            }

            string protocol = (string)protocolValue;
            if (!MajorMatches(protocol))
            {
                // §3.1 — a different major version is close 4000, not 4003, and nothing in the frame
                // may be processed. A different **minor** is never a reason to reject anything.
                problem = "unsupported protocol '" + protocol + "', this mod speaks " + Protocol;
                closeCode = CloseProtocolUnsupported;
                type = null;
                data = null;
                return false;
            }

            type = (string)typeValue;
            data = body;
            return true;
        }

        /// <summary>
        /// §3.1, §14 A16 — split at the **last** '/', then split the remainder at the **first** '.'.
        /// A missing '.' means minor 0, so the M2 string "contract-a/1" reads as major 1, minor 0.
        /// </summary>
        internal static bool TryParseProtocol(string protocol, out int major, out int minor)
        {
            major = 0;
            minor = 0;
            if (string.IsNullOrEmpty(protocol))
            {
                return false;
            }

            int slash = protocol.LastIndexOf('/');
            if (slash <= 0 || slash == protocol.Length - 1)
            {
                return false;
            }

            if (!string.Equals(protocol.Substring(0, slash), ProtocolName, StringComparison.Ordinal))
            {
                return false;
            }

            string version = protocol.Substring(slash + 1);
            int dot = version.IndexOf('.');
            string majorText = (dot < 0) ? version : version.Substring(0, dot);
            string minorText = (dot < 0) ? "0" : version.Substring(dot + 1);

            return TryDecimal(majorText, out major) && TryDecimal(minorText, out minor);
        }

        private static bool TryDecimal(string text, out int value)
        {
            value = 0;
            if (string.IsNullOrEmpty(text))
            {
                return false;
            }

            foreach (char c in text)
            {
                if (c < '0' || c > '9')
                {
                    return false;
                }
            }

            return int.TryParse(text, NumberStyles.None, CultureInfo.InvariantCulture, out value);
        }

        /// <summary>
        /// §3.1 — two peers are compatible when the **major** part of 'protocol' is equal. The minor
        /// is a capability statement, never a negotiation, and never a reason to close.
        /// </summary>
        internal static bool MajorMatches(string protocol)
        {
            return TryParseProtocol(protocol, out int major, out int _) && major == ProtocolMajor;
        }

        /// <summary>
        /// §13 A10 — the one equality test for two simulation half-extents. S starts life as a C#
        /// 32-bit float and reaches the sidecar as a float64, so an exact comparison is forbidden
        /// (§4.1). The max(1, …) term keeps the tolerance absolute near zero and relative at
        /// simulation scale. A non-finite value is never equal to anything.
        /// </summary>
        internal static bool SimulationSizeEqual(double a, double b)
        {
            if (double.IsNaN(a) || double.IsInfinity(a) || double.IsNaN(b) || double.IsInfinity(b))
            {
                return false;
            }

            double scale = Math.Max(1.0, Math.Max(Math.Abs(a), Math.Abs(b)));
            return Math.Abs(a - b) <= SimSizeEpsilon * scale;
        }

        // ---- edge helpers -------------------------------------------------------------------

        internal static bool TryParseEdge(string text, out Edge edge)
        {
            edge = Edge.None;
            if (string.IsNullOrEmpty(text))
            {
                return false;
            }

            switch (text.Trim().ToUpperInvariant())
            {
                case "N": edge = Edge.N; return true;
                case "S": edge = Edge.S; return true;
                case "E": edge = Edge.E; return true;
                case "W": edge = Edge.W; return true;
                default: return false;
            }
        }

        internal static string EdgeName(Edge edge)
        {
            return edge.ToString();
        }

        /// <summary>
        /// §4.2 — N↔S, E↔W. Under the ring (D8) this is what turns the configured **export** edge
        /// into the passive **entry** edge: export east, receive west.
        /// </summary>
        internal static Edge OppositeEdge(Edge edge)
        {
            switch (edge)
            {
                case Edge.N: return Edge.S;
                case Edge.S: return Edge.N;
                case Edge.E: return Edge.W;
                case Edge.W: return Edge.E;
                default: return Edge.None;
            }
        }

        // ---- data-field readers ---------------------------------------------------------------
        // Every one is strict: a missing field or a wrong JSON type fails, so a defect surfaces as a
        // NACK instead of a silently-defaulted value.

        internal static bool TryString(JObject data, string name, out string value)
        {
            value = null;
            if (!(data[name] is JValue token) || token.Type != JTokenType.String)
            {
                return false;
            }

            value = (string)token;
            return value != null;
        }

        /// <summary>
        /// The one field a frame cannot be answered without: every MIGRATE_IN_ACK and every
        /// MIGRATE_IN_NACK is keyed on it (§5.8, §5.9), so a frame that carries no usable value has no
        /// reply channel at all (§13, amendment A2). An empty string counts as absent, because the
        /// sidecar rejects it in the reply and would close 4003 on our answer.
        /// </summary>
        internal static bool TryMigrationId(JObject data, out string value)
        {
            return TryString(data, "migrationId", out value) && value.Length > 0;
        }

        internal static bool TryFloat(JObject data, string name, out float value)
        {
            value = 0f;
            JToken token = data[name];
            if (token == null || (token.Type != JTokenType.Float && token.Type != JTokenType.Integer))
            {
                return false;
            }

            double raw = (double)token;
            if (double.IsNaN(raw) || double.IsInfinity(raw))
            {
                return false;
            }

            value = (float)raw;
            return true;
        }

        internal static bool TryInt(JObject data, string name, out int value)
        {
            value = 0;
            JToken token = data[name];
            if (token == null || (token.Type != JTokenType.Integer && token.Type != JTokenType.Float))
            {
                return false;
            }

            double raw = (double)token;
            if (double.IsNaN(raw) || double.IsInfinity(raw) || raw < int.MinValue || raw > int.MaxValue)
            {
                return false;
            }

            value = (int)raw;
            return true;
        }

        internal static bool TryLong(JObject data, string name, out long value)
        {
            value = 0L;
            JToken token = data[name];
            if (token == null || (token.Type != JTokenType.Integer && token.Type != JTokenType.Float))
            {
                return false;
            }

            double raw = (double)token;
            if (double.IsNaN(raw) || double.IsInfinity(raw))
            {
                return false;
            }

            value = (long)raw;
            return true;
        }

        internal static bool TryBool(JObject data, string name, out bool value)
        {
            value = false;
            if (!(data[name] is JValue token) || token.Type != JTokenType.Boolean)
            {
                return false;
            }

            value = (bool)token;
            return true;
        }

        internal static bool TryVector(JObject data, string name, out float x, out float y)
        {
            x = 0f;
            y = 0f;
            if (!(data[name] is JObject vector))
            {
                return false;
            }

            return TryFloat(vector, "x", out x) && TryFloat(vector, "y", out y);
        }

        internal static bool IsFinite(float value)
        {
            return !float.IsNaN(value) && !float.IsInfinity(value);
        }

        /// <summary>
        /// Lower-case hex SHA-256 of the UTF-8 bytes of a payload string — bit for bit the same
        /// function the sidecar stores as <c>payloadHash</c> (<c>go/internal/bb8.Hash</c>). Logging it
        /// on both sides of every hop is what turns "the blob survived" into a checkable claim across
        /// two game processes, without ever putting a 100 kB payload in the log.
        /// </summary>
        internal static string Sha256Hex(string payload)
        {
            if (payload == null)
            {
                return string.Empty;
            }

            try
            {
                using (System.Security.Cryptography.SHA256 sha = System.Security.Cryptography.SHA256.Create())
                {
                    byte[] digest = sha.ComputeHash(System.Text.Encoding.UTF8.GetBytes(payload));
                    System.Text.StringBuilder hex = new System.Text.StringBuilder(digest.Length * 2);
                    foreach (byte b in digest)
                    {
                        hex.Append(b.ToString("x2", CultureInfo.InvariantCulture));
                    }

                    return hex.ToString();
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not hash a payload: {e.Message}");
                return string.Empty;
            }
        }
    }
}
