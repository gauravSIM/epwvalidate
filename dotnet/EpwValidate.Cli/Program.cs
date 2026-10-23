// Command epwvalidate checks EPW weather files.
//
//     epwvalidate [--json] [--profile simien] file.epw [more.epw ...]
//
// Exit code: 0 = all files valid, 1 = at least one file invalid,
// 2 = a file could not be read or no file was given.

using System;
using System.Collections.Generic;
using System.IO;
using System.Text;
using EpwValidate;

var asJson = false;
var profileName = "";
var paths = new List<string>();
var expectProfile = false;
foreach (var a in args)
{
    if (expectProfile)
    {
        profileName = a;
        expectProfile = false;
    }
    else if (a is "--json" or "-json") asJson = true;
    else if (a is "--profile" or "-profile") expectProfile = true;
    else if (a.StartsWith("--profile=", StringComparison.Ordinal) || a.StartsWith("-profile=", StringComparison.Ordinal))
        profileName = a[(a.IndexOf('=') + 1)..];
    else if (a is "-h" or "--help" or "-help")
    {
        paths.Clear();
        break;
    }
    else paths.Add(a);
}
if (paths.Count == 0)
{
    Console.Error.WriteLine("usage: epwvalidate [--json] [--profile simien] file.epw [more.epw ...]");
    return 2;
}

var profile = EpwValidator.ParseProfile(profileName);
if (profile is null)
{
    Console.Error.WriteLine($"unknown profile \"{profileName}\" (known profiles: simien)");
    return 2;
}
var opt = new EpwValidator.Options(profile);

Console.OutputEncoding = new UTF8Encoding(false);
var code = 0;
var reports = new List<(string Path, Report Report)>();
foreach (var path in paths)
{
    Report rep;
    try
    {
        rep = EpwValidator.ValidateFile(path, opt);
    }
    catch (Exception e) when (e is IOException or UnauthorizedAccessException)
    {
        Console.Error.WriteLine($"{path}: {e.Message}");
        code = 2;
        continue;
    }
    if (!rep.Valid && code == 0) code = 1;
    if (asJson) reports.Add((path, rep));
    else Console.Out.Write(EpwValidator.FormatText(path, rep));
}

if (asJson)
{
    using var stdout = Console.OpenStandardOutput();
    using (var w = EpwValidator.JsonWriter(stdout, indented: true))
    {
        w.WriteStartArray();
        foreach (var (path, rep) in reports) rep.WriteJson(w, ("file", path));
        w.WriteEndArray();
    }
    stdout.Write("\n"u8);
}
return code;
