package epwvalidate

// Tests for the simien profile (SPEC.md §11.1). They are deliberately not part
// of the conformance corpus in testdata/expected.json: that file records the
// default report for every test file, and the profile must not change it.

import (
	"strings"
	"testing"
)

var simienOpt = Options{Profile: ProfileSimien}

// rules returns the rule IDs reported, in the order they appear.
func rules(rep Report) []string {
	var out []string
	for _, is := range rep.Errors {
		out = append(out, is.Rule)
	}
	for _, is := range rep.Warnings {
		out = append(out, is.Rule)
	}
	return out
}

func hasRule(rep Report, rule string) bool {
	for _, r := range rules(rep) {
		if r == rule {
			return true
		}
	}
	return false
}

func issueFor(t *testing.T, rep Report, rule string) Issue {
	t.Helper()
	for _, is := range append(append([]Issue{}, rep.Errors...), rep.Warnings...) {
		if is.Rule == rule {
			return is
		}
	}
	t.Fatalf("no %s issue; got %v", rule, rules(rep))
	return Issue{}
}

func TestSimienFullYearFileIsUsable(t *testing.T) {
	loadBases(t)

	rep := ValidateBytesWith(epw(osloBase), simienOpt)

	if !rep.Valid {
		t.Fatalf("a real full-year TMYx file should be usable as a SIMIEN climate file; got %v", rep.Errors)
	}
	for _, r := range rules(rep) {
		if strings.HasPrefix(r, "S-") {
			t.Errorf("unexpected profile finding %s: %s", r, FormatText("oslo", rep))
		}
	}
}

func TestSimienProfileDoesNotChangeTheDefaultReport(t *testing.T) {
	// The whole point of a profile: asking a second question must not change
	// the answer to the first. Python, Node and .NET still conform without it.
	loadBases(t)

	for _, name := range []string{"oslo", "hurdal"} {
		base := osloBase
		if name == "hurdal" {
			base = hurdalBas
		}
		plain := ValidateBytes(epw(base))
		explicit := ValidateBytesWith(epw(base), Options{Profile: ProfileNone})

		if FormatText(name, plain) != FormatText(name, explicit) {
			t.Errorf("%s: an explicit ProfileNone differs from the default", name)
		}
	}
}

func TestSimienMissingValueIsAnError(t *testing.T) {
	// 9999 is EPW's "no reading". The SIMIEN CSV has no missing code, so that
	// hour would reach the core as 9999 W/m2 in a column whose range is 0-1400.
	loadBases(t)
	lines := append([]string{}, osloBase...)
	lines[headerLines+100] = setField(lines[headerLines+100], 13, "9999")

	rep := ValidateBytesWith(epw(lines), simienOpt)

	if rep.Valid {
		t.Fatal("a missing global horizontal value should make the file unusable")
	}
	is := issueFor(t, rep, "S-COL-1")
	if is.Field != "Global Horizontal Radiation" {
		t.Errorf("field = %q, want the column name", is.Field)
	}
	if is.Line != headerLines+101 {
		t.Errorf("line = %d, want %d (the first missing hour)", is.Line, headerLines+101)
	}
	if !strings.Contains(is.Message, "missing on 1 of 8760 rows") {
		t.Errorf("message does not report the count: %q", is.Message)
	}
	// S-COL-1 replaces D-MIS-2 for this column, so it is reported once.
	if hasRule(rep, "D-MIS-2") {
		t.Error("D-MIS-2 reported as well as S-COL-1")
	}
}

func TestSimienMissingValueStaysAWarningWithoutTheProfile(t *testing.T) {
	loadBases(t)
	lines := append([]string{}, osloBase...)
	lines[headerLines+100] = setField(lines[headerLines+100], 13, "9999")

	rep := ValidateBytes(epw(lines))

	if !rep.Valid {
		t.Error("a missing value is allowed by the EPW rules")
	}
	if !hasRule(rep, "D-MIS-2") {
		t.Errorf("want D-MIS-2 without the profile; got %v", rules(rep))
	}
}

func TestSimienIgnoresMissingStationPressure(t *testing.T) {
	// Pressure is core for EnergyPlus (D-MIS-1) but SIMIEN never reads it, so
	// a file without it is still usable. This asymmetry is why the profile is
	// a separate column set rather than a reinterpretation of "core".
	loadBases(t)
	lines := append([]string{}, osloBase...)
	for i := headerLines; i < len(lines); i++ {
		lines[i] = setField(lines[i], 9, "999999")
	}

	rep := ValidateBytesWith(epw(lines), simienOpt)

	if hasRule(rep, "S-COL-1") {
		t.Error("missing station pressure should not be a profile error")
	}
	if !hasRule(rep, "D-MIS-1") {
		t.Error("the EPW rule for a missing core column should still fire")
	}
}

func TestSimienMissingAlbedoIsFilled(t *testing.T) {
	loadBases(t)
	lines := append([]string{}, osloBase...)
	for i := headerLines; i < len(lines); i++ {
		lines[i] = setField(lines[i], 32, "999")
	}

	rep := ValidateBytesWith(epw(lines), simienOpt)

	if !rep.Valid {
		t.Fatalf("a missing albedo has a default and should not refuse the file; got %v", rep.Errors)
	}
	is := issueFor(t, rep, "S-ALB-1")
	if !strings.Contains(is.Message, "0.20") {
		t.Errorf("message does not name the default: %q", is.Message)
	}
}

func TestSimienPartialYearIsNotUsable(t *testing.T) {
	// A week is a correct EPW (SPEC §0) and cannot make a SIMIEN climate file.
	loadBases(t)

	rep := ValidateBytesWith(epw(week()), simienOpt)

	if rep.Valid {
		t.Fatal("a one-week file should not be usable as a SIMIEN climate file")
	}
	if !hasRule(rep, "S-ROWS-1") {
		t.Errorf("want S-ROWS-1; got %v", rules(rep))
	}
	if plain := ValidateBytes(epw(week())); !plain.Valid {
		t.Error("the same file must stay a valid EPW without the profile")
	}
}

func TestSimienLeapDayIsNotUsable(t *testing.T) {
	loadBases(t)
	lines := append([]string{}, osloBase...)
	setYear(lines, 0, 2020)
	setLeapYear(lines, true)
	lines = addFeb29(lines)

	rep := ValidateBytesWith(epw(lines), simienOpt)

	if !hasRule(rep, "S-LEAP-1") {
		t.Fatalf("want S-LEAP-1; got %v", rules(rep))
	}
	is := issueFor(t, rep, "S-LEAP-1")
	if !strings.Contains(is.Message, "24 rows for 29 February") {
		t.Errorf("message does not report the count: %q", is.Message)
	}
}

func TestSimienSubHourlyIsNotUsable(t *testing.T) {
	rep, err := ValidateFileWith("../testdata/partial/subhourly_15min_one_day.epw", simienOpt)
	if err != nil {
		t.Fatal(err)
	}

	if !hasRule(rep, "S-RATE-1") {
		t.Fatalf("want S-RATE-1; got %v", rules(rep))
	}
	if plain, _ := ValidateFile("../testdata/partial/subhourly_15min_one_day.epw"); !plain.Valid {
		t.Error("15-minute data is a valid EPW without the profile")
	}
}

func TestParseProfile(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Profile
		ok   bool
	}{
		{"", ProfileNone, true},
		{"simien", ProfileSimien, true},
		{"SIMIEN", ProfileSimien, true},
		{"  simien  ", ProfileSimien, true},
		{"energyplus", ProfileNone, false},
	} {
		got, ok := ParseProfile(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseProfile(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
