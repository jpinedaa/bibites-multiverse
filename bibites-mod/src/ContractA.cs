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
    /// The wire vocabulary of contracts/contract-a.md, version contract-a/1: the envelope, the nine
    /// message types, the close codes, the NACK taxonomy and the mod-owned tunables of §10.
    ///
    /// Nothing here touches Unity, so it is safe to call from the socket thread.
    /// </summary>
    internal static class ContractA
    {
        internal const string Protocol = "contract-a/1";
        internal const string ProtocolPrefix = "contract-a/";
        internal const string ProtocolMajor = "1";
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
        internal const int CloseSectorMismatch = 4001;
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
        internal const int MaxFrameBytes = 8388608;
        internal const int MaxPayloadBytes = 4194304;
        internal const float MigrationCooldownSeconds = 5f;
        internal const float EntryImmunitySeconds = 5f;

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
                // may be processed.
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

        /// <summary>§3.1 — two peers are compatible when the major part of 'protocol' is equal.</summary>
        internal static bool MajorMatches(string protocol)
        {
            if (string.IsNullOrEmpty(protocol) || !protocol.StartsWith(ProtocolPrefix, StringComparison.Ordinal))
            {
                return false;
            }

            return protocol.Substring(ProtocolPrefix.Length) == ProtocolMajor;
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
    }
}
