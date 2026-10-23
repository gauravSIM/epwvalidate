// Types for epwvalidate.js. The report objects have exactly the shape of the
// JSON output of every language version.

/** One finding. */
export interface Issue {
  severity: 'error' | 'warning';
  /** Rule ID from SPEC.md, e.g. "D-RNG-1". */
  rule: string;
  /** 1-based line number; 0 = whole file. */
  line: number;
  /** 0-based field index; -1 = whole line. */
  column: number;
  /** Field name, e.g. "Dry Bulb Temperature". Absent when not applicable. */
  field?: string;
  /** Raw text of the offending value. Absent when not applicable. */
  value?: string;
  message: string;
}

/** Counts for the whole file. */
export interface Summary {
  lines: number;
  dataRows: number;
  /** Rows declared by DATA PERIODS; -1 if it could not be read. */
  expectedRows: number;
  periods: number;
  recordsPerHour: number;
  /** All errors, including ones not listed. */
  errorCount: number;
  /** All warnings, including ones not listed. */
  warningCount: number;
  /** Missing values per data column. */
  missing: Record<string, number>;
  /** All issues per rule, including ones not listed. */
  ruleCounts: Record<string, number>;
}

/** The result of validating one file. The file is acceptable when valid is true; warnings do not affect valid. */
export interface Report {
  valid: boolean;
  errors: Issue[];
  warnings: Issue[];
  summary: Summary;
}

/** How many issues of one rule are listed in a report. */
export const MAX_ISSUES_PER_RULE: number;

/** The EPW rules alone: is this a correct EPW file. */
export const PROFILE_NONE: '';
/** Also: can the file become a SIMIEN hourly climate file (SPEC.md §11.1). */
export const PROFILE_SIMIEN: 'simien';

/** An opt-in set of extra rules for one consumer of the file (SPEC.md §11). */
export type Profile = typeof PROFILE_NONE | typeof PROFILE_SIMIEN;

/** Changes what a validation run checks. The default is the EPW rules alone. */
export interface Options {
  /** A consumer's rules, added on top of the EPW rules. Never removes or weakens one. */
  profile?: Profile;
}

/**
 * Turns a profile name into a profile, or null when it is unknown. The empty string gives
 * PROFILE_NONE. Names are matched case-insensitively.
 */
export function parseProfile(name: string): Profile | null;

/** Validates the EPW file at path. Throws only if the file cannot be read. */
export function validateFile(path: string, options?: Options): Report;

/** Validates the content of an EPW file. */
export function validate(data: Uint8Array | string, options?: Options): Report;

/** Formats one issue as "line 79, Dry Bulb Temperature: ... [D-RNG-1]". */
export function formatIssue(issue: Issue): string;

/** Returns the report as the command-line tool prints it. */
export function formatText(name: string, report: Report): string;

/** Runs the command-line tool with the given arguments and returns its exit code. */
export function main(args: string[]): number;
