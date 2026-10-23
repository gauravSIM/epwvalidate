"""epwvalidate: checks EnergyPlus Weather (EPW) files.

The rules, and the rule IDs used in every Issue, are defined in SPEC.md. This
file is a self-contained port of the Go reference version and gives exactly the
same results (checked against testdata/expected.json). It needs Python 3.9+
and nothing outside the standard library.

Library use:

    from epwvalidate import validate_file
    report = validate_file("file.epw")   # raises OSError only if the file cannot be read
    if not report.valid:
        for issue in report.errors:
            print(issue)                  # "line 79, Dry Bulb Temperature: ... [D-RNG-1]"

Command line:

    python epwvalidate.py [--json] file.epw [more.epw ...]

Exit code: 0 = all files valid, 1 = at least one file invalid,
2 = a file could not be read or no file was given.
"""

from __future__ import annotations

import json
import math
import re
import sys
from collections import namedtuple
from dataclasses import dataclass, field as dataclass_field
from typing import Dict, List, Optional, Tuple, Union

__all__ = ["Issue", "Summary", "Report", "MAX_ISSUES_PER_RULE", "validate_file", "validate", "format_text"]

# MAX_ISSUES_PER_RULE limits how many issues of one rule are listed in a
# Report. Summary.rule_counts always holds the full count.
MAX_ISSUES_PER_RULE = 20


@dataclass
class Issue:
    """One finding."""

    severity: str  # "error" or "warning"
    rule: str  # rule ID from the spec, e.g. "D-RNG-1"
    line: int  # 1-based line number; 0 = whole file
    column: int  # 0-based field index; -1 = whole line
    field: str  # field name, e.g. "Dry Bulb Temperature"; may be ""
    value: str  # raw text of the offending value; may be ""
    message: str

    def __str__(self) -> str:
        s = f"line {self.line}" if self.line > 0 else "file"
        if self.field:
            s += f", {self.field}"
        return f"{s}: {self.message} [{self.rule}]"

    def to_dict(self) -> dict:
        d = {"severity": self.severity, "rule": self.rule, "line": self.line, "column": self.column}
        if self.field:
            d["field"] = self.field
        if self.value:
            d["value"] = self.value
        d["message"] = self.message
        return d


@dataclass
class Summary:
    """Counts for the whole file."""

    lines: int = 0
    data_rows: int = 0
    expected_rows: int = -1  # rows declared by DATA PERIODS; -1 if it could not be read
    periods: int = 0
    records_per_hour: int = 1
    error_count: int = 0  # all errors, including ones not listed
    warning_count: int = 0  # all warnings, including ones not listed
    missing: Dict[str, int] = dataclass_field(default_factory=dict)  # missing values per data column
    rule_counts: Dict[str, int] = dataclass_field(default_factory=dict)  # all issues per rule

    def to_dict(self) -> dict:
        return {
            "lines": self.lines,
            "dataRows": self.data_rows,
            "expectedRows": self.expected_rows,
            "periods": self.periods,
            "recordsPerHour": self.records_per_hour,
            "errorCount": self.error_count,
            "warningCount": self.warning_count,
            "missing": dict(self.missing),
            "ruleCounts": dict(self.rule_counts),
        }


@dataclass
class Report:
    """The result of validating one file. The file is acceptable when valid is
    True; warnings do not affect valid."""

    valid: bool = False
    errors: List[Issue] = dataclass_field(default_factory=list)
    warnings: List[Issue] = dataclass_field(default_factory=list)
    summary: Summary = dataclass_field(default_factory=Summary)

    def to_dict(self) -> dict:
        """The report as a dict with the same keys as the JSON output of every
        language version."""
        return {
            "valid": self.valid,
            "errors": [i.to_dict() for i in self.errors],
            "warnings": [i.to_dict() for i in self.warnings],
            "summary": self.summary.to_dict(),
        }


# Profiles name an opt-in set of extra rules for one consumer of the file
# (SPEC.md section 11). PROFILE_NONE checks the EPW rules only.
PROFILE_NONE = ""
PROFILE_SIMIEN = "simien"

_PROFILES = (PROFILE_NONE, PROFILE_SIMIEN)


class Options:
    """Changes what a validation run checks. The default is the EPW rules
    alone, which is what validate() and validate_file() use."""

    __slots__ = ("profile",)

    def __init__(self, profile: str = PROFILE_NONE) -> None:
        self.profile = profile


def parse_profile(name: str) -> Optional[str]:
    """Turns a profile name into a profile, or None when it is unknown. The
    empty string gives PROFILE_NONE. Names are matched case-insensitively."""
    v = (name or "").strip().lower()
    return v if v in _PROFILES else None


def validate_file(path: str, options: Optional[Options] = None) -> Report:
    """Validates the EPW file at path. Raises OSError only for I/O failures;
    problems with the file's content are in the Report."""
    with open(path, "rb") as f:
        return validate(f.read(), options)


def validate(data: Union[bytes, str], options: Optional[Options] = None) -> Report:
    """Validates the content of an EPW file."""
    if isinstance(data, str):
        data = data.encode("utf-8")
    c = _Checker(options or Options())
    c.run(data)
    c.rep.valid = c.rep.summary.error_count == 0
    return c.rep


def format_text(name: str, rep: Report) -> str:
    """Returns the report as the command-line tool prints it."""
    s = rep.summary
    out = f"{name}: {'VALID' if rep.valid else 'INVALID'} - {s.data_rows} data rows"
    if s.expected_rows >= 0:
        out += f" (DATA PERIODS declares {s.expected_rows})"
    out += f", {s.error_count} errors, {s.warning_count} warnings\n"
    for i in rep.errors:
        out += f"  error    {i}\n"
    for i in rep.warnings:
        out += f"  warning  {i}\n"
    for rule in sorted(r for r, n in s.rule_counts.items() if n > MAX_ISSUES_PER_RULE):
        out += f"  ... and {s.rule_counts[rule] - MAX_ISSUES_PER_RULE} more [{rule}]\n"
    return out


# ---- Text helpers that behave like the Go version ----

# Whitespace as Go's strings.TrimSpace sees it.
_SPACE = "\t\n\v\f\r \x85\xa0                　"
_SPACE_RE = re.compile("[" + _SPACE + "]+")


def _trim(s: str) -> str:
    return s.strip(_SPACE)


_NUMBER_RE = re.compile(r"[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?")
_INT_RE = re.compile(r"[+-]?[0-9]+")


def _parse_number(s: str) -> Optional[float]:
    """Plain decimal numbers only: no "NaN", "Inf", hex or thousands separators."""
    s = _trim(s)
    if not _NUMBER_RE.fullmatch(s):
        return None
    v = float(s)
    return v if math.isfinite(v) else None


def _parse_int(s: str) -> Optional[int]:
    s = _trim(s)
    if not _INT_RE.fullmatch(s):
        return None
    v = int(s)
    return v if -(2**63) <= v < 2**63 else None


def _fmt_g(x: float) -> str:
    """Formats a float like Go's %g for the values used in messages."""
    if x == int(x):
        if x == 0 and math.copysign(1, x) < 0:
            return "-0"
        return str(int(x))
    return repr(x)


def _quote(s: str) -> str:
    """Quotes a string like Go's %q."""
    out = ['"']
    for ch in s:
        o = ord(ch)
        if ch == '"':
            out.append('\\"')
        elif ch == "\\":
            out.append("\\\\")
        elif ch in "\a\b\f\n\r\t\v":
            out.append({"\a": "\\a", "\b": "\\b", "\f": "\\f", "\n": "\\n", "\r": "\\r", "\t": "\\t", "\v": "\\v"}[ch])
        elif o < 0x20 or o == 0x7F:
            out.append("\\x%02x" % o)
        elif not ch.isprintable():
            out.append("\\u%04x" % o if o < 0x10000 else "\\U%08x" % o)
        else:
            out.append(ch)
    out.append('"')
    return "".join(out)


def _truncate(s: str, n: int) -> str:
    return s if len(s) <= n else s[:n] + "..."


def _round_half_away(x: float) -> float:
    return math.copysign(math.floor(abs(x) + 0.5), x)


# A bound is one side of an allowed range; strict means the bound itself is not
# allowed (> or <). None means no bound.
_Bound = namedtuple("_Bound", "v strict")


def _gt(v: float) -> _Bound:
    return _Bound(v, True)


def _ge(v: float) -> _Bound:
    return _Bound(v, False)


_lt, _le = _gt, _ge


def _in_range(v: float, lo: Optional[_Bound], hi: Optional[_Bound]) -> bool:
    if lo is not None and (v < lo.v or (lo.strict and v == lo.v)):
        return False
    if hi is not None and (v > hi.v or (hi.strict and v == hi.v)):
        return False
    return True


def _range_text(lo: Optional[_Bound], hi: Optional[_Bound]) -> str:
    parts = []
    if lo is not None:
        parts.append(f"{'>' if lo.strict else '>='} {_fmt_g(lo.v)}")
    if hi is not None:
        parts.append(f"{'<' if hi.strict else '<='} {_fmt_g(hi.v)}")
    return " and ".join(parts)


# ---- Dates (SPEC.md §5.9) ----

_MONTH_NAMES = ["january", "february", "march", "april", "may", "june",
                "july", "august", "september", "october", "november", "december"]
_WEEKDAY_NAMES = ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"]


def _month_index(s: str) -> int:
    """Returns 1-12 for a full or 3-letter month name, 0 otherwise."""
    s = _trim(s).lower()
    for i, n in enumerate(_MONTH_NAMES):
        if s == n or s == n[:3]:
            return i + 1
    return 0


def _weekday_index(s: str) -> Optional[int]:
    s = _trim(s).lower()
    for i, n in enumerate(_WEEKDAY_NAMES):
        if s == n or s == n[:3]:
            return i
    return None


def _is_leap(y: int) -> bool:
    return y % 4 == 0 and (y % 100 != 0 or y % 400 == 0)


def _days_in(month: int, feb29: bool) -> int:
    if month == 2:
        return 29 if feb29 else 28
    if month in (4, 6, 9, 11):
        return 30
    return 31


class _CalDate(namedtuple("_CalDate", "y m d")):
    """A month/day, with a year only when the date had one (y = 0 otherwise)."""

    def before(self, b: "_CalDate") -> bool:
        if self.y != 0 and b.y != 0 and self.y != b.y:
            return self.y < b.y
        if self.m != b.m:
            return self.m < b.m
        return self.d < b.d

    def __str__(self) -> str:
        return f"{self.m}/{self.d}/{self.y}" if self.y != 0 else f"{self.m}/{self.d}"


_ZERO_DATE = _CalDate(0, 0, 0)


class _DateError(Exception):
    pass


def _parse_date(s: str, allow_year: bool = False, allow_weekday: bool = False) -> _CalDate:
    """Checks one header date field and returns it as month/day (plus year for
    m/d/y). Weekday forms return a zero date. Raises _DateError."""
    s = _trim(s)
    if s == "":
        raise _DateError("date is empty")

    if "/" in s:
        parts = s.split("/")
        if len(parts) == 3 and not allow_year:
            raise _DateError(f"date {_quote(s)} has a year; m/d/y is allowed only in DATA PERIODS")
        if len(parts) not in (2, 3):
            raise _DateError(f"date {_quote(s)} is not in m/d form")
        n = []
        for p in parts:
            v = _parse_int(p.replace(" ", ""))
            if v is None:
                raise _DateError(f"date {_quote(s)} is not in m/d form")
            n.append(v)
        d = _CalDate(0, n[0], n[1])
        if len(parts) == 3:
            if n[2] < 1:
                raise _DateError(f"date {_quote(s)} has an invalid year")
            d = _CalDate(n[2], n[0], n[1])
        _check_month_day(s, d)
        return d

    j = _parse_int(s)
    if j is not None:
        if j < 1 or j > 366:
            raise _DateError(f"day of year {j} is outside 1-366")
        return _julian_to_date(j)

    tok = [t for t in _SPACE_RE.split(s) if t]
    if len(tok) == 2:
        m = _month_index(tok[0])
        if m > 0:
            d = _parse_int(tok[1])
            if d is not None:
                cd = _CalDate(0, m, d)
                _check_month_day(s, cd)
                return cd
        m = _month_index(tok[1])
        if m > 0:
            d = _parse_int(tok[0])
            if d is not None:
                cd = _CalDate(0, m, d)
                _check_month_day(s, cd)
                return cd
    if len(tok) == 4 and tok[2].lower() == "in":
        if not allow_weekday:
            raise _DateError(f"weekday date {_quote(s)} is allowed only for holidays and daylight saving")
        n = _parse_int(tok[0])
        if _weekday_index(tok[1]) is not None and _month_index(tok[3]) > 0 and (
            tok[0].lower() == "last" or (n is not None and 1 <= n <= 5)
        ):
            return _ZERO_DATE
    raise _DateError(f"{_quote(s)} is not a recognised EPW date (use m/d, e.g. 1/31)")


def _check_month_day(s: str, d: _CalDate) -> None:
    if d.m < 1 or d.m > 12:
        raise _DateError(f"date {_quote(s)}: month {d.m} is outside 1-12")
    feb29 = d.y == 0 or _is_leap(d.y)
    if d.d < 1 or d.d > _days_in(d.m, feb29):
        raise _DateError(f"date {_quote(s)}: day {d.d} does not exist in month {d.m}")


def _julian_to_date(j: int) -> _CalDate:
    """Converts a day of year (non-leap calendar) to month/day; 366 = 31 December."""
    if j == 366:
        return _CalDate(0, 12, 31)
    m = 1
    while j > _days_in(m, False):
        j -= _days_in(m, False)
        m += 1
    return _CalDate(0, m, j)


# ---- Header records (SPEC.md §4-§5) ----

_HEADER_LINES = 8
_HEADER_KEYWORDS = [
    "LOCATION",
    "DESIGN CONDITIONS",
    "TYPICAL/EXTREME PERIODS",
    "GROUND TEMPERATURES",
    "HOLIDAYS/DAYLIGHT SAVINGS",
    "COMMENTS 1",
    "COMMENTS 2",
    "DATA PERIODS",
]


def _matches_keyword(pos: int, s: str) -> bool:
    s = _trim(s).upper()
    return s == _HEADER_KEYWORDS[pos] or (pos == 4 and s == "HOLIDAYS/DAYLIGHT SAVING")


def _header_position(s: str) -> int:
    for i in range(_HEADER_LINES):
        if _matches_keyword(i, s):
            return i
    return -1


def _split_csv(line: str) -> List[str]:
    """Splits a header line, honouring double-quoted fields. Inside a quoted
    field "" is a quote; a quote that does not end the field is kept as text."""
    fields = []
    while True:
        if not line.startswith('"'):
            i = line.find(",")
            if i < 0:
                fields.append(line)
                return fields
            fields.append(line[:i])
            line = line[i + 1:]
            continue
        b = []
        line = line[1:]
        while True:
            i = line.find('"')
            if i < 0:
                b.append(line)
                fields.append("".join(b))
                return fields
            b.append(line[:i])
            line = line[i + 1:]
            if line.startswith('"'):
                b.append('"')
                line = line[1:]
            elif line == "":
                fields.append("".join(b))
                return fields
            elif line[0] == ",":
                line = line[1:]
                break
            else:
                b.append('"')
        fields.append("".join(b))


def _trim_trailing_empty(f: List[str]) -> Tuple[List[str], bool]:
    n = len(f)
    while n > 1 and _trim(f[n - 1]) == "":
        n -= 1
    return f[:n], n < len(f)


def _field(f: List[str], i: int) -> str:
    return _trim(f[i]) if i < len(f) else ""


_DESIGN_SECTIONS = ["Heating", "Cooling", "Extremes"]
# Number of Heating, Cooling and Extremes values for known ASHRAE Handbook editions (H-DC-5).
_DESIGN_LAYOUTS = {
    "2021 ASHRAE Handbook": (16, 32, 15),
    "2025 ASHRAE Handbook": (16, 32, 27),
}

_Period = namedtuple("_Period", "name start end")


def _period_label(p: _Period) -> str:
    return p.name or "unnamed"


# ---- Data rows (SPEC.md §6-§9) ----

_NUM_FIELDS = 35


class _Column:
    __slots__ = ("name", "lo", "hi", "missing", "core", "simien", "warn_negative")

    def __init__(self, name, lo=None, hi=None, missing=0.0, core=False, simien=False, warn_negative=False):
        self.name, self.lo, self.hi, self.missing = name, lo, hi, missing
        self.core, self.simien, self.warn_negative = core, simien, warn_negative


_COLUMNS = [
    _Column("Year"),
    _Column("Month"),
    _Column("Day"),
    _Column("Hour"),
    _Column("Minute"),
    _Column("Data Source and Uncertainty Flags"),
    _Column("Dry Bulb Temperature", _gt(-70), _lt(70), 99.9, core=True, simien=True),
    _Column("Dew Point Temperature", _gt(-70), _lt(70), 99.9),
    _Column("Relative Humidity", _ge(0), _le(110), 999, core=True, simien=True),
    _Column("Atmospheric Station Pressure", _gt(31000), _lt(120000), 999999, core=True),
    _Column("Extraterrestrial Horizontal Radiation", _ge(0), None, 9999),
    _Column("Extraterrestrial Direct Normal Radiation", _ge(0), None, 9999),
    _Column("Horizontal Infrared Radiation Intensity", _ge(0), None, 9999),
    _Column("Global Horizontal Radiation", _ge(0), None, 9999, core=True, simien=True),
    _Column("Direct Normal Radiation", _ge(0), None, 9999, core=True, simien=True),
    _Column("Diffuse Horizontal Radiation", _ge(0), None, 9999, core=True, simien=True),
    _Column("Global Horizontal Illuminance", _ge(0), None, 999900),
    _Column("Direct Normal Illuminance", _ge(0), None, 999900),
    _Column("Diffuse Horizontal Illuminance", _ge(0), None, 999900),
    _Column("Zenith Luminance", _ge(0), None, 9999),
    _Column("Wind Direction", _ge(0), _le(360), 999, core=True, simien=True),
    _Column("Wind Speed", _ge(0), _le(40), 999, core=True, simien=True),
    _Column("Total Sky Cover", _ge(0), _le(10), 99),
    _Column("Opaque Sky Cover", _ge(0), _le(10), 99),
    _Column("Visibility", missing=9999, warn_negative=True),
    _Column("Ceiling Height", missing=99999, warn_negative=True),
    _Column("Present Weather Observation"),
    _Column("Present Weather Codes"),
    _Column("Precipitable Water", missing=999, warn_negative=True),
    _Column("Aerosol Optical Depth", missing=0.999, warn_negative=True),
    _Column("Snow Depth", missing=999, warn_negative=True),
    _Column("Days Since Last Snowfall", missing=99, warn_negative=True),
    _Column("Albedo", missing=999, warn_negative=True),
    _Column("Liquid Precipitation Depth", missing=999, warn_negative=True),
    _Column("Liquid Precipitation Quantity", missing=99, warn_negative=True),
]

_COL_YEAR, _COL_MINUTE, _COL_FLAGS = 0, 4, 5
_COL_DRY_BULB, _COL_DEW_POINT, _COL_REL_HUMIDITY = 6, 7, 8
_COL_WX_OBS, _COL_WX_CODES = 26, 27
_COL_ALBEDO = 32

# simien profile (SPEC.md section 11.1): one full year of hourly values,
# 365 x 24, and what a missing albedo is filled with. The standard SIMIEN
# station files carry that constant for the whole year.
_SIMIEN_ROWS = 8760
_SIMIEN_DEFAULT_ALBEDO = "0.20"


def _is_nine_digits(s: str) -> bool:
    return len(s) == 9 and all("0" <= ch <= "9" for ch in s)


def _flags_follow_format(s: str) -> bool:
    """22 pairs of a source letter (A-I or ?) and an uncertainty digit."""
    if len(s) != 44:
        return False
    for i in range(0, 44, 2):
        if s[i] not in "ABCDEFGHI?" or not ("0" <= s[i + 1] <= "9"):
            return False
    return True


# ---- Time sequence (SPEC.md §7) ----


class _Stamp(namedtuple("_Stamp", "y m d h step")):
    """The time of one data row. step is the record within the hour; y = 0 means
    the year is unknown."""

    def __str__(self) -> str:
        t = f"{self.m}/{self.d}/{self.y} hour {self.h}" if self.y != 0 else f"{self.m}/{self.d} hour {self.h}"
        if self.step > 1:
            t += f" step {self.step}"
        return t


def _same_time(a: _Stamp, b: _Stamp, with_year: bool) -> bool:
    return a.m == b.m and a.d == b.d and a.h == b.h and a.step == b.step and (not with_year or a.y == b.y)


# ---- The checker ----


class _Checker:
    def __init__(self, opt: Optional["Options"] = None) -> None:
        self.rep = Report()
        self.opt = opt or Options()
        self.feb29 = 0  # rows for 29 February that the EPW rules allow
        self.first_feb29 = 0
        self.leap_yes = False  # leap-year field is Yes
        self.per_hour = 1  # records per hour from DATA PERIODS
        self.periods: Optional[List[_Period]] = None  # None when DATA PERIODS could not be read
        self.rows_checked = 0  # data rows with 35 fields
        self.missing = [0] * _NUM_FIELDS
        self.first_missing = [0] * _NUM_FIELDS
        self.bad_flags = 0
        self.first_bad_flags = 0
        self.bad_flags_value = ""
        # time-sequence state (see _seq_row)
        self.seq_p = 0
        self.seq_inside = False
        self.seq_cur = _Stamp(0, 0, 0, 0, 0)
        self.seq_done = False
        self.seq_extra = 0
        self.seq_first_extra = 0
        self.feb_year: List[int] = []

    # -- issues --

    def _add(self, sev: str, rule: str, line: int, col: int, field: str, value: str, msg: str) -> None:
        s = self.rep.summary
        s.rule_counts[rule] = s.rule_counts.get(rule, 0) + 1
        if sev == "error":
            s.error_count += 1
        else:
            s.warning_count += 1
        if s.rule_counts[rule] > MAX_ISSUES_PER_RULE:
            return
        issue = Issue(sev, rule, line, col, field, value, msg)
        (self.rep.errors if sev == "error" else self.rep.warnings).append(issue)

    def _err(self, rule, line, col, field, value, msg):
        self._add("error", rule, line, col, field, value, msg)

    def _warn(self, rule, line, col, field, value, msg):
        self._add("warning", rule, line, col, field, value, msg)

    # -- file level --

    def run(self, data: bytes) -> None:
        if data.strip(b" \t\n\r\v\f") == b"":
            self._err("F-1", 0, -1, "", "", "the file is empty")
            return
        if data.startswith(b"\xef\xbb\xbf"):
            self._err("F-2", 1, -1, "", "", "the file starts with a UTF-8 byte-order mark (BOM); save it without BOM")
            data = data[3:]
        crlf = data.count(b"\r\n")
        lf = data.count(b"\n") - crlf
        if crlf > 0 and lf > 0:
            self._warn("F-7", 0, -1, "", "", f"mixed line endings: {crlf} lines end with CRLF and {lf} with LF")

        raw = data.split(b"\n")
        raw = [r[:-1] if r.endswith(b"\r") else r for r in raw]
        lines = [r.decode("utf-8", "replace") for r in raw]
        while lines and _trim(lines[-1]) == "":
            lines.pop()
            raw.pop()
        self.rep.summary.lines = len(lines)

        for i, l in enumerate(lines):
            try:
                raw[i].decode("utf-8")
            except UnicodeDecodeError:
                self._err("F-1", i + 1, -1, "", "", "line is not valid UTF-8; save the file as UTF-8")
            if _trim(l).endswith(";"):
                self._err("F-5", i + 1, -1, "", "", "line ends with ';' (semicolons do not end lines in EPW)")

        data_start = self._check_headers(lines)
        for i in range(data_start, len(lines)):
            if _trim(lines[i]) == "":
                self._err("F-4", i + 1, -1, "", "", "empty line inside the data")
                continue
            self._check_row(i + 1, lines[i])
        self._finish_data()
        self._finish_sequence()

    # -- headers --

    def _check_headers(self, lines: List[str]) -> int:
        """Checks the 8 header records and returns the index of the first data line."""
        i = 0
        pos = 0
        while pos < _HEADER_LINES:
            if i >= len(lines):
                self._err("H-ORD-1", 0, -1, "", "",
                          f"the file ends before the {_HEADER_KEYWORDS[pos]} record (it has {len(lines)} lines)")
                return i
            ln = i + 1
            line = lines[i]
            first = line.split(",", 1)[0]
            q = _header_position(first)
            if q > pos or q < 0:
                found = f"the {_HEADER_KEYWORDS[max(q, 0)]} record"
                if q < 0:
                    found = _quote(_truncate(_trim(first), 40))
                    if _parse_int(first) is not None:
                        found = "a data row"
                self._err("H-ORD-1", ln, 0, "", _trim(first),
                          f"the {_HEADER_KEYWORDS[pos]} record is missing: line {ln} should hold it, but holds {found}")
                pos += 1
                continue
            if q < pos:
                self._err("H-ORD-1", ln, 0, "", _trim(first),
                          f"the {_HEADER_KEYWORDS[q]} record is out of order or repeated; it belongs on line {q + 1}")
                i += 1
                continue
            i += 1
            if pos == 0:
                if '"' in line:
                    self._err("F-6", ln, -1, "LOCATION", "",
                              "LOCATION must not contain double quotes; many EPW readers split this line on bare commas")
                f = line.split(",")
            else:
                f = _split_csv(line)
            if pos not in (5, 6):
                f, trimmed = _trim_trailing_empty(f)
                if trimmed:
                    kw = _HEADER_KEYWORDS[pos]
                    self._warn("H-TRAIL", ln, -1, kw, "", f"{kw} ends with empty fields; they are ignored")
            if pos == 0:
                self._check_location(ln, f)
            elif pos == 1:
                self._check_design_conditions(ln, f)
            elif pos == 2:
                self._check_typical_extreme(ln, f)
            elif pos == 3:
                self._check_ground_temperatures(ln, f)
            elif pos == 4:
                self._check_holidays(ln, f)
            elif pos in (5, 6):
                if _field(f, 1) == "":
                    kw = _HEADER_KEYWORDS[pos]
                    self._err("H-COM-1", ln, 1, kw, "", f"{kw} has no text")
            elif pos == 7:
                self._check_data_periods(ln, f)
            pos += 1
        return i

    def _header_number(self, rule, ln, f, i, name, lo, hi) -> Tuple[float, bool]:
        s = _field(f, i)
        v = _parse_number(s)
        if v is None:
            self._err(rule, ln, i, name, s, f"{name} {_quote(s)} is not a number")
            return 0.0, False
        if not _in_range(v, lo, hi):
            self._err(rule, ln, i, name, s, f"{name} = {s} is outside the allowed range {_range_text(lo, hi)}")
            return v, False
        return v, True

    def _header_count(self, rule, ln, f, name) -> Optional[int]:
        s = _field(f, 1)
        n = _parse_int(s)
        if n is None or n < 0:
            self._err(rule, ln, 1, name, s, f"{name} {_quote(s)} is not a whole number >= 0")
            return None
        return n

    def _check_location(self, ln, f):
        if len(f) != 10:
            self._err("H-LOC-1", ln, -1, "LOCATION", "",
                      f"LOCATION has {len(f)} fields, expected 10 (keyword, city, region, country, source, WMO, "
                      "latitude, longitude, time zone, elevation)")
        if _field(f, 1) == "":
            self._err("H-LOC-2", ln, 1, "City", "", "city is empty")
        for i, name in ((2, "State Province Region"), (3, "Country"), (4, "Source"), (5, "WMO")):
            if _field(f, i) == "":
                self._err("H-LOC-3", ln, i, name, "", f"{name} is empty")
        self._header_number("H-LOC-4", ln, f, 6, "Latitude", _ge(-90), _le(90))
        lon, lon_ok = self._header_number("H-LOC-5", ln, f, 7, "Longitude", _ge(-180), _le(180))
        tz, tz_ok = self._header_number("H-LOC-6", ln, f, 8, "Time Zone", _ge(-12), _le(12))
        self._header_number("H-LOC-7", ln, f, 9, "Elevation", _ge(-1000), _lt(9999.9))
        if lon_ok and tz_ok:
            want = _round_half_away(lon / 15)
            if abs(tz - want) > 3:
                self._warn("H-LOC-8", ln, 8, "Time Zone", _field(f, 8),
                           f"time zone {_fmt_g(tz)} h is far from the {_fmt_g(want)} h expected at longitude "
                           f"{_fmt_g(lon)}; check the sign")

    def _check_design_conditions(self, ln, f):
        n = self._header_count("H-DC-1", ln, f, "Number of Design Conditions")
        if n is None:
            return
        if n == 0:
            if len(f) > 2:
                self._err("H-DC-2", ln, 2, "DESIGN CONDITIONS", "",
                          "number of design conditions is 0, but the record has more fields")
            return
        if _field(f, 2) == "":
            self._err("H-DC-3", ln, 2, "Design Condition Source", "", "design condition source is empty")
            return
        self._check_design_sections(ln, n, f)

    def _check_design_sections(self, ln, n, f):
        rec = "DESIGN CONDITIONS"
        i = 3
        if _field(f, i) == "" and _field(f, i + 1).lower() == _DESIGN_SECTIONS[0].lower():
            i += 1  # some writers leave one empty field after the source
        counts = [0, 0, 0]
        for s, name in enumerate(_DESIGN_SECTIONS):
            if _field(f, i).lower() != name.lower():
                self._err("H-DC-4", ln, i, rec, _field(f, i),
                          f"expected the {name} section at field {i + 1}, found {_quote(_truncate(_field(f, i), 20))}")
                return
            i += 1
            start = i
            while i < len(f) and (s == len(_DESIGN_SECTIONS) - 1 or
                                  _field(f, i).lower() != _DESIGN_SECTIONS[s + 1].lower()):
                v = _field(f, i)
                if v == "":
                    self._err("H-DC-4", ln, i, name + " value", v, f"{name} value at field {i + 1} is empty")
                elif _parse_number(v) is None and v.lower() != "n/a":  # ASHRAE writes N/A for "not available"
                    self._err("H-DC-4", ln, i, name + " value", v,
                              f"{name} value {_quote(v)} at field {i + 1} is not a number")
                i += 1
            counts[s] = i - start
            if counts[s] == 0:
                self._err("H-DC-4", ln, i, rec, "", f"the {name} section has no values")
        if n != 1:
            return
        for edition, want in _DESIGN_LAYOUTS.items():
            if edition in _field(f, 2) and tuple(counts) != want:
                self._err("H-DC-5", ln, -1, rec, "",
                          f"{edition} design conditions have {want[0]} Heating, {want[1]} Cooling and {want[2]} "
                          f"Extremes values; found {counts[0]}, {counts[1]} and {counts[2]}")

    def _check_typical_extreme(self, ln, f):
        n = self._header_count("H-TEP-1", ln, f, "Number of Typical/Extreme Periods")
        if n is None:
            return
        want = 2 + 4 * n
        if len(f) != want:
            self._err("H-TEP-2", ln, -1, "TYPICAL/EXTREME PERIODS", "",
                      f"{n} periods declared, so {want} fields are expected; found {len(f)}")
            return
        for k in range(n):
            b = 2 + 4 * k
            if _field(f, b) == "":
                self._err("H-TEP-5", ln, b, f"Period {k + 1} Name", "", "typical/extreme period name is empty")
            t = _field(f, b + 1).lower()
            if t == "":
                self._err("H-TEP-5", ln, b + 1, f"Period {k + 1} Type", "", "typical/extreme period type is empty")
            elif t not in ("typical", "extreme"):
                self._warn("H-TEP-3", ln, b + 1, f"Period {k + 1} Type", _field(f, b + 1),
                           f"period type {_quote(_field(f, b + 1))} is not Typical or Extreme")
            for j, what in enumerate(("Start Day", "End Day")):
                s = _field(f, b + 2 + j)
                try:
                    _parse_date(s)
                except _DateError as e:
                    self._err("H-TEP-4", ln, b + 2 + j, f"Period {k + 1} {what}", s, str(e))

    def _check_ground_temperatures(self, ln, f):
        n = self._header_count("H-GT-1", ln, f, "Number of Ground Temperature Depths")
        if n is None:
            return
        want = 2 + 16 * n
        if len(f) != want:
            self._err("H-GT-2", ln, -1, "GROUND TEMPERATURES", "",
                      f"{n} depths declared, so {want} fields are expected; found {len(f)}")
            return
        prev = -math.inf
        for k in range(n):
            b = 2 + 16 * k
            depth, ok = self._header_number("H-GT-3", ln, f, b, f"Depth {k + 1}", _gt(0), None)
            if ok:
                if depth < prev:
                    self._warn("H-GT-6", ln, b, f"Depth {k + 1}", _field(f, b), "depths are not in increasing order")
                prev = depth
            for j, name in enumerate(("Soil Conductivity", "Soil Density", "Soil Specific Heat")):
                if _field(f, b + 1 + j) != "":
                    self._header_number("H-GT-4", ln, f, b + 1 + j, f"Depth {k + 1} {name}", _gt(0), None)
            for m in range(12):
                self._header_number("H-GT-5", ln, f, b + 4 + m, f"Depth {k + 1} month {m + 1} ground temperature",
                                    _gt(-70), _lt(70))

    def _check_holidays(self, ln, f):
        leap = _field(f, 1).lower()
        if leap == "yes":
            self.leap_yes = True
        elif leap != "no":
            self._err("H-HOL-1", ln, 1, "LeapYear Observed", _field(f, 1),
                      f"leap-year field {_quote(_field(f, 1))} must be Yes or No")
        for j, name in enumerate(("Daylight Saving Start Day", "Daylight Saving End Day")):
            s = _field(f, 2 + j)
            if s == "0":
                continue
            try:
                _parse_date(s, allow_weekday=True)
            except _DateError as e:
                self._err("H-HOL-2", ln, 2 + j, name, s, f"{e} (use 0 for no daylight saving)")
        n = _parse_int(_field(f, 4))
        if n is None or n < 0:
            self._err("H-HOL-3", ln, 4, "Number of Holidays", _field(f, 4),
                      f"number of holidays {_quote(_field(f, 4))} is not a whole number >= 0")
            return
        want = 5 + 2 * n
        if len(f) != want:
            self._err("H-HOL-3", ln, -1, "HOLIDAYS/DAYLIGHT SAVINGS", "",
                      f"{n} holidays declared, so {want} fields are expected; found {len(f)}")
            return
        for k in range(n):
            b = 5 + 2 * k
            if _field(f, b) == "":
                self._err("H-HOL-4", ln, b, f"Holiday {k + 1} Name", "", "holiday name is empty")
            try:
                _parse_date(_field(f, b + 1), allow_weekday=True)
            except _DateError as e:
                self._err("H-HOL-4", ln, b + 1, f"Holiday {k + 1} Day", _field(f, b + 1), str(e))

    def _check_data_periods(self, ln, f):
        rec = "DATA PERIODS"
        p = _parse_int(_field(f, 1))
        if p is None or p < 1:
            self._err("H-DP-1", ln, 1, "Number of Data Periods", _field(f, 1),
                      f"number of data periods {_quote(_field(f, 1))} is not a whole number >= 1")
            return
        r = _parse_int(_field(f, 2))
        if r is None or r < 1 or r > 60 or 60 % r != 0:
            self._err("H-DP-2", ln, 2, "Number of Records per Hour", _field(f, 2),
                      f"records per hour {_quote(_field(f, 2))} must be one of 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60")
            return
        self.per_hour = r
        self.rep.summary.records_per_hour = r
        want = 3 + 4 * p
        if len(f) != want:
            self._err("H-DP-3", ln, -1, rec, "", f"{p} data periods declared, so {want} fields are expected; found {len(f)}")
            return

        periods = []
        ok = True
        for k in range(p):
            b = 3 + 4 * k
            name = _field(f, b)
            if name == "":
                self._err("H-DP-4", ln, b, f"Data Period {k + 1} Name", "", "data period name is empty")
            if _weekday_index(_field(f, b + 1)) is None:
                self._err("H-DP-5", ln, b + 1, f"Data Period {k + 1} Start Day of Week", _field(f, b + 1),
                          f"start day of week {_quote(_field(f, b + 1))} is not a weekday name (Sunday ... Saturday)")
            ends = [_ZERO_DATE, _ZERO_DATE]
            for j, what in enumerate(("Start Day", "End Day")):
                s = _field(f, b + 2 + j)
                try:
                    ends[j] = _parse_date(s, allow_year=True)
                except _DateError as e:
                    self._err("H-DP-6", ln, b + 2 + j, f"Data Period {k + 1} {what}", s, str(e))
                    ok = False
            periods.append(_Period(name, ends[0], ends[1]))
        if not ok:
            return

        for k, pr in enumerate(periods):
            if (pr.start.y == 0) != (pr.end.y == 0):
                self._err("H-DP-7", ln, -1, rec, "", f"data period {k + 1}: give a year on both start and end, or on neither")
                ok = False
            elif pr.end.before(pr.start):
                self._err("H-DP-7", ln, -1, rec, "",
                          f"data period {k + 1} ends ({pr.end}) before it starts ({pr.start}); a period cannot run "
                          "past 31 December, so split it into two periods or use m/d/y dates")
                ok = False
        if not ok:
            return
        for i in range(len(periods)):
            for j in range(i + 1, len(periods)):
                a, b = periods[i], periods[j]
                if not (a.end.before(b.start) or b.end.before(a.start)):
                    self._err("H-DP-8", ln, -1, rec, "",
                              f"data periods {i + 1} ({a.start}-{a.end}) and {j + 1} ({b.start}-{b.end}) overlap")
        for k in range(1, len(periods)):
            if periods[k].start.before(periods[k - 1].start):
                self._warn("H-DP-9", ln, -1, rec, "",
                           f"data period {k + 1} starts before data period {k}; list periods in time order")
        self.periods = periods
        self.feb_year = [0] * len(periods)
        self.rep.summary.periods = p

    # -- data rows --

    def _check_row(self, ln: int, line: str) -> None:
        if '"' in line:
            self._err("F-6", ln, -1, "", "", "data rows must not contain double quotes")
        f = line.split(",")
        if _header_position(f[0]) >= 0:
            self._err("H-ORD-3", ln, 0, "", _field(f, 0), f"header record {_quote(_field(f, 0))} found among the data rows")
            return
        self.rep.summary.data_rows += 1
        if len(f) != _NUM_FIELDS:
            self._err("D-ROW-1", ln, -1, "", "", f"row has {len(f)} fields, expected 35")

        t = [0, 0, 0, 0, 0]
        time_ok = len(f) >= 5
        for i in range(min(5, len(f))):
            v = _parse_int(f[i])
            if v is None:
                name = _COLUMNS[i].name
                self._err("D-ROW-2", ln, i, name, f[i], f"{name} {_quote(f[i])} is not a whole number")
                time_ok = False
                v = 0
            t[i] = v
        if time_ok:
            self._check_time(ln, t)
        else:
            self._seq_blind()
        if len(f) != _NUM_FIELDS:
            return
        self.rows_checked += 1
        self._check_flags(ln, _trim(f[_COL_FLAGS]))

        vals = [0.0] * _NUM_FIELDS
        present = [False] * _NUM_FIELDS
        for i in range(_COL_DRY_BULB, _NUM_FIELDS):
            col = _COLUMNS[i]
            s = _trim(f[i])
            if i == _COL_WX_CODES:
                if not _is_nine_digits(s):
                    self._err("D-ROW-4", ln, i, col.name, s, f"present weather codes {_quote(s)} must be exactly 9 digits")
                continue
            v = _parse_number(s)
            if v is None:
                self._err("D-ROW-3", ln, i, col.name, s, f"{col.name} {_quote(s)} is not a number")
                continue
            if i == _COL_WX_OBS:
                if v != 0 and v != 9:
                    self._err("D-ROW-5", ln, i, col.name, s, f"present weather observation {s} must be 0 or 9")
                continue
            if col.missing > 0 and v >= col.missing:
                if self.missing[i] == 0:
                    self.first_missing[i] = ln
                self.missing[i] += 1
                continue
            present[i], vals[i] = True, v
            if not _in_range(v, col.lo, col.hi):
                self._err("D-RNG-1", ln, i, col.name, s,
                          f"{col.name} = {s} is outside the allowed range {_range_text(col.lo, col.hi)} "
                          f"(missing values are >= {_fmt_g(col.missing)})")
            if col.warn_negative and v < 0:
                self._warn("D-RNG-2", ln, i, col.name, s, f"{col.name} {s} is negative")

        if present[_COL_DRY_BULB] and present[_COL_DEW_POINT] and vals[_COL_DEW_POINT] > vals[_COL_DRY_BULB] + 0.5:
            self._warn("D-CON-1", ln, _COL_DEW_POINT, _COLUMNS[_COL_DEW_POINT].name, f[_COL_DEW_POINT],
                       f"dew point {f[_COL_DEW_POINT]} is above dry bulb {f[_COL_DRY_BULB]}")
        if present[_COL_REL_HUMIDITY] and vals[_COL_REL_HUMIDITY] > 100:
            self._warn("D-CON-2", ln, _COL_REL_HUMIDITY, _COLUMNS[_COL_REL_HUMIDITY].name, f[_COL_REL_HUMIDITY],
                       f"relative humidity {f[_COL_REL_HUMIDITY]} is above 100")

    def _check_flags(self, ln: int, s: str) -> None:
        if s == "":
            self._err("D-ROW-7", ln, _COL_FLAGS, _COLUMNS[_COL_FLAGS].name, "", "data source and uncertainty flags are empty")
            return
        if _flags_follow_format(s):
            return
        if self.bad_flags == 0:
            self.first_bad_flags, self.bad_flags_value = ln, s
        self.bad_flags += 1

    def _finish_data(self) -> None:
        if self.bad_flags > 0:
            self._warn("D-ROW-6", self.first_bad_flags, _COL_FLAGS, _COLUMNS[_COL_FLAGS].name, self.bad_flags_value,
                       f"{self.bad_flags} of {self.rows_checked} rows have flags that are not the classic 44 "
                       "characters (a source letter A-I or ? followed by a digit, 22 times); this is common and allowed")
        if self.rows_checked == 0:
            return
        for i, col in enumerate(_COLUMNS):
            n = self.missing[i]
            if n == 0:
                continue
            self.rep.summary.missing[col.name] = n
            # S-COL-1 replaces D-MIS-1 and D-MIS-2 for the columns it covers,
            # so one missing column is reported once (SPEC.md section 11).
            if self._simien_profile() and col.simien:
                continue
            if col.core and n == self.rows_checked:
                self._err("D-MIS-1", 0, i, col.name, "",
                          f"{col.name} is missing on every row; the file cannot be used for simulation")
            elif col.core:
                self._warn("D-MIS-2", self.first_missing[i], i, col.name, "",
                           f"{col.name} is missing on {n} of {self.rows_checked} rows (first at line "
                           f"{self.first_missing[i]}); many viewers plot missing codes as real values")
            elif n == self.rows_checked:
                self._warn("D-MIS-3", 0, i, col.name, "", f"{col.name} is missing on every row")
        self._finish_simien()

    # -- the simien profile (SPEC.md section 11.1) --

    def _simien_profile(self) -> bool:
        return self.opt.profile == PROFILE_SIMIEN

    def _finish_simien(self) -> None:
        """Can this EPW become a SIMIEN hourly climate file? A different
        question from "is this a correct EPW", so these rules are opt-in and
        never change the default report.

        The target is a 12-column CSV holding one full year. Two of its
        columns, solar altitude and azimuth, are computed from the LOCATION
        coordinates and are not carried by the EPW. The rest are read straight
        across -- which is why a missing value matters here and not in an EPW:
        the CSV has no missing code, so an unfilled hour is read as a real
        number."""
        if not self._simien_profile():
            return

        for i, col in enumerate(_COLUMNS):
            n = self.missing[i]
            if n == 0 or not col.simien:
                continue
            self._err("S-COL-1", self.first_missing[i], i, col.name, "",
                      f"{col.name} is missing on {n} of {self.rows_checked} rows (first at line "
                      f"{self.first_missing[i]}); a SIMIEN climate file has no missing code, so the missing "
                      "hours would be read as real values")

        # Albedo is a ground property, not weather, and the standard station
        # files carry one constant for the year -- so a missing value has an
        # answer, and refusing a year of good weather over it would be the rule
        # biting the wrong case.
        n = self.missing[_COL_ALBEDO]
        if n > 0:
            self._warn("S-ALB-1", self.first_missing[_COL_ALBEDO], _COL_ALBEDO, _COLUMNS[_COL_ALBEDO].name, "",
                       f"albedo is missing on {n} of {self.rows_checked} rows (first at line "
                       f"{self.first_missing[_COL_ALBEDO]}); it will be filled with {_SIMIEN_DEFAULT_ALBEDO}")

        if self.feb29 > 0:
            self._err("S-LEAP-1", self.first_feb29, 2, "Day", "29",
                      f"the file has {self.feb29} rows for 29 February (first at line {self.first_feb29}); "
                      "a SIMIEN climate file numbers days 1-365")

        if self.per_hour != 1:
            self._err("S-RATE-1", 0, -1, "", "",
                      f"DATA PERIODS declares {self.per_hour} records per hour; a SIMIEN climate file is hourly, "
                      "and EPW radiation is Wh/m2 per record, which equals W/m2 only at one record per hour")

        # Counted in rows rather than periods: how the year is split across
        # DATA PERIODS is the EPW rules' business (section 7), not this
        # profile's.
        rows = self.rep.summary.data_rows
        if rows != _SIMIEN_ROWS:
            self._err("S-ROWS-1", 0, -1, "", "",
                      f"the file has {rows} data rows; a SIMIEN climate file is one full year, "
                      f"{_SIMIEN_ROWS} hourly rows")

    def _check_time(self, ln: int, t: List[int]) -> None:
        y, m, d, h, minute = t
        if y < 1900 or y > 2100:
            self._warn("D-SEQ-7", ln, _COL_YEAR, _COLUMNS[_COL_YEAR].name, str(y), f"year {y} is outside 1900-2100")
        if m < 1 or m > 12 or d < 1 or d > _days_in(m, True) or h < 1 or h > 24:
            self._err("D-SEQ-1", ln, -1, "", f"{m}/{d} hour {h}",
                      f"month {m}, day {d}, hour {h} is not a valid time (month 1-12, a day that exists in the "
                      "month, hour 1-24)")
            self._seq_blind()
            return
        if m == 2 and d == 29:
            if not (self.leap_yes and _is_leap(y)):
                reason = f"{y} is not a leap year" if self.leap_yes else "the leap-year field is No"
                self._err("D-SEQ-5", ln, 2, "Day", "29", f"row for 29 February, but {reason}")
                return  # not part of the time sequence
            # Allowed by the EPW rules; counted for S-LEAP-1 (SPEC.md 11.1).
            if self.feb29 == 0:
                self.first_feb29 = ln
            self.feb29 += 1
        step = 1
        if self.per_hour == 1:
            if minute < 0 or minute > 60:
                self._err("D-SEQ-2", ln, _COL_MINUTE, _COLUMNS[_COL_MINUTE].name, str(minute),
                          f"minute {minute} is outside 0-60")
        else:
            iv = 60 // self.per_hour
            if minute < iv or minute > 60 or minute % iv != 0:
                allowed = f"{iv}, {2 * iv}, ..., 60"
                if self.per_hour <= 4:
                    allowed = ", ".join(str(k * iv) for k in range(1, self.per_hour + 1))
                self._err("D-SEQ-2", ln, _COL_MINUTE, _COLUMNS[_COL_MINUTE].name, str(minute),
                          f"minute {minute}: with {self.per_hour} records per hour the minutes must be {allowed}")
                self._seq_blind()
                return
            step = minute // iv
        self._seq_row(ln, _Stamp(y, m, d, h, step))

    # -- time sequence --

    def _period_start(self, p: int) -> _Stamp:
        s = self.periods[p].start
        return _Stamp(s.y, s.m, s.d, 1, 1)

    def _is_period_end(self, s: _Stamp, p: int) -> bool:
        e = self.periods[p].end
        return s.m == e.m and s.d == e.d and s.h == 24 and s.step == self.per_hour and (e.y == 0 or s.y == e.y)

    def _after_period_end(self, s: _Stamp, p: int) -> bool:
        e = self.periods[p].end
        return e.before(_CalDate(s.y if e.y != 0 else 0, s.m, s.d))

    def _next(self, s: _Stamp) -> _Stamp:
        if s.step < self.per_hour:
            return s._replace(step=s.step + 1)
        if s.h < 24:
            return s._replace(h=s.h + 1, step=1)
        feb29 = self.leap_yes and s.y != 0 and _is_leap(s.y)
        if s.d < _days_in(s.m, feb29):
            return s._replace(d=s.d + 1, h=1, step=1)
        if s.m < 12:
            return s._replace(m=s.m + 1, d=1, h=1, step=1)
        return _Stamp(s.y + 1 if s.y != 0 else 0, 1, 1, 1, 1)

    def _close_period(self) -> None:
        self.seq_p += 1
        self.seq_inside = False
        if self.seq_p >= len(self.periods):
            self.seq_done = True

    def _seq_blind(self) -> None:
        """Moves one time step on for a row whose time could not be read."""
        if self.periods is None or self.seq_done:
            return
        if not self.seq_inside:
            self.seq_inside = True
            self.seq_cur = self._period_start(self.seq_p)
        else:
            self.seq_cur = self._next(self.seq_cur)
        if self._is_period_end(self.seq_cur, self.seq_p):
            self._close_period()

    def _seq_row(self, ln: int, s: _Stamp) -> None:
        """Matches a row against the declared periods. After a row that does not
        follow, checking carries on from that row."""
        if self.periods is None:
            return
        if self.seq_done:
            if self.seq_extra == 0:
                self.seq_first_extra = ln
            self.seq_extra += 1
            return
        pr = self.periods[self.seq_p]
        with_year = pr.start.y != 0

        if not self.seq_inside:
            self.seq_inside = True
            start = self._period_start(self.seq_p)
            if not _same_time(s, start, with_year):
                self._err("D-SEQ-3", ln, -1, "", str(s),
                          f"data period {self.seq_p + 1} ({_period_label(pr)}) starts at {pr.start}, but its first "
                          f"row is {s}")
        else:
            exp = self._next(self.seq_cur)
            cur = self.seq_cur
            if _same_time(s, exp, False):
                if with_year and s.y != exp.y:
                    self._err("D-SEQ-4", ln, _COL_YEAR, _COLUMNS[_COL_YEAR].name, str(s.y),
                              f"year {s.y}, expected {exp.y} (data period {self.seq_p + 1} has m/d/y dates)")
                elif not with_year and s.y != cur.y and s.m == cur.m:
                    self._err("D-SEQ-4", ln, _COL_YEAR, _COLUMNS[_COL_YEAR].name, str(s.y),
                              f"year changes from {cur.y} to {s.y} inside month {s.m}; it may change only when "
                              "the month changes")
            elif exp.m == 2 and exp.d == 29 and s.m == 3 and s.d == 1 and s.h == 1 and s.step == 1:
                self._err("D-SEQ-5", ln, -1, "", str(s),
                          f"29 February {cur.y} is missing: the leap-year field is Yes and {cur.y} is a leap year")
            elif self.seq_p + 1 < len(self.periods) and _same_time(
                    s, self._period_start(self.seq_p + 1), self.periods[self.seq_p + 1].start.y != 0):
                self._err("D-SEQ-3", ln, -1, "", str(s),
                          f"data period {self.seq_p + 1} ({_period_label(pr)}) ends early: expected {exp}, but data "
                          f"period {self.seq_p + 2} starts here")
                self._close_period()
                self._seq_row(ln, s)
                return
            elif self._after_period_end(s, self.seq_p):
                self._err("D-SEQ-3", ln, -1, "", str(s),
                          f"row {s} is after the end of data period {self.seq_p + 1} ({_period_label(pr)}, ends "
                          f"{pr.end})")
                self._close_period()
                self._seq_row(ln, s)
                return
            else:
                self._err("D-SEQ-3", ln, -1, "", str(s),
                          f"expected {exp}, found {s} (rows must follow each other one time step apart)")
        if s.m == 2:
            self.feb_year[self.seq_p] = s.y
        self.seq_cur = s
        if self._is_period_end(s, self.seq_p):
            self._close_period()

    def _period_days(self, k: int) -> int:
        """Counts the days in period k, with 29 February when the leap-year field
        and the year of the period's February rows say so."""
        pr = self.periods[k]
        y, m, d = pr.start
        n = 1
        while not (m == pr.end.m and d == pr.end.d and (pr.end.y == 0 or y == pr.end.y)):
            yy = self.feb_year[k] if pr.start.y == 0 else y
            feb29 = self.leap_yes and yy != 0 and _is_leap(yy)
            if d < _days_in(m, feb29):
                d += 1
            else:
                d, m = 1, m + 1
                if m > 12:
                    m, y = 1, y + 1
            n += 1
            if n > 366 * 200:  # cannot happen once H-DP-7 has passed; guard against a loop
                break
        return n

    def _finish_sequence(self) -> None:
        if self.periods is None:
            return
        expected = 0
        spans = []
        for k, pr in enumerate(self.periods):
            expected += self._period_days(k) * 24 * self.per_hour
            spans.append(f"{pr.start}-{pr.end}")
        s = self.rep.summary
        s.expected_rows = expected
        if s.data_rows == expected:
            return
        msg = (f"DATA PERIODS ({', '.join(spans)}, {self.per_hour} per hour) declares {expected} rows, "
               f"but the file has {s.data_rows} data rows")
        if self.seq_extra > 0:
            msg += f"; {self.seq_extra} rows come after the last period ends (first at line {self.seq_first_extra})"
        self._err("D-SEQ-6", 0, -1, "", "", msg)


# ---- Command line ----


def main(argv: Optional[List[str]] = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    as_json = False
    profile_name = ""
    paths = []
    expect_profile = False
    for a in args:
        if expect_profile:
            profile_name, expect_profile = a, False
        elif a in ("--json", "-json"):
            as_json = True
        elif a in ("--profile", "-profile"):
            expect_profile = True
        elif a.startswith("--profile=") or a.startswith("-profile="):
            profile_name = a.split("=", 1)[1]
        elif a in ("-h", "--help", "-help"):
            paths = []
            break
        else:
            paths.append(a)
    if not paths:
        print("usage: epwvalidate [--json] [--profile simien] file.epw [more.epw ...]", file=sys.stderr)
        return 2
    profile = parse_profile(profile_name)
    if profile is None:
        print(f'unknown profile "{profile_name}" (known profiles: simien)', file=sys.stderr)
        return 2
    opt = Options(profile)
    try:
        sys.stdout.reconfigure(encoding="utf-8", newline="\n")  # type: ignore[attr-defined]
    except (AttributeError, ValueError):
        pass

    code = 0
    reports = []
    for path in paths:
        try:
            rep = validate_file(path, opt)
        except OSError as e:
            print(f"{path}: {e}", file=sys.stderr)
            code = 2
            continue
        if not rep.valid and code == 0:
            code = 1
        if as_json:
            reports.append({"file": path, **rep.to_dict()})
        else:
            sys.stdout.write(format_text(path, rep))
    if as_json:
        sys.stdout.write(json.dumps(reports, indent=2, ensure_ascii=False) + "\n")
    return code


if __name__ == "__main__":
    sys.exit(main())
