package epwvalidate

// Test fixtures: partial-year files that must be valid and broken files that
// must be invalid. They are built from two real files in EPW_format/epw_examples
// and written to testdata/partial and testdata/broken by:
//
//	go test -run TestFixtures -update
//
// Broken files are named <RULE>__<description>.epw, where RULE is the error
// the file must produce.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/partial and testdata/broken")

const (
	osloFile   = "EPW_format/epw_examples/NOR_AK_Oslo.AP-Gardermoen.013840_TMYx.2007-2021.epw"
	hurdalFile = "EPW_format/epw_examples/NOR_AK_Hurdal.NOR076_NORCERRA.epw"
)

var (
	baseOnce  sync.Once
	osloBase  []string
	hurdalBas []string
)

func loadBases(t *testing.T) {
	t.Helper()
	baseOnce.Do(func() {
		osloBase = readLines(osloFile)
		hurdalBas = readLines(hurdalFile)
	})
	if osloBase == nil || hurdalBas == nil {
		t.Fatal("could not read the base files in EPW_format/epw_examples/")
	}
}

func readLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\r\n"), "\r\n")
}

// epw joins lines into a CRLF file, like the real EPW files.
func epw(lines []string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

func fields(line string) []string { return strings.Split(line, ",") }

func setField(line string, i int, v string) string {
	f := fields(line)
	f[i] = v
	return strings.Join(f, ",")
}

func intField(line string, i int) int {
	v, _ := strconv.Atoi(fields(line)[i])
	return v
}

// build makes a file from base's first 7 header lines, a DATA PERIODS line and
// the base rows that keep accepts, in file order.
func build(base []string, dataPeriods string, keep func(m, d int) bool) []string {
	out := append([]string{}, base[:7]...)
	out = append(out, dataPeriods)
	for _, row := range base[headerLines:] {
		if keep(intField(row, 1), intField(row, 2)) {
			out = append(out, row)
		}
	}
	return out
}

// setYear sets the year of every data row in month m (all months if m = 0).
func setYear(lines []string, m, year int) {
	for i := headerLines; i < len(lines); i++ {
		if m == 0 || intField(lines[i], 1) == m {
			lines[i] = setField(lines[i], 0, strconv.Itoa(year))
		}
	}
}

func setLeapYear(lines []string, yes bool) {
	v := "No"
	if yes {
		v = "Yes"
	}
	lines[4] = setField(lines[4], 1, v)
}

// addFeb29 inserts 24 rows for 29 February (copies of 28 February) after the
// last 28 February row.
func addFeb29(lines []string) []string {
	last := -1
	var extra []string
	for i := headerLines; i < len(lines); i++ {
		if intField(lines[i], 1) == 2 && intField(lines[i], 2) == 28 {
			last = i
			extra = append(extra, setField(lines[i], 2, "29"))
		}
	}
	out := append([]string{}, lines[:last+1]...)
	out = append(out, extra...)
	return append(out, lines[last+1:]...)
}

func insertAt(lines []string, i int, extra ...string) []string {
	out := append([]string{}, lines[:i]...)
	out = append(out, extra...)
	return append(out, lines[i:]...)
}

func removeAt(lines []string, i int) []string {
	out := append([]string{}, lines[:i]...)
	return append(out, lines[i+1:]...)
}

const dpWeek = "DATA PERIODS,1,1,Week,Monday,1/ 8,1/14"

func jan8to14(m, d int) bool { return m == 1 && d >= 8 && d <= 14 }

// week is the base for most broken files: 8 headers + 168 rows.
func week() []string { return build(osloBase, dpWeek, jan8to14) }

// row returns the index in a file's lines of data row k (0-based).
func row(k int) int { return headerLines + k }

type fixture struct {
	dir  string // "partial" (must be valid) or "broken" (must be invalid)
	name string
	rule string // broken: error rule the file must produce
	rows int    // partial: expected data rows
	make func() []byte
}

func (f fixture) fileName() string {
	if f.dir == "broken" {
		return f.rule + "__" + f.name + ".epw"
	}
	return f.name + ".epw"
}

func partial(name string, rows int, mk func() []string) fixture {
	return fixture{dir: "partial", name: name, rows: rows, make: func() []byte { return epw(mk()) }}
}

func broken(rule, name string, mk func() []string) fixture {
	return fixture{dir: "broken", name: name, rule: rule, make: func() []byte { return epw(mk()) }}
}

// mutWeek is a broken file made by changing the one-week file.
func mutWeek(rule, name string, change func(l []string) []string) fixture {
	return broken(rule, name, func() []string { return change(week()) })
}

func mutRow(rule, name string, k, col int, v string) fixture {
	return mutWeek(rule, name, func(l []string) []string {
		l[row(k)] = setField(l[row(k)], col, v)
		return l
	})
}

func mutHeader(rule, name string, i, col int, v string) fixture {
	return mutWeek(rule, name, func(l []string) []string {
		l[i] = setField(l[i], col, v)
		return l
	})
}

func setHeader(rule, name string, i int, line string) fixture {
	return mutWeek(rule, name, func(l []string) []string {
		l[i] = line
		return l
	})
}

func fixtures() []fixture {
	febToMar5 := func(m, d int) bool { return m == 2 || m == 3 && d <= 5 }
	return []fixture{
		// ---- Partial files: all valid ----
		partial("one_day_jul15", 24, func() []string {
			return build(osloBase, "DATA PERIODS,1,1,Day,Monday,7/15,7/15", func(m, d int) bool { return m == 7 && d == 15 })
		}),
		partial("one_week_jan8-14", 168, week),
		partial("one_month_january", 744, func() []string {
			return build(osloBase, "DATA PERIODS,1,1,January,Sunday,1/ 1,1/31", func(m, d int) bool { return m == 1 })
		}),
		partial("three_months_jan-mar", 2160, func() []string {
			return build(osloBase, "DATA PERIODS,1,1,Q1,Sunday,1/ 1,3/31", func(m, d int) bool { return m <= 3 })
		}),
		partial("two_periods_jan2019_jul2021", 1488, func() []string {
			l := build(osloBase, "DATA PERIODS,2,1,Jan,Sunday,1/ 1,1/31,Jul,Friday,7/ 1,7/31",
				func(m, d int) bool { return m == 1 || m == 7 })
			setYear(l, 1, 2019)
			setYear(l, 7, 2021)
			return l
		}),
		partial("four_representative_months_2015-2018", 2952, func() []string {
			l := build(osloBase, "DATA PERIODS,4,1,Jan,Sunday,1/ 1,1/31,Apr,Wednesday,4/ 1,4/30,"+
				"Jul,Friday,7/ 1,7/31,Oct,Monday,10/ 1,10/31",
				func(m, d int) bool { return m == 1 || m == 4 || m == 7 || m == 10 })
			for i, m := range []int{1, 4, 7, 10} {
				setYear(l, m, 2015+i)
			}
			return l
		}),
		partial("leap_year_feb-mar5_2020", 816, func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Leap,Saturday,2/ 1,3/ 5", febToMar5)
			setLeapYear(l, true)
			setYear(l, 0, 2020)
			return addFeb29(l)
		}),
		partial("leap_flag_yes_but_non_leap_year_2019", 816-24, func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,NoLeap,Friday,2/ 1,3/ 5", febToMar5)
			setLeapYear(l, true)
			setYear(l, 0, 2019)
			return l
		}),
		partial("subhourly_15min_one_day", 96, func() []string {
			day := build(osloBase, "DATA PERIODS,1,4,Day,Monday,7/15,7/15", func(m, d int) bool { return m == 7 && d == 15 })
			out := append([]string{}, day[:headerLines]...)
			for _, r := range day[headerLines:] {
				for _, minute := range []string{"15", "30", "45", "60"} {
					out = append(out, setField(r, 4, minute))
				}
			}
			return out
		}),
		partial("mdy_dates_one_day_2019", 24, func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Day,Monday,7/15/2019,7/15/2019",
				func(m, d int) bool { return m == 7 && d == 15 })
			setYear(l, 0, 2019)
			return l
		}),
		partial("mdy_dates_across_new_year", 14*24, func() []string {
			l := append([]string{}, osloBase[:7]...)
			l = append(l, "DATA PERIODS,1,1,Winter,Tuesday,12/25/2018,1/ 7/2019")
			for _, r := range osloBase[headerLines:] {
				if intField(r, 1) == 12 && intField(r, 2) >= 25 {
					l = append(l, setField(r, 0, "2018"))
				}
			}
			for _, r := range osloBase[headerLines:] {
				if intField(r, 1) == 1 && intField(r, 2) <= 7 {
					l = append(l, setField(r, 0, "2019"))
				}
			}
			return l
		}),
		partial("julian_dates_january", 744, func() []string {
			return build(osloBase, "DATA PERIODS,1,1,January,Sunday,1,31", func(m, d int) bool { return m == 1 })
		}),
		partial("named_month_dates_january", 744, func() []string {
			return build(osloBase, "DATA PERIODS,1,1,January,Sun,Jan 1,31 January", func(m, d int) bool { return m == 1 })
		}),
		partial("norcerra_one_week", 168, func() []string {
			return build(hurdalBas, dpWeek, jan8to14)
		}),
		partial("holidays_daylight_saving_without_s", 168, func() []string {
			l := week()
			l[4] = strings.Replace(l[4], "SAVINGS", "SAVING", 1)
			return l
		}),
		partial("daylight_saving_and_holidays", 168, func() []string {
			l := week()
			l[4] = "HOLIDAYS/DAYLIGHT SAVINGS,No,Last Sunday in March,Last Sunday in October,2,New Year,1/1,Constitution Day,May 17"
			return l
		}),
		{dir: "partial", name: "lf_line_endings_one_week", rows: 168, make: func() []byte {
			return []byte(strings.Join(week(), "\n") + "\n")
		}},
		{dir: "partial", name: "trailing_blank_lines_one_week", rows: 168, make: func() []byte {
			return append(epw(week()), "\r\n\r\n"...)
		}},

		// ---- Broken files: all invalid with the named rule ----
		// File level
		{dir: "broken", rule: "F-1", name: "empty_file", make: func() []byte { return nil }},
		{dir: "broken", rule: "F-1", name: "latin1_city_name", make: func() []byte {
			b := epw(week())
			return bytes.Replace(b, []byte("Oslo.AP-Gardermoen"), []byte("\xc5lesund"), 1)
		}},
		{dir: "broken", rule: "F-2", name: "utf8_bom", make: func() []byte {
			return append([]byte{0xEF, 0xBB, 0xBF}, epw(week())...)
		}},
		mutWeek("F-4", "blank_line_in_data", func(l []string) []string { return insertAt(l, row(10), "") }),
		mutWeek("F-5", "semicolon_line_end", func(l []string) []string { l[row(5)] += ";"; return l }),
		mutHeader("F-6", "quoted_city_with_comma", 0, 1, `"Oslo, Gardermoen"`),
		mutRow("F-6", "quote_in_data_row", 3, 5, `"flags"`),

		// Header order
		mutWeek("H-ORD-1", "headers_3_and_4_swapped", func(l []string) []string { l[2], l[3] = l[3], l[2]; return l }),
		mutWeek("H-ORD-1", "comments_2_missing", func(l []string) []string { return removeAt(l, 6) }),
		mutWeek("H-ORD-1", "only_header_lines", func(l []string) []string { return l[:5] }),
		mutWeek("H-ORD-3", "header_record_inside_data", func(l []string) []string {
			return insertAt(l, row(10), "COMMENTS 1,stray line")
		}),

		// LOCATION
		mutWeek("H-LOC-1", "location_9_fields", func(l []string) []string {
			l[0] = strings.Join(fields(l[0])[:9], ",")
			return l
		}),
		mutHeader("H-LOC-2", "empty_city", 0, 1, ""),
		mutHeader("H-LOC-3", "empty_country", 0, 3, ""),
		mutHeader("H-LOC-3", "empty_wmo", 0, 5, ""),
		mutHeader("H-LOC-4", "latitude_95", 0, 6, "95"),
		mutHeader("H-LOC-4", "latitude_not_a_number", 0, 6, "60N"),
		mutHeader("H-LOC-5", "longitude_200", 0, 7, "200"),
		mutHeader("H-LOC-6", "time_zone_14", 0, 8, "14"),
		mutHeader("H-LOC-7", "elevation_10000", 0, 9, "10000"),

		// Other headers
		mutHeader("H-DC-1", "design_conditions_count_not_number", 1, 1, "x"),
		setHeader("H-DC-2", "design_conditions_0_with_data", 1, "DESIGN CONDITIONS,0,Some source"),
		mutWeek("H-TEP-2", "typical_periods_count_mismatch", func(l []string) []string {
			l[2] = setField(l[2], 1, "7")
			return l
		}),
		mutHeader("H-TEP-4", "typical_period_bad_date", 2, 4, "13/40"),
		mutWeek("H-GT-2", "ground_temps_3_declared_2_given", func(l []string) []string {
			l[3] = strings.Join(fields(l[3])[:2+16*2], ",")
			return l
		}),
		mutHeader("H-GT-3", "ground_depth_negative", 3, 2, "-0.5"),
		mutHeader("H-GT-5", "ground_temp_80", 3, 6, "80"),
		mutHeader("H-HOL-1", "leap_year_maybe", 4, 1, "Maybe"),
		mutHeader("H-HOL-2", "daylight_saving_bad_date", 4, 2, "2/31"),
		setHeader("H-HOL-3", "holidays_count_mismatch", 4, "HOLIDAYS/DAYLIGHT SAVINGS,No,0,0,1"),

		// DATA PERIODS
		setHeader("H-DP-1", "zero_data_periods", 7, "DATA PERIODS,0,1"),
		setHeader("H-DP-2", "seven_records_per_hour", 7, "DATA PERIODS,1,7,Week,Monday,1/ 8,1/14"),
		setHeader("H-DP-3", "data_periods_field_count", 7, "DATA PERIODS,2,1,Week,Monday,1/ 8,1/14"),
		setHeader("H-DP-5", "bad_weekday", 7, "DATA PERIODS,1,1,Week,Funday,1/ 8,1/14"),
		setHeader("H-DP-6", "data_period_feb30", 7, "DATA PERIODS,1,1,Week,Monday,2/30,3/ 6"),
		setHeader("H-DP-6", "data_period_weekday_date", 7, "DATA PERIODS,1,1,Week,Monday,2 Monday in January,1/14"),
		setHeader("H-DP-7", "period_end_before_start", 7, "DATA PERIODS,1,1,Week,Monday,1/14,1/ 8"),
		setHeader("H-DP-7", "period_wraps_year_end", 7, "DATA PERIODS,1,1,Winter,Monday,12/25,1/ 7"),
		setHeader("H-DP-7", "year_on_start_only", 7, "DATA PERIODS,1,1,Week,Monday,1/ 8/2019,1/14"),
		setHeader("H-DP-8", "overlapping_periods", 7, "DATA PERIODS,2,1,A,Monday,1/ 8,1/10,B,Wednesday,1/10,1/14"),

		// Row structure
		mutWeek("D-ROW-1", "row_with_34_fields", func(l []string) []string {
			l[row(20)] = strings.Join(fields(l[row(20)])[:34], ",")
			return l
		}),
		mutWeek("D-ROW-1", "row_with_36_fields", func(l []string) []string { l[row(20)] += ",0"; return l }),
		mutRow("D-ROW-2", "hour_not_integer", 30, 3, "7.5"),
		mutRow("D-ROW-3", "dry_bulb_text", 40, 6, "abc"),
		mutRow("D-ROW-3", "pressure_empty", 40, 9, ""),
		mutRow("D-ROW-3", "wind_speed_nan", 40, 21, "NaN"),
		mutRow("D-ROW-4", "weather_codes_8_digits", 50, 27, "92999999"),
		mutRow("D-ROW-4", "weather_codes_letters", 50, 27, "9299999x9"),
		mutRow("D-ROW-5", "weather_observation_5", 50, 26, "5"),
		mutRow("D-ROW-7", "empty_flags", 60, 5, ""),

		// Value ranges
		mutRow("D-RNG-1", "dry_bulb_75", 70, 6, "75.0"),
		mutRow("D-RNG-1", "dry_bulb_minus_70", 70, 6, "-70"),
		mutRow("D-RNG-1", "dew_point_80", 70, 7, "80"),
		mutRow("D-RNG-1", "relative_humidity_120", 70, 8, "120"),
		mutRow("D-RNG-1", "pressure_30000", 70, 9, "30000"),
		mutRow("D-RNG-1", "global_horizontal_negative", 70, 13, "-5"),
		mutRow("D-RNG-1", "direct_normal_negative", 70, 14, "-1"),
		mutRow("D-RNG-1", "illuminance_negative", 70, 16, "-100"),
		mutRow("D-RNG-1", "wind_direction_400", 70, 20, "400"),
		mutRow("D-RNG-1", "wind_speed_45", 70, 21, "45"),
		mutRow("D-RNG-1", "total_sky_cover_11", 70, 22, "11"),
		mutRow("D-RNG-1", "opaque_sky_cover_negative", 70, 23, "-1"),

		// Time sequence
		mutRow("D-SEQ-1", "hour_25", 80, 3, "25"),
		mutRow("D-SEQ-1", "hour_0", 80, 3, "0"),
		mutRow("D-SEQ-1", "month_13", 80, 1, "13"),
		mutRow("D-SEQ-1", "january_32", 80, 2, "32"),
		mutRow("D-SEQ-2", "minute_61", 80, 4, "61"),
		mutWeek("D-SEQ-2", "subhourly_wrong_minutes", func(l []string) []string {
			out := append([]string{}, l[:7]...)
			out = append(out, "DATA PERIODS,1,2,Day,Monday,1/ 8,1/ 8")
			for _, r := range l[row(0):row(24)] {
				out = append(out, setField(r, 4, "0"), setField(r, 4, "30"))
			}
			return out
		}),
		mutWeek("D-SEQ-3", "one_row_deleted", func(l []string) []string { return removeAt(l, row(100)) }),
		mutWeek("D-SEQ-3", "one_row_duplicated", func(l []string) []string {
			return insertAt(l, row(100), l[row(100)])
		}),
		mutWeek("D-SEQ-3", "two_rows_swapped", func(l []string) []string {
			l[row(100)], l[row(101)] = l[row(101)], l[row(100)]
			return l
		}),
		mutWeek("D-SEQ-3", "starts_on_wrong_day", func(l []string) []string { return removeAtRange(l, row(0), row(24)) }),
		broken("D-SEQ-3", "second_period_starts_wrong_day", func() []string {
			return build(osloBase, "DATA PERIODS,2,1,Jan,Sunday,1/ 1,1/31,Jul,Friday,7/ 2,7/31",
				func(m, d int) bool { return m == 1 || m == 7 && d <= 30 })
		}),
		mutWeek("D-SEQ-4", "year_changes_mid_month", func(l []string) []string {
			for i := row(50); i < len(l); i++ {
				l[i] = setField(l[i], 0, "1999")
			}
			return l
		}),
		broken("D-SEQ-4", "mdy_period_wrong_year", func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Day,Monday,7/15/2019,7/16/2019",
				func(m, d int) bool { return m == 7 && (d == 15 || d == 16) })
			setYear(l, 0, 2019)
			for i := row(24); i < len(l); i++ {
				l[i] = setField(l[i], 0, "2020")
			}
			return l
		}),
		broken("D-SEQ-5", "feb29_rows_with_leap_no", func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Feb,Saturday,2/ 1,3/ 5", febToMar5)
			setYear(l, 0, 2020)
			return addFeb29(l)
		}),
		broken("D-SEQ-5", "feb29_missing_with_leap_yes", func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Feb,Saturday,2/ 1,3/ 5", febToMar5)
			setLeapYear(l, true)
			setYear(l, 0, 2020)
			return l
		}),
		broken("D-SEQ-5", "feb29_rows_in_non_leap_year", func() []string {
			l := build(osloBase, "DATA PERIODS,1,1,Feb,Friday,2/ 1,3/ 5", febToMar5)
			setLeapYear(l, true)
			setYear(l, 0, 2019)
			return addFeb29(l)
		}),
		// Missing header rows and missing parts of header rows
		mutWeek("H-ORD-1", "location_row_missing", func(l []string) []string { return removeAt(l, 0) }),
		mutWeek("H-ORD-1", "design_conditions_row_missing", func(l []string) []string { return removeAt(l, 1) }),
		mutWeek("H-ORD-1", "ground_temperatures_row_missing", func(l []string) []string { return removeAt(l, 3) }),
		mutWeek("H-ORD-1", "data_periods_row_missing", func(l []string) []string { return removeAt(l, 7) }),
		mutWeek("H-DC-4", "design_conditions_cut_short", func(l []string) []string {
			l[1] = strings.Join(fields(l[1])[:40], ",")
			return l
		}),
		mutHeader("H-DC-4", "design_conditions_value_empty", 1, 30, ""),
		mutHeader("H-DC-4", "design_conditions_cooling_label_missing", 1, 20, ""),
		mutWeek("H-DC-5", "design_conditions_extremes_short", func(l []string) []string {
			l[1] = strings.Join(fields(l[1])[:len(fields(l[1]))-3], ",")
			return l
		}),
		mutHeader("H-TEP-5", "typical_period_name_empty", 2, 2, ""),
		mutHeader("H-TEP-5", "typical_period_type_empty", 2, 3, ""),
		mutHeader("H-GT-2", "ground_temps_last_month_empty", 3, 49, ""),
		mutWeek("H-HOL-3", "holidays_cut_short", func(l []string) []string {
			l[4] = "HOLIDAYS/DAYLIGHT SAVINGS,No,0,0"
			return l
		}),
		setHeader("H-COM-1", "comments_1_empty", 5, "COMMENTS 1,"),
		setHeader("H-COM-1", "comments_2_keyword_only", 6, "COMMENTS 2"),
		mutHeader("H-DP-4", "data_period_name_empty", 7, 3, ""),
		setHeader("H-DP-3", "data_periods_end_date_missing", 7, "DATA PERIODS,1,1,Week,Monday,1/ 8"),

		setHeader("D-SEQ-6", "declares_month_has_week", 7, "DATA PERIODS,1,1,January,Sunday,1/ 1,1/31"),
		mutWeek("D-SEQ-6", "last_day_missing", func(l []string) []string { return l[:len(l)-24] }),
		broken("D-SEQ-6", "extra_day_after_period", func() []string {
			l := week()
			for _, r := range osloBase[headerLines:] {
				if intField(r, 1) == 1 && intField(r, 2) == 15 {
					l = append(l, r)
				}
			}
			return l
		}),
		mutWeek("D-SEQ-6", "no_data_rows", func(l []string) []string { return l[:headerLines] }),

		// Missing data
		mutWeek("D-MIS-1", "dry_bulb_all_missing", func(l []string) []string { return setColumn(l, 6, "99.9") }),
		mutWeek("D-MIS-1", "wind_speed_all_missing", func(l []string) []string { return setColumn(l, 21, "999") }),
		mutWeek("D-MIS-1", "global_horizontal_all_missing", func(l []string) []string { return setColumn(l, 13, "9999") }),
	}
}

func removeAtRange(lines []string, from, to int) []string {
	out := append([]string{}, lines[:from]...)
	return append(out, lines[to:]...)
}

func setColumn(lines []string, col int, v string) []string {
	for i := headerLines; i < len(lines); i++ {
		lines[i] = setField(lines[i], col, v)
	}
	return lines
}

// TestFixtures builds every fixture in memory, checks the expected result,
// and checks that the copy on disk is up to date (or rewrites it with -update).
func TestFixtures(t *testing.T) {
	loadBases(t)
	seen := map[string]bool{}
	for _, fx := range fixtures() {
		fx := fx
		if seen[fx.fileName()] {
			t.Fatalf("duplicate fixture name %s", fx.fileName())
		}
		seen[fx.fileName()] = true
		t.Run(fx.dir+"/"+fx.fileName(), func(t *testing.T) {
			data := fx.make()
			path := filepath.Join("testdata", fx.dir, fx.fileName())
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			} else if disk, err := os.ReadFile(path); err != nil || !bytes.Equal(disk, data) {
				t.Errorf("%s is missing or out of date; run: go test -run TestFixtures -update", path)
			}
			checkFixture(t, fx, ValidateBytes(data))
		})
	}
}

func checkFixture(t *testing.T, fx fixture, rep Report) {
	t.Helper()
	switch fx.dir {
	case "partial":
		if !rep.Valid {
			t.Fatalf("expected valid, got:\n%s", describe(rep))
		}
		if fx.rows != 0 && (rep.Summary.DataRows != fx.rows || rep.Summary.ExpectedRows != fx.rows) {
			t.Errorf("rows: got %d data rows, %d expected by DATA PERIODS; want %d",
				rep.Summary.DataRows, rep.Summary.ExpectedRows, fx.rows)
		}
	case "broken":
		if rep.Valid {
			t.Fatalf("expected invalid with %s, but the file is valid", fx.rule)
		}
		if !hasError(rep, fx.rule) {
			t.Errorf("expected error %s, got:\n%s", fx.rule, describe(rep))
		}
	}
}

func hasError(rep Report, rule string) bool {
	for _, is := range rep.Errors {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

func describe(rep Report) string {
	var b strings.Builder
	for i, is := range append(rep.Errors, rep.Warnings...) {
		if i == 15 {
			fmt.Fprintf(&b, "  ...\n")
			break
		}
		fmt.Fprintf(&b, "  %-7s %s\n", is.Severity, is)
	}
	return b.String()
}
