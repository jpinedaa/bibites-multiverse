using System;
using System.Collections.Generic;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace BibitesMultiverse
{
    /// <summary>Structural JSON comparison: the token paths where two payloads differ.</summary>
    internal static class JsonDiff
    {
        private const int ValuePreview = 60;

        internal static List<string> Compare(JToken before, JToken after, int max)
        {
            List<string> diffs = new List<string>();
            Walk("$", before, after, diffs, max);
            return diffs;
        }

        private static void Walk(string path, JToken before, JToken after, List<string> diffs, int max)
        {
            if (diffs.Count >= max)
            {
                return;
            }

            if (before == null && after == null)
            {
                return;
            }

            if (before == null)
            {
                diffs.Add($"{path}: absent before -> {Show(after)}");
                return;
            }

            if (after == null)
            {
                diffs.Add($"{path}: {Show(before)} -> absent after");
                return;
            }

            if (before.Type != after.Type)
            {
                diffs.Add($"{path}: type {before.Type} -> {after.Type} ({Show(before)} -> {Show(after)})");
                return;
            }

            if (before.Type == JTokenType.Object)
            {
                JObject a = (JObject)before;
                JObject b = (JObject)after;
                List<string> names = new List<string>();
                foreach (JProperty property in a.Properties())
                {
                    names.Add(property.Name);
                }

                foreach (JProperty property in b.Properties())
                {
                    if (!names.Contains(property.Name))
                    {
                        names.Add(property.Name);
                    }
                }

                foreach (string name in names)
                {
                    if (diffs.Count >= max)
                    {
                        return;
                    }

                    Walk($"{path}.{name}", a[name], b[name], diffs, max);
                }

                return;
            }

            if (before.Type == JTokenType.Array)
            {
                JArray a = (JArray)before;
                JArray b = (JArray)after;
                if (a.Count != b.Count)
                {
                    diffs.Add($"{path}: array length {a.Count} -> {b.Count}");
                }

                int shared = Math.Min(a.Count, b.Count);
                for (int i = 0; i < shared; i++)
                {
                    if (diffs.Count >= max)
                    {
                        return;
                    }

                    Walk($"{path}[{i}]", a[i], b[i], diffs, max);
                }

                return;
            }

            if (!JToken.DeepEquals(before, after))
            {
                diffs.Add($"{path}: {Show(before)} -> {Show(after)}");
            }
        }

        private static string Show(JToken token)
        {
            string text;
            try
            {
                text = token.ToString(Formatting.None);
            }
            catch (Exception)
            {
                text = "<unprintable>";
            }

            if (text.Length > ValuePreview)
            {
                text = text.Substring(0, ValuePreview) + "…";
            }

            return text;
        }
    }
}
