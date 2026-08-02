using BepInEx;
using BepInEx.Logging;

namespace BibitesMultiverse
{
    [BepInPlugin(Guid, Name, Version)]
    public class MultiversePlugin : BaseUnityPlugin
    {
        public const string Guid = "dev.multiverse.bibites";
        public const string Name = "Bibites Multiverse";
        public const string Version = "0.1.0";

        internal static ManualLogSource Log;

        private void Awake()
        {
            Log = Logger;
            Log.LogInfo($"{Name} {Version} loaded — M1 environment smoke test.");
        }
    }
}
