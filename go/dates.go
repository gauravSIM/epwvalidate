package epwvalidate

import (
	"fmt"
	"strings"
)

var monthNames = [12]string{"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december"}

var weekdayNames = [7]string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// monthIndex returns 1-12 for a full or 3-letter month name, 0 otherwise.
func monthIndex(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	for i, n := range monthNames {
		if s == n || s == n[:3] {
			return i + 1
		}
	}
	return 0
}

// weekdayIndex returns 0 (Sunday) to 6 for a full or 3-letter weekday name.
func weekdayIndex(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for i, n := range weekdayNames {
		if s == n || s == n[:3] {
			return i, true
		}
	}
	return 0, false
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func daysIn(month int, feb29 bool) int {
	switch month {
	case 2:
		if feb29 {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// calDate is a month/day, with a year only when the date had one (y = 0 otherwise).
type calDate struct{ y, m, d int }

func (a calDate) before(b calDate) bool {
	if a.y != 0 && b.y != 0 && a.y != b.y {
		return a.y < b.y
	}
	if a.m != b.m {
		return a.m < b.m
	}
	return a.d < b.d
}

func (a calDate) String() string {
	if a.y != 0 {
		return fmt.Sprintf("%d/%d/%d", a.m, a.d, a.y)
	}
	return fmt.Sprintf("%d/%d", a.m, a.d)
}

type dateOpts struct {
	allowYear    bool // m/d/y (DATA PERIODS only)
	allowWeekday bool // "2 Sunday in March", "Last Sunday in October" (holidays and daylight saving only)
}

// parseDate checks one header date field (SPEC.md §5.9). It returns the date as
// month/day (plus year for m/d/y). Weekday forms return a zero calDate.
func parseDate(s string, o dateOpts) (calDate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return calDate{}, fmt.Errorf("date is empty")
	}

	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) == 3 && !o.allowYear {
			return calDate{}, fmt.Errorf("date %q has a year; m/d/y is allowed only in DATA PERIODS", s)
		}
		if len(parts) != 2 && len(parts) != 3 {
			return calDate{}, fmt.Errorf("date %q is not in m/d form", s)
		}
		var n [3]int
		for i, p := range parts {
			v, ok := parseInt(strings.ReplaceAll(p, " ", ""))
			if !ok {
				return calDate{}, fmt.Errorf("date %q is not in m/d form", s)
			}
			n[i] = v
		}
		d := calDate{m: n[0], d: n[1]}
		if len(parts) == 3 {
			if n[2] < 1 {
				return calDate{}, fmt.Errorf("date %q has an invalid year", s)
			}
			d.y = n[2]
		}
		return d, checkMonthDay(s, d)
	}

	if j, ok := parseInt(s); ok {
		if j < 1 || j > 366 {
			return calDate{}, fmt.Errorf("day of year %d is outside 1-366", j)
		}
		return julianToDate(j), nil
	}

	tok := strings.Fields(s)
	if len(tok) == 2 {
		if m := monthIndex(tok[0]); m > 0 {
			if d, ok := parseInt(tok[1]); ok {
				cd := calDate{m: m, d: d}
				return cd, checkMonthDay(s, cd)
			}
		}
		if m := monthIndex(tok[1]); m > 0 {
			if d, ok := parseInt(tok[0]); ok {
				cd := calDate{m: m, d: d}
				return cd, checkMonthDay(s, cd)
			}
		}
	}
	if len(tok) == 4 && strings.EqualFold(tok[2], "in") {
		if !o.allowWeekday {
			return calDate{}, fmt.Errorf("weekday date %q is allowed only for holidays and daylight saving", s)
		}
		_, wok := weekdayIndex(tok[1])
		n, nok := parseInt(tok[0])
		if wok && monthIndex(tok[3]) > 0 && (strings.EqualFold(tok[0], "last") || nok && n >= 1 && n <= 5) {
			return calDate{}, nil
		}
	}
	return calDate{}, fmt.Errorf("%q is not a recognised EPW date (use m/d, e.g. 1/31)", s)
}

func checkMonthDay(s string, d calDate) error {
	if d.m < 1 || d.m > 12 {
		return fmt.Errorf("date %q: month %d is outside 1-12", s, d.m)
	}
	feb29 := d.y == 0 || isLeap(d.y)
	if d.d < 1 || d.d > daysIn(d.m, feb29) {
		return fmt.Errorf("date %q: day %d does not exist in month %d", s, d.d, d.m)
	}
	return nil
}

// julianToDate converts a day of year (non-leap calendar) to month/day.
// Day 366 is taken as 31 December.
func julianToDate(j int) calDate {
	if j == 366 {
		return calDate{m: 12, d: 31}
	}
	m := 1
	for j > daysIn(m, false) {
		j -= daysIn(m, false)
		m++
	}
	return calDate{m: m, d: j}
}
