using System.Collections.Generic;
using System.Globalization;
using System.Text;

namespace BibitesMultiverse
{
    /// <summary>
    /// The applied state of every declared export edge, as one immutable object
    /// (contract-a.md §5.4, §11.2, §15 A18).
    ///
    /// Three rules of the contract are structural here rather than remembered:
    ///
    /// * **`EDGE_STATUS` is full state, not a delta.** A new snapshot starts with every declared
    ///   export edge CLOSED, and only the entries the frame actually carried reopen it. A declared
    ///   edge with no entry is therefore closed by construction, and an empty <c>edges</c> array is a
    ///   legal way to close everything.
    /// * **The frame is applied atomically.** The client builds a whole snapshot and publishes it with
    ///   one reference assignment. A capture decision in the corner reads both edges in the same
    ///   FixedUpdate (§4.3.2), and a half-applied frame could export an organism through an edge that
    ///   just closed.
    /// * **The tick reads it once.** <c>EDGE_STATUS</c> is applied in Update and the bands are tested
    ///   in FixedUpdate; taking one reference at the top of the organism loop is what stops a frame
    ///   that lands mid-tick from changing the answer half way through.
    /// </summary>
    internal sealed class EdgeStates
    {
        private const int Slots = 5; // Edge.None, N, S, E, W

        private readonly bool[] open = new bool[Slots];
        private readonly string[] reason = new string[Slots];
        private readonly bool[] peerSizeKnown = new bool[Slots];
        private readonly float[] peerSize = new float[Slots];

        /// <summary>The EDGE_STATUS epoch this snapshot came from. 0 means "none applied yet".</summary>
        internal long Epoch { get; private set; }

        private EdgeStates()
        {
        }

        /// <summary>
        /// §5.4 receiver obligations — until the mod has applied its first <c>EDGE_STATUS</c>, and
        /// whenever it is disconnected, **every export edge is closed**. This is the fail-safe: a mod
        /// that cannot reach its sidecar quietly stops migrating instead of losing organisms.
        /// </summary>
        internal static EdgeStates AllClosed(IList<Edge> exportEdges, long epoch, string why)
        {
            EdgeStates states = new EdgeStates { Epoch = epoch };
            if (exportEdges != null)
            {
                foreach (Edge edge in exportEdges)
                {
                    states.reason[(int)edge] = why;
                }
            }

            return states;
        }

        /// <summary>Fill one edge in a snapshot that is still being built. Never called after publication.</summary>
        internal void Set(Edge edge, bool isOpen, string why, bool sizeKnown, float size)
        {
            int slot = (int)edge;
            open[slot] = isOpen;
            reason[slot] = why;
            peerSizeKnown[slot] = sizeKnown;
            peerSize[slot] = size;
        }

        internal bool IsOpen(Edge edge)
        {
            return open[(int)edge];
        }

        internal string Reason(Edge edge)
        {
            return reason[(int)edge] ?? ContractA.ReasonNoPeer;
        }

        internal bool PeerSizeKnown(Edge edge)
        {
            return peerSizeKnown[(int)edge];
        }

        internal float PeerSize(Edge edge)
        {
            return peerSize[(int)edge];
        }

        /// <summary>
        /// A copy with one edge forced closed — §9.1/§15 A22's "close **only** that edge until the next
        /// EDGE_STATUS". A mod that closes every edge because one NACK arrived throws away a live lane,
        /// and that is exactly the bug the amendment names.
        /// </summary>
        internal EdgeStates WithEdgeClosed(Edge edge, string why)
        {
            EdgeStates copy = new EdgeStates { Epoch = Epoch };
            for (int slot = 0; slot < Slots; slot++)
            {
                copy.open[slot] = open[slot];
                copy.reason[slot] = reason[slot];
                copy.peerSizeKnown[slot] = peerSizeKnown[slot];
                copy.peerSize[slot] = peerSize[slot];
            }

            copy.Set(edge, false, why, false, 0f);
            return copy;
        }

        /// <summary>One log-friendly line: <c>E:open(peer_live,S=2000.00) N:closed(no_peer)</c>.</summary>
        internal string Describe(IList<Edge> exportEdges)
        {
            if (exportEdges == null || exportEdges.Count == 0)
            {
                return "<no export edge>";
            }

            StringBuilder text = new StringBuilder();
            for (int i = 0; i < exportEdges.Count; i++)
            {
                Edge edge = exportEdges[i];
                if (i > 0)
                {
                    text.Append(' ');
                }

                text.Append(ContractA.EdgeName(edge)).Append(':').Append(IsOpen(edge) ? "open" : "closed");
                text.Append('(').Append(Reason(edge));
                if (PeerSizeKnown(edge))
                {
                    text.Append(",peerS=").Append(PeerSize(edge).ToString("F2", CultureInfo.InvariantCulture));
                }

                text.Append(')');
            }

            return text.ToString();
        }
    }
}
