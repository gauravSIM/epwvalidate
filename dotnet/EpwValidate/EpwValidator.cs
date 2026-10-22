// epwvalidate: checks EnergyPlus Weather (EPW) files.
//
// The rules, and the rule IDs used in every Issue, are defined in SPEC.md.
// This file is a self-contained port of the Go reference version and gives
// exactly the same results (checked against testdata/expected.json). It needs
// .NET 8+ and nothing outside the base class library; copy it into any project.
//
// Use:
//
//     var report = EpwValidator.ValidateFile("file.epw"); // throws only if the file cannot be read
//     if (!report.Valid)
//         foreach (var issue in report.Errors)
//             Console.WriteLine(issue); // "line 79, Dry Bulb Temperature: ... [D-RNG-1]"
//
// report.ToJson() gives the same JSON as every other language version.

using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text;
using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.RegularExpressions;

namespace EpwValidate;

/// <summary>Whether an issue makes the file invalid.</summary>
public enum Severity
{
    /// <summary>The file is not acceptable.</summary>
    Error,
    /// <summary>The file is acceptable, but something looks wrong.</summary>
    Warning,
}

/// <summary>One finding.</summary>
public sealed class Issue
{
    public Severity Severity { get; init; }
    /// <summary>Rule ID from SPEC.md, e.g. "D-RNG-1".</summary>
    public string Rule { get; init; } = "";
    /// <summary>1-based line number; 0 = whole file.</summary>
    public int Line { get; init; }
    /// <summary>0-based field index; -1 = whole line.</summary>
    public int Column { get; init; }
    /// <summary>Field name, e.g. "Dry Bulb Temperature"; may be empty.</summary>
    public string Field { get; init; } = "";
    /// <summary>Raw text of the offending value; may be empty.</summary>
    public string Value { get; init; } = "";
    public string Message { get; init; } = "";

    public override string ToString()
    {
        var s = Line > 0 ? $"line {Line}" : "file";
        if (Field != "") s += $", {Field}";
        return $"{s}: {Message} [{Rule}]";
    }

    internal void WriteJson(Utf8JsonWriter w)
    {
        w.WriteStartObject();
        w.WriteString("severity", Severity == Severity.Error ? "error" : "warning");
        w.WriteString("rule", Rule);
        w.WriteNumber("line", Line);
        w.WriteNumber("column", Column);
        if (Field != "") w.WriteString("field", Field);
        if (Value != "") w.WriteString("value", Value);
        w.WriteString("message", Message);
        w.WriteEndObject();
    }
}

/// <summary>Counts for the whole file.</summary>
public sealed class Summary
{
    public int Lines { get; internal set; }
    public int DataRows { get; internal set; }
    /// <summary>Rows declared by DATA PERIODS; -1 if it could not be read.</summary>
    public long ExpectedRows { get; internal set; } = -1;
    public long Periods { get; internal set; }
    public long RecordsPerHour { get; internal set; } = 1;
    /// <summary>All errors, including ones not listed.</summary>
    public int ErrorCount { get; internal set; }
    /// <summary>All warnings, including ones not listed.</summary>
    public int WarningCount { get; internal set; }
    /// <summary>Missing values per data column.</summary>
    public Dictionary<string, int> Missing { get; } = new();
    /// <summary>All issues per rule, including ones not listed.</summary>
    public Dictionary<string, int> RuleCounts { get; } = new();

    internal void WriteJson(Utf8JsonWriter w)
    {
        w.WriteStartObject();
        w.WriteNumber("lines", Lines);
        w.WriteNumber("dataRows", DataRows);
        w.WriteNumber("expectedRows", ExpectedRows);
        w.WriteNumber("periods", Periods);
        w.WriteNumber("recordsPerHour", RecordsPerHour);
        w.WriteNumber("errorCount", ErrorCount);
        w.WriteNumber("warningCount", WarningCount);
        w.WriteStartObject("missing");
        foreach (var kv in Missing) w.WriteNumber(kv.Key, kv.Value);
        w.WriteEndObject();
        w.WriteStartObject("ruleCounts");
        foreach (var kv in RuleCounts) w.WriteNumber(kv.Key, kv.Value);
        w.WriteEndObject();
        w.WriteEndObject();
    }
}

/// <summary>The result of validating one file. The file is acceptable when Valid is true; warnings do not affect Valid.</summary>
public sealed class Report
{
    public bool Valid { get; internal set; }
    public List<Issue> Errors { get; } = new();
    public List<Issue> Warnings { get; } = new();
    public Summary Summary { get; } = new();

    /// <summary>The report as JSON, with the same keys as every other language version.</summary>
    public string ToJson(bool indented = true)
    {
        using var ms = new MemoryStream();
        using (var w = EpwValidator.JsonWriter(ms, indented)) WriteJson(w);
        return Encoding.UTF8.GetString(ms.ToArray());
    }

    /// <summary>Writes the report as a JSON object; extra properties (such as a file name) are written first.</summary>
    public void WriteJson(Utf8JsonWriter w, params (string Name, string Value)[] extra)
    {
        w.WriteStartObject();
        foreach (var (name, value) in extra) w.WriteString(name, value);
        w.WriteBoolean("valid", Valid);
        w.WriteStartArray("errors");
        foreach (var i in Errors) i.WriteJson(w);
        w.WriteEndArray();
        w.WriteStartArray("warnings");
        foreach (var i in Warnings) i.WriteJson(w);
        w.WriteEndArray();
        w.WritePropertyName("summary");
        Summary.WriteJson(w);
        w.WriteEndObject();
    }
}

/// <summary>Validates EPW files.</summary>
public static class EpwValidator
{
    /// <summary>How many issues of one rule are listed in a Report; Summary.RuleCounts always holds the full count.</summary>
    public const int MaxIssuesPerRule = 20;

    /// <summary>Validates the EPW file at path. Throws only for I/O failures; problems with the content are in the Report.</summary>
    public static Report ValidateFile(string path) => Validate(File.ReadAllBytes(path));

    /// <summary>Validates the content of an EPW file.</summary>
    public static Report Validate(byte[] data) => Invariant(() =>
    {
        var c = new Checker();
        c.Run(data);
        c.Rep.Valid = c.Rep.Summary.ErrorCount == 0;
        return c.Rep;
    });

    /// <summary>Validates the content of an EPW file given as text.</summary>
    public static Report Validate(string text) => Validate(Encoding.UTF8.GetBytes(text));

    /// <summary>Returns the report as the command-line tool prints it.</summary>
    public static string FormatText(string name, Report rep) => Invariant(() =>
    {
        var s = rep.Summary;
        var b = new StringBuilder();
        b.Append($"{name}: {(rep.Valid ? "VALID" : "INVALID")} - {s.DataRows} data rows");
        if (s.ExpectedRows >= 0) b.Append($" (DATA PERIODS declares {s.ExpectedRows})");
        b.Append($", {s.ErrorCount} errors, {s.WarningCount} warnings\n");
        foreach (var i in rep.Errors) b.Append($"  error    {i}\n");
        foreach (var i in rep.Warnings) b.Append($"  warning  {i}\n");
        foreach (var rule in s.RuleCounts.Where(kv => kv.Value > MaxIssuesPerRule).Select(kv => kv.Key).OrderBy(r => r, StringComparer.Ordinal))
            b.Append($"  ... and {s.RuleCounts[rule] - MaxIssuesPerRule} more [{rule}]\n");
        return b.ToString();
    });

    /// <summary>Runs f with the invariant culture, so numbers in messages look the same on every machine.</summary>
    static T Invariant<T>(Func<T> f)
    {
        var culture = CultureInfo.CurrentCulture;
        CultureInfo.CurrentCulture = CultureInfo.InvariantCulture;
        try
        {
            return f();
        }
        finally
        {
            CultureInfo.CurrentCulture = culture;
        }
    }

    /// <summary>A JSON writer that leaves characters such as &gt; unescaped, like the other language versions.</summary>
    public static Utf8JsonWriter JsonWriter(Stream stream, bool indented) =>
        new(stream, new JsonWriterOptions { Indented = indented, Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping });

    // ---- Text helpers that behave like the Go version ----

    static readonly Regex NumberRe = new(@"\A[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?\z", RegexOptions.CultureInvariant);
    static readonly Regex IntRe = new(@"\A[+-]?[0-9]+\z", RegexOptions.CultureInvariant);

    // string.Trim() uses the same whitespace set as Go's strings.TrimSpace.
    static string T(string s) => s.Trim();

    /// <summary>Plain decimal numbers only: no "NaN", "Inf", hex or thousands separators.</summary>
    static double? ParseNumber(string s)
    {
        s = T(s);
        if (!NumberRe.IsMatch(s)) return null;
        var v = double.Parse(s, NumberStyles.Float, CultureInfo.InvariantCulture);
        return double.IsFinite(v) ? v : null;
    }

    static long? ParseInt(string s)
    {
        s = T(s);
        if (!IntRe.IsMatch(s)) return null;
        return long.TryParse(s, NumberStyles.AllowLeadingSign, CultureInfo.InvariantCulture, out var v) ? v : null;
    }

    /// <summary>Formats a number like Go's %g for the values used in messages.</summary>
    static string G(double x)
    {
        if (x == Math.Floor(x) && Math.Abs(x) < 1e15)
            return x == 0 && double.IsNegative(x) ? "-0" : ((long)x).ToString(CultureInfo.InvariantCulture);
        return x.ToString("R", CultureInfo.InvariantCulture);
    }

    /// <summary>Quotes a string like Go's %q.</summary>
    static string Q(string s)
    {
        var b = new StringBuilder("\"");
        foreach (var r in s.EnumerateRunes())
        {
            int o = r.Value;
            switch (o)
            {
                case '"': b.Append("\\\""); continue;
                case '\\': b.Append("\\\\"); continue;
                case 7: b.Append("\\a"); continue;
                case '\b': b.Append("\\b"); continue;
                case '\f': b.Append("\\f"); continue;
                case '\n': b.Append("\\n"); continue;
                case '\r': b.Append("\\r"); continue;
                case '\t': b.Append("\\t"); continue;
                case '\v': b.Append("\\v"); continue;
            }
            if (o < 0x20 || o == 0x7f) b.Append($"\\x{o:x2}");
            else if (o != ' ' && !IsPrintable(r)) b.Append(o < 0x10000 ? $"\\u{o:x4}" : $"\\U{o:x8}");
            else b.Append(r.ToString());
        }
        return b.Append('"').ToString();
    }

    static bool IsPrintable(Rune r) => Rune.GetUnicodeCategory(r) switch
    {
        UnicodeCategory.Control or UnicodeCategory.Format or UnicodeCategory.Surrogate or UnicodeCategory.PrivateUse
            or UnicodeCategory.OtherNotAssigned or UnicodeCategory.SpaceSeparator or UnicodeCategory.LineSeparator
            or UnicodeCategory.ParagraphSeparator => false,
        _ => true,
    };

    static string Truncate(string s, int n)
    {
        var runes = s.EnumerateRunes().ToList();
        return runes.Count <= n ? s : string.Concat(runes.Take(n).Select(r => r.ToString())) + "...";
    }

    static double RoundHalfAway(double x) => Math.CopySign(Math.Floor(Math.Abs(x) + 0.5), x);

    static bool EqFold(string a, string b) => string.Equals(a, b, StringComparison.OrdinalIgnoreCase);

    // A bound is one side of an allowed range; Strict means the bound itself is not allowed (> or <).
    internal readonly record struct Bound(double V, bool Strict);

    static Bound? Gt(double v) => new Bound(v, true);
    static Bound? Ge(double v) => new Bound(v, false);
    static Bound? Lt(double v) => new Bound(v, true);
    static Bound? Le(double v) => new Bound(v, false);

    static bool InRange(double v, Bound? lo, Bound? hi)
    {
        if (lo is { } l && (v < l.V || (l.Strict && v == l.V))) return false;
        if (hi is { } h && (v > h.V || (h.Strict && v == h.V))) return false;
        return true;
    }

    static string RangeText(Bound? lo, Bound? hi)
    {
        var parts = new List<string>();
        if (lo is { } l) parts.Add($"{(l.Strict ? ">" : ">=")} {G(l.V)}");
        if (hi is { } h) parts.Add($"{(h.Strict ? "<" : "<=")} {G(h.V)}");
        return string.Join(" and ", parts);
    }

    // ---- Dates (SPEC.md §5.9) ----

    static readonly string[] MonthNames = { "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december" };
    static readonly string[] WeekdayNames = { "sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday" };

    /// <summary>Returns 1-12 for a full or 3-letter month name, 0 otherwise.</summary>
    static int MonthIndex(string s)
    {
        s = T(s).ToLowerInvariant();
        for (int i = 0; i < MonthNames.Length; i++)
            if (s == MonthNames[i] || s == MonthNames[i][..3]) return i + 1;
        return 0;
    }

    /// <summary>Returns 0 (Sunday) to 6, or -1.</summary>
    static int WeekdayIndex(string s)
    {
        s = T(s).ToLowerInvariant();
        for (int i = 0; i < WeekdayNames.Length; i++)
            if (s == WeekdayNames[i] || s == WeekdayNames[i][..3]) return i;
        return -1;
    }

    static bool IsLeap(long y) => y % 4 == 0 && (y % 100 != 0 || y % 400 == 0);

    static long DaysIn(long month, bool feb29) => month switch
    {
        2 => feb29 ? 29 : 28,
        4 or 6 or 9 or 11 => 30,
        _ => 31,
    };

    /// <summary>A month/day, with a year only when the date had one (Y = 0 otherwise).</summary>
    internal readonly record struct CalDate(long Y, long M, long D)
    {
        public bool Before(CalDate b)
        {
            if (Y != 0 && b.Y != 0 && Y != b.Y) return Y < b.Y;
            if (M != b.M) return M < b.M;
            return D < b.D;
        }

        public override string ToString() => Y != 0 ? $"{M}/{D}/{Y}" : $"{M}/{D}";
    }

    sealed class DateException(string message) : Exception(message);

    /// <summary>Checks one header date field and returns it as month/day (plus year for m/d/y). Weekday forms return a zero date.</summary>
    static CalDate ParseDate(string s, bool allowYear = false, bool allowWeekday = false)
    {
        s = T(s);
        if (s == "") throw new DateException("date is empty");

        if (s.Contains('/'))
        {
            var parts = s.Split('/');
            if (parts.Length == 3 && !allowYear) throw new DateException($"date {Q(s)} has a year; m/d/y is allowed only in DATA PERIODS");
            if (parts.Length != 2 && parts.Length != 3) throw new DateException($"date {Q(s)} is not in m/d form");
            var n = new long[3];
            for (int i = 0; i < parts.Length; i++)
                n[i] = ParseInt(parts[i].Replace(" ", "")) ?? throw new DateException($"date {Q(s)} is not in m/d form");
            var d = new CalDate(0, n[0], n[1]);
            if (parts.Length == 3)
            {
                if (n[2] < 1) throw new DateException($"date {Q(s)} has an invalid year");
                d = new CalDate(n[2], n[0], n[1]);
            }
            CheckMonthDay(s, d);
            return d;
        }

        if (ParseInt(s) is { } j)
        {
            if (j < 1 || j > 366) throw new DateException($"day of year {j} is outside 1-366");
            return JulianToDate(j);
        }

        var tok = Regex.Split(s, @"\s+").Where(t => t != "").ToArray();
        if (tok.Length == 2)
        {
            var m = MonthIndex(tok[0]);
            if (m > 0 && ParseInt(tok[1]) is { } d1)
            {
                var cd = new CalDate(0, m, d1);
                CheckMonthDay(s, cd);
                return cd;
            }
            m = MonthIndex(tok[1]);
            if (m > 0 && ParseInt(tok[0]) is { } d2)
            {
                var cd = new CalDate(0, m, d2);
                CheckMonthDay(s, cd);
                return cd;
            }
        }
        if (tok.Length == 4 && EqFold(tok[2], "in"))
        {
            if (!allowWeekday) throw new DateException($"weekday date {Q(s)} is allowed only for holidays and daylight saving");
            var n = ParseInt(tok[0]);
            if (WeekdayIndex(tok[1]) >= 0 && MonthIndex(tok[3]) > 0 && (EqFold(tok[0], "last") || (n is >= 1 and <= 5)))
                return default;
        }
        throw new DateException($"{Q(s)} is not a recognised EPW date (use m/d, e.g. 1/31)");
    }

    static void CheckMonthDay(string s, CalDate d)
    {
        if (d.M < 1 || d.M > 12) throw new DateException($"date {Q(s)}: month {d.M} is outside 1-12");
        var feb29 = d.Y == 0 || IsLeap(d.Y);
        if (d.D < 1 || d.D > DaysIn(d.M, feb29)) throw new DateException($"date {Q(s)}: day {d.D} does not exist in month {d.M}");
    }

    /// <summary>Converts a day of year (non-leap calendar) to month/day; 366 = 31 December.</summary>
    static CalDate JulianToDate(long j)
    {
        if (j == 366) return new CalDate(0, 12, 31);
        long m = 1;
        while (j > DaysIn(m, false))
        {
            j -= DaysIn(m, false);
            m++;
        }
        return new CalDate(0, m, j);
    }

    // ---- Header records (SPEC.md §4-§5) ----

    const int HeaderLines = 8;
    static readonly string[] HeaderKeywords =
    {
        "LOCATION", "DESIGN CONDITIONS", "TYPICAL/EXTREME PERIODS", "GROUND TEMPERATURES",
        "HOLIDAYS/DAYLIGHT SAVINGS", "COMMENTS 1", "COMMENTS 2", "DATA PERIODS",
    };

    static bool MatchesKeyword(int pos, string s)
    {
        s = T(s).ToUpperInvariant();
        return s == HeaderKeywords[pos] || (pos == 4 && s == "HOLIDAYS/DAYLIGHT SAVING");
    }

    static int HeaderPosition(string s)
    {
        for (int i = 0; i < HeaderLines; i++)
            if (MatchesKeyword(i, s)) return i;
        return -1;
    }

    /// <summary>Splits a header line, honouring double-quoted fields. Inside a quoted field "" is a quote; a quote that does not end the field is kept as text.</summary>
    static List<string> SplitCsv(string line)
    {
        var fields = new List<string>();
        while (true)
        {
            if (!line.StartsWith('"'))
            {
                int i = line.IndexOf(',');
                if (i < 0)
                {
                    fields.Add(line);
                    return fields;
                }
                fields.Add(line[..i]);
                line = line[(i + 1)..];
                continue;
            }
            var b = new StringBuilder();
            line = line[1..];
            while (true)
            {
                int i = line.IndexOf('"');
                if (i < 0)
                {
                    fields.Add(b.Append(line).ToString());
                    return fields;
                }
                b.Append(line, 0, i);
                line = line[(i + 1)..];
                if (line.StartsWith('"'))
                {
                    b.Append('"');
                    line = line[1..];
                }
                else if (line == "")
                {
                    fields.Add(b.ToString());
                    return fields;
                }
                else if (line[0] == ',')
                {
                    line = line[1..];
                    break;
                }
                else
                {
                    b.Append('"');
                }
            }
            fields.Add(b.ToString());
        }
    }

    static (List<string>, bool) TrimTrailingEmpty(List<string> f)
    {
        int n = f.Count;
        while (n > 1 && T(f[n - 1]) == "") n--;
        return (f.GetRange(0, n), n < f.Count);
    }

    static string F(List<string> f, long i) => i < f.Count ? T(f[(int)i]) : "";

    static readonly string[] DesignSections = { "Heating", "Cooling", "Extremes" };

    // Number of Heating, Cooling and Extremes values for known ASHRAE Handbook editions (H-DC-5).
    static readonly (string Edition, int[] Counts)[] DesignLayouts =
    {
        ("2021 ASHRAE Handbook", new[] { 16, 32, 15 }),
        ("2025 ASHRAE Handbook", new[] { 16, 32, 27 }),
    };

    internal sealed record Period(string Name, CalDate Start, CalDate End)
    {
        public string Label => Name == "" ? "unnamed" : Name;
    }

    // ---- Data rows (SPEC.md §6-§9) ----

    const int NumFields = 35;

    internal sealed record Column(string Name, Bound? Lo = null, Bound? Hi = null, double Missing = 0, bool Core = false, bool WarnNegative = false);

    static readonly Column[] Columns =
    {
        new("Year"),
        new("Month"),
        new("Day"),
        new("Hour"),
        new("Minute"),
        new("Data Source and Uncertainty Flags"),
        new("Dry Bulb Temperature", Gt(-70), Lt(70), 99.9, Core: true),
        new("Dew Point Temperature", Gt(-70), Lt(70), 99.9),
        new("Relative Humidity", Ge(0), Le(110), 999, Core: true),
        new("Atmospheric Station Pressure", Gt(31000), Lt(120000), 999999, Core: true),
        new("Extraterrestrial Horizontal Radiation", Ge(0), null, 9999),
        new("Extraterrestrial Direct Normal Radiation", Ge(0), null, 9999),
        new("Horizontal Infrared Radiation Intensity", Ge(0), null, 9999),
        new("Global Horizontal Radiation", Ge(0), null, 9999, Core: true),
        new("Direct Normal Radiation", Ge(0), null, 9999, Core: true),
        new("Diffuse Horizontal Radiation", Ge(0), null, 9999, Core: true),
        new("Global Horizontal Illuminance", Ge(0), null, 999900),
        new("Direct Normal Illuminance", Ge(0), null, 999900),
        new("Diffuse Horizontal Illuminance", Ge(0), null, 999900),
        new("Zenith Luminance", Ge(0), null, 9999),
        new("Wind Direction", Ge(0), Le(360), 999, Core: true),
        new("Wind Speed", Ge(0), Le(40), 999, Core: true),
        new("Total Sky Cover", Ge(0), Le(10), 99),
        new("Opaque Sky Cover", Ge(0), Le(10), 99),
        new("Visibility", Missing: 9999, WarnNegative: true),
        new("Ceiling Height", Missing: 99999, WarnNegative: true),
        new("Present Weather Observation"),
        new("Present Weather Codes"),
        new("Precipitable Water", Missing: 999, WarnNegative: true),
        new("Aerosol Optical Depth", Missing: 0.999, WarnNegative: true),
        new("Snow Depth", Missing: 999, WarnNegative: true),
        new("Days Since Last Snowfall", Missing: 99, WarnNegative: true),
        new("Albedo", Missing: 999, WarnNegative: true),
        new("Liquid Precipitation Depth", Missing: 999, WarnNegative: true),
        new("Liquid Precipitation Quantity", Missing: 99, WarnNegative: true),
    };

    const int ColYear = 0, ColMinute = 4, ColFlags = 5, ColDryBulb = 6, ColDewPoint = 7, ColRelHumidity = 8, ColWxObs = 26, ColWxCodes = 27;

    static bool IsNineDigits(string s) => s.Length == 9 && s.All(ch => ch is >= '0' and <= '9');

    /// <summary>22 pairs of a source letter (A-I or ?) and an uncertainty digit.</summary>
    static bool FlagsFollowFormat(string s)
    {
        if (s.Length != 44) return false;
        for (int i = 0; i < 44; i += 2)
            if (!"ABCDEFGHI?".Contains(s[i]) || s[i + 1] is < '0' or > '9') return false;
        return true;
    }

    // ---- Time sequence (SPEC.md §7) ----

    /// <summary>The time of one data row. Step is the record within the hour; Y = 0 means the year is unknown.</summary>
    internal readonly record struct Stamp(long Y, long M, long D, long H, long Step)
    {
        public override string ToString()
        {
            var t = Y != 0 ? $"{M}/{D}/{Y} hour {H}" : $"{M}/{D} hour {H}";
            if (Step > 1) t += $" step {Step}";
            return t;
        }
    }

    static bool SameTime(Stamp a, Stamp b, bool withYear) =>
        a.M == b.M && a.D == b.D && a.H == b.H && a.Step == b.Step && (!withYear || a.Y == b.Y);

    // ---- The checker ----

    sealed class Checker
    {
        public readonly Report Rep = new();
        bool leapYes; // leap-year field is Yes
        long perHour = 1; // records per hour from DATA PERIODS
        List<Period>? periods; // null when DATA PERIODS could not be read
        int rowsChecked; // data rows with 35 fields
        readonly int[] missing = new int[NumFields];
        readonly int[] firstMissing = new int[NumFields];
        int badFlags;
        int firstBadFlags;
        string badFlagsValue = "";
        // time-sequence state (see SeqRow)
        int seqP;
        bool seqInside;
        Stamp seqCur;
        bool seqDone;
        int seqExtra;
        int seqFirstExtra;
        long[] febYear = Array.Empty<long>();

        static readonly UTF8Encoding StrictUtf8 = new(false, true);

        // -- issues --

        void Add(Severity sev, string rule, int line, long col, string field, string value, string msg)
        {
            var s = Rep.Summary;
            s.RuleCounts[rule] = s.RuleCounts.GetValueOrDefault(rule) + 1;
            if (sev == Severity.Error) s.ErrorCount++;
            else s.WarningCount++;
            if (s.RuleCounts[rule] > MaxIssuesPerRule) return;
            var issue = new Issue { Severity = sev, Rule = rule, Line = line, Column = (int)col, Field = field, Value = value, Message = msg };
            (sev == Severity.Error ? Rep.Errors : Rep.Warnings).Add(issue);
        }

        void Err(string rule, int line, long col, string field, string value, string msg) => Add(Severity.Error, rule, line, col, field, value, msg);

        void Warn(string rule, int line, long col, string field, string value, string msg) => Add(Severity.Warning, rule, line, col, field, value, msg);

        // -- file level --

        public void Run(byte[] data)
        {
            if (data.All(b => b is (byte)' ' or (byte)'\t' or (byte)'\n' or (byte)'\r' or 0x0b or 0x0c))
            {
                Err("F-1", 0, -1, "", "", "the file is empty");
                return;
            }
            int offset = 0;
            if (data.Length >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf)
            {
                Err("F-2", 1, -1, "", "", "the file starts with a UTF-8 byte-order mark (BOM); save it without BOM");
                offset = 3;
            }

            var raw = new List<ArraySegment<byte>>();
            int crlf = 0, lf = 0, start = offset;
            for (int i = offset; i < data.Length; i++)
            {
                if (data[i] != (byte)'\n') continue;
                if (i > offset && data[i - 1] == (byte)'\r') crlf++;
                else lf++;
                raw.Add(new ArraySegment<byte>(data, start, i - start));
                start = i + 1;
            }
            raw.Add(new ArraySegment<byte>(data, start, data.Length - start));
            if (crlf > 0 && lf > 0) Warn("F-7", 0, -1, "", "", $"mixed line endings: {crlf} lines end with CRLF and {lf} with LF");

            for (int i = 0; i < raw.Count; i++)
                if (raw[i].Count > 0 && raw[i][raw[i].Count - 1] == (byte)'\r') raw[i] = raw[i].Slice(0, raw[i].Count - 1);
            var lines = raw.Select(r => Encoding.UTF8.GetString(r.Array!, r.Offset, r.Count)).ToList();
            while (lines.Count > 0 && T(lines[^1]) == "")
            {
                lines.RemoveAt(lines.Count - 1);
                raw.RemoveAt(raw.Count - 1);
            }
            Rep.Summary.Lines = lines.Count;

            for (int i = 0; i < lines.Count; i++)
            {
                try
                {
                    StrictUtf8.GetString(raw[i].Array!, raw[i].Offset, raw[i].Count);
                }
                catch (DecoderFallbackException)
                {
                    Err("F-1", i + 1, -1, "", "", "line is not valid UTF-8; save the file as UTF-8");
                }
                if (T(lines[i]).EndsWith(';')) Err("F-5", i + 1, -1, "", "", "line ends with ';' (semicolons do not end lines in EPW)");
            }

            int dataStart = CheckHeaders(lines);
            for (int i = dataStart; i < lines.Count; i++)
            {
                if (T(lines[i]) == "")
                {
                    Err("F-4", i + 1, -1, "", "", "empty line inside the data");
                    continue;
                }
                CheckRow(i + 1, lines[i]);
            }
            FinishData();
            FinishSequence();
        }

        // -- headers --

        /// <summary>Checks the 8 header records and returns the index of the first data line.</summary>
        int CheckHeaders(List<string> lines)
        {
            int i = 0, pos = 0;
            while (pos < HeaderLines)
            {
                if (i >= lines.Count)
                {
                    Err("H-ORD-1", 0, -1, "", "", $"the file ends before the {HeaderKeywords[pos]} record (it has {lines.Count} lines)");
                    return i;
                }
                int ln = i + 1;
                var line = lines[i];
                int comma = line.IndexOf(',');
                var first = comma < 0 ? line : line[..comma];
                int q = HeaderPosition(first);
                if (q > pos || q < 0)
                {
                    var found = $"the {HeaderKeywords[Math.Max(q, 0)]} record";
                    if (q < 0)
                    {
                        found = Q(Truncate(T(first), 40));
                        if (ParseInt(first) != null) found = "a data row";
                    }
                    Err("H-ORD-1", ln, 0, "", T(first), $"the {HeaderKeywords[pos]} record is missing: line {ln} should hold it, but holds {found}");
                    pos++;
                    continue;
                }
                if (q < pos)
                {
                    Err("H-ORD-1", ln, 0, "", T(first), $"the {HeaderKeywords[q]} record is out of order or repeated; it belongs on line {q + 1}");
                    i++;
                    continue;
                }
                i++;
                List<string> f;
                if (pos == 0)
                {
                    if (line.Contains('"')) Err("F-6", ln, -1, "LOCATION", "", "LOCATION must not contain double quotes; many EPW readers split this line on bare commas");
                    f = line.Split(',').ToList();
                }
                else
                {
                    f = SplitCsv(line);
                }
                if (pos != 5 && pos != 6)
                {
                    (f, var trimmed) = TrimTrailingEmpty(f);
                    if (trimmed) Warn("H-TRAIL", ln, -1, HeaderKeywords[pos], "", $"{HeaderKeywords[pos]} ends with empty fields; they are ignored");
                }
                switch (pos)
                {
                    case 0: CheckLocation(ln, f); break;
                    case 1: CheckDesignConditions(ln, f); break;
                    case 2: CheckTypicalExtreme(ln, f); break;
                    case 3: CheckGroundTemperatures(ln, f); break;
                    case 4: CheckHolidays(ln, f); break;
                    case 5:
                    case 6:
                        if (F(f, 1) == "") Err("H-COM-1", ln, 1, HeaderKeywords[pos], "", $"{HeaderKeywords[pos]} has no text");
                        break;
                    case 7: CheckDataPeriods(ln, f); break;
                }
                pos++;
            }
            return i;
        }

        /// <summary>Checks that field i is a number inside [lo, hi].</summary>
        (double, bool) HeaderNumber(string rule, int ln, List<string> f, long i, string name, Bound? lo, Bound? hi)
        {
            var s = F(f, i);
            if (ParseNumber(s) is not { } v)
            {
                Err(rule, ln, i, name, s, $"{name} {Q(s)} is not a number");
                return (0, false);
            }
            if (!InRange(v, lo, hi))
            {
                Err(rule, ln, i, name, s, $"{name} = {s} is outside the allowed range {RangeText(lo, hi)}");
                return (v, false);
            }
            return (v, true);
        }

        /// <summary>Reads the "number of ..." field at index 1; null if invalid.</summary>
        long? HeaderCount(string rule, int ln, List<string> f, string name)
        {
            var s = F(f, 1);
            if (ParseInt(s) is { } n && n >= 0) return n;
            Err(rule, ln, 1, name, s, $"{name} {Q(s)} is not a whole number >= 0");
            return null;
        }

        void CheckLocation(int ln, List<string> f)
        {
            if (f.Count != 10)
                Err("H-LOC-1", ln, -1, "LOCATION", "", $"LOCATION has {f.Count} fields, expected 10 (keyword, city, region, country, source, WMO, latitude, longitude, time zone, elevation)");
            if (F(f, 1) == "") Err("H-LOC-2", ln, 1, "City", "", "city is empty");
            foreach (var (i, name) in new[] { (2, "State Province Region"), (3, "Country"), (4, "Source"), (5, "WMO") })
                if (F(f, i) == "") Err("H-LOC-3", ln, i, name, "", $"{name} is empty");
            HeaderNumber("H-LOC-4", ln, f, 6, "Latitude", Ge(-90), Le(90));
            var (lon, lonOk) = HeaderNumber("H-LOC-5", ln, f, 7, "Longitude", Ge(-180), Le(180));
            var (tz, tzOk) = HeaderNumber("H-LOC-6", ln, f, 8, "Time Zone", Ge(-12), Le(12));
            HeaderNumber("H-LOC-7", ln, f, 9, "Elevation", Ge(-1000), Lt(9999.9));
            if (lonOk && tzOk)
            {
                var want = RoundHalfAway(lon / 15);
                if (Math.Abs(tz - want) > 3)
                    Warn("H-LOC-8", ln, 8, "Time Zone", F(f, 8), $"time zone {G(tz)} h is far from the {G(want)} h expected at longitude {G(lon)}; check the sign");
            }
        }

        void CheckDesignConditions(int ln, List<string> f)
        {
            if (HeaderCount("H-DC-1", ln, f, "Number of Design Conditions") is not { } n) return;
            if (n == 0)
            {
                if (f.Count > 2) Err("H-DC-2", ln, 2, "DESIGN CONDITIONS", "", "number of design conditions is 0, but the record has more fields");
                return;
            }
            if (F(f, 2) == "")
            {
                Err("H-DC-3", ln, 2, "Design Condition Source", "", "design condition source is empty");
                return;
            }
            CheckDesignSections(ln, n, f);
        }

        void CheckDesignSections(int ln, long n, List<string> f)
        {
            const string rec = "DESIGN CONDITIONS";
            int i = 3;
            if (F(f, i) == "" && EqFold(F(f, i + 1), DesignSections[0])) i++; // some writers leave one empty field after the source
            var counts = new int[3];
            for (int s = 0; s < DesignSections.Length; s++)
            {
                var name = DesignSections[s];
                if (!EqFold(F(f, i), name))
                {
                    Err("H-DC-4", ln, i, rec, F(f, i), $"expected the {name} section at field {i + 1}, found {Q(Truncate(F(f, i), 20))}");
                    return;
                }
                i++;
                int start = i;
                while (i < f.Count && (s == DesignSections.Length - 1 || !EqFold(F(f, i), DesignSections[s + 1])))
                {
                    var v = F(f, i);
                    if (v == "") Err("H-DC-4", ln, i, name + " value", v, $"{name} value at field {i + 1} is empty");
                    else if (ParseNumber(v) == null && !EqFold(v, "N/A")) // ASHRAE writes N/A for "not available"
                        Err("H-DC-4", ln, i, name + " value", v, $"{name} value {Q(v)} at field {i + 1} is not a number");
                    i++;
                }
                counts[s] = i - start;
                if (counts[s] == 0) Err("H-DC-4", ln, i, rec, "", $"the {name} section has no values");
            }
            if (n != 1) return;
            foreach (var (edition, want) in DesignLayouts)
                if (F(f, 2).Contains(edition, StringComparison.Ordinal) && !counts.SequenceEqual(want))
                    Err("H-DC-5", ln, -1, rec, "", $"{edition} design conditions have {want[0]} Heating, {want[1]} Cooling and {want[2]} Extremes values; found {counts[0]}, {counts[1]} and {counts[2]}");
        }

        void CheckTypicalExtreme(int ln, List<string> f)
        {
            if (HeaderCount("H-TEP-1", ln, f, "Number of Typical/Extreme Periods") is not { } n) return;
            long want = 2 + 4 * n;
            if (f.Count != want)
            {
                Err("H-TEP-2", ln, -1, "TYPICAL/EXTREME PERIODS", "", $"{n} periods declared, so {want} fields are expected; found {f.Count}");
                return;
            }
            for (long k = 0; k < n; k++)
            {
                long b = 2 + 4 * k;
                if (F(f, b) == "") Err("H-TEP-5", ln, b, $"Period {k + 1} Name", "", "typical/extreme period name is empty");
                var t = F(f, b + 1).ToLowerInvariant();
                if (t == "") Err("H-TEP-5", ln, b + 1, $"Period {k + 1} Type", "", "typical/extreme period type is empty");
                else if (t != "typical" && t != "extreme")
                    Warn("H-TEP-3", ln, b + 1, $"Period {k + 1} Type", F(f, b + 1), $"period type {Q(F(f, b + 1))} is not Typical or Extreme");
                string[] whats = { "Start Day", "End Day" };
                for (int j = 0; j < 2; j++)
                {
                    var s = F(f, b + 2 + j);
                    try
                    {
                        ParseDate(s);
                    }
                    catch (DateException e)
                    {
                        Err("H-TEP-4", ln, b + 2 + j, $"Period {k + 1} {whats[j]}", s, e.Message);
                    }
                }
            }
        }

        void CheckGroundTemperatures(int ln, List<string> f)
        {
            if (HeaderCount("H-GT-1", ln, f, "Number of Ground Temperature Depths") is not { } n) return;
            long want = 2 + 16 * n;
            if (f.Count != want)
            {
                Err("H-GT-2", ln, -1, "GROUND TEMPERATURES", "", $"{n} depths declared, so {want} fields are expected; found {f.Count}");
                return;
            }
            double prev = double.NegativeInfinity;
            for (long k = 0; k < n; k++)
            {
                long b = 2 + 16 * k;
                var (depth, ok) = HeaderNumber("H-GT-3", ln, f, b, $"Depth {k + 1}", Gt(0), null);
                if (ok)
                {
                    if (depth < prev) Warn("H-GT-6", ln, b, $"Depth {k + 1}", F(f, b), "depths are not in increasing order");
                    prev = depth;
                }
                string[] soil = { "Soil Conductivity", "Soil Density", "Soil Specific Heat" };
                for (int j = 0; j < 3; j++)
                    if (F(f, b + 1 + j) != "") HeaderNumber("H-GT-4", ln, f, b + 1 + j, $"Depth {k + 1} {soil[j]}", Gt(0), null);
                for (int m = 0; m < 12; m++)
                    HeaderNumber("H-GT-5", ln, f, b + 4 + m, $"Depth {k + 1} month {m + 1} ground temperature", Gt(-70), Lt(70));
            }
        }

        void CheckHolidays(int ln, List<string> f)
        {
            var leap = F(f, 1).ToLowerInvariant();
            if (leap == "yes") leapYes = true;
            else if (leap != "no") Err("H-HOL-1", ln, 1, "LeapYear Observed", F(f, 1), $"leap-year field {Q(F(f, 1))} must be Yes or No");
            string[] names = { "Daylight Saving Start Day", "Daylight Saving End Day" };
            for (int j = 0; j < 2; j++)
            {
                var s = F(f, 2 + j);
                if (s == "0") continue;
                try
                {
                    ParseDate(s, allowWeekday: true);
                }
                catch (DateException e)
                {
                    Err("H-HOL-2", ln, 2 + j, names[j], s, $"{e.Message} (use 0 for no daylight saving)");
                }
            }
            if (ParseInt(F(f, 4)) is not { } n || n < 0)
            {
                Err("H-HOL-3", ln, 4, "Number of Holidays", F(f, 4), $"number of holidays {Q(F(f, 4))} is not a whole number >= 0");
                return;
            }
            long want = 5 + 2 * n;
            if (f.Count != want)
            {
                Err("H-HOL-3", ln, -1, "HOLIDAYS/DAYLIGHT SAVINGS", "", $"{n} holidays declared, so {want} fields are expected; found {f.Count}");
                return;
            }
            for (long k = 0; k < n; k++)
            {
                long b = 5 + 2 * k;
                if (F(f, b) == "") Err("H-HOL-4", ln, b, $"Holiday {k + 1} Name", "", "holiday name is empty");
                try
                {
                    ParseDate(F(f, b + 1), allowWeekday: true);
                }
                catch (DateException e)
                {
                    Err("H-HOL-4", ln, b + 1, $"Holiday {k + 1} Day", F(f, b + 1), e.Message);
                }
            }
        }

        void CheckDataPeriods(int ln, List<string> f)
        {
            const string rec = "DATA PERIODS";
            if (ParseInt(F(f, 1)) is not { } p || p < 1)
            {
                Err("H-DP-1", ln, 1, "Number of Data Periods", F(f, 1), $"number of data periods {Q(F(f, 1))} is not a whole number >= 1");
                return;
            }
            if (ParseInt(F(f, 2)) is not { } r || r < 1 || r > 60 || 60 % r != 0)
            {
                Err("H-DP-2", ln, 2, "Number of Records per Hour", F(f, 2), $"records per hour {Q(F(f, 2))} must be one of 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60");
                return;
            }
            perHour = r;
            Rep.Summary.RecordsPerHour = r;
            long want = 3 + 4 * p;
            if (f.Count != want)
            {
                Err("H-DP-3", ln, -1, rec, "", $"{p} data periods declared, so {want} fields are expected; found {f.Count}");
                return;
            }

            var list = new List<Period>();
            bool ok = true;
            string[] whats = { "Start Day", "End Day" };
            for (long k = 0; k < p; k++)
            {
                long b = 3 + 4 * k;
                var name = F(f, b);
                if (name == "") Err("H-DP-4", ln, b, $"Data Period {k + 1} Name", "", "data period name is empty");
                if (WeekdayIndex(F(f, b + 1)) < 0)
                    Err("H-DP-5", ln, b + 1, $"Data Period {k + 1} Start Day of Week", F(f, b + 1), $"start day of week {Q(F(f, b + 1))} is not a weekday name (Sunday ... Saturday)");
                var ends = new CalDate[2];
                for (int j = 0; j < 2; j++)
                {
                    var s = F(f, b + 2 + j);
                    try
                    {
                        ends[j] = ParseDate(s, allowYear: true);
                    }
                    catch (DateException e)
                    {
                        Err("H-DP-6", ln, b + 2 + j, $"Data Period {k + 1} {whats[j]}", s, e.Message);
                        ok = false;
                    }
                }
                list.Add(new Period(name, ends[0], ends[1]));
            }
            if (!ok) return;

            for (int k = 0; k < list.Count; k++)
            {
                var pr = list[k];
                if ((pr.Start.Y == 0) != (pr.End.Y == 0))
                {
                    Err("H-DP-7", ln, -1, rec, "", $"data period {k + 1}: give a year on both start and end, or on neither");
                    ok = false;
                }
                else if (pr.End.Before(pr.Start))
                {
                    Err("H-DP-7", ln, -1, rec, "", $"data period {k + 1} ends ({pr.End}) before it starts ({pr.Start}); a period cannot run past 31 December, so split it into two periods or use m/d/y dates");
                    ok = false;
                }
            }
            if (!ok) return;
            for (int i = 0; i < list.Count; i++)
                for (int j = i + 1; j < list.Count; j++)
                {
                    var a = list[i];
                    var b = list[j];
                    if (!(a.End.Before(b.Start) || b.End.Before(a.Start)))
                        Err("H-DP-8", ln, -1, rec, "", $"data periods {i + 1} ({a.Start}-{a.End}) and {j + 1} ({b.Start}-{b.End}) overlap");
                }
            for (int k = 1; k < list.Count; k++)
                if (list[k].Start.Before(list[k - 1].Start))
                    Warn("H-DP-9", ln, -1, rec, "", $"data period {k + 1} starts before data period {k}; list periods in time order");
            periods = list;
            febYear = new long[list.Count];
            Rep.Summary.Periods = p;
        }

        // -- data rows --

        void CheckRow(int ln, string line)
        {
            if (line.Contains('"')) Err("F-6", ln, -1, "", "", "data rows must not contain double quotes");
            var f = line.Split(',');
            if (HeaderPosition(f[0]) >= 0)
            {
                Err("H-ORD-3", ln, 0, "", T(f[0]), $"header record {Q(T(f[0]))} found among the data rows");
                return;
            }
            Rep.Summary.DataRows++;
            if (f.Length != NumFields) Err("D-ROW-1", ln, -1, "", "", $"row has {f.Length} fields, expected 35");

            var t = new long[5];
            bool timeOk = f.Length >= 5;
            for (int i = 0; i < 5 && i < f.Length; i++)
            {
                if (ParseInt(f[i]) is { } v)
                {
                    t[i] = v;
                }
                else
                {
                    var name = Columns[i].Name;
                    Err("D-ROW-2", ln, i, name, f[i], $"{name} {Q(f[i])} is not a whole number");
                    timeOk = false;
                }
            }
            if (timeOk) CheckTime(ln, t);
            else SeqBlind();
            if (f.Length != NumFields) return;
            rowsChecked++;
            CheckFlags(ln, T(f[ColFlags]));

            var vals = new double[NumFields];
            var present = new bool[NumFields];
            for (int i = ColDryBulb; i < NumFields; i++)
            {
                var c = Columns[i];
                var s = T(f[i]);
                if (i == ColWxCodes)
                {
                    if (!IsNineDigits(s)) Err("D-ROW-4", ln, i, c.Name, s, $"present weather codes {Q(s)} must be exactly 9 digits");
                    continue;
                }
                if (ParseNumber(s) is not { } v)
                {
                    Err("D-ROW-3", ln, i, c.Name, s, $"{c.Name} {Q(s)} is not a number");
                    continue;
                }
                if (i == ColWxObs)
                {
                    if (v != 0 && v != 9) Err("D-ROW-5", ln, i, c.Name, s, $"present weather observation {s} must be 0 or 9");
                    continue;
                }
                if (c.Missing > 0 && v >= c.Missing)
                {
                    if (missing[i] == 0) firstMissing[i] = ln;
                    missing[i]++;
                    continue;
                }
                present[i] = true;
                vals[i] = v;
                if (!InRange(v, c.Lo, c.Hi))
                    Err("D-RNG-1", ln, i, c.Name, s, $"{c.Name} = {s} is outside the allowed range {RangeText(c.Lo, c.Hi)} (missing values are >= {G(c.Missing)})");
                if (c.WarnNegative && v < 0) Warn("D-RNG-2", ln, i, c.Name, s, $"{c.Name} {s} is negative");
            }

            if (present[ColDryBulb] && present[ColDewPoint] && vals[ColDewPoint] > vals[ColDryBulb] + 0.5)
                Warn("D-CON-1", ln, ColDewPoint, Columns[ColDewPoint].Name, f[ColDewPoint], $"dew point {f[ColDewPoint]} is above dry bulb {f[ColDryBulb]}");
            if (present[ColRelHumidity] && vals[ColRelHumidity] > 100)
                Warn("D-CON-2", ln, ColRelHumidity, Columns[ColRelHumidity].Name, f[ColRelHumidity], $"relative humidity {f[ColRelHumidity]} is above 100");
        }

        void CheckFlags(int ln, string s)
        {
            if (s == "")
            {
                Err("D-ROW-7", ln, ColFlags, Columns[ColFlags].Name, "", "data source and uncertainty flags are empty");
                return;
            }
            if (FlagsFollowFormat(s)) return;
            if (badFlags == 0)
            {
                firstBadFlags = ln;
                badFlagsValue = s;
            }
            badFlags++;
        }

        void FinishData()
        {
            if (badFlags > 0)
                Warn("D-ROW-6", firstBadFlags, ColFlags, Columns[ColFlags].Name, badFlagsValue, $"{badFlags} of {rowsChecked} rows have flags that are not the classic 44 characters (a source letter A-I or ? followed by a digit, 22 times); this is common and allowed");
            if (rowsChecked == 0) return;
            for (int i = 0; i < NumFields; i++)
            {
                var c = Columns[i];
                int n = missing[i];
                if (n == 0) continue;
                Rep.Summary.Missing[c.Name] = n;
                if (c.Core && n == rowsChecked)
                    Err("D-MIS-1", 0, i, c.Name, "", $"{c.Name} is missing on every row; the file cannot be used for simulation");
                else if (c.Core)
                    Warn("D-MIS-2", firstMissing[i], i, c.Name, "", $"{c.Name} is missing on {n} of {rowsChecked} rows (first at line {firstMissing[i]}); many viewers plot missing codes as real values");
                else if (n == rowsChecked)
                    Warn("D-MIS-3", 0, i, c.Name, "", $"{c.Name} is missing on every row");
            }
        }

        void CheckTime(int ln, long[] t)
        {
            long y = t[0], m = t[1], d = t[2], h = t[3], minute = t[4];
            if (y < 1900 || y > 2100) Warn("D-SEQ-7", ln, ColYear, Columns[ColYear].Name, y.ToString(CultureInfo.InvariantCulture), $"year {y} is outside 1900-2100");
            if (m < 1 || m > 12 || d < 1 || d > DaysIn(m, true) || h < 1 || h > 24)
            {
                Err("D-SEQ-1", ln, -1, "", $"{m}/{d} hour {h}", $"month {m}, day {d}, hour {h} is not a valid time (month 1-12, a day that exists in the month, hour 1-24)");
                SeqBlind();
                return;
            }
            if (m == 2 && d == 29 && !(leapYes && IsLeap(y)))
            {
                var reason = leapYes ? $"{y} is not a leap year" : "the leap-year field is No";
                Err("D-SEQ-5", ln, 2, "Day", "29", $"row for 29 February, but {reason}");
                return; // not part of the time sequence
            }
            long step = 1;
            if (perHour == 1)
            {
                if (minute < 0 || minute > 60) Err("D-SEQ-2", ln, ColMinute, Columns[ColMinute].Name, minute.ToString(CultureInfo.InvariantCulture), $"minute {minute} is outside 0-60");
            }
            else
            {
                long iv = 60 / perHour;
                if (minute < iv || minute > 60 || minute % iv != 0)
                {
                    var allowed = $"{iv}, {2 * iv}, ..., 60";
                    if (perHour <= 4) allowed = string.Join(", ", Enumerable.Range(1, (int)perHour).Select(k => (k * iv).ToString(CultureInfo.InvariantCulture)));
                    Err("D-SEQ-2", ln, ColMinute, Columns[ColMinute].Name, minute.ToString(CultureInfo.InvariantCulture), $"minute {minute}: with {perHour} records per hour the minutes must be {allowed}");
                    SeqBlind();
                    return;
                }
                step = minute / iv;
            }
            SeqRow(ln, new Stamp(y, m, d, h, step));
        }

        // -- time sequence --

        Stamp PeriodStart(int p)
        {
            var s = periods![p].Start;
            return new Stamp(s.Y, s.M, s.D, 1, 1);
        }

        bool IsPeriodEnd(Stamp s, int p)
        {
            var e = periods![p].End;
            return s.M == e.M && s.D == e.D && s.H == 24 && s.Step == perHour && (e.Y == 0 || s.Y == e.Y);
        }

        bool AfterPeriodEnd(Stamp s, int p)
        {
            var e = periods![p].End;
            return e.Before(new CalDate(e.Y != 0 ? s.Y : 0, s.M, s.D));
        }

        Stamp Next(Stamp s)
        {
            if (s.Step < perHour) return s with { Step = s.Step + 1 };
            if (s.H < 24) return s with { H = s.H + 1, Step = 1 };
            var feb29 = leapYes && s.Y != 0 && IsLeap(s.Y);
            if (s.D < DaysIn(s.M, feb29)) return s with { D = s.D + 1, H = 1, Step = 1 };
            if (s.M < 12) return s with { M = s.M + 1, D = 1, H = 1, Step = 1 };
            return new Stamp(s.Y != 0 ? s.Y + 1 : 0, 1, 1, 1, 1);
        }

        void ClosePeriod()
        {
            seqP++;
            seqInside = false;
            if (seqP >= periods!.Count) seqDone = true;
        }

        /// <summary>Moves one time step on for a row whose time could not be read.</summary>
        void SeqBlind()
        {
            if (periods == null || seqDone) return;
            if (!seqInside)
            {
                seqInside = true;
                seqCur = PeriodStart(seqP);
            }
            else
            {
                seqCur = Next(seqCur);
            }
            if (IsPeriodEnd(seqCur, seqP)) ClosePeriod();
        }

        /// <summary>Matches a row against the declared periods. After a row that does not follow, checking carries on from that row.</summary>
        void SeqRow(int ln, Stamp s)
        {
            if (periods == null) return;
            if (seqDone)
            {
                if (seqExtra == 0) seqFirstExtra = ln;
                seqExtra++;
                return;
            }
            var pr = periods[seqP];
            bool withYear = pr.Start.Y != 0;

            if (!seqInside)
            {
                seqInside = true;
                if (!SameTime(s, PeriodStart(seqP), withYear))
                    Err("D-SEQ-3", ln, -1, "", s.ToString(), $"data period {seqP + 1} ({pr.Label}) starts at {pr.Start}, but its first row is {s}");
            }
            else
            {
                var exp = Next(seqCur);
                var cur = seqCur;
                if (SameTime(s, exp, false))
                {
                    if (withYear && s.Y != exp.Y)
                        Err("D-SEQ-4", ln, ColYear, Columns[ColYear].Name, s.Y.ToString(CultureInfo.InvariantCulture), $"year {s.Y}, expected {exp.Y} (data period {seqP + 1} has m/d/y dates)");
                    else if (!withYear && s.Y != cur.Y && s.M == cur.M)
                        Err("D-SEQ-4", ln, ColYear, Columns[ColYear].Name, s.Y.ToString(CultureInfo.InvariantCulture), $"year changes from {cur.Y} to {s.Y} inside month {s.M}; it may change only when the month changes");
                }
                else if (exp.M == 2 && exp.D == 29 && s.M == 3 && s.D == 1 && s.H == 1 && s.Step == 1)
                {
                    Err("D-SEQ-5", ln, -1, "", s.ToString(), $"29 February {cur.Y} is missing: the leap-year field is Yes and {cur.Y} is a leap year");
                }
                else if (seqP + 1 < periods.Count && SameTime(s, PeriodStart(seqP + 1), periods[seqP + 1].Start.Y != 0))
                {
                    Err("D-SEQ-3", ln, -1, "", s.ToString(), $"data period {seqP + 1} ({pr.Label}) ends early: expected {exp}, but data period {seqP + 2} starts here");
                    ClosePeriod();
                    SeqRow(ln, s);
                    return;
                }
                else if (AfterPeriodEnd(s, seqP))
                {
                    Err("D-SEQ-3", ln, -1, "", s.ToString(), $"row {s} is after the end of data period {seqP + 1} ({pr.Label}, ends {pr.End})");
                    ClosePeriod();
                    SeqRow(ln, s);
                    return;
                }
                else
                {
                    Err("D-SEQ-3", ln, -1, "", s.ToString(), $"expected {exp}, found {s} (rows must follow each other one time step apart)");
                }
            }
            if (s.M == 2) febYear[seqP] = s.Y;
            seqCur = s;
            if (IsPeriodEnd(s, seqP)) ClosePeriod();
        }

        /// <summary>Counts the days in period k, with 29 February when the leap-year field and the year of the period's February rows say so.</summary>
        long PeriodDays(int k)
        {
            var pr = periods![k];
            long y = pr.Start.Y, m = pr.Start.M, d = pr.Start.D;
            long n = 1;
            while (!(m == pr.End.M && d == pr.End.D && (pr.End.Y == 0 || y == pr.End.Y)))
            {
                var yy = pr.Start.Y == 0 ? febYear[k] : y;
                var feb29 = leapYes && yy != 0 && IsLeap(yy);
                if (d < DaysIn(m, feb29))
                {
                    d++;
                }
                else
                {
                    d = 1;
                    m++;
                    if (m > 12)
                    {
                        m = 1;
                        y++;
                    }
                }
                n++;
                if (n > 366 * 200) break; // cannot happen once H-DP-7 has passed; guard against a loop
            }
            return n;
        }

        void FinishSequence()
        {
            if (periods == null) return;
            long expected = 0;
            var spans = new List<string>();
            for (int k = 0; k < periods.Count; k++)
            {
                expected += PeriodDays(k) * 24 * perHour;
                spans.Add($"{periods[k].Start}-{periods[k].End}");
            }
            var s = Rep.Summary;
            s.ExpectedRows = expected;
            if (s.DataRows == expected) return;
            var msg = $"DATA PERIODS ({string.Join(", ", spans)}, {perHour} per hour) declares {expected} rows, but the file has {s.DataRows} data rows";
            if (seqExtra > 0) msg += $"; {seqExtra} rows come after the last period ends (first at line {seqFirstExtra})";
            Err("D-SEQ-6", 0, -1, "", "", msg);
        }
    }
}
