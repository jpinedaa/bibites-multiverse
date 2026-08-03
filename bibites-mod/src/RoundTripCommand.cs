using System;
using System.Collections;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The manual M1 dev command: press F9 inside a running simulation to run one round trip on the
    /// selected organism (or the first living one). All of the work lives in <see cref="RoundTrip"/>.
    /// </summary>
    public class RoundTripCommand : MonoBehaviour
    {
        public const KeyCode Hotkey = KeyCode.F9;

        private bool running;

        private void Update()
        {
            try
            {
                if (!Input.GetKeyDown(Hotkey))
                {
                    return;
                }

                if (running)
                {
                    MultiversePlugin.Log.LogWarning("Round trip already running — ignoring this key press.");
                    return;
                }

                running = true;
                StartCoroutine(RunRoundTrip());
            }
            catch (Exception e)
            {
                running = false;
                MultiversePlugin.Log.LogError($"Round trip failed to start: {e}");
            }
        }

        private IEnumerator RunRoundTrip()
        {
            yield return RoundTrip.Run(new RoundTripResult());
            running = false;
        }
    }
}
