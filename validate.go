// Package epwvalidate checks EnergyPlus Weather (EPW) files. The rules, and
// the rule IDs used in every Issue, are defined in SPEC.md.
package epwvalidate

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Severity tells whether an issue makes the file invalid.
type Severity int

const (
	Error   Severity = iota // the file is not acceptable
	Warning                 // the file is acceptable, but something looks wrong
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// MarshalText makes a Severity appear as "error" or "warning" in JSON.
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Issue is one finding.
type Issue struct {
	Severity Severity `json:"severity"`
	Rule     string   `json:"rule"`            // rule ID from the spec, e.g. "D-RNG-1"
	Line     int      `json:"line"`            // 1-based line number; 0 = whole file
	Column   int      `json:"column"`          // 0-based field index; -1 = whole line
	Field    string   `json:"field,omitempty"` // field name, e.g. "Dry Bulb Temperature"
	Value    string   `json:"value,omitempty"` // raw text of the offending value
	Message  string   `json:"message"`
}

func (i Issue) String() string {
	var b strings.Builder
	if i.Line > 0 {
		fmt.Fprintf(&b, "line %d", i.Line)
	} else {
		b.WriteString("file")
	}
	if i.Field != "" {
		fmt.Fprintf(&b, ", %s", i.Field)
	}
	fmt.Fprintf(&b, ": %s [%s]", i.Message, i.Rule)
	return b.String()
}

// Summary holds counts for the whole file.
type Summary struct {
	Lines          int            `json:"lines"`
	DataRows       int            `json:"dataRows"`
	ExpectedRows   int            `json:"expectedRows"` // rows declared by DATA PERIODS; -1 if it could not be read
	Periods        int            `json:"periods"`
	RecordsPerHour int            `json:"recordsPerHour"`
	ErrorCount     int            `json:"errorCount"`   // all errors, including ones not listed
	WarningCount   int            `json:"warningCount"` // all warnings, including ones not listed
	Missing        map[string]int `json:"missing"`      // missing values per data column
	RuleCounts     map[string]int `json:"ruleCounts"`   // all issues per rule, including ones not listed
}

// Report is the result of validating one file. The file is acceptable when
// Valid is true; warnings do not affect Valid.
type Report struct {
	Valid    bool    `json:"valid"`
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
	Summary  Summary `json:"summary"`
}

// MaxIssuesPerRule limits how many issues of one rule are listed in a Report.
// Summary.RuleCounts always holds the full count.
const MaxIssuesPerRule = 20

// ValidateFile validates the EPW file at path. The error is only for I/O
// failures; problems with the file's content are in the Report.
func ValidateFile(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	return ValidateBytes(data), nil
}

// Validate validates an EPW file read from r.
func Validate(r io.Reader) (Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Report{}, err
	}
	return ValidateBytes(data), nil
}

// ValidateBytes validates the content of an EPW file.
func ValidateBytes(data []byte) Report {
	c := &checker{
		perHour: 1,
		rep: Report{
			Errors:   []Issue{},
			Warnings: []Issue{},
			Summary: Summary{
				ExpectedRows:   -1,
				RecordsPerHour: 1,
				Missing:        map[string]int{},
				RuleCounts:     map[string]int{},
			},
		},
	}
	c.run(data)
	c.rep.Valid = c.rep.Summary.ErrorCount == 0
	return c.rep
}

// checker holds the state of one validation run.
type checker struct {
	rep Report

	leapYes bool     // HOLIDAYS/DAYLIGHT SAVINGS leap-year field is Yes
	perHour int      // records per hour from DATA PERIODS (1 if unreadable)
	periods []period // nil when DATA PERIODS could not be read

	rowsChecked  int // data rows with 35 fields
	missing      [numFields]int
	firstMissing [numFields]int

	badFlags      int
	firstBadFlags int
	badFlagsValue string

	seq seqState
}

func (c *checker) add(sev Severity, rule string, line, col int, fieldName, value, format string, args ...any) {
	s := &c.rep.Summary
	s.RuleCounts[rule]++
	if sev == Error {
		s.ErrorCount++
	} else {
		s.WarningCount++
	}
	if s.RuleCounts[rule] > MaxIssuesPerRule {
		return
	}
	is := Issue{Severity: sev, Rule: rule, Line: line, Column: col, Field: fieldName, Value: value,
		Message: fmt.Sprintf(format, args...)}
	if sev == Error {
		c.rep.Errors = append(c.rep.Errors, is)
	} else {
		c.rep.Warnings = append(c.rep.Warnings, is)
	}
}

func (c *checker) errf(rule string, line, col int, fieldName, value, format string, args ...any) {
	c.add(Error, rule, line, col, fieldName, value, format, args...)
}

func (c *checker) warnf(rule string, line, col int, fieldName, value, format string, args ...any) {
	c.add(Warning, rule, line, col, fieldName, value, format, args...)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func (c *checker) run(data []byte) {
	if len(bytes.TrimSpace(data)) == 0 {
		c.errf("F-1", 0, -1, "", "", "the file is empty")
		return
	}
	if bytes.HasPrefix(data, utf8BOM) {
		c.errf("F-2", 1, -1, "", "", "the file starts with a UTF-8 byte-order mark (BOM); save it without BOM")
		data = data[len(utf8BOM):]
	}
	text := string(data)
	crlf := strings.Count(text, "\r\n")
	if lf := strings.Count(text, "\n") - crlf; crlf > 0 && lf > 0 {
		c.warnf("F-7", 0, -1, "", "", "mixed line endings: %d lines end with CRLF and %d with LF", crlf, lf)
	}

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	c.rep.Summary.Lines = len(lines)

	for i, l := range lines {
		if !utf8.ValidString(l) {
			c.errf("F-1", i+1, -1, "", "", "line is not valid UTF-8; save the file as UTF-8")
		}
		if strings.HasSuffix(strings.TrimSpace(l), ";") {
			c.errf("F-5", i+1, -1, "", "", "line ends with ';' (semicolons do not end lines in EPW)")
		}
	}

	dataStart := c.checkHeaders(lines)
	for i := dataStart; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			c.errf("F-4", i+1, -1, "", "", "empty line inside the data")
			continue
		}
		c.checkRow(i+1, lines[i])
	}
	c.finishData()
	c.finishSequence()
}

var (
	numberRE = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
	intRE    = regexp.MustCompile(`^[+-]?\d+$`)
)

// parseNumber accepts plain decimal numbers only: no "NaN", "Inf", hex or
// thousands separators.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if !numberRE.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func parseInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if !intRE.MatchString(s) {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	return v, err == nil
}

// bound is one side of an allowed range. strict means the bound itself is
// not allowed (> or <).
type bound struct {
	set, strict bool
	v           float64
}

func gt(v float64) bound { return bound{set: true, strict: true, v: v} }
func ge(v float64) bound { return bound{set: true, v: v} }
func lt(v float64) bound { return bound{set: true, strict: true, v: v} }
func le(v float64) bound { return bound{set: true, v: v} }

func inRange(v float64, lo, hi bound) bool {
	if lo.set && (v < lo.v || lo.strict && v == lo.v) {
		return false
	}
	if hi.set && (v > hi.v || hi.strict && v == hi.v) {
		return false
	}
	return true
}

func rangeText(lo, hi bound) string {
	var parts []string
	if lo.set {
		op := ">="
		if lo.strict {
			op = ">"
		}
		parts = append(parts, fmt.Sprintf("%s %g", op, lo.v))
	}
	if hi.set {
		op := "<="
		if hi.strict {
			op = "<"
		}
		parts = append(parts, fmt.Sprintf("%s %g", op, hi.v))
	}
	return strings.Join(parts, " and ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
