using System;
using System.Diagnostics;
using System.Reflection;
using HarmonyLib;
using ManagementScripts;
using OneUseScripts;

namespace BibitesMultiverse
{
    /// <summary>
    /// The decomposition of <c>writeMs</c>. It measures only; it changes nothing about the save.
    ///
    /// **Why it exists.** The save-stall watch item ran for five days on one opaque number:
    /// <c>writeMs</c> is ~98% of <c>stallMs</c> and covers the whole of
    /// <c>SaveSystem.CreateSave</c>, so every hypothesis about the stall — the screenshot, the
    /// species history, the zip, the host — argued over the same undivided total. The 2026-08-10
    /// analysis narrowed it from the outside (a save costs about what its **uncompressed** payload
    /// costs, and that payload is mostly <c>speciesData.json</c>), but the outside cannot say
    /// **where inside the call** the time goes, and it leaves a residual host multiplier of about
    /// 2x that is common-mode across all five worlds. Four timers settle both questions from the
    /// inside.
    ///
    /// **The four phases**, in the order <c>CreateSave</c> reaches them:
    ///
    /// * <c>lineageMs</c> — <c>GlobalLineageManager.SaveState()</c>, which builds the whole
    ///   <c>speciesData.json</c> JObject tree: one <c>JObject</c> per brain node and per synapse of
    ///   every **recorded** species, through uncached reflection and an <c>Enum.Parse</c> per gene
    ///   per species. Pure CPU and allocation, no file touched. This is the term that grows with a
    ///   world's evolutionary history and never with its population.
    /// * <c>binMs</c> — <c>GlobalLineageManager.BytesSpace()</c> plus
    ///   <c>SaveStateBin(byte[], int)</c>, the <c>data.bin</c> lineage block. The game calls
    ///   <c>BytesSpace</c> twice and <c>SaveStateBin</c> once per save, so this is the sum of three
    ///   entries, and <c>SpeciesHistoryGuard</c>'s repair rides inside all three.
    /// * <c>guardMs</c> — that repair, timed separately. It is a **subset of <c>binMs</c>**, not a
    ///   fifth phase: subtract it from <c>binMs</c> to price the game's own bin work.
    /// * <c>shotMs</c> — <c>ScreenShotHandler.SaveScreenshotToZip</c>, exclusive of the
    ///   <c>img.png</c> archive write it contains (that write is counted in <c>zipMs</c>, and
    ///   subtracted back out here). Headless still pays the <c>Texture2D</c> allocation, the
    ///   <c>Apply</c> and the PNG encode — only the pixels are lost — so this timer is the one that
    ///   says what a blank thumbnail actually costs.
    /// * <c>zipMs</c> — every <c>SaveSystem.WriteJObjectToArchive</c> and
    ///   <c>WriteFileToArchive</c> call, summed, with <c>zipN</c> counting them. Each one
    ///   materialises its entry as a managed string and deflates it at
    ///   <c>CompressionLevel.Optimal</c>, so this is the string + deflate + file-write cost and it
    ///   should track the uncompressed byte total.
    ///
    /// What is **not** in any of them — <c>writeMs - (lineageMs + binMs + shotMs + zipMs)</c> — is
    /// the rest of <c>CreateSave</c>: settings serialization, one JObject per bibite, egg, pellet
    /// and pheromone, and <c>AddTemplatesToArchive</c>'s <c>File.ReadAllText</c> +
    /// <c>JObject.Parse</c> per scenario template. Call it the remainder and read it by subtraction;
    /// giving it its own timer would mean patching a dozen more methods for a term that is already
    /// determined.
    ///
    /// **Failure is not fatal.** If a patch will not resolve, the timers stay at zero and the save
    /// path is untouched — a `[M4-PHASE]` error line says so once and every `[M4-SAVE]` line then
    /// reads `lineageMs=0 binMs=0 shotMs=0 zipMs=0`, which is the honest reading of "not measured"
    /// and is distinguishable from a real zero because a real save cannot have `zipMs=0`.
    ///
    /// Every line carries the grepable prefix <c>[M4-PHASE]</c>.
    /// </summary>
    internal static class SavePhases
    {
        internal const string Prefix = "[M4-PHASE]";

        private static Harmony harmony;

        /// <summary>True once the four patches are on. False means every reading is a zero.</summary>
        internal static bool Armed { get; private set; }

        // One stopwatch per phase, plus a depth counter so a re-entrant call cannot stop a span its
        // caller opened. The game does not nest these today; the counters are cheap insurance
        // against a game update that starts to.
        private static readonly Stopwatch LineageClock = new Stopwatch();
        private static readonly Stopwatch BinClock = new Stopwatch();
        private static readonly Stopwatch ShotClock = new Stopwatch();
        private static readonly Stopwatch ZipClock = new Stopwatch();
        private static readonly Stopwatch GuardClock = new Stopwatch();

        private static int lineageDepth;
        private static int binDepth;
        private static int shotDepth;
        private static int zipDepth;
        private static int guardDepth;

        private static int zipCalls;

        // The zip time accrued when the screenshot span opened. img.png is written through
        // WriteFileToArchive from inside SaveScreenshotToZip, so shotMs would otherwise double-count
        // it against zipMs.
        private static long shotZipMarkMs;
        private static long shotZipInnerMs;

        internal static long LineageMs { get { return LineageClock.ElapsedMilliseconds; } }
        internal static long BinMs { get { return BinClock.ElapsedMilliseconds; } }
        internal static long GuardMs { get { return GuardClock.ElapsedMilliseconds; } }
        internal static long ZipMs { get { return ZipClock.ElapsedMilliseconds; } }
        internal static int ZipCalls { get { return zipCalls; } }

        /// <summary>The screenshot, exclusive of the img.png archive write counted in <c>zipMs</c>.</summary>
        internal static long ShotMs
        {
            get
            {
                long ms = ShotClock.ElapsedMilliseconds - shotZipInnerMs;
                return (ms > 0) ? ms : 0;
            }
        }

        /// <summary>
        /// Zero every accumulator. <c>WorldSaver.Write</c> calls this immediately before it drives
        /// <c>CreateSave</c>, so a reading always belongs to exactly one save.
        /// </summary>
        internal static void Reset()
        {
            LineageClock.Reset();
            BinClock.Reset();
            ShotClock.Reset();
            ZipClock.Reset();
            GuardClock.Reset();
            lineageDepth = binDepth = shotDepth = zipDepth = guardDepth = 0;
            zipCalls = 0;
            shotZipMarkMs = 0;
            shotZipInnerMs = 0;
        }

        /// <summary>
        /// The `key=value` fragment appended to the <c>[M4-SAVE] event=SAVED</c> line, in
        /// <c>CreateSave</c>'s own order. <c>guardMs</c> is inside <c>binMs</c>; the remainder of
        /// <c>writeMs</c> is everything else in the call.
        /// </summary>
        internal static string Fields()
        {
            return "lineageMs=" + LineageMs
                 + " binMs=" + BinMs
                 + " guardMs=" + GuardMs
                 + " shotMs=" + ShotMs
                 + " zipMs=" + ZipMs
                 + " zipN=" + ZipCalls;
        }

        // ---- arming ------------------------------------------------------------------------------

        /// <summary>
        /// Patch the four spans. Called once from <c>MultiversePlugin.Awake</c>, before the saver
        /// exists. Returns false when the game API has moved; the save path is unaffected either way.
        /// </summary>
        internal static bool Apply(string id)
        {
            if (harmony != null)
            {
                return Armed;
            }

            try
            {
                MethodInfo saveState = AccessTools.Method(typeof(GlobalLineageManager), "SaveState");
                MethodInfo bytesSpace = AccessTools.Method(typeof(GlobalLineageManager), "BytesSpace");
                MethodInfo saveStateBin = AccessTools.Method(
                    typeof(GlobalLineageManager), "SaveStateBin", new[] { typeof(byte[]), typeof(int) });
                MethodInfo shot = AccessTools.Method(typeof(ScreenShotHandler), "SaveScreenshotToZip");
                MethodInfo writeJObject = AccessTools.Method(typeof(SaveSystem), "WriteJObjectToArchive");
                MethodInfo writeFile = AccessTools.Method(typeof(SaveSystem), "WriteFileToArchive");

                if (saveState == null || bytesSpace == null || saveStateBin == null
                    || shot == null || writeJObject == null || writeFile == null)
                {
                    MultiversePlugin.Log.LogError(
                        $"{Prefix} the save path could not be resolved for timing — SaveState={saveState != null} " +
                        $"BytesSpace={bytesSpace != null} SaveStateBin={saveStateBin != null} " +
                        $"SaveScreenshotToZip={shot != null} WriteJObjectToArchive={writeJObject != null} " +
                        $"WriteFileToArchive={writeFile != null}. The game API changed. Saves are unaffected; " +
                        "every phase on the [M4-SAVE] line will read 0.");
                    return false;
                }

                harmony = new Harmony(id + ".savephases");

                Patch(saveState, nameof(LineageEnter), nameof(LineageExit));
                Patch(bytesSpace, nameof(BinEnter), nameof(BinExit));
                Patch(saveStateBin, nameof(BinEnter), nameof(BinExit));
                Patch(shot, nameof(ShotEnter), nameof(ShotExit));
                Patch(writeJObject, nameof(ZipEnter), nameof(ZipExit));
                Patch(writeFile, nameof(ZipEnter), nameof(ZipExit));

                Armed = true;
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} armed — every [M4-SAVE] event=SAVED line now carries lineageMs (speciesData.json " +
                    "built), binMs (data.bin lineage block, guardMs inside it), shotMs (the 400x400 thumbnail, " +
                    "exclusive of its archive write) and zipMs/zipN (every archive entry: string + deflate + " +
                    "write). The remainder of writeMs is the rest of CreateSave — bibites, eggs, pellets, " +
                    "pheromones, settings and the per-save template re-read. Measurement only.");
                return true;
            }
            catch (Exception e)
            {
                harmony = null;
                Armed = false;
                MultiversePlugin.Log.LogError(
                    $"{Prefix} could not time the save path — {e}. Saves are unaffected; every phase reads 0.");
                return false;
            }
        }

        private static void Patch(MethodInfo target, string enter, string exit)
        {
            harmony.Patch(
                target,
                prefix: new HarmonyMethod(typeof(SavePhases).GetMethod(enter, BindingFlags.Static | BindingFlags.NonPublic)),
                postfix: new HarmonyMethod(typeof(SavePhases).GetMethod(exit, BindingFlags.Static | BindingFlags.NonPublic)));
        }

        // ---- the spans ---------------------------------------------------------------------------
        //
        // A throw out of any of these would land inside the game's own save, which is exactly the
        // mistake SpeciesHistoryGuard's comment warns against. Nothing here can throw — a Stopwatch
        // and an int — but the depth guards mean an unbalanced call cannot leave a clock running into
        // the next save either, because Reset() zeroes both.

        private static void LineageEnter() { if (lineageDepth++ == 0) { LineageClock.Start(); } }
        private static void LineageExit() { if (--lineageDepth <= 0) { lineageDepth = 0; LineageClock.Stop(); } }

        private static void BinEnter() { if (binDepth++ == 0) { BinClock.Start(); } }
        private static void BinExit() { if (--binDepth <= 0) { binDepth = 0; BinClock.Stop(); } }

        private static void ZipEnter() { zipCalls++; if (zipDepth++ == 0) { ZipClock.Start(); } }
        private static void ZipExit() { if (--zipDepth <= 0) { zipDepth = 0; ZipClock.Stop(); } }

        private static void ShotEnter()
        {
            if (shotDepth++ == 0)
            {
                shotZipMarkMs = ZipClock.ElapsedMilliseconds;
                ShotClock.Start();
            }
        }

        private static void ShotExit()
        {
            if (--shotDepth <= 0)
            {
                shotDepth = 0;
                ShotClock.Stop();
                shotZipInnerMs += ZipClock.ElapsedMilliseconds - shotZipMarkMs;
            }
        }

        /// <summary>
        /// Bracket <c>SpeciesHistoryGuard</c>'s repair. Called from the guard itself rather than
        /// patched, because the guard is ours and a Harmony patch on a Harmony prefix is a knot.
        /// </summary>
        internal static void GuardEnter() { if (guardDepth++ == 0) { GuardClock.Start(); } }

        internal static void GuardExit() { if (--guardDepth <= 0) { guardDepth = 0; GuardClock.Stop(); } }
    }
}
