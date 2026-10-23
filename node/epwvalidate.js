'use strict';
/**
 * epwvalidate: checks EnergyPlus Weather (EPW) files.
 *
 * The rules, and the rule IDs used in every issue, are defined in SPEC.md.
 * This file is a self-contained port of the Go reference version and gives
 * exactly the same results (checked against testdata/expected.json). It needs
 * Node 18+ and nothing outside Node's standard library. Types are in
 * epwvalidate.d.ts.
 *
 * Library use:
 *
 *   const { validateFile, formatIssue } = require('./epwvalidate');
 *   const report = validateFile('file.epw'); // throws only if the file cannot be read
 *   if (!report.valid) for (const issue of report.errors) console.log(formatIssue(issue));
 *
 * A report is a plain object with the same keys as the JSON output of every
 * language version, so JSON.stringify(report) can be returned from an API as is.
 *
 * Command line:
 *
 *   node epwvalidate.js [--json] file.epw [more.epw ...]
 *
 * Exit code: 0 = all files valid, 1 = at least one file invalid,
 * 2 = a file could not be read or no file was given.
 */

const fs = require('fs');

/** How many issues of one rule are listed in a report; summary.ruleCounts always holds the full count. */
const MAX_ISSUES_PER_RULE = 20;

/**
 * Validates the EPW file at path. Throws only for I/O failures; problems with
 * the file's content are in the report.
 * @param {string} path
 */
function validateFile(path, options) {
  return validate(fs.readFileSync(path), options);
}

/**
 * Validates the content of an EPW file.
 * @param {Uint8Array | string} data
 */
function validate(data, options) {
  const bytes = typeof data === 'string' ? Buffer.from(data, 'utf8') : Buffer.from(data.buffer, data.byteOffset, data.byteLength);
  const c = new Checker(options);
  c.run(bytes);
  c.rep.valid = c.rep.summary.errorCount === 0;
  return c.rep;
}

/** Formats one issue as "line 79, Dry Bulb Temperature: ... [D-RNG-1]". */
function formatIssue(issue) {
  let s = issue.line > 0 ? `line ${issue.line}` : 'file';
  if (issue.field) s += `, ${issue.field}`;
  return `${s}: ${issue.message} [${issue.rule}]`;
}

/** Returns the report as the command-line tool prints it. */
function formatText(name, rep) {
  const s = rep.summary;
  let out = `${name}: ${rep.valid ? 'VALID' : 'INVALID'} - ${s.dataRows} data rows`;
  if (s.expectedRows >= 0) out += ` (DATA PERIODS declares ${s.expectedRows})`;
  out += `, ${s.errorCount} errors, ${s.warningCount} warnings\n`;
  for (const i of rep.errors) out += `  error    ${formatIssue(i)}\n`;
  for (const i of rep.warnings) out += `  warning  ${formatIssue(i)}\n`;
  const more = Object.keys(s.ruleCounts).filter((r) => s.ruleCounts[r] > MAX_ISSUES_PER_RULE).sort();
  for (const rule of more) out += `  ... and ${s.ruleCounts[rule] - MAX_ISSUES_PER_RULE} more [${rule}]\n`;
  return out;
}

// ---- Text helpers that behave like the Go version ----

// Whitespace as Go's strings.TrimSpace sees it.
const SPACE = '\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000';
const TRIM_RE = new RegExp(`^[${SPACE}]+|[${SPACE}]+$`, 'g');
const SPACE_SPLIT_RE = new RegExp(`[${SPACE}]+`);

const trim = (s) => s.replace(TRIM_RE, '');

const NUMBER_RE = /^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$/;
const INT_RE = /^[+-]?[0-9]+$/;
const INT64_MIN = -(2n ** 63n);
const INT64_MAX = 2n ** 63n - 1n;

/** Plain decimal numbers only: no "NaN", "Inf", hex or thousands separators. Returns null if not a number. */
function parseNumber(s) {
  s = trim(s);
  if (!NUMBER_RE.test(s)) return null;
  const v = Number(s);
  return Number.isFinite(v) ? v : null;
}

/** Returns null if s is not a whole number that fits in 64 bits. */
function parseInt64(s) {
  s = trim(s);
  if (!INT_RE.test(s)) return null;
  const v = BigInt(s);
  return v < INT64_MIN || v > INT64_MAX ? null : Number(v);
}

/** Formats a number like Go's %g for the values used in messages. */
function fmtG(x) {
  if (Object.is(x, -0)) return '-0';
  return String(x);
}

const PRINTABLE_RE = /[\p{L}\p{M}\p{N}\p{P}\p{S}]/u;
const ESCAPES = { '\x07': '\\a', '\b': '\\b', '\f': '\\f', '\n': '\\n', '\r': '\\r', '\t': '\\t', '\v': '\\v' };

/** Quotes a string like Go's %q. */
function quote(s) {
  let out = '"';
  for (const ch of s) {
    const o = ch.codePointAt(0);
    if (ch === '"') out += '\\"';
    else if (ch === '\\') out += '\\\\';
    else if (ESCAPES[ch]) out += ESCAPES[ch];
    else if (o < 0x20 || o === 0x7f) out += '\\x' + o.toString(16).padStart(2, '0');
    else if (ch !== ' ' && !PRINTABLE_RE.test(ch)) out += o < 0x10000 ? '\\u' + o.toString(16).padStart(4, '0') : '\\U' + o.toString(16).padStart(8, '0');
    else out += ch;
  }
  return out + '"';
}

function truncate(s, n) {
  const cp = Array.from(s);
  return cp.length <= n ? s : cp.slice(0, n).join('') + '...';
}

const roundHalfAway = (x) => Math.sign(x) * Math.floor(Math.abs(x) + 0.5);

// A bound is one side of an allowed range; strict means the bound itself is
// not allowed (> or <). null means no bound.
const gt = (v) => ({ v, strict: true });
const ge = (v) => ({ v, strict: false });
const lt = gt;
const le = ge;

function inRange(v, lo, hi) {
  if (lo && (v < lo.v || (lo.strict && v === lo.v))) return false;
  if (hi && (v > hi.v || (hi.strict && v === hi.v))) return false;
  return true;
}

function rangeText(lo, hi) {
  const parts = [];
  if (lo) parts.push(`${lo.strict ? '>' : '>='} ${fmtG(lo.v)}`);
  if (hi) parts.push(`${hi.strict ? '<' : '<='} ${fmtG(hi.v)}`);
  return parts.join(' and ');
}

// ---- Dates (SPEC.md §5.9) ----

const MONTH_NAMES = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];
const WEEKDAY_NAMES = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];

/** Returns 1-12 for a full or 3-letter month name, 0 otherwise. */
function monthIndex(s) {
  s = trim(s).toLowerCase();
  const i = MONTH_NAMES.findIndex((n) => s === n || s === n.slice(0, 3));
  return i + 1;
}

/** Returns 0 (Sunday) to 6, or -1. */
function weekdayIndex(s) {
  s = trim(s).toLowerCase();
  return WEEKDAY_NAMES.findIndex((n) => s === n || s === n.slice(0, 3));
}

const isLeap = (y) => y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);

function daysIn(month, feb29) {
  if (month === 2) return feb29 ? 29 : 28;
  return [4, 6, 9, 11].includes(month) ? 30 : 31;
}

/** A month/day, with a year only when the date had one (y = 0 otherwise). */
class CalDate {
  constructor(y, m, d) {
    this.y = y;
    this.m = m;
    this.d = d;
  }
  before(b) {
    if (this.y !== 0 && b.y !== 0 && this.y !== b.y) return this.y < b.y;
    if (this.m !== b.m) return this.m < b.m;
    return this.d < b.d;
  }
  toString() {
    return this.y !== 0 ? `${this.m}/${this.d}/${this.y}` : `${this.m}/${this.d}`;
  }
}

const ZERO_DATE = new CalDate(0, 0, 0);

class DateError extends Error {}

/**
 * Checks one header date field and returns it as month/day (plus year for
 * m/d/y). Weekday forms return a zero date. Throws DateError.
 */
function parseDate(s, allowYear = false, allowWeekday = false) {
  s = trim(s);
  if (s === '') throw new DateError('date is empty');

  if (s.includes('/')) {
    const parts = s.split('/');
    if (parts.length === 3 && !allowYear) throw new DateError(`date ${quote(s)} has a year; m/d/y is allowed only in DATA PERIODS`);
    if (parts.length !== 2 && parts.length !== 3) throw new DateError(`date ${quote(s)} is not in m/d form`);
    const n = [];
    for (const p of parts) {
      const v = parseInt64(p.split(' ').join(''));
      if (v === null) throw new DateError(`date ${quote(s)} is not in m/d form`);
      n.push(v);
    }
    let d = new CalDate(0, n[0], n[1]);
    if (parts.length === 3) {
      if (n[2] < 1) throw new DateError(`date ${quote(s)} has an invalid year`);
      d = new CalDate(n[2], n[0], n[1]);
    }
    checkMonthDay(s, d);
    return d;
  }

  const j = parseInt64(s);
  if (j !== null) {
    if (j < 1 || j > 366) throw new DateError(`day of year ${j} is outside 1-366`);
    return julianToDate(j);
  }

  const tok = s.split(SPACE_SPLIT_RE).filter((t) => t !== '');
  if (tok.length === 2) {
    let m = monthIndex(tok[0]);
    if (m > 0) {
      const d = parseInt64(tok[1]);
      if (d !== null) {
        const cd = new CalDate(0, m, d);
        checkMonthDay(s, cd);
        return cd;
      }
    }
    m = monthIndex(tok[1]);
    if (m > 0) {
      const d = parseInt64(tok[0]);
      if (d !== null) {
        const cd = new CalDate(0, m, d);
        checkMonthDay(s, cd);
        return cd;
      }
    }
  }
  if (tok.length === 4 && tok[2].toLowerCase() === 'in') {
    if (!allowWeekday) throw new DateError(`weekday date ${quote(s)} is allowed only for holidays and daylight saving`);
    const n = parseInt64(tok[0]);
    if (weekdayIndex(tok[1]) >= 0 && monthIndex(tok[3]) > 0 && (tok[0].toLowerCase() === 'last' || (n !== null && n >= 1 && n <= 5))) {
      return ZERO_DATE;
    }
  }
  throw new DateError(`${quote(s)} is not a recognised EPW date (use m/d, e.g. 1/31)`);
}

function checkMonthDay(s, d) {
  if (d.m < 1 || d.m > 12) throw new DateError(`date ${quote(s)}: month ${d.m} is outside 1-12`);
  const feb29 = d.y === 0 || isLeap(d.y);
  if (d.d < 1 || d.d > daysIn(d.m, feb29)) throw new DateError(`date ${quote(s)}: day ${d.d} does not exist in month ${d.m}`);
}

/** Converts a day of year (non-leap calendar) to month/day; 366 = 31 December. */
function julianToDate(j) {
  if (j === 366) return new CalDate(0, 12, 31);
  let m = 1;
  while (j > daysIn(m, false)) {
    j -= daysIn(m, false);
    m++;
  }
  return new CalDate(0, m, j);
}

// ---- Header records (SPEC.md §4-§5) ----

const HEADER_LINES = 8;
const HEADER_KEYWORDS = [
  'LOCATION',
  'DESIGN CONDITIONS',
  'TYPICAL/EXTREME PERIODS',
  'GROUND TEMPERATURES',
  'HOLIDAYS/DAYLIGHT SAVINGS',
  'COMMENTS 1',
  'COMMENTS 2',
  'DATA PERIODS',
];

function matchesKeyword(pos, s) {
  s = trim(s).toUpperCase();
  return s === HEADER_KEYWORDS[pos] || (pos === 4 && s === 'HOLIDAYS/DAYLIGHT SAVING');
}

function headerPosition(s) {
  for (let i = 0; i < HEADER_LINES; i++) if (matchesKeyword(i, s)) return i;
  return -1;
}

/**
 * Splits a header line, honouring double-quoted fields. Inside a quoted field
 * "" is a quote; a quote that does not end the field is kept as text.
 */
function splitCSV(line) {
  const fields = [];
  for (;;) {
    if (!line.startsWith('"')) {
      const i = line.indexOf(',');
      if (i < 0) {
        fields.push(line);
        return fields;
      }
      fields.push(line.slice(0, i));
      line = line.slice(i + 1);
      continue;
    }
    let b = '';
    line = line.slice(1);
    for (;;) {
      const i = line.indexOf('"');
      if (i < 0) {
        fields.push(b + line);
        return fields;
      }
      b += line.slice(0, i);
      line = line.slice(i + 1);
      if (line.startsWith('"')) {
        b += '"';
        line = line.slice(1);
      } else if (line === '') {
        fields.push(b);
        return fields;
      } else if (line[0] === ',') {
        line = line.slice(1);
        break;
      } else {
        b += '"';
      }
    }
    fields.push(b);
  }
}

function trimTrailingEmpty(f) {
  let n = f.length;
  while (n > 1 && trim(f[n - 1]) === '') n--;
  return [f.slice(0, n), n < f.length];
}

const field = (f, i) => (i < f.length ? trim(f[i]) : '');

const DESIGN_SECTIONS = ['Heating', 'Cooling', 'Extremes'];
// Number of Heating, Cooling and Extremes values for known ASHRAE Handbook editions (H-DC-5).
const DESIGN_LAYOUTS = {
  '2021 ASHRAE Handbook': [16, 32, 15],
  '2025 ASHRAE Handbook': [16, 32, 27],
};

const periodLabel = (p) => p.name || 'unnamed';

// ---- Data rows (SPEC.md §6-§9) ----

const NUM_FIELDS = 35;

const col = (name, lo = null, hi = null, missing = 0, opts = {}) => ({ name, lo, hi, missing, core: !!opts.core, simien: !!opts.simien, warnNegative: !!opts.warnNegative });
const CORE = { core: true };
// Core columns the simien profile also reads (SPEC.md section 11.1). Station
// pressure is core but SIMIEN never reads it, so it stays plain CORE.
const CORE_SIMIEN = { core: true, simien: true };
const NEG = { warnNegative: true };

const COLUMNS = [
  col('Year'),
  col('Month'),
  col('Day'),
  col('Hour'),
  col('Minute'),
  col('Data Source and Uncertainty Flags'),
  col('Dry Bulb Temperature', gt(-70), lt(70), 99.9, CORE_SIMIEN),
  col('Dew Point Temperature', gt(-70), lt(70), 99.9),
  col('Relative Humidity', ge(0), le(110), 999, CORE_SIMIEN),
  col('Atmospheric Station Pressure', gt(31000), lt(120000), 999999, CORE),
  col('Extraterrestrial Horizontal Radiation', ge(0), null, 9999),
  col('Extraterrestrial Direct Normal Radiation', ge(0), null, 9999),
  col('Horizontal Infrared Radiation Intensity', ge(0), null, 9999),
  col('Global Horizontal Radiation', ge(0), null, 9999, CORE_SIMIEN),
  col('Direct Normal Radiation', ge(0), null, 9999, CORE_SIMIEN),
  col('Diffuse Horizontal Radiation', ge(0), null, 9999, CORE_SIMIEN),
  col('Global Horizontal Illuminance', ge(0), null, 999900),
  col('Direct Normal Illuminance', ge(0), null, 999900),
  col('Diffuse Horizontal Illuminance', ge(0), null, 999900),
  col('Zenith Luminance', ge(0), null, 9999),
  col('Wind Direction', ge(0), le(360), 999, CORE_SIMIEN),
  col('Wind Speed', ge(0), le(40), 999, CORE_SIMIEN),
  col('Total Sky Cover', ge(0), le(10), 99),
  col('Opaque Sky Cover', ge(0), le(10), 99),
  col('Visibility', null, null, 9999, NEG),
  col('Ceiling Height', null, null, 99999, NEG),
  col('Present Weather Observation'),
  col('Present Weather Codes'),
  col('Precipitable Water', null, null, 999, NEG),
  col('Aerosol Optical Depth', null, null, 0.999, NEG),
  col('Snow Depth', null, null, 999, NEG),
  col('Days Since Last Snowfall', null, null, 99, NEG),
  col('Albedo', null, null, 999, NEG),
  col('Liquid Precipitation Depth', null, null, 999, NEG),
  col('Liquid Precipitation Quantity', null, null, 99, NEG),
];

const COL_YEAR = 0;
const COL_MINUTE = 4;
const COL_FLAGS = 5;
const COL_DRY_BULB = 6;
const COL_DEW_POINT = 7;
const COL_REL_HUMIDITY = 8;
const COL_WX_OBS = 26;
const COL_WX_CODES = 27;
const COL_ALBEDO = 32;

// Profiles name an opt-in set of extra rules for one consumer of the file
// (SPEC.md section 11). PROFILE_NONE checks the EPW rules only.
const PROFILE_NONE = '';
const PROFILE_SIMIEN = 'simien';
const PROFILES = [PROFILE_NONE, PROFILE_SIMIEN];

// simien profile: one full year of hourly values, 365 x 24, and what a missing
// albedo is filled with. The standard SIMIEN station files carry that constant
// for the whole year.
const SIMIEN_ROWS = 8760;
const SIMIEN_DEFAULT_ALBEDO = '0.20';

/**
 * Turns a profile name into a profile, or null when it is unknown. The empty
 * string gives PROFILE_NONE. Names are matched case-insensitively.
 * @param {string} name
 */
function parseProfile(name) {
  const v = String(name || '').trim().toLowerCase();
  return PROFILES.includes(v) ? v : null;
}

const isNineDigits = (s) => /^[0-9]{9}$/.test(s);

/** 22 pairs of a source letter (A-I or ?) and an uncertainty digit. */
const flagsFollowFormat = (s) => /^([A-I?][0-9]){22}$/.test(s);

// ---- Time sequence (SPEC.md §7) ----

/** The time of one data row. step is the record within the hour; y = 0 means the year is unknown. */
class Stamp {
  constructor(y, m, d, h, step) {
    this.y = y;
    this.m = m;
    this.d = d;
    this.h = h;
    this.step = step;
  }
  toString() {
    let t = this.y !== 0 ? `${this.m}/${this.d}/${this.y} hour ${this.h}` : `${this.m}/${this.d} hour ${this.h}`;
    if (this.step > 1) t += ` step ${this.step}`;
    return t;
  }
}

const sameTime = (a, b, withYear) => a.m === b.m && a.d === b.d && a.h === b.h && a.step === b.step && (!withYear || a.y === b.y);

// ---- Line handling ----

const utf8Strict = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
const utf8Loose = new TextDecoder('utf-8', { ignoreBOM: true });

function isBlankBytes(b) {
  for (const x of b) if (x !== 0x20 && x !== 0x09 && x !== 0x0a && x !== 0x0d && x !== 0x0b && x !== 0x0c) return false;
  return true;
}

// ---- The checker ----

class Checker {
  constructor(options) {
    this.opt = { profile: PROFILE_NONE, ...(options || {}) };
    this.feb29 = 0; // rows for 29 February that the EPW rules allow
    this.firstFeb29 = 0;
    this.rep = {
      valid: false,
      errors: [],
      warnings: [],
      summary: {
        lines: 0,
        dataRows: 0,
        expectedRows: -1,
        periods: 0,
        recordsPerHour: 1,
        errorCount: 0,
        warningCount: 0,
        missing: {},
        ruleCounts: {},
      },
    };
    this.leapYes = false; // leap-year field is Yes
    this.perHour = 1; // records per hour from DATA PERIODS
    this.periods = null; // null when DATA PERIODS could not be read
    this.rowsChecked = 0; // data rows with 35 fields
    this.missing = new Array(NUM_FIELDS).fill(0);
    this.firstMissing = new Array(NUM_FIELDS).fill(0);
    this.badFlags = 0;
    this.firstBadFlags = 0;
    this.badFlagsValue = '';
    // time-sequence state (see seqRow)
    this.seqP = 0;
    this.seqInside = false;
    this.seqCur = new Stamp(0, 0, 0, 0, 0);
    this.seqDone = false;
    this.seqExtra = 0;
    this.seqFirstExtra = 0;
    this.febYear = [];
  }

  // -- issues --

  add(severity, rule, line, column, fieldName, value, message) {
    const s = this.rep.summary;
    s.ruleCounts[rule] = (s.ruleCounts[rule] || 0) + 1;
    if (severity === 'error') s.errorCount++;
    else s.warningCount++;
    if (s.ruleCounts[rule] > MAX_ISSUES_PER_RULE) return;
    const issue = { severity, rule, line, column };
    if (fieldName) issue.field = fieldName;
    if (value) issue.value = value;
    issue.message = message;
    (severity === 'error' ? this.rep.errors : this.rep.warnings).push(issue);
  }

  err(rule, line, column, fieldName, value, message) {
    this.add('error', rule, line, column, fieldName, value, message);
  }

  warn(rule, line, column, fieldName, value, message) {
    this.add('warning', rule, line, column, fieldName, value, message);
  }

  // -- file level --

  run(data) {
    if (isBlankBytes(data)) {
      this.err('F-1', 0, -1, '', '', 'the file is empty');
      return;
    }
    if (data.length >= 3 && data[0] === 0xef && data[1] === 0xbb && data[2] === 0xbf) {
      this.err('F-2', 1, -1, '', '', 'the file starts with a UTF-8 byte-order mark (BOM); save it without BOM');
      data = data.subarray(3);
    }

    const raw = [];
    let crlf = 0;
    let lf = 0;
    let start = 0;
    for (let i = 0; i < data.length; i++) {
      if (data[i] !== 0x0a) continue;
      if (i > 0 && data[i - 1] === 0x0d) crlf++;
      else lf++;
      raw.push(data.subarray(start, i));
      start = i + 1;
    }
    raw.push(data.subarray(start));
    if (crlf > 0 && lf > 0) this.warn('F-7', 0, -1, '', '', `mixed line endings: ${crlf} lines end with CRLF and ${lf} with LF`);

    for (let i = 0; i < raw.length; i++) {
      const r = raw[i];
      if (r.length > 0 && r[r.length - 1] === 0x0d) raw[i] = r.subarray(0, r.length - 1);
    }
    const lines = raw.map((r) => utf8Loose.decode(r));
    while (lines.length > 0 && trim(lines[lines.length - 1]) === '') {
      lines.pop();
      raw.pop();
    }
    this.rep.summary.lines = lines.length;

    for (let i = 0; i < lines.length; i++) {
      try {
        utf8Strict.decode(raw[i]);
      } catch {
        this.err('F-1', i + 1, -1, '', '', 'line is not valid UTF-8; save the file as UTF-8');
      }
      if (trim(lines[i]).endsWith(';')) this.err('F-5', i + 1, -1, '', '', "line ends with ';' (semicolons do not end lines in EPW)");
    }

    const dataStart = this.checkHeaders(lines);
    for (let i = dataStart; i < lines.length; i++) {
      if (trim(lines[i]) === '') {
        this.err('F-4', i + 1, -1, '', '', 'empty line inside the data');
        continue;
      }
      this.checkRow(i + 1, lines[i]);
    }
    this.finishData();
    this.finishSequence();
  }

  // -- headers --

  /** Checks the 8 header records and returns the index of the first data line. */
  checkHeaders(lines) {
    let i = 0;
    let pos = 0;
    while (pos < HEADER_LINES) {
      if (i >= lines.length) {
        this.err('H-ORD-1', 0, -1, '', '', `the file ends before the ${HEADER_KEYWORDS[pos]} record (it has ${lines.length} lines)`);
        return i;
      }
      const ln = i + 1;
      const line = lines[i];
      const comma = line.indexOf(',');
      const first = comma < 0 ? line : line.slice(0, comma);
      const q = headerPosition(first);
      if (q > pos || q < 0) {
        let found = `the ${HEADER_KEYWORDS[Math.max(q, 0)]} record`;
        if (q < 0) {
          found = quote(truncate(trim(first), 40));
          if (parseInt64(first) !== null) found = 'a data row';
        }
        this.err('H-ORD-1', ln, 0, '', trim(first), `the ${HEADER_KEYWORDS[pos]} record is missing: line ${ln} should hold it, but holds ${found}`);
        pos++;
        continue;
      }
      if (q < pos) {
        this.err('H-ORD-1', ln, 0, '', trim(first), `the ${HEADER_KEYWORDS[q]} record is out of order or repeated; it belongs on line ${q + 1}`);
        i++;
        continue;
      }
      i++;
      let f;
      if (pos === 0) {
        if (line.includes('"')) this.err('F-6', ln, -1, 'LOCATION', '', 'LOCATION must not contain double quotes; many EPW readers split this line on bare commas');
        f = line.split(',');
      } else {
        f = splitCSV(line);
      }
      if (pos !== 5 && pos !== 6) {
        let trimmed;
        [f, trimmed] = trimTrailingEmpty(f);
        if (trimmed) this.warn('H-TRAIL', ln, -1, HEADER_KEYWORDS[pos], '', `${HEADER_KEYWORDS[pos]} ends with empty fields; they are ignored`);
      }
      switch (pos) {
        case 0:
          this.checkLocation(ln, f);
          break;
        case 1:
          this.checkDesignConditions(ln, f);
          break;
        case 2:
          this.checkTypicalExtreme(ln, f);
          break;
        case 3:
          this.checkGroundTemperatures(ln, f);
          break;
        case 4:
          this.checkHolidays(ln, f);
          break;
        case 5:
        case 6:
          if (field(f, 1) === '') this.err('H-COM-1', ln, 1, HEADER_KEYWORDS[pos], '', `${HEADER_KEYWORDS[pos]} has no text`);
          break;
        case 7:
          this.checkDataPeriods(ln, f);
          break;
      }
      pos++;
    }
    return i;
  }

  /** Checks that field i is a number inside [lo, hi]; returns [value, ok]. */
  headerNumber(rule, ln, f, i, name, lo, hi) {
    const s = field(f, i);
    const v = parseNumber(s);
    if (v === null) {
      this.err(rule, ln, i, name, s, `${name} ${quote(s)} is not a number`);
      return [0, false];
    }
    if (!inRange(v, lo, hi)) {
      this.err(rule, ln, i, name, s, `${name} = ${s} is outside the allowed range ${rangeText(lo, hi)}`);
      return [v, false];
    }
    return [v, true];
  }

  /** Reads the "number of ..." field at index 1; returns null if invalid. */
  headerCount(rule, ln, f, name) {
    const s = field(f, 1);
    const n = parseInt64(s);
    if (n === null || n < 0) {
      this.err(rule, ln, 1, name, s, `${name} ${quote(s)} is not a whole number >= 0`);
      return null;
    }
    return n;
  }

  checkLocation(ln, f) {
    if (f.length !== 10) {
      this.err('H-LOC-1', ln, -1, 'LOCATION', '', `LOCATION has ${f.length} fields, expected 10 (keyword, city, region, country, source, WMO, latitude, longitude, time zone, elevation)`);
    }
    if (field(f, 1) === '') this.err('H-LOC-2', ln, 1, 'City', '', 'city is empty');
    for (const [i, name] of [[2, 'State Province Region'], [3, 'Country'], [4, 'Source'], [5, 'WMO']]) {
      if (field(f, i) === '') this.err('H-LOC-3', ln, i, name, '', `${name} is empty`);
    }
    this.headerNumber('H-LOC-4', ln, f, 6, 'Latitude', ge(-90), le(90));
    const [lon, lonOK] = this.headerNumber('H-LOC-5', ln, f, 7, 'Longitude', ge(-180), le(180));
    const [tz, tzOK] = this.headerNumber('H-LOC-6', ln, f, 8, 'Time Zone', ge(-12), le(12));
    this.headerNumber('H-LOC-7', ln, f, 9, 'Elevation', ge(-1000), lt(9999.9));
    if (lonOK && tzOK) {
      const want = roundHalfAway(lon / 15);
      if (Math.abs(tz - want) > 3) {
        this.warn('H-LOC-8', ln, 8, 'Time Zone', field(f, 8), `time zone ${fmtG(tz)} h is far from the ${fmtG(want)} h expected at longitude ${fmtG(lon)}; check the sign`);
      }
    }
  }

  checkDesignConditions(ln, f) {
    const n = this.headerCount('H-DC-1', ln, f, 'Number of Design Conditions');
    if (n === null) return;
    if (n === 0) {
      if (f.length > 2) this.err('H-DC-2', ln, 2, 'DESIGN CONDITIONS', '', 'number of design conditions is 0, but the record has more fields');
      return;
    }
    if (field(f, 2) === '') {
      this.err('H-DC-3', ln, 2, 'Design Condition Source', '', 'design condition source is empty');
      return;
    }
    this.checkDesignSections(ln, n, f);
  }

  checkDesignSections(ln, n, f) {
    const rec = 'DESIGN CONDITIONS';
    const eq = (a, b) => a.toLowerCase() === b.toLowerCase();
    let i = 3;
    if (field(f, i) === '' && eq(field(f, i + 1), DESIGN_SECTIONS[0])) i++; // some writers leave one empty field after the source
    const counts = [0, 0, 0];
    for (let s = 0; s < DESIGN_SECTIONS.length; s++) {
      const name = DESIGN_SECTIONS[s];
      if (!eq(field(f, i), name)) {
        this.err('H-DC-4', ln, i, rec, field(f, i), `expected the ${name} section at field ${i + 1}, found ${quote(truncate(field(f, i), 20))}`);
        return;
      }
      i++;
      const start = i;
      while (i < f.length && (s === DESIGN_SECTIONS.length - 1 || !eq(field(f, i), DESIGN_SECTIONS[s + 1]))) {
        const v = field(f, i);
        if (v === '') {
          this.err('H-DC-4', ln, i, name + ' value', v, `${name} value at field ${i + 1} is empty`);
        } else if (parseNumber(v) === null && v.toLowerCase() !== 'n/a') {
          // ASHRAE writes N/A for "not available"
          this.err('H-DC-4', ln, i, name + ' value', v, `${name} value ${quote(v)} at field ${i + 1} is not a number`);
        }
        i++;
      }
      counts[s] = i - start;
      if (counts[s] === 0) this.err('H-DC-4', ln, i, rec, '', `the ${name} section has no values`);
    }
    if (n !== 1) return;
    for (const [edition, want] of Object.entries(DESIGN_LAYOUTS)) {
      if (field(f, 2).includes(edition) && counts.some((c, k) => c !== want[k])) {
        this.err('H-DC-5', ln, -1, rec, '', `${edition} design conditions have ${want[0]} Heating, ${want[1]} Cooling and ${want[2]} Extremes values; found ${counts[0]}, ${counts[1]} and ${counts[2]}`);
      }
    }
  }

  checkTypicalExtreme(ln, f) {
    const n = this.headerCount('H-TEP-1', ln, f, 'Number of Typical/Extreme Periods');
    if (n === null) return;
    const want = 2 + 4 * n;
    if (f.length !== want) {
      this.err('H-TEP-2', ln, -1, 'TYPICAL/EXTREME PERIODS', '', `${n} periods declared, so ${want} fields are expected; found ${f.length}`);
      return;
    }
    for (let k = 0; k < n; k++) {
      const b = 2 + 4 * k;
      if (field(f, b) === '') this.err('H-TEP-5', ln, b, `Period ${k + 1} Name`, '', 'typical/extreme period name is empty');
      const t = field(f, b + 1).toLowerCase();
      if (t === '') {
        this.err('H-TEP-5', ln, b + 1, `Period ${k + 1} Type`, '', 'typical/extreme period type is empty');
      } else if (t !== 'typical' && t !== 'extreme') {
        this.warn('H-TEP-3', ln, b + 1, `Period ${k + 1} Type`, field(f, b + 1), `period type ${quote(field(f, b + 1))} is not Typical or Extreme`);
      }
      ['Start Day', 'End Day'].forEach((what, j) => {
        const s = field(f, b + 2 + j);
        try {
          parseDate(s);
        } catch (e) {
          if (!(e instanceof DateError)) throw e;
          this.err('H-TEP-4', ln, b + 2 + j, `Period ${k + 1} ${what}`, s, e.message);
        }
      });
    }
  }

  checkGroundTemperatures(ln, f) {
    const n = this.headerCount('H-GT-1', ln, f, 'Number of Ground Temperature Depths');
    if (n === null) return;
    const want = 2 + 16 * n;
    if (f.length !== want) {
      this.err('H-GT-2', ln, -1, 'GROUND TEMPERATURES', '', `${n} depths declared, so ${want} fields are expected; found ${f.length}`);
      return;
    }
    let prev = -Infinity;
    for (let k = 0; k < n; k++) {
      const b = 2 + 16 * k;
      const [depth, ok] = this.headerNumber('H-GT-3', ln, f, b, `Depth ${k + 1}`, gt(0), null);
      if (ok) {
        if (depth < prev) this.warn('H-GT-6', ln, b, `Depth ${k + 1}`, field(f, b), 'depths are not in increasing order');
        prev = depth;
      }
      ['Soil Conductivity', 'Soil Density', 'Soil Specific Heat'].forEach((name, j) => {
        if (field(f, b + 1 + j) !== '') this.headerNumber('H-GT-4', ln, f, b + 1 + j, `Depth ${k + 1} ${name}`, gt(0), null);
      });
      for (let m = 0; m < 12; m++) {
        this.headerNumber('H-GT-5', ln, f, b + 4 + m, `Depth ${k + 1} month ${m + 1} ground temperature`, gt(-70), lt(70));
      }
    }
  }

  checkHolidays(ln, f) {
    const leap = field(f, 1).toLowerCase();
    if (leap === 'yes') this.leapYes = true;
    else if (leap !== 'no') this.err('H-HOL-1', ln, 1, 'LeapYear Observed', field(f, 1), `leap-year field ${quote(field(f, 1))} must be Yes or No`);
    ['Daylight Saving Start Day', 'Daylight Saving End Day'].forEach((name, j) => {
      const s = field(f, 2 + j);
      if (s === '0') return;
      try {
        parseDate(s, false, true);
      } catch (e) {
        if (!(e instanceof DateError)) throw e;
        this.err('H-HOL-2', ln, 2 + j, name, s, `${e.message} (use 0 for no daylight saving)`);
      }
    });
    const n = parseInt64(field(f, 4));
    if (n === null || n < 0) {
      this.err('H-HOL-3', ln, 4, 'Number of Holidays', field(f, 4), `number of holidays ${quote(field(f, 4))} is not a whole number >= 0`);
      return;
    }
    const want = 5 + 2 * n;
    if (f.length !== want) {
      this.err('H-HOL-3', ln, -1, 'HOLIDAYS/DAYLIGHT SAVINGS', '', `${n} holidays declared, so ${want} fields are expected; found ${f.length}`);
      return;
    }
    for (let k = 0; k < n; k++) {
      const b = 5 + 2 * k;
      if (field(f, b) === '') this.err('H-HOL-4', ln, b, `Holiday ${k + 1} Name`, '', 'holiday name is empty');
      try {
        parseDate(field(f, b + 1), false, true);
      } catch (e) {
        if (!(e instanceof DateError)) throw e;
        this.err('H-HOL-4', ln, b + 1, `Holiday ${k + 1} Day`, field(f, b + 1), e.message);
      }
    }
  }

  checkDataPeriods(ln, f) {
    const rec = 'DATA PERIODS';
    const p = parseInt64(field(f, 1));
    if (p === null || p < 1) {
      this.err('H-DP-1', ln, 1, 'Number of Data Periods', field(f, 1), `number of data periods ${quote(field(f, 1))} is not a whole number >= 1`);
      return;
    }
    const r = parseInt64(field(f, 2));
    if (r === null || r < 1 || r > 60 || 60 % r !== 0) {
      this.err('H-DP-2', ln, 2, 'Number of Records per Hour', field(f, 2), `records per hour ${quote(field(f, 2))} must be one of 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60`);
      return;
    }
    this.perHour = r;
    this.rep.summary.recordsPerHour = r;
    const want = 3 + 4 * p;
    if (f.length !== want) {
      this.err('H-DP-3', ln, -1, rec, '', `${p} data periods declared, so ${want} fields are expected; found ${f.length}`);
      return;
    }

    const periods = [];
    let ok = true;
    for (let k = 0; k < p; k++) {
      const b = 3 + 4 * k;
      const name = field(f, b);
      if (name === '') this.err('H-DP-4', ln, b, `Data Period ${k + 1} Name`, '', 'data period name is empty');
      if (weekdayIndex(field(f, b + 1)) < 0) {
        this.err('H-DP-5', ln, b + 1, `Data Period ${k + 1} Start Day of Week`, field(f, b + 1), `start day of week ${quote(field(f, b + 1))} is not a weekday name (Sunday ... Saturday)`);
      }
      const ends = [ZERO_DATE, ZERO_DATE];
      ['Start Day', 'End Day'].forEach((what, j) => {
        const s = field(f, b + 2 + j);
        try {
          ends[j] = parseDate(s, true, false);
        } catch (e) {
          if (!(e instanceof DateError)) throw e;
          this.err('H-DP-6', ln, b + 2 + j, `Data Period ${k + 1} ${what}`, s, e.message);
          ok = false;
        }
      });
      periods.push({ name, start: ends[0], end: ends[1] });
    }
    if (!ok) return;

    periods.forEach((pr, k) => {
      if ((pr.start.y === 0) !== (pr.end.y === 0)) {
        this.err('H-DP-7', ln, -1, rec, '', `data period ${k + 1}: give a year on both start and end, or on neither`);
        ok = false;
      } else if (pr.end.before(pr.start)) {
        this.err('H-DP-7', ln, -1, rec, '', `data period ${k + 1} ends (${pr.end}) before it starts (${pr.start}); a period cannot run past 31 December, so split it into two periods or use m/d/y dates`);
        ok = false;
      }
    });
    if (!ok) return;
    for (let i = 0; i < periods.length; i++) {
      for (let j = i + 1; j < periods.length; j++) {
        const a = periods[i];
        const b = periods[j];
        if (!(a.end.before(b.start) || b.end.before(a.start))) {
          this.err('H-DP-8', ln, -1, rec, '', `data periods ${i + 1} (${a.start}-${a.end}) and ${j + 1} (${b.start}-${b.end}) overlap`);
        }
      }
    }
    for (let k = 1; k < periods.length; k++) {
      if (periods[k].start.before(periods[k - 1].start)) {
        this.warn('H-DP-9', ln, -1, rec, '', `data period ${k + 1} starts before data period ${k}; list periods in time order`);
      }
    }
    this.periods = periods;
    this.febYear = new Array(periods.length).fill(0);
    this.rep.summary.periods = p;
  }

  // -- data rows --

  checkRow(ln, line) {
    if (line.includes('"')) this.err('F-6', ln, -1, '', '', 'data rows must not contain double quotes');
    const f = line.split(',');
    if (headerPosition(f[0]) >= 0) {
      this.err('H-ORD-3', ln, 0, '', field(f, 0), `header record ${quote(field(f, 0))} found among the data rows`);
      return;
    }
    this.rep.summary.dataRows++;
    if (f.length !== NUM_FIELDS) this.err('D-ROW-1', ln, -1, '', '', `row has ${f.length} fields, expected 35`);

    const t = [0, 0, 0, 0, 0];
    let timeOK = f.length >= 5;
    for (let i = 0; i < 5 && i < f.length; i++) {
      const v = parseInt64(f[i]);
      if (v === null) {
        const name = COLUMNS[i].name;
        this.err('D-ROW-2', ln, i, name, f[i], `${name} ${quote(f[i])} is not a whole number`);
        timeOK = false;
      } else {
        t[i] = v;
      }
    }
    if (timeOK) this.checkTime(ln, t);
    else this.seqBlind();
    if (f.length !== NUM_FIELDS) return;
    this.rowsChecked++;
    this.checkFlags(ln, trim(f[COL_FLAGS]));

    const vals = new Array(NUM_FIELDS).fill(0);
    const present = new Array(NUM_FIELDS).fill(false);
    for (let i = COL_DRY_BULB; i < NUM_FIELDS; i++) {
      const c = COLUMNS[i];
      const s = trim(f[i]);
      if (i === COL_WX_CODES) {
        if (!isNineDigits(s)) this.err('D-ROW-4', ln, i, c.name, s, `present weather codes ${quote(s)} must be exactly 9 digits`);
        continue;
      }
      const v = parseNumber(s);
      if (v === null) {
        this.err('D-ROW-3', ln, i, c.name, s, `${c.name} ${quote(s)} is not a number`);
        continue;
      }
      if (i === COL_WX_OBS) {
        if (v !== 0 && v !== 9) this.err('D-ROW-5', ln, i, c.name, s, `present weather observation ${s} must be 0 or 9`);
        continue;
      }
      if (c.missing > 0 && v >= c.missing) {
        if (this.missing[i] === 0) this.firstMissing[i] = ln;
        this.missing[i]++;
        continue;
      }
      present[i] = true;
      vals[i] = v;
      if (!inRange(v, c.lo, c.hi)) {
        this.err('D-RNG-1', ln, i, c.name, s, `${c.name} = ${s} is outside the allowed range ${rangeText(c.lo, c.hi)} (missing values are >= ${fmtG(c.missing)})`);
      }
      if (c.warnNegative && v < 0) this.warn('D-RNG-2', ln, i, c.name, s, `${c.name} ${s} is negative`);
    }

    if (present[COL_DRY_BULB] && present[COL_DEW_POINT] && vals[COL_DEW_POINT] > vals[COL_DRY_BULB] + 0.5) {
      this.warn('D-CON-1', ln, COL_DEW_POINT, COLUMNS[COL_DEW_POINT].name, f[COL_DEW_POINT], `dew point ${f[COL_DEW_POINT]} is above dry bulb ${f[COL_DRY_BULB]}`);
    }
    if (present[COL_REL_HUMIDITY] && vals[COL_REL_HUMIDITY] > 100) {
      this.warn('D-CON-2', ln, COL_REL_HUMIDITY, COLUMNS[COL_REL_HUMIDITY].name, f[COL_REL_HUMIDITY], `relative humidity ${f[COL_REL_HUMIDITY]} is above 100`);
    }
  }

  checkFlags(ln, s) {
    if (s === '') {
      this.err('D-ROW-7', ln, COL_FLAGS, COLUMNS[COL_FLAGS].name, '', 'data source and uncertainty flags are empty');
      return;
    }
    if (flagsFollowFormat(s)) return;
    if (this.badFlags === 0) {
      this.firstBadFlags = ln;
      this.badFlagsValue = s;
    }
    this.badFlags++;
  }

  finishData() {
    if (this.badFlags > 0) {
      this.warn('D-ROW-6', this.firstBadFlags, COL_FLAGS, COLUMNS[COL_FLAGS].name, this.badFlagsValue, `${this.badFlags} of ${this.rowsChecked} rows have flags that are not the classic 44 characters (a source letter A-I or ? followed by a digit, 22 times); this is common and allowed`);
    }
    if (this.rowsChecked === 0) return;
    COLUMNS.forEach((c, i) => {
      const n = this.missing[i];
      if (n === 0) return;
      this.rep.summary.missing[c.name] = n;
      // S-COL-1 replaces D-MIS-1 and D-MIS-2 for the columns it covers, so one
      // missing column is reported once (SPEC.md section 11).
      if (this.simienProfile() && c.simien) return;
      if (c.core && n === this.rowsChecked) {
        this.err('D-MIS-1', 0, i, c.name, '', `${c.name} is missing on every row; the file cannot be used for simulation`);
      } else if (c.core) {
        this.warn('D-MIS-2', this.firstMissing[i], i, c.name, '', `${c.name} is missing on ${n} of ${this.rowsChecked} rows (first at line ${this.firstMissing[i]}); many viewers plot missing codes as real values`);
      } else if (n === this.rowsChecked) {
        this.warn('D-MIS-3', 0, i, c.name, '', `${c.name} is missing on every row`);
      }
    });
    this.finishSimien();
  }

  simienProfile() {
    return this.opt.profile === PROFILE_SIMIEN;
  }

  /**
   * The simien profile (SPEC.md section 11.1): can this EPW become a SIMIEN
   * hourly climate file? A different question from "is this a correct EPW", so
   * these rules are opt-in and never change the default report.
   *
   * The target is a 12-column CSV holding one full year. Two of its columns,
   * solar altitude and azimuth, are computed from the LOCATION coordinates and
   * are not carried by the EPW. The rest are read straight across -- which is
   * why a missing value matters here and not in an EPW: the CSV has no missing
   * code, so an unfilled hour is read as a real number.
   */
  finishSimien() {
    if (!this.simienProfile()) return;

    COLUMNS.forEach((c, i) => {
      const n = this.missing[i];
      if (n === 0 || !c.simien) return;
      this.err('S-COL-1', this.firstMissing[i], i, c.name, '', `${c.name} is missing on ${n} of ${this.rowsChecked} rows (first at line ${this.firstMissing[i]}); a SIMIEN climate file has no missing code, so the missing hours would be read as real values`);
    });

    // Albedo is a ground property, not weather, and the standard station files
    // carry one constant for the year -- so a missing value has an answer, and
    // refusing a year of good weather over it would be the rule biting the
    // wrong case.
    const alb = this.missing[COL_ALBEDO];
    if (alb > 0) {
      this.warn('S-ALB-1', this.firstMissing[COL_ALBEDO], COL_ALBEDO, COLUMNS[COL_ALBEDO].name, '', `albedo is missing on ${alb} of ${this.rowsChecked} rows (first at line ${this.firstMissing[COL_ALBEDO]}); it will be filled with ${SIMIEN_DEFAULT_ALBEDO}`);
    }

    if (this.feb29 > 0) {
      this.err('S-LEAP-1', this.firstFeb29, 2, 'Day', '29', `the file has ${this.feb29} rows for 29 February (first at line ${this.firstFeb29}); a SIMIEN climate file numbers days 1-365`);
    }

    if (this.perHour !== 1) {
      this.err('S-RATE-1', 0, -1, '', '', `DATA PERIODS declares ${this.perHour} records per hour; a SIMIEN climate file is hourly, and EPW radiation is Wh/m2 per record, which equals W/m2 only at one record per hour`);
    }

    // Counted in rows rather than periods: how the year is split across DATA
    // PERIODS is the EPW rules' business (section 7), not this profile's.
    const rows = this.rep.summary.dataRows;
    if (rows !== SIMIEN_ROWS) {
      this.err('S-ROWS-1', 0, -1, '', '', `the file has ${rows} data rows; a SIMIEN climate file is one full year, ${SIMIEN_ROWS} hourly rows`);
    }
  }

  checkTime(ln, t) {
    const [y, m, d, h, minute] = t;
    if (y < 1900 || y > 2100) this.warn('D-SEQ-7', ln, COL_YEAR, COLUMNS[COL_YEAR].name, String(y), `year ${y} is outside 1900-2100`);
    if (m < 1 || m > 12 || d < 1 || d > daysIn(m, true) || h < 1 || h > 24) {
      this.err('D-SEQ-1', ln, -1, '', `${m}/${d} hour ${h}`, `month ${m}, day ${d}, hour ${h} is not a valid time (month 1-12, a day that exists in the month, hour 1-24)`);
      this.seqBlind();
      return;
    }
    if (m === 2 && d === 29) {
      if (!(this.leapYes && isLeap(y))) {
        const reason = this.leapYes ? `${y} is not a leap year` : 'the leap-year field is No';
        this.err('D-SEQ-5', ln, 2, 'Day', '29', `row for 29 February, but ${reason}`);
        return; // not part of the time sequence
      }
      // Allowed by the EPW rules; counted for S-LEAP-1 (SPEC.md 11.1).
      if (this.feb29 === 0) this.firstFeb29 = ln;
      this.feb29++;
    }
    let step = 1;
    if (this.perHour === 1) {
      if (minute < 0 || minute > 60) this.err('D-SEQ-2', ln, COL_MINUTE, COLUMNS[COL_MINUTE].name, String(minute), `minute ${minute} is outside 0-60`);
    } else {
      const iv = 60 / this.perHour;
      if (minute < iv || minute > 60 || minute % iv !== 0) {
        let allowed = `${iv}, ${2 * iv}, ..., 60`;
        if (this.perHour <= 4) allowed = Array.from({ length: this.perHour }, (_, k) => String((k + 1) * iv)).join(', ');
        this.err('D-SEQ-2', ln, COL_MINUTE, COLUMNS[COL_MINUTE].name, String(minute), `minute ${minute}: with ${this.perHour} records per hour the minutes must be ${allowed}`);
        this.seqBlind();
        return;
      }
      step = minute / iv;
    }
    this.seqRow(ln, new Stamp(y, m, d, h, step));
  }

  // -- time sequence --

  periodStart(p) {
    const s = this.periods[p].start;
    return new Stamp(s.y, s.m, s.d, 1, 1);
  }

  isPeriodEnd(s, p) {
    const e = this.periods[p].end;
    return s.m === e.m && s.d === e.d && s.h === 24 && s.step === this.perHour && (e.y === 0 || s.y === e.y);
  }

  afterPeriodEnd(s, p) {
    const e = this.periods[p].end;
    return e.before(new CalDate(e.y !== 0 ? s.y : 0, s.m, s.d));
  }

  next(s) {
    if (s.step < this.perHour) return new Stamp(s.y, s.m, s.d, s.h, s.step + 1);
    if (s.h < 24) return new Stamp(s.y, s.m, s.d, s.h + 1, 1);
    const feb29 = this.leapYes && s.y !== 0 && isLeap(s.y);
    if (s.d < daysIn(s.m, feb29)) return new Stamp(s.y, s.m, s.d + 1, 1, 1);
    if (s.m < 12) return new Stamp(s.y, s.m + 1, 1, 1, 1);
    return new Stamp(s.y !== 0 ? s.y + 1 : 0, 1, 1, 1, 1);
  }

  closePeriod() {
    this.seqP++;
    this.seqInside = false;
    if (this.seqP >= this.periods.length) this.seqDone = true;
  }

  /** Moves one time step on for a row whose time could not be read. */
  seqBlind() {
    if (this.periods === null || this.seqDone) return;
    if (!this.seqInside) {
      this.seqInside = true;
      this.seqCur = this.periodStart(this.seqP);
    } else {
      this.seqCur = this.next(this.seqCur);
    }
    if (this.isPeriodEnd(this.seqCur, this.seqP)) this.closePeriod();
  }

  /** Matches a row against the declared periods. After a row that does not follow, checking carries on from that row. */
  seqRow(ln, s) {
    if (this.periods === null) return;
    if (this.seqDone) {
      if (this.seqExtra === 0) this.seqFirstExtra = ln;
      this.seqExtra++;
      return;
    }
    const pr = this.periods[this.seqP];
    const withYear = pr.start.y !== 0;

    if (!this.seqInside) {
      this.seqInside = true;
      const start = this.periodStart(this.seqP);
      if (!sameTime(s, start, withYear)) {
        this.err('D-SEQ-3', ln, -1, '', String(s), `data period ${this.seqP + 1} (${periodLabel(pr)}) starts at ${pr.start}, but its first row is ${s}`);
      }
    } else {
      const exp = this.next(this.seqCur);
      const cur = this.seqCur;
      if (sameTime(s, exp, false)) {
        if (withYear && s.y !== exp.y) {
          this.err('D-SEQ-4', ln, COL_YEAR, COLUMNS[COL_YEAR].name, String(s.y), `year ${s.y}, expected ${exp.y} (data period ${this.seqP + 1} has m/d/y dates)`);
        } else if (!withYear && s.y !== cur.y && s.m === cur.m) {
          this.err('D-SEQ-4', ln, COL_YEAR, COLUMNS[COL_YEAR].name, String(s.y), `year changes from ${cur.y} to ${s.y} inside month ${s.m}; it may change only when the month changes`);
        }
      } else if (exp.m === 2 && exp.d === 29 && s.m === 3 && s.d === 1 && s.h === 1 && s.step === 1) {
        this.err('D-SEQ-5', ln, -1, '', String(s), `29 February ${cur.y} is missing: the leap-year field is Yes and ${cur.y} is a leap year`);
      } else if (this.seqP + 1 < this.periods.length && sameTime(s, this.periodStart(this.seqP + 1), this.periods[this.seqP + 1].start.y !== 0)) {
        this.err('D-SEQ-3', ln, -1, '', String(s), `data period ${this.seqP + 1} (${periodLabel(pr)}) ends early: expected ${exp}, but data period ${this.seqP + 2} starts here`);
        this.closePeriod();
        this.seqRow(ln, s);
        return;
      } else if (this.afterPeriodEnd(s, this.seqP)) {
        this.err('D-SEQ-3', ln, -1, '', String(s), `row ${s} is after the end of data period ${this.seqP + 1} (${periodLabel(pr)}, ends ${pr.end})`);
        this.closePeriod();
        this.seqRow(ln, s);
        return;
      } else {
        this.err('D-SEQ-3', ln, -1, '', String(s), `expected ${exp}, found ${s} (rows must follow each other one time step apart)`);
      }
    }
    if (s.m === 2) this.febYear[this.seqP] = s.y;
    this.seqCur = s;
    if (this.isPeriodEnd(s, this.seqP)) this.closePeriod();
  }

  /** Counts the days in period k, with 29 February when the leap-year field and the year of the period's February rows say so. */
  periodDays(k) {
    const pr = this.periods[k];
    let { y, m, d } = pr.start;
    let n = 1;
    while (!(m === pr.end.m && d === pr.end.d && (pr.end.y === 0 || y === pr.end.y))) {
      const yy = pr.start.y === 0 ? this.febYear[k] : y;
      const feb29 = this.leapYes && yy !== 0 && isLeap(yy);
      if (d < daysIn(m, feb29)) {
        d++;
      } else {
        d = 1;
        m++;
        if (m > 12) {
          m = 1;
          y++;
        }
      }
      n++;
      if (n > 366 * 200) break; // cannot happen once H-DP-7 has passed; guard against a loop
    }
    return n;
  }

  finishSequence() {
    if (this.periods === null) return;
    let expected = 0;
    const spans = [];
    this.periods.forEach((pr, k) => {
      expected += this.periodDays(k) * 24 * this.perHour;
      spans.push(`${pr.start}-${pr.end}`);
    });
    const s = this.rep.summary;
    s.expectedRows = expected;
    if (s.dataRows === expected) return;
    let msg = `DATA PERIODS (${spans.join(', ')}, ${this.perHour} per hour) declares ${expected} rows, but the file has ${s.dataRows} data rows`;
    if (this.seqExtra > 0) msg += `; ${this.seqExtra} rows come after the last period ends (first at line ${this.seqFirstExtra})`;
    this.err('D-SEQ-6', 0, -1, '', '', msg);
  }
}

// ---- Command line ----

function main(args) {
  let asJSON = false;
  let profileName = '';
  let paths = [];
  let expectProfile = false;
  for (const a of args) {
    if (expectProfile) {
      profileName = a;
      expectProfile = false;
    } else if (a === '--json' || a === '-json') asJSON = true;
    else if (a === '--profile' || a === '-profile') expectProfile = true;
    else if (a.startsWith('--profile=') || a.startsWith('-profile=')) profileName = a.slice(a.indexOf('=') + 1);
    else if (a === '-h' || a === '--help' || a === '-help') {
      paths = [];
      break;
    } else paths.push(a);
  }
  if (paths.length === 0) {
    process.stderr.write('usage: epwvalidate [--json] [--profile simien] file.epw [more.epw ...]\n');
    return 2;
  }
  const profile = parseProfile(profileName);
  if (profile === null) {
    process.stderr.write(`unknown profile "${profileName}" (known profiles: simien)\n`);
    return 2;
  }
  const opt = { profile };
  let code = 0;
  const reports = [];
  for (const path of paths) {
    let rep;
    try {
      rep = validateFile(path, opt);
    } catch (e) {
      process.stderr.write(`${path}: ${e.message}\n`);
      code = 2;
      continue;
    }
    if (!rep.valid && code === 0) code = 1;
    if (asJSON) reports.push({ file: path, ...rep });
    else process.stdout.write(formatText(path, rep));
  }
  if (asJSON) process.stdout.write(JSON.stringify(reports, null, 2) + '\n');
  return code;
}

module.exports = { validate, validateFile, formatIssue, formatText, parseProfile, PROFILE_NONE, PROFILE_SIMIEN, MAX_ISSUES_PER_RULE, main };

if (require.main === module) {
  process.exitCode = main(process.argv.slice(2));
}
