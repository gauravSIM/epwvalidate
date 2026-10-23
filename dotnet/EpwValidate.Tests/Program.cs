// Tests for EpwValidator.cs. Run from this folder: dotnet run
//
// The Go version is the reference: for every test file, this version must give
// exactly the result stored in ../../testdata/expected.json, and its text
// output for the broken files must match ../../testdata/broken_results.txt.
// The checks run under a Norwegian culture to prove that the output does not
// depend on the machine's culture.

using System;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text.Json.Nodes;
using EpwValidate;

var root = AppContext.BaseDirectory;
while (!File.Exists(Path.Combine(root, "testdata", "expected.json")))
    root = Path.GetDirectoryName(root.TrimEnd(Path.DirectorySeparatorChar)) ?? throw new Exception("testdata/expected.json not found");

CultureInfo.CurrentCulture = new CultureInfo("nb-NO");
var failures = 0;

void Check(string name, bool ok, string detail = "")
{
    Console.WriteLine($"{(ok ? "ok  " : "FAIL")} {name}");
    if (!ok)
    {
        failures++;
        if (detail != "") Console.WriteLine(detail);
    }
}

var expected = JsonNode.Parse(File.ReadAllText(Path.Combine(root, "testdata", "expected.json")))!.AsObject();
var mismatches = expected
    .Where(kv => !JsonNode.DeepEquals(kv.Value, JsonNode.Parse(EpwValidator.ValidateFile(Path.Combine(root, kv.Key)).ToJson())))
    .Select(kv => kv.Key)
    .ToList();
Check($"every test file gives the expected result ({expected.Count} files)", expected.Count > 100 && mismatches.Count == 0,
    string.Join("\n", mismatches.Take(10).Select(m => "  differs: " + m)));

var simienOpt = new EpwValidator.Options(EpwValidator.Profile.Simien);
var expectedSimien = JsonNode.Parse(File.ReadAllText(Path.Combine(root, "testdata", "expected_simien.json")))!.AsObject();
var simienMismatches = expectedSimien
    .Where(kv => !JsonNode.DeepEquals(kv.Value, JsonNode.Parse(EpwValidator.ValidateFile(Path.Combine(root, kv.Key), simienOpt).ToJson())))
    .Select(kv => kv.Key)
    .ToList();
Check($"the simien profile gives the expected result ({expectedSimien.Count} files)",
    expectedSimien.Count > 100 && simienMismatches.Count == 0,
    string.Join("\n", simienMismatches.Take(10).Select(m => "  differs: " + m)));

Check("ParseProfile",
    EpwValidator.ParseProfile("") == EpwValidator.Profile.None
    && EpwValidator.ParseProfile("  SIMIEN ") == EpwValidator.Profile.Simien
    && EpwValidator.ParseProfile("energyplus") is null);

var text = string.Concat(expected
    .Select(kv => kv.Key)
    .Where(p => p.StartsWith("testdata/broken/", StringComparison.Ordinal))
    .OrderBy(p => p, StringComparer.Ordinal)
    .Select(p => EpwValidator.FormatText(p["testdata/".Length..], EpwValidator.ValidateFile(Path.Combine(root, p))) + "\n"));
var want = File.ReadAllText(Path.Combine(root, "testdata", "broken_results.txt")).Replace("\r\n", "\n");
Check("text output matches broken_results.txt", text == want);

var hurdal = File.ReadAllBytes(Path.Combine(root, "EPW_format", "epw_examples", "NOR_AK_Hurdal.NOR076_NORCERRA.epw"));
Check("validate accepts bytes and text", EpwValidator.Validate(hurdal).Valid && EpwValidator.Validate(System.Text.Encoding.UTF8.GetString(hurdal)).Valid);

var empty = EpwValidator.Validate(Array.Empty<byte>());
Check("empty input is invalid", !empty.Valid && empty.Errors[0].ToString() == "file: the file is empty [F-1]");

var threw = false;
try
{
    EpwValidator.ValidateFile(Path.Combine(root, "testdata", "does-not-exist.epw"));
}
catch (IOException)
{
    threw = true;
}
Check("missing file throws", threw);

Console.WriteLine(failures == 0 ? "PASS" : $"{failures} FAILED");
return failures == 0 ? 0 : 1;
