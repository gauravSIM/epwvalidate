package epwvalidate

import (
	"fmt"
	"math"
	"strings"
)

const headerLines = 8

var headerKeywords = [headerLines]string{
	"LOCATION",
	"DESIGN CONDITIONS",
	"TYPICAL/EXTREME PERIODS",
	"GROUND TEMPERATURES",
	"HOLIDAYS/DAYLIGHT SAVINGS",
	"COMMENTS 1",
	"COMMENTS 2",
	"DATA PERIODS",
}

// matchesKeyword reports whether s is the keyword expected on header line pos
// (0-based). "HOLIDAYS/DAYLIGHT SAVING" is accepted too (spec H-ORD-2).
func matchesKeyword(pos int, s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s == headerKeywords[pos] || (pos == 4 && s == "HOLIDAYS/DAYLIGHT SAVING")
}

func isHeaderKeyword(s string) bool { return headerPosition(s) >= 0 }

// splitCSV splits a header line, honouring double-quoted fields. Inside a
// quoted field "" is a quote; a quote that does not end the field is kept as
// text. Every language version uses this same algorithm.
func splitCSV(line string) []string {
	var fields []string
	for {
		if !strings.HasPrefix(line, `"`) {
			i := strings.IndexByte(line, ',')
			if i < 0 {
				return append(fields, line)
			}
			fields = append(fields, line[:i])
			line = line[i+1:]
			continue
		}
		var b strings.Builder
		line = line[1:]
		for {
			i := strings.IndexByte(line, '"')
			if i < 0 {
				b.WriteString(line)
				return append(fields, b.String())
			}
			b.WriteString(line[:i])
			line = line[i+1:]
			if strings.HasPrefix(line, `"`) {
				b.WriteByte('"')
				line = line[1:]
			} else if line == "" {
				return append(fields, b.String())
			} else if line[0] == ',' {
				line = line[1:]
				break
			} else {
				b.WriteByte('"')
			}
		}
		fields = append(fields, b.String())
	}
}

func trimTrailingEmpty(f []string) ([]string, bool) {
	n := len(f)
	for n > 1 && strings.TrimSpace(f[n-1]) == "" {
		n--
	}
	return f[:n], n < len(f)
}

// field returns f[i] without surrounding spaces, or "" if it does not exist.
func field(f []string, i int) string {
	if i < len(f) {
		return strings.TrimSpace(f[i])
	}
	return ""
}

// checkHeaders checks the 8 header records and returns the index of the first
// data line. A missing record is reported once and the following lines are
// matched against the records after it.
func (c *checker) checkHeaders(lines []string) int {
	i := 0 // next line to read
	for pos := 0; pos < headerLines; pos++ {
		if i >= len(lines) {
			c.errf("H-ORD-1", 0, -1, "", "", "the file ends before the %s record (it has %d lines)",
				headerKeywords[pos], len(lines))
			return i
		}
		ln := i + 1
		line := lines[i]
		first, _, _ := strings.Cut(line, ",")
		switch q := headerPosition(first); {
		case q > pos || q < 0:
			found := fmt.Sprintf("the %s record", headerKeywords[max(q, 0)])
			if q < 0 {
				found = fmt.Sprintf("%q", truncate(strings.TrimSpace(first), 40))
				if _, isNum := parseInt(first); isNum {
					found = "a data row"
				}
			}
			c.errf("H-ORD-1", ln, 0, "", strings.TrimSpace(first), "the %s record is missing: line %d should hold "+
				"it, but holds %s", headerKeywords[pos], ln, found)
			continue // check the same line against the next record
		case q < pos:
			c.errf("H-ORD-1", ln, 0, "", strings.TrimSpace(first), "the %s record is out of order or repeated; "+
				"it belongs on line %d", headerKeywords[q], q+1)
			i++
			pos-- // look for the same record on the next line
			continue
		}
		i++
		var f []string
		if pos == 0 {
			if strings.Contains(line, `"`) {
				c.errf("F-6", ln, -1, "LOCATION", "",
					"LOCATION must not contain double quotes; many EPW readers split this line on bare commas")
			}
			f = strings.Split(line, ",")
		} else {
			f = splitCSV(line)
		}
		if pos != 5 && pos != 6 {
			var trimmed bool
			if f, trimmed = trimTrailingEmpty(f); trimmed {
				c.warnf("H-TRAIL", ln, -1, headerKeywords[pos], "", "%s ends with empty fields; they are ignored",
					headerKeywords[pos])
			}
		}
		switch pos {
		case 0:
			c.checkLocation(ln, f)
		case 1:
			c.checkDesignConditions(ln, f)
		case 2:
			c.checkTypicalExtreme(ln, f)
		case 3:
			c.checkGroundTemperatures(ln, f)
		case 4:
			c.checkHolidays(ln, f)
		case 5, 6:
			if field(f, 1) == "" {
				c.errf("H-COM-1", ln, 1, headerKeywords[pos], "", "%s has no text", headerKeywords[pos])
			}
		case 7:
			c.checkDataPeriods(ln, f)
		}
	}
	return i
}

// headerPosition returns the 0-based position of a header keyword, or -1.
func headerPosition(s string) int {
	for i := range headerKeywords {
		if matchesKeyword(i, s) {
			return i
		}
	}
	return -1
}

// headerNumber checks that field i is a number inside [lo, hi].
func (c *checker) headerNumber(rule string, ln int, f []string, i int, name string, lo, hi bound) (float64, bool) {
	s := field(f, i)
	v, ok := parseNumber(s)
	if !ok {
		c.errf(rule, ln, i, name, s, "%s %q is not a number", name, s)
		return 0, false
	}
	if !inRange(v, lo, hi) {
		c.errf(rule, ln, i, name, s, "%s = %s is outside the allowed range %s", name, s, rangeText(lo, hi))
		return v, false
	}
	return v, true
}

// headerCount reads the "number of ..." field at index 1.
func (c *checker) headerCount(rule string, ln int, f []string, name string) (int, bool) {
	s := field(f, 1)
	n, ok := parseInt(s)
	if !ok || n < 0 {
		c.errf(rule, ln, 1, name, s, "%s %q is not a whole number >= 0", name, s)
		return 0, false
	}
	return n, true
}

func (c *checker) checkLocation(ln int, f []string) {
	if len(f) != 10 {
		c.errf("H-LOC-1", ln, -1, "LOCATION", "", "LOCATION has %d fields, expected 10 (keyword, city, region, "+
			"country, source, WMO, latitude, longitude, time zone, elevation)", len(f))
	}
	if field(f, 1) == "" {
		c.errf("H-LOC-2", ln, 1, "City", "", "city is empty")
	}
	for i, name := range []string{2: "State Province Region", 3: "Country", 4: "Source", 5: "WMO"} {
		if i >= 2 && field(f, i) == "" {
			c.errf("H-LOC-3", ln, i, name, "", "%s is empty", name)
		}
	}
	c.headerNumber("H-LOC-4", ln, f, 6, "Latitude", ge(-90), le(90))
	lon, lonOK := c.headerNumber("H-LOC-5", ln, f, 7, "Longitude", ge(-180), le(180))
	tz, tzOK := c.headerNumber("H-LOC-6", ln, f, 8, "Time Zone", ge(-12), le(12))
	c.headerNumber("H-LOC-7", ln, f, 9, "Elevation", ge(-1000), lt(9999.9))
	if lonOK && tzOK {
		if want := math.Round(lon / 15); math.Abs(tz-want) > 3 {
			c.warnf("H-LOC-8", ln, 8, "Time Zone", field(f, 8),
				"time zone %g h is far from the %g h expected at longitude %g; check the sign", tz, want, lon)
		}
	}
}

func (c *checker) checkDesignConditions(ln int, f []string) {
	n, ok := c.headerCount("H-DC-1", ln, f, "Number of Design Conditions")
	if !ok {
		return
	}
	if n == 0 {
		if len(f) > 2 {
			c.errf("H-DC-2", ln, 2, "DESIGN CONDITIONS", "",
				"number of design conditions is 0, but the record has more fields")
		}
		return
	}
	if field(f, 2) == "" {
		c.errf("H-DC-3", ln, 2, "Design Condition Source", "", "design condition source is empty")
		return
	}
	c.checkDesignSections(ln, n, f)
}

var designSections = [3]string{"Heating", "Cooling", "Extremes"}

// designLayouts is the number of Heating, Cooling and Extremes values for the
// ASHRAE Handbook editions in the example files (spec H-DC-5).
var designLayouts = map[string][3]int{
	"2021 ASHRAE Handbook": {16, 32, 15},
	"2025 ASHRAE Handbook": {16, 32, 27},
}

// checkDesignSections checks that the Heating, Cooling and Extremes sections
// are all there, in order, and hold only numbers (spec H-DC-4).
func (c *checker) checkDesignSections(ln, n int, f []string) {
	const rec = "DESIGN CONDITIONS"
	i := 3
	if field(f, i) == "" && strings.EqualFold(field(f, i+1), designSections[0]) {
		i++ // some writers leave one empty field after the source
	}
	var counts [3]int
	for s, name := range designSections {
		if !strings.EqualFold(field(f, i), name) {
			c.errf("H-DC-4", ln, i, rec, field(f, i), "expected the %s section at field %d, found %q",
				name, i+1, truncate(field(f, i), 20))
			return
		}
		i++
		start := i
		for i < len(f) && (s == len(designSections)-1 || !strings.EqualFold(field(f, i), designSections[s+1])) {
			if v := field(f, i); v == "" {
				c.errf("H-DC-4", ln, i, name+" value", v, "%s value at field %d is empty", name, i+1)
			} else if _, ok := parseNumber(v); !ok && !strings.EqualFold(v, "N/A") { // ASHRAE writes N/A for "not available"
				c.errf("H-DC-4", ln, i, name+" value", v, "%s value %q at field %d is not a number", name, v, i+1)
			}
			i++
		}
		counts[s] = i - start
		if counts[s] == 0 {
			c.errf("H-DC-4", ln, i, rec, "", "the %s section has no values", name)
		}
	}
	if n != 1 {
		return
	}
	for edition, want := range designLayouts {
		if strings.Contains(field(f, 2), edition) && counts != want {
			c.errf("H-DC-5", ln, -1, rec, "", "%s design conditions have %d Heating, %d Cooling and %d Extremes "+
				"values; found %d, %d and %d", edition, want[0], want[1], want[2], counts[0], counts[1], counts[2])
		}
	}
}

func (c *checker) checkTypicalExtreme(ln int, f []string) {
	n, ok := c.headerCount("H-TEP-1", ln, f, "Number of Typical/Extreme Periods")
	if !ok {
		return
	}
	if want := 2 + 4*n; len(f) != want {
		c.errf("H-TEP-2", ln, -1, "TYPICAL/EXTREME PERIODS", "",
			"%d periods declared, so %d fields are expected; found %d", n, want, len(f))
		return
	}
	for k := 0; k < n; k++ {
		b := 2 + 4*k
		if field(f, b) == "" {
			c.errf("H-TEP-5", ln, b, fmt.Sprintf("Period %d Name", k+1), "", "typical/extreme period name is empty")
		}
		if t := strings.ToLower(field(f, b+1)); t == "" {
			c.errf("H-TEP-5", ln, b+1, fmt.Sprintf("Period %d Type", k+1), "", "typical/extreme period type is empty")
		} else if t != "typical" && t != "extreme" {
			c.warnf("H-TEP-3", ln, b+1, fmt.Sprintf("Period %d Type", k+1), field(f, b+1),
				"period type %q is not Typical or Extreme", field(f, b+1))
		}
		for j, what := range []string{"Start Day", "End Day"} {
			s := field(f, b+2+j)
			if _, err := parseDate(s, dateOpts{}); err != nil {
				c.errf("H-TEP-4", ln, b+2+j, fmt.Sprintf("Period %d %s", k+1, what), s, "%v", err)
			}
		}
	}
}

func (c *checker) checkGroundTemperatures(ln int, f []string) {
	n, ok := c.headerCount("H-GT-1", ln, f, "Number of Ground Temperature Depths")
	if !ok {
		return
	}
	if want := 2 + 16*n; len(f) != want {
		c.errf("H-GT-2", ln, -1, "GROUND TEMPERATURES", "",
			"%d depths declared, so %d fields are expected; found %d", n, want, len(f))
		return
	}
	prev := math.Inf(-1)
	for k := 0; k < n; k++ {
		b := 2 + 16*k
		depth, ok := c.headerNumber("H-GT-3", ln, f, b, fmt.Sprintf("Depth %d", k+1), gt(0), bound{})
		if ok {
			if depth < prev {
				c.warnf("H-GT-6", ln, b, fmt.Sprintf("Depth %d", k+1), field(f, b), "depths are not in increasing order")
			}
			prev = depth
		}
		for j, name := range []string{"Soil Conductivity", "Soil Density", "Soil Specific Heat"} {
			if field(f, b+1+j) != "" {
				c.headerNumber("H-GT-4", ln, f, b+1+j, fmt.Sprintf("Depth %d %s", k+1, name), gt(0), bound{})
			}
		}
		for m := 0; m < 12; m++ {
			c.headerNumber("H-GT-5", ln, f, b+4+m, fmt.Sprintf("Depth %d month %d ground temperature", k+1, m+1),
				gt(-70), lt(70))
		}
	}
}

func (c *checker) checkHolidays(ln int, f []string) {
	switch strings.ToLower(field(f, 1)) {
	case "yes":
		c.leapYes = true
	case "no":
	default:
		c.errf("H-HOL-1", ln, 1, "LeapYear Observed", field(f, 1), "leap-year field %q must be Yes or No", field(f, 1))
	}
	for j, name := range []string{"Daylight Saving Start Day", "Daylight Saving End Day"} {
		s := field(f, 2+j)
		if s == "0" {
			continue
		}
		if _, err := parseDate(s, dateOpts{allowWeekday: true}); err != nil {
			c.errf("H-HOL-2", ln, 2+j, name, s, "%v (use 0 for no daylight saving)", err)
		}
	}
	n, ok := parseInt(field(f, 4))
	if !ok || n < 0 {
		c.errf("H-HOL-3", ln, 4, "Number of Holidays", field(f, 4),
			"number of holidays %q is not a whole number >= 0", field(f, 4))
		return
	}
	if want := 5 + 2*n; len(f) != want {
		c.errf("H-HOL-3", ln, -1, "HOLIDAYS/DAYLIGHT SAVINGS", "",
			"%d holidays declared, so %d fields are expected; found %d", n, want, len(f))
		return
	}
	for k := 0; k < n; k++ {
		b := 5 + 2*k
		if field(f, b) == "" {
			c.errf("H-HOL-4", ln, b, fmt.Sprintf("Holiday %d Name", k+1), "", "holiday name is empty")
		}
		if _, err := parseDate(field(f, b+1), dateOpts{allowWeekday: true}); err != nil {
			c.errf("H-HOL-4", ln, b+1, fmt.Sprintf("Holiday %d Day", k+1), field(f, b+1), "%v", err)
		}
	}
}

// period is one data period from the DATA PERIODS record.
type period struct {
	name       string
	start, end calDate
}

func (p period) label() string {
	if p.name == "" {
		return "unnamed"
	}
	return p.name
}

func (c *checker) checkDataPeriods(ln int, f []string) {
	const rec = "DATA PERIODS"
	p, ok := parseInt(field(f, 1))
	if !ok || p < 1 {
		c.errf("H-DP-1", ln, 1, "Number of Data Periods", field(f, 1),
			"number of data periods %q is not a whole number >= 1", field(f, 1))
		return
	}
	r, ok := parseInt(field(f, 2))
	if !ok || r < 1 || r > 60 || 60%r != 0 {
		c.errf("H-DP-2", ln, 2, "Number of Records per Hour", field(f, 2),
			"records per hour %q must be one of 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60", field(f, 2))
		return
	}
	c.perHour = r
	c.rep.Summary.RecordsPerHour = r
	if want := 3 + 4*p; len(f) != want {
		c.errf("H-DP-3", ln, -1, rec, "", "%d data periods declared, so %d fields are expected; found %d",
			p, want, len(f))
		return
	}

	periods := make([]period, 0, p)
	ok = true
	for k := 0; k < p; k++ {
		b := 3 + 4*k
		pr := period{name: field(f, b)}
		if pr.name == "" {
			c.errf("H-DP-4", ln, b, fmt.Sprintf("Data Period %d Name", k+1), "", "data period name is empty")
		}
		if _, wok := weekdayIndex(field(f, b+1)); !wok {
			c.errf("H-DP-5", ln, b+1, fmt.Sprintf("Data Period %d Start Day of Week", k+1), field(f, b+1),
				"start day of week %q is not a weekday name (Sunday ... Saturday)", field(f, b+1))
		}
		for j, what := range []string{"Start Day", "End Day"} {
			s := field(f, b+2+j)
			d, err := parseDate(s, dateOpts{allowYear: true})
			if err != nil {
				c.errf("H-DP-6", ln, b+2+j, fmt.Sprintf("Data Period %d %s", k+1, what), s, "%v", err)
				ok = false
				continue
			}
			if j == 0 {
				pr.start = d
			} else {
				pr.end = d
			}
		}
		periods = append(periods, pr)
	}
	if !ok {
		return
	}

	for k, pr := range periods {
		if (pr.start.y == 0) != (pr.end.y == 0) {
			c.errf("H-DP-7", ln, -1, rec, "", "data period %d: give a year on both start and end, or on neither", k+1)
			ok = false
		} else if pr.end.before(pr.start) {
			c.errf("H-DP-7", ln, -1, rec, "", "data period %d ends (%s) before it starts (%s); a period cannot "+
				"run past 31 December, so split it into two periods or use m/d/y dates", k+1, pr.end, pr.start)
			ok = false
		}
	}
	if !ok {
		return
	}
	for i := range periods {
		for j := i + 1; j < len(periods); j++ {
			a, b := periods[i], periods[j]
			if !(a.end.before(b.start) || b.end.before(a.start)) {
				c.errf("H-DP-8", ln, -1, rec, "", "data periods %d (%s-%s) and %d (%s-%s) overlap",
					i+1, a.start, a.end, j+1, b.start, b.end)
			}
		}
	}
	for k := 1; k < len(periods); k++ {
		if periods[k].start.before(periods[k-1].start) {
			c.warnf("H-DP-9", ln, -1, rec, "", "data period %d starts before data period %d; list periods in time order",
				k+1, k)
		}
	}
	c.periods = periods
	c.seq.febYear = make([]int, len(periods))
	c.rep.Summary.Periods = p
}
