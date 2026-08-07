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
    /// The OPTIONAL <c>species</c> block of contract-a.md §5.3 and §5.7 (added — §16, A30): the
    /// migrant's species **name**, and its immediate parent species' name when there is one. Four
    /// strings, no nesting, and no id of any kind — an id is world-local by construction
    /// (<c>Species.speciesMaxID</c>), which is the whole defect §16 closes.
    ///
    /// This is pure wire data and it never touches Unity. The names are only ever read from the live
    /// <c>Species</c> record on the sending side and compared byte for byte on the receiving one, so
    /// nothing here trims, folds, normalizes or re-generates a name (§5.3).
    /// </summary>
    internal sealed class SpeciesIdentity
    {
        internal string GenericName;
        internal string SpecificName;
        internal string ParentGenericName;
        internal string ParentSpecificName;

        /// <summary>§5.3 — the two parent fields are **all-or-nothing**: both or neither.</summary>
        internal bool HasParent => ParentGenericName != null && ParentSpecificName != null;

        /// <summary>
        /// The game's own <c>Species.name</c> (<c>Species.cs:85</c>): the two halves joined by exactly
        /// one U+0020. This is the string the importer matches on, ordinal and case-sensitive.
        /// </summary>
        internal string Name => GenericName + " " + SpecificName;

        /// <summary>The same assembly for the parent pair, or null when there is no parent.</summary>
        internal string ParentName => HasParent ? ParentGenericName + " " + ParentSpecificName : null;

        /// <summary>
        /// §5.3's shape rules, which are the same in both directions: both halves REQUIRED, non-empty,
        /// at most <see cref="ContractA.MaxSpeciesNameBytes"/> UTF-8 bytes, no leading or trailing
        /// whitespace; the parent pair all-or-nothing and under the same string rules.
        /// </summary>
        internal bool Validate(out string problem)
        {
            if (!ContractA.IsValidSpeciesNameHalf(GenericName, out problem))
            {
                problem = "genericName " + problem;
                return false;
            }

            if (!ContractA.IsValidSpeciesNameHalf(SpecificName, out problem))
            {
                problem = "specificName " + problem;
                return false;
            }

            if ((ParentGenericName == null) != (ParentSpecificName == null))
            {
                problem = "the parent pair is all-or-nothing and exactly one half is present";
                return false;
            }

            if (HasParent)
            {
                if (!ContractA.IsValidSpeciesNameHalf(ParentGenericName, out problem))
                {
                    problem = "parentGenericName " + problem;
                    return false;
                }

                if (!ContractA.IsValidSpeciesNameHalf(ParentSpecificName, out problem))
                {
                    problem = "parentSpecificName " + problem;
                    return false;
                }
            }

            problem = null;
            return true;
        }

        public override string ToString()
        {
            return HasParent ? Name + "<-" + ParentName : Name;
        }
    }

    /// <summary>What the <c>species</c> block on one MIGRATE_IN turned out to be (§5.7, §16 A30/A32).</summary>
    internal enum SpeciesBlockState
    {
        /// <summary>No <c>species</c> key at all. **Valid**, and the absent-block rule covers it.</summary>
        Absent,

        /// <summary>A block that satisfies every shape rule of §5.3.</summary>
        Present,

        /// <summary>
        /// A block that breaks one. Treated as absent, logged once as a sidecar defect (the sidecar was
        /// supposed to have stripped it, §5.3), and **never** a NACK (§9.2, §16 A32).
        /// </summary>
        Malformed
    }

    /// <summary>
    /// The wire vocabulary of contracts/contract-a.md, version contract-a/2.2: the envelope, the nine
    /// message types, the close codes, the NACK taxonomy and the mod-owned tunables of §10.
    ///
    /// Nothing here touches Unity, so it is safe to call from the socket thread.
    /// </summary>
    internal static class ContractA
    {
        /// <summary>
        /// §3, §17 A37 — this release is **2.2**. A35 adds two OPTIONAL fields to one message
        /// (<c>species</c> and its sibling <c>truncated</c>, on HEARTBEAT) and removes nothing, which
        /// §3.1's own test answers with a **minor** bump; the major, the message catalogue, the enums
        /// and the close codes are all untouched, so the URL path does not move and a
        /// <c>contract-a/2.1</c> or <c>contract-a/2.0</c> peer stays compatible by construction. The
        /// minor is a capability statement, never a negotiation: this side detects the feature by the
        /// **presence of the field** and never by arithmetic on the minor.
        ///
        /// 2.1 (§16, A33) added the species block to MIGRATE_OUT and MIGRATE_IN. 2.0 (§15, A23) was the
        /// first breaking change: A18 removed <c>exportEdge</c> for <c>exportEdges</c>, a field removal
        /// and a type change together.
        /// </summary>
        internal const string Protocol = "contract-a/2.2";

        internal const string ProtocolName = "contract-a";
        internal const int ProtocolMajor = 2;
        internal const int ProtocolMinor = 2;

        /// <summary>§3.1, §15 A23 — the path is major-scoped, so a major bump moves it. Serves every contract-a/2.x.</summary>
        internal const string UrlPath = "/contract-a/v2";
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
        /// §5.3, §16 A30 — the maximum size of **one half** of a species name, in UTF-8 bytes. It is a
        /// shape rule of the block, not a licence to shorten a name: a half that is over is a malformed
        /// block, and a malformed block is dropped whole, never truncated (§5.7).
        /// </summary>
        internal const int MaxSpeciesNameBytes = 64;

        /// <summary>
        /// §10, §17 A35 — the maximum number of entries in <c>HEARTBEAT.species</c>. It is a **wire
        /// bound, not a display preference**: it is what keeps a stats block — and the <c>PEER_STATUS</c>
        /// broadcast that republishes six of them — bounded, and no party may raise it unilaterally.
        /// The mod keeps the **largest** entries and drops the tail, which is why the census is sorted
        /// by the sender and not by the page (<see cref="SpeciesCensus"/>).
        /// </summary>
        internal const int SpeciesCensusMax = 32;

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

        // §4.2's opposite-edge function used to live here, and it is gone with the caller that needed
        // it: the mod derived its *passive* entry edges from its export edges, and D17 retired that
        // category (§18, A38). The pairing rule itself is unchanged and still on the wire — an organism
        // that leaves through `E` arrives through `W`, one that leaves through `W` arrives through `E` —
        // but it is the sidecar that applies it, and this mod never computed an entryEdge.

        /// <summary>
        /// The order two edges are declared and evaluated in: `E`, `N`, `W`, `S`.
        ///
        /// It is not cosmetic. §4.3.2's corner rule says the **larger outward component** wins and that
        /// `E` takes the tie, and the capture loop implements the tie as "the first declared edge keeps
        /// the win". Canonicalizing the declared set into this order is therefore what makes
        /// `velocity.x ≥ velocity.y → "E"` true without a special case, and what stops the answer
        /// depending on the order somebody typed into MULTIVERSE_EXPORT_EDGES.
        /// </summary>
        internal static int EdgeRank(Edge edge)
        {
            switch (edge)
            {
                case Edge.E: return 0;
                case Edge.N: return 1;
                case Edge.W: return 2;
                case Edge.S: return 3;
                default: return 4;
            }
        }

        /// <summary>
        /// Every edge, already in <see cref="EdgeRank"/> order. It is both the whole of
        /// <c>borderEdges</c> (§5.1, §15 A18) and, under two-way lanes, the default <c>exportEdges</c>
        /// (§18, A38). Never mutated: the two lists it seeds are copies.
        /// </summary>
        internal static readonly Edge[] AllEdges = { Edge.E, Edge.N, Edge.W, Edge.S };

        /// <summary>A comma-joined edge list for a log line or a message: <c>E,N</c>.</summary>
        internal static string EdgeNames(System.Collections.Generic.IList<Edge> edges)
        {
            if (edges == null || edges.Count == 0)
            {
                return string.Empty;
            }

            System.Text.StringBuilder text = new System.Text.StringBuilder();
            for (int i = 0; i < edges.Count; i++)
            {
                if (i > 0)
                {
                    text.Append(',');
                }

                text.Append(EdgeName(edges[i]));
            }

            return text.ToString();
        }

        /// <summary>§5.1 — an edge-enum JSON array, in the canonical order.</summary>
        internal static JArray EdgeArray(System.Collections.Generic.IList<Edge> edges)
        {
            JArray array = new JArray();
            if (edges != null)
            {
                foreach (Edge edge in edges)
                {
                    array.Add(EdgeName(edge));
                }
            }

            return array;
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

        // ---- §5.3, §5.7, §16 A30 — the OPTIONAL species block ----------------------------------

        /// <summary>
        /// §16 A34 — the exporting mod's whitespace normalization, and the **only** place in the system
        /// allowed to alter a name. Trim leading and trailing whitespace, and collapse every internal
        /// whitespace run to a single U+0020. Applied at the source, before validation.
        ///
        /// It exists because the game's own name generator issues names the wire rule then refuses:
        /// about 2% of generated halves carry an edge space, which shows up as a doubled space in
        /// <c>Species.name</c> ("Izus  copedylanus") or a trailing one ("Banagellus polatus "). Before
        /// A34 those organisms migrated with **no** species block at all — conformant and lossy.
        ///
        /// The wire rule itself does not move: a name on the wire still carries no edge whitespace, and
        /// no sidecar, relay or archive may normalize one (A30's opacity rule). Normalization happens
        /// here, at the source, or nowhere.
        ///
        /// Returns the same instance when nothing changed, so the caller can compare by reference or by
        /// value to tell whether a name was repaired.
        /// </summary>
        internal static string NormalizeSpeciesWhitespace(string value)
        {
            if (value == null)
            {
                return null;
            }

            System.Text.StringBuilder builder = new System.Text.StringBuilder(value.Length);
            bool pendingSpace = false;

            for (int i = 0; i < value.Length; i++)
            {
                char c = value[i];
                if (char.IsWhiteSpace(c))
                {
                    // Leading whitespace is dropped (builder still empty); an internal run becomes one
                    // U+0020, flushed only once a non-space follows; a trailing run is never flushed.
                    pendingSpace = builder.Length > 0;
                    continue;
                }

                if (pendingSpace)
                {
                    builder.Append(' ');
                    pendingSpace = false;
                }

                builder.Append(c);
            }

            string normalized = builder.ToString();
            return string.Equals(normalized, value, StringComparison.Ordinal) ? value : normalized;
        }

        /// <summary>
        /// §5.3 — one half of a species name: non-empty, at most <see cref="MaxSpeciesNameBytes"/>
        /// UTF-8 bytes, and with no leading or trailing whitespace. The value is never repaired *here*:
        /// the caller drops the whole block instead, because half a name is not a weaker identity, it is
        /// a different one (§5.7). Repair belongs to the exporter, which runs
        /// <see cref="NormalizeSpeciesWhitespace"/> before it validates (§16, A34).
        /// </summary>
        internal static bool IsValidSpeciesNameHalf(string value, out string problem)
        {
            if (value == null)
            {
                problem = "is missing";
                return false;
            }

            if (value.Length == 0)
            {
                problem = "is empty";
                return false;
            }

            if (char.IsWhiteSpace(value[0]) || char.IsWhiteSpace(value[value.Length - 1]))
            {
                problem = "has leading or trailing whitespace";
                return false;
            }

            int bytes = System.Text.Encoding.UTF8.GetByteCount(value);
            if (bytes > MaxSpeciesNameBytes)
            {
                problem = "is " + bytes.ToString(CultureInfo.InvariantCulture) + " UTF-8 bytes, over the "
                    + MaxSpeciesNameBytes.ToString(CultureInfo.InvariantCulture) + "-byte limit";
                return false;
            }

            problem = null;
            return true;
        }

        /// <summary>
        /// §5.3 sender side — the block as it goes on the wire, or null when there is nothing valid to
        /// send. Never a NACK and never a reason to hold an organism: the caller logs
        /// <paramref name="problem"/> once and sends the migration without the block, which the
        /// receiver answers with the absent-block rule (§16, A32).
        /// </summary>
        internal static JObject SpeciesBlock(SpeciesIdentity identity, out string problem)
        {
            problem = null;
            if (identity == null)
            {
                return null;
            }

            if (!identity.Validate(out problem))
            {
                return null;
            }

            JObject block = new JObject
            {
                ["genericName"] = identity.GenericName,
                ["specificName"] = identity.SpecificName
            };

            if (identity.HasParent)
            {
                block["parentGenericName"] = identity.ParentGenericName;
                block["parentSpecificName"] = identity.ParentSpecificName;
            }

            return block;
        }

        /// <summary>
        /// §5.7 receiver side — read the OPTIONAL block off a MIGRATE_IN. Three outcomes and no fourth:
        /// absent (valid), present and well shaped, or malformed. A malformed block is **never**
        /// partially applied and never NACKed (§9.2, §16 A32); the caller logs it and takes the
        /// absent-block rule. Unknown members inside the block are ignored silently (§9.3).
        /// </summary>
        internal static SpeciesBlockState ReadSpecies(JObject data, out SpeciesIdentity identity, out string problem)
        {
            identity = null;
            problem = null;

            JToken token = data["species"];
            if (token == null || token.Type == JTokenType.Null)
            {
                return SpeciesBlockState.Absent;
            }

            if (!(token is JObject block))
            {
                problem = "'species' is present but is not a JSON object";
                return SpeciesBlockState.Malformed;
            }

            if (!TryString(block, "genericName", out string genericName))
            {
                problem = "'species.genericName' is missing or is not a string";
                return SpeciesBlockState.Malformed;
            }

            if (!TryString(block, "specificName", out string specificName))
            {
                problem = "'species.specificName' is missing or is not a string";
                return SpeciesBlockState.Malformed;
            }

            JToken parentGenericToken = block["parentGenericName"];
            JToken parentSpecificToken = block["parentSpecificName"];
            bool hasParentGeneric = parentGenericToken != null && parentGenericToken.Type != JTokenType.Null;
            bool hasParentSpecific = parentSpecificToken != null && parentSpecificToken.Type != JTokenType.Null;

            if (hasParentGeneric != hasParentSpecific)
            {
                problem = "the parent pair is all-or-nothing (§5.3) and this block carries exactly one half";
                return SpeciesBlockState.Malformed;
            }

            string parentGenericName = null;
            string parentSpecificName = null;
            if (hasParentGeneric)
            {
                if (!TryString(block, "parentGenericName", out parentGenericName)
                    || !TryString(block, "parentSpecificName", out parentSpecificName))
                {
                    problem = "'species.parentGenericName' / 'species.parentSpecificName' are present but are not both strings";
                    return SpeciesBlockState.Malformed;
                }
            }

            SpeciesIdentity candidate = new SpeciesIdentity
            {
                GenericName = genericName,
                SpecificName = specificName,
                ParentGenericName = parentGenericName,
                ParentSpecificName = parentSpecificName
            };

            if (!candidate.Validate(out problem))
            {
                return SpeciesBlockState.Malformed;
            }

            identity = candidate;
            return SpeciesBlockState.Present;
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
