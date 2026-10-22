package epwvalidate

import (
	"encoding/csv"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealFiles: every example file in EPW_format/epw_examples must be valid
// (SPEC.md §11).
// Set EPWVALIDATE_SAMPLES to a folder to also check every .epw file below it.
func TestRealFiles(t *testing.T) {
	files, _ := filepath.Glob("EPW_format/epw_examples/*.epw")
	if dir := os.Getenv("EPWVALIDATE_SAMPLES"); dir != "" {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "broken" || d.Name() == "partial") {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".epw") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		t.Fatal("no EPW files found")
	}
	for _, path := range files {
		path := path
		t.Run(path, func(t *testing.T) {
			rep, err := ValidateFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.Valid {
				t.Fatalf("expected valid, got:\n%s", describe(rep))
			}
			if rep.Summary.DataRows != rep.Summary.ExpectedRows {
				t.Errorf("data rows %d, DATA PERIODS declares %d", rep.Summary.DataRows, rep.Summary.ExpectedRows)
			}
		})
	}
}

// TestFixtureFiles checks the files on disk by their folder and name alone,
// so new files can be dropped into testdata/partial or testdata/broken
// (named <RULE>__<description>.epw) without changing any code.
func TestFixtureFiles(t *testing.T) {
	for _, dir := range []string{"partial", "broken"} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*.epw"))
		if len(files) == 0 {
			t.Fatalf("no files in testdata/%s; run: go test -run TestFixtures -update", dir)
		}
		for _, path := range files {
			path := path
			fx := fixture{dir: dir}
			if dir == "broken" {
				fx.rule, _, _ = strings.Cut(filepath.Base(path), "__")
			}
			t.Run(dir+"/"+filepath.Base(path), func(t *testing.T) {
				rep, err := ValidateFile(path)
				if err != nil {
					t.Fatal(err)
				}
				checkFixture(t, fx, rep)
			})
		}
	}
}

// A single missing, extra or swapped row must give a small number of
// errors, not one per following row.
func TestSequenceErrorsDoNotCascade(t *testing.T) {
	loadBases(t)
	cases := []struct {
		name   string
		change func([]string) []string
		max    int
	}{
		{"deleted", func(l []string) []string { return removeAt(l, row(100)) }, 1},
		{"duplicated", func(l []string) []string { return insertAt(l, row(100), l[row(100)]) }, 1},
		{"swapped", func(l []string) []string { l[row(100)], l[row(101)] = l[row(101)], l[row(100)]; return l }, 3},
		{"bad hour", func(l []string) []string { l[row(100)] = setField(l[row(100)], 3, "25"); return l }, 0},
	}
	for _, tc := range cases {
		rep := ValidateBytes(epw(tc.change(week())))
		if n := rep.Summary.RuleCounts["D-SEQ-3"]; n > tc.max {
			t.Errorf("%s: %d D-SEQ-3 errors, want at most %d:\n%s", tc.name, n, tc.max, describe(rep))
		}
	}
}

func TestFullYearSequence(t *testing.T) {
	loadBases(t)
	l := append([]string{}, osloBase...)
	setLeapYear(l, true)
	setYear(l, 0, 2020)
	rep := ValidateBytes(epw(addFeb29(l)))
	if !rep.Valid || rep.Summary.DataRows != 8784 || rep.Summary.ExpectedRows != 8784 {
		t.Errorf("leap year 2020 full year: valid=%v rows=%d expected=%d\n%s",
			rep.Valid, rep.Summary.DataRows, rep.Summary.ExpectedRows, describe(rep))
	}

	// 8760 rows declared as a full leap year: 29 February is missing.
	l = append([]string{}, osloBase...)
	setLeapYear(l, true)
	setYear(l, 0, 2020)
	rep = ValidateBytes(epw(l))
	if !hasError(rep, "D-SEQ-5") || !hasError(rep, "D-SEQ-6") {
		t.Errorf("leap Yes without 29 February: want D-SEQ-5 and D-SEQ-6, got:\n%s", describe(rep))
	}
}

func TestIssueLimitAndCounts(t *testing.T) {
	loadBases(t)
	l := week()
	for i := headerLines; i < len(l); i++ {
		l[i] = setField(l[i], 21, "50")
	}
	rep := ValidateBytes(epw(l))
	if got := rep.Summary.RuleCounts["D-RNG-1"]; got != 168 {
		t.Errorf("RuleCounts[D-RNG-1] = %d, want 168", got)
	}
	listed := 0
	for _, is := range rep.Errors {
		if is.Rule == "D-RNG-1" {
			listed++
			if is.Line == 0 || is.Column != 21 || is.Field != "Wind Speed" || is.Value != "50" {
				t.Errorf("issue fields not filled in: %+v", is)
			}
		}
	}
	if listed != MaxIssuesPerRule {
		t.Errorf("listed %d D-RNG-1 issues, want %d", listed, MaxIssuesPerRule)
	}
	if rep.Summary.ErrorCount != 168 {
		t.Errorf("ErrorCount = %d, want 168", rep.Summary.ErrorCount)
	}
}

func TestWarnings(t *testing.T) {
	loadBases(t)
	cases := []struct {
		name   string
		change func([]string) []string
		rule   string
	}{
		{"dew point above dry bulb", func(l []string) []string {
			l[row(5)] = setField(setField(l[row(5)], 6, "5.0"), 7, "8.0")
			return l
		}, "D-CON-1"},
		{"humidity 105", func(l []string) []string { l[row(5)] = setField(l[row(5)], 8, "105"); return l }, "D-CON-2"},
		{"some dry bulb missing", func(l []string) []string { l[row(5)] = setField(l[row(5)], 6, "99.9"); return l }, "D-MIS-2"},
		{"snow depth negative", func(l []string) []string { l[row(5)] = setField(l[row(5)], 30, "-1"); return l }, "D-RNG-2"},
		{"year 1850", func(l []string) []string { return setColumn(l, 0, "1850") }, "D-SEQ-7"},
		{"time zone sign swapped", func(l []string) []string { l[0] = setField(l[0], 8, "-8.0"); return l }, "H-LOC-8"},
		{"trailing commas", func(l []string) []string { l[7] += ",,"; return l }, "H-TRAIL"},
		{"period type odd", func(l []string) []string { l[2] = setField(l[2], 3, "Normal"); return l }, "H-TEP-3"},
		{"periods out of order", func(l []string) []string {
			out := append([]string{}, l[:7]...)
			out = append(out, "DATA PERIODS,2,1,B,Thursday,1/11,1/14,A,Monday,1/ 8,1/10")
			out = append(out, l[row(72):]...)
			return append(out, l[row(0):row(72)]...)
		}, "H-DP-9"},
	}
	for _, tc := range cases {
		rep := ValidateBytes(epw(tc.change(week())))
		if !rep.Valid {
			t.Errorf("%s: expected valid (warning only), got:\n%s", tc.name, describe(rep))
		}
		found := false
		for _, is := range rep.Warnings {
			found = found || is.Rule == tc.rule
		}
		if !found {
			t.Errorf("%s: expected warning %s, got:\n%s", tc.name, tc.rule, describe(rep))
		}
	}
}

func TestMixedLineEndings(t *testing.T) {
	loadBases(t)
	l := week()
	data := strings.Join(l[:20], "\r\n") + "\n" + strings.Join(l[20:], "\r\n") + "\r\n"
	rep := ValidateBytes([]byte(data))
	if !rep.Valid || rep.Summary.RuleCounts["F-7"] != 1 {
		t.Errorf("mixed line endings: want valid with one F-7 warning, got:\n%s", describe(rep))
	}
}

func TestValidateReader(t *testing.T) {
	f, err := os.Open(osloFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rep, err := Validate(f)
	if err != nil || !rep.Valid {
		t.Errorf("Validate(reader): err=%v valid=%v", err, rep.Valid)
	}
	if _, err := ValidateFile("testdata/does-not-exist.epw"); err == nil {
		t.Error("ValidateFile on a missing file should return an error")
	}
}

func TestParseDate(t *testing.T) {
	plain, withYear, withWeekday := dateOpts{}, dateOpts{allowYear: true}, dateOpts{allowWeekday: true}
	cases := []struct {
		in   string
		opts dateOpts
		want calDate
		ok   bool
	}{
		{"1/ 1", plain, calDate{m: 1, d: 1}, true},
		{"12/31", plain, calDate{m: 12, d: 31}, true},
		{" 8/ 2 ", plain, calDate{m: 8, d: 2}, true},
		{"2/29", plain, calDate{m: 2, d: 29}, true},
		{"2/30", plain, calDate{}, false},
		{"4/31", plain, calDate{}, false},
		{"13/1", plain, calDate{}, false},
		{"0/5", plain, calDate{}, false},
		{"1/1/2019", withYear, calDate{y: 2019, m: 1, d: 1}, true},
		{"1/1/2019", plain, calDate{}, false},
		{"2/29/2019", withYear, calDate{}, false},
		{"2/29/2020", withYear, calDate{y: 2020, m: 2, d: 29}, true},
		{"32", plain, calDate{m: 2, d: 1}, true},
		{"365", plain, calDate{m: 12, d: 31}, true},
		{"0", plain, calDate{}, false},
		{"367", plain, calDate{}, false},
		{"5 Jan", plain, calDate{m: 1, d: 5}, true},
		{"January 5", plain, calDate{m: 1, d: 5}, true},
		{"jan 31", plain, calDate{m: 1, d: 31}, true},
		{"Feb 30", plain, calDate{}, false},
		{"Last Sunday in October", withWeekday, calDate{}, true},
		{"2 Sun in Mar", withWeekday, calDate{}, true},
		{"Last Sunday in October", plain, calDate{}, false},
		{"6 Sunday in March", withWeekday, calDate{}, false},
		{"", plain, calDate{}, false},
		{"tomorrow", plain, calDate{}, false},
		{"1-1", plain, calDate{}, false},
	}
	for _, tc := range cases {
		got, err := parseDate(tc.in, tc.opts)
		if (err == nil) != tc.ok {
			t.Errorf("parseDate(%q): err=%v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if tc.ok && got != tc.want {
			t.Errorf("parseDate(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseNumber(t *testing.T) {
	good := []string{"0", "-22.10", "+1.5", ".5", "5.", "1e3", "0.9990", " 7 "}
	bad := []string{"", "abc", "NaN", "Inf", "0x10", "1,5", "1.2.3", "--1", "1_000"}
	for _, s := range good {
		if _, ok := parseNumber(s); !ok {
			t.Errorf("parseNumber(%q) rejected", s)
		}
	}
	for _, s := range bad {
		if _, ok := parseNumber(s); ok {
			t.Errorf("parseNumber(%q) accepted", s)
		}
	}
}

// TestHeaderPartsMissing removes every header row, cuts every header row
// short at every field, and blanks every header field one at a time. Each
// change must make the file invalid. The only fields that may be empty are
// the soil properties in GROUND TEMPERATURES and the empty field some
// writers put after the DESIGN CONDITIONS source.
func TestHeaderPartsMissing(t *testing.T) {
	loadBases(t)
	base := week()
	mayBeEmpty := func(pos, i int) bool {
		if pos == 3 && i >= 2 {
			k := (i - 2) % 16
			return k >= 1 && k <= 3
		}
		return false
	}
	check := func(name string, lines []string) {
		t.Helper()
		if rep := ValidateBytes(epw(lines)); rep.Valid {
			t.Errorf("%s: file is still valid", name)
		}
	}
	for pos := 0; pos < headerLines; pos++ {
		check(fmt.Sprintf("%s row deleted", headerKeywords[pos]), removeAt(base, pos))

		f := splitCSV(base[pos])
		for k := 1; k < len(f); k++ {
			l := append([]string{}, base...)
			l[pos] = joinCSV(f[:k])
			check(fmt.Sprintf("%s cut to %d of %d fields", headerKeywords[pos], k, len(f)), l)
		}
		for i := 1; i < len(f); i++ {
			if mayBeEmpty(pos, i) {
				continue
			}
			g := append([]string{}, f...)
			g[i] = ""
			l := append([]string{}, base...)
			l[pos] = joinCSV(g)
			check(fmt.Sprintf("%s field %d (%q) blanked", headerKeywords[pos], i, f[i]), l)
		}
	}
}

func joinCSV(f []string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(f)
	w.Flush()
	return strings.TrimRight(b.String(), "\n")
}
