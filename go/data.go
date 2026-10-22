package epwvalidate

import (
	"fmt"
	"strings"
)

const numFields = 35

// column describes one data-row field (SPEC.md §8). A value >= missing counts as
// missing; missing = 0 means the column has no missing code.
type column struct {
	name         string
	lo, hi       bound
	missing      float64
	core         bool // needed for simulation (spec D-MIS-1)
	warnNegative bool // spec D-RNG-2
}

var columns = [numFields]column{
	{name: "Year"},
	{name: "Month"},
	{name: "Day"},
	{name: "Hour"},
	{name: "Minute"},
	{name: "Data Source and Uncertainty Flags"},
	{name: "Dry Bulb Temperature", lo: gt(-70), hi: lt(70), missing: 99.9, core: true},
	{name: "Dew Point Temperature", lo: gt(-70), hi: lt(70), missing: 99.9},
	{name: "Relative Humidity", lo: ge(0), hi: le(110), missing: 999, core: true},
	{name: "Atmospheric Station Pressure", lo: gt(31000), hi: lt(120000), missing: 999999, core: true},
	{name: "Extraterrestrial Horizontal Radiation", lo: ge(0), missing: 9999},
	{name: "Extraterrestrial Direct Normal Radiation", lo: ge(0), missing: 9999},
	{name: "Horizontal Infrared Radiation Intensity", lo: ge(0), missing: 9999},
	{name: "Global Horizontal Radiation", lo: ge(0), missing: 9999, core: true},
	{name: "Direct Normal Radiation", lo: ge(0), missing: 9999, core: true},
	{name: "Diffuse Horizontal Radiation", lo: ge(0), missing: 9999, core: true},
	{name: "Global Horizontal Illuminance", lo: ge(0), missing: 999900},
	{name: "Direct Normal Illuminance", lo: ge(0), missing: 999900},
	{name: "Diffuse Horizontal Illuminance", lo: ge(0), missing: 999900},
	{name: "Zenith Luminance", lo: ge(0), missing: 9999},
	{name: "Wind Direction", lo: ge(0), hi: le(360), missing: 999, core: true},
	{name: "Wind Speed", lo: ge(0), hi: le(40), missing: 999, core: true},
	{name: "Total Sky Cover", lo: ge(0), hi: le(10), missing: 99},
	{name: "Opaque Sky Cover", lo: ge(0), hi: le(10), missing: 99},
	{name: "Visibility", missing: 9999, warnNegative: true},
	{name: "Ceiling Height", missing: 99999, warnNegative: true},
	{name: "Present Weather Observation"},
	{name: "Present Weather Codes"},
	{name: "Precipitable Water", missing: 999, warnNegative: true},
	{name: "Aerosol Optical Depth", missing: 0.999, warnNegative: true},
	{name: "Snow Depth", missing: 999, warnNegative: true},
	{name: "Days Since Last Snowfall", missing: 99, warnNegative: true},
	{name: "Albedo", missing: 999, warnNegative: true},
	{name: "Liquid Precipitation Depth", missing: 999, warnNegative: true},
	{name: "Liquid Precipitation Quantity", missing: 99, warnNegative: true},
}

const (
	colYear        = 0
	colMinute      = 4
	colFlags       = 5
	colDryBulb     = 6
	colDewPoint    = 7
	colRelHumidity = 8
	colWxObs       = 26
	colWxCodes     = 27
)

func (c *checker) checkRow(ln int, line string) {
	if strings.Contains(line, `"`) {
		c.errf("F-6", ln, -1, "", "", "data rows must not contain double quotes")
	}
	f := strings.Split(line, ",")
	if isHeaderKeyword(f[0]) {
		c.errf("H-ORD-3", ln, 0, "", field(f, 0), "header record %q found among the data rows", field(f, 0))
		return
	}
	c.rep.Summary.DataRows++
	if len(f) != numFields {
		c.errf("D-ROW-1", ln, -1, "", "", "row has %d fields, expected 35", len(f))
	}

	var t [5]int
	timeOK := len(f) >= 5
	for i := 0; i < 5 && i < len(f); i++ {
		v, ok := parseInt(f[i])
		if !ok {
			c.errf("D-ROW-2", ln, i, columns[i].name, f[i], "%s %q is not a whole number", columns[i].name, f[i])
			timeOK = false
		}
		t[i] = v
	}
	if timeOK {
		c.checkTime(ln, t)
	} else {
		c.seqBlind()
	}
	if len(f) != numFields {
		return
	}
	c.rowsChecked++
	c.checkFlags(ln, strings.TrimSpace(f[colFlags]))

	var vals [numFields]float64
	var present [numFields]bool
	for i := colDryBulb; i < numFields; i++ {
		col := columns[i]
		s := strings.TrimSpace(f[i])
		if i == colWxCodes {
			if !isNineDigits(s) {
				c.errf("D-ROW-4", ln, i, col.name, s, "present weather codes %q must be exactly 9 digits", s)
			}
			continue
		}
		v, ok := parseNumber(s)
		if !ok {
			c.errf("D-ROW-3", ln, i, col.name, s, "%s %q is not a number", col.name, s)
			continue
		}
		if i == colWxObs {
			if v != 0 && v != 9 {
				c.errf("D-ROW-5", ln, i, col.name, s, "present weather observation %s must be 0 or 9", s)
			}
			continue
		}
		if col.missing > 0 && v >= col.missing {
			if c.missing[i] == 0 {
				c.firstMissing[i] = ln
			}
			c.missing[i]++
			continue
		}
		present[i], vals[i] = true, v
		if !inRange(v, col.lo, col.hi) {
			c.errf("D-RNG-1", ln, i, col.name, s, "%s = %s is outside the allowed range %s (missing values are >= %g)",
				col.name, s, rangeText(col.lo, col.hi), col.missing)
		}
		if col.warnNegative && v < 0 {
			c.warnf("D-RNG-2", ln, i, col.name, s, "%s %s is negative", col.name, s)
		}
	}

	if present[colDryBulb] && present[colDewPoint] && vals[colDewPoint] > vals[colDryBulb]+0.5 {
		c.warnf("D-CON-1", ln, colDewPoint, columns[colDewPoint].name, f[colDewPoint],
			"dew point %s is above dry bulb %s", f[colDewPoint], f[colDryBulb])
	}
	if present[colRelHumidity] && vals[colRelHumidity] > 100 {
		c.warnf("D-CON-2", ln, colRelHumidity, columns[colRelHumidity].name, f[colRelHumidity],
			"relative humidity %s is above 100", f[colRelHumidity])
	}
}

func isNineDigits(s string) bool {
	if len(s) != 9 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// checkFlags checks field 5. Flags that do not have the classic form are counted
// and reported once at the end (D-ROW-6), since real files differ a lot.
func (c *checker) checkFlags(ln int, s string) {
	if s == "" {
		c.errf("D-ROW-7", ln, colFlags, columns[colFlags].name, "", "data source and uncertainty flags are empty")
		return
	}
	if flagsFollowFormat(s) {
		return
	}
	if c.badFlags == 0 {
		c.firstBadFlags, c.badFlagsValue = ln, s
	}
	c.badFlags++
}

// flagsFollowFormat: 22 pairs of a source letter (A-I or ?) and an
// uncertainty digit (SPEC.md D-ROW-6).
func flagsFollowFormat(s string) bool {
	if len(s) != 44 {
		return false
	}
	for i := 0; i < len(s); i += 2 {
		if !strings.ContainsRune("ABCDEFGHI?", rune(s[i])) || s[i+1] < '0' || s[i+1] > '9' {
			return false
		}
	}
	return true
}

func (c *checker) finishData() {
	if c.badFlags > 0 {
		c.warnf("D-ROW-6", c.firstBadFlags, colFlags, columns[colFlags].name, c.badFlagsValue,
			"%d of %d rows have flags that are not the classic 44 characters (a source letter A-I or ? "+
				"followed by a digit, 22 times); this is common and allowed", c.badFlags, c.rowsChecked)
	}
	if c.rowsChecked == 0 {
		return
	}
	for i, col := range columns {
		n := c.missing[i]
		if n == 0 {
			continue
		}
		c.rep.Summary.Missing[col.name] = n
		switch {
		case col.core && n == c.rowsChecked:
			c.errf("D-MIS-1", 0, i, col.name, "",
				"%s is missing on every row; the file cannot be used for simulation", col.name)
		case col.core:
			c.warnf("D-MIS-2", c.firstMissing[i], i, col.name, "",
				"%s is missing on %d of %d rows (first at line %d); many viewers plot missing codes as real values",
				col.name, n, c.rowsChecked, c.firstMissing[i])
		case n == c.rowsChecked:
			c.warnf("D-MIS-3", 0, i, col.name, "", "%s is missing on every row", col.name)
		}
	}
}

func (c *checker) checkTime(ln int, t [5]int) {
	y, m, d, h, minute := t[0], t[1], t[2], t[3], t[4]
	if y < 1900 || y > 2100 {
		c.warnf("D-SEQ-7", ln, colYear, columns[colYear].name, fmt.Sprint(y), "year %d is outside 1900-2100", y)
	}
	if m < 1 || m > 12 || d < 1 || d > daysIn(m, true) || h < 1 || h > 24 {
		c.errf("D-SEQ-1", ln, -1, "", fmt.Sprintf("%d/%d hour %d", m, d, h),
			"month %d, day %d, hour %d is not a valid time (month 1-12, a day that exists in the month, hour 1-24)",
			m, d, h)
		c.seqBlind()
		return
	}
	if m == 2 && d == 29 && !(c.leapYes && isLeap(y)) {
		reason := "the leap-year field is No"
		if c.leapYes {
			reason = fmt.Sprintf("%d is not a leap year", y)
		}
		c.errf("D-SEQ-5", ln, 2, "Day", "29", "row for 29 February, but %s", reason)
		return // not part of the time sequence
	}
	step := 1
	if c.perHour == 1 {
		if minute < 0 || minute > 60 {
			c.errf("D-SEQ-2", ln, colMinute, columns[colMinute].name, fmt.Sprint(minute),
				"minute %d is outside 0-60", minute)
		}
	} else {
		iv := 60 / c.perHour
		if minute%iv != 0 || minute < iv || minute > 60 {
			allowed := fmt.Sprintf("%d, %d, ..., 60", iv, 2*iv)
			if c.perHour <= 4 {
				var m []string
				for k := 1; k <= c.perHour; k++ {
					m = append(m, fmt.Sprint(k*iv))
				}
				allowed = strings.Join(m, ", ")
			}
			c.errf("D-SEQ-2", ln, colMinute, columns[colMinute].name, fmt.Sprint(minute),
				"minute %d: with %d records per hour the minutes must be %s", minute, c.perHour, allowed)
			c.seqBlind()
			return
		}
		step = minute / iv
	}
	c.seqRow(ln, stamp{y, m, d, h, step})
}
