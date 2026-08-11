using System;
using System.IO;

namespace BibitesMultiverse
{
    /// <summary>
    /// Contract A's bearer token, as this mod presents it (contracts/contract-a.md §21, A47).
    ///
    /// **It is not Contract B's credential.** Contract B's is a per-peer credential bound to a
    /// <c>peerId</c> and carrying one of three disjoint grants; this one binds **two processes on one
    /// machine**, names no identity, carries no grant and answers no question about the map. They are
    /// different secrets, in different files, on different wires, and the only thing they have in
    /// common is the word *bearer*. Nothing in this mod ever reads Contract B's <c>MULTIVERSE_TOKEN</c>.
    ///
    /// **What the mod owes A47**, and every one of these is implemented here or at the one call site
    /// in <see cref="WebSocketTransport"/>:
    ///
    /// * present <c>Authorization: Bearer &lt;token&gt;</c> on **every** dial, on the HTTP upgrade and
    ///   never in a frame — a value that authenticates the connection cannot ride a message the
    ///   connection had to be open to carry (§5.1);
    /// * read it from the configured source **on the socket thread, at dial time, every dial**
    ///   (§11.2). That is what makes A47's rotation — delete the file, restart the sidecar — cost one
    ///   reconnect instead of a game restart;
    /// * never mint one, never derive one, and never read one from anywhere but the configured
    ///   source;
    /// * **never write the value to the BepInEx log**, at any level, in whole or in prefix. That log
    ///   is a file this project's own runbook asks players to archive and share. The *path* is not the
    ///   secret and is logged freely, which is what makes a misconfiguration diagnosable.
    ///
    /// The two sources are §10's <c>contractATokenFile</c>. They are read in the same order the Go
    /// side's <c>internal/modtoken</c> reads them, because the two halves of one wire may not disagree
    /// about which variable wins: the **file wins over the literal**, and a file that is named but
    /// unreadable is a refusal to fall back rather than a licence to look elsewhere.
    ///
    /// Nothing here touches Unity, so it is safe to call from the socket thread — which is the only
    /// place it is called from.
    /// </summary>
    internal static class ContractAToken
    {
        /// <summary>
        /// §10, §21 A47 — the file both processes read, on the same machine (D9). The sidecar's flag
        /// form is <c>--contract-a-token-file</c> and its environment form is this same name, so a rig
        /// that exports one variable configures both ends.
        /// </summary>
        internal const string EnvTokenFile = "MULTIVERSE_CONTRACT_A_TOKEN_FILE";

        /// <summary>
        /// §10, §21 A47 — the mod's alternative: the value directly. The sidecar deliberately does not
        /// read it, because the sidecar owns the file it mints.
        /// </summary>
        internal const string EnvToken = "MULTIVERSE_CONTRACT_A_TOKEN";

        /// <summary>§10 — <c>contractATokenFile</c>'s leaf, under the sidecar's own state directory.</summary>
        internal const string DefaultFileName = "contract-a.token";

        internal const string HeaderName = "Authorization";
        private const string HeaderScheme = "Bearer ";

        /// <summary>
        /// What one dial resolved. <see cref="Value"/> is the only field that carries the secret, it is
        /// held no longer than the dial that uses it, and nothing here formats it into a string.
        /// </summary>
        internal struct Resolution
        {
            /// <summary>The token, or null when there is nothing to present.</summary>
            internal string Value;

            /// <summary>True when a source is configured, whether or not it yielded a usable value.</summary>
            internal bool Configured;

            /// <summary>Where the value came from, in words. **Never the value** — a path or a variable name.</summary>
            internal string Source;

            /// <summary>Why a configured source yielded nothing, or null when it yielded something.</summary>
            internal string Problem;

            /// <summary>
            /// The one line the transport logs when the resolution changes. It names the source and the
            /// consequence, and it never names the value.
            /// </summary>
            internal string Summary
            {
                get
                {
                    if (Value != null)
                    {
                        return "contract A: the bearer token of §21 A47 comes from " + Source +
                               " and rides every dial as an " + HeaderName + " header. Its value is never logged.";
                    }

                    if (Configured)
                    {
                        return "contract A: the bearer token of §21 A47 is configured but unusable — " + Problem +
                               ". The dial carries no " + HeaderName + " header and a sidecar that enforces A47 " +
                               "answers HTTP 401. The source is re-read on every dial, so the sidecar minting the " +
                               "file is enough to fix this; nothing here invents a token to get past a refusal.";
                    }

                    return "contract A: no bearer token is configured (" + EnvTokenFile + " and " + EnvToken +
                           " are both unset), so the dial carries no " + HeaderName + " header. A sidecar that " +
                           "enforces §21 A47 answers HTTP 401; one that was started with its own insecure " +
                           "no-token switch accepts the connection. Point " + EnvTokenFile + " at the sidecar's " +
                           "<data-dir>/" + DefaultFileName + " to authenticate.";
                }
            }

            /// <summary>The remedy sentence of A47's paired log line, which differs only in its verb.</summary>
            internal string Remedy
            {
                get
                {
                    return Configured
                        ? "this machine's operator re-points " + EnvTokenFile + " at the sidecar's token file"
                        : "this machine's operator points " + EnvTokenFile + " at the sidecar's <data-dir>/" +
                          DefaultFileName;
                }
            }
        }

        /// <summary>
        /// Resolve the token for **one** dial. Called on the socket thread, once per connect attempt
        /// (§11.2): it is a file read, so it belongs off the main thread with the connect, and reading
        /// it per dial is what makes a rotated token cost one reconnect rather than a game restart.
        ///
        /// It never throws and it never logs — the caller owns both, because logging crosses a thread.
        /// </summary>
        internal static Resolution Resolve()
        {
            Resolution resolution = new Resolution();

            string path = Trimmed(Read(EnvTokenFile));
            if (path.Length > 0)
            {
                // The file wins outright. A named-but-unreadable file does **not** fall through to the
                // literal: "read one from anywhere but its configured source" (A47) forbids the
                // fallback, and during the rollout window an absent file simply means the sidecar has
                // not minted it yet — the next dial reads again.
                resolution.Configured = true;
                resolution.Source = "the first line of " + path + " (" + EnvTokenFile + ")";
                resolution.Value = ReadFirstLine(path, out resolution.Problem);
                return resolution;
            }

            string literal = Trimmed(Read(EnvToken));
            if (literal.Length > 0)
            {
                resolution.Configured = true;
                resolution.Source = "the value of " + EnvToken;
                resolution.Value = literal;
                return resolution;
            }

            return resolution;
        }

        /// <summary>The header value for a resolved token. The only place the scheme is spelled.</summary>
        internal static string HeaderValue(string token)
        {
            return HeaderScheme + token;
        }

        /// <summary>
        /// The startup line for the configuration summary: which source is set, and the path when there
        /// is one. It reads no file — the read belongs on the socket thread — and it never reports the
        /// value of <see cref="EnvToken"/>, only that it is set.
        /// </summary>
        internal static string DescribeConfiguration()
        {
            string path = Trimmed(Read(EnvTokenFile));
            if (path.Length > 0)
            {
                return EnvTokenFile + "=" + path + " (read on every dial; the value is never logged)";
            }

            return Trimmed(Read(EnvToken)).Length > 0
                ? EnvTokenFile + "=<unset> " + EnvToken + "=<set, value never logged>"
                : EnvTokenFile + "=<unset> " + EnvToken + "=<unset> (the dial carries no " + HeaderName +
                  " header; an enforcing sidecar answers HTTP 401)";
        }

        /// <summary>
        /// The token file holds one line. Cut at the first CR or LF and trim, which is byte for byte
        /// what the Go side does when it reads the same file — the comparison on the other end is
        /// constant-time over the raw bytes, so a stray newline would be a rejection nobody could see.
        /// </summary>
        private static string ReadFirstLine(string path, out string problem)
        {
            string text;
            try
            {
                text = File.ReadAllText(path);
            }
            catch (Exception e)
            {
                // The message carries the path and the reason, never the contents.
                problem = e.GetType().Name + ": " + e.Message;
                return null;
            }

            int end = text.IndexOfAny(new[] { '\r', '\n' });
            string line = (end >= 0 ? text.Substring(0, end) : text).Trim();
            if (line.Length == 0)
            {
                problem = path + " is empty";
                return null;
            }

            problem = null;
            return line;
        }

        private static string Read(string name)
        {
            try
            {
                return Environment.GetEnvironmentVariable(name);
            }
            catch (Exception)
            {
                // A restricted environment is indistinguishable from an unset variable here, and the
                // dial that follows reports the consequence either way.
                return null;
            }
        }

        private static string Trimmed(string value)
        {
            return (value ?? string.Empty).Trim();
        }
    }
}
