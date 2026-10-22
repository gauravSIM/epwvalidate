package epwvalidate

import (
	"fmt"
	"strings"
)

// stamp is the time of one data row. step is the record within the hour
// (1..records per hour). y = 0 means the year is unknown.
type stamp struct{ y, m, d, h, step int }

func (s stamp) String() string {
	t := fmt.Sprintf("%d/%d hour %d", s.m, s.d, s.h)
	if s.y != 0 {
		t = fmt.Sprintf("%d/%d/%d hour %d", s.m, s.d, s.y, s.h)
	}
	if s.step > 1 {
		t += fmt.Sprintf(" step %d", s.step)
	}
	return t
}

func sameTime(a, b stamp, withYear bool) bool {
	return a.m == b.m && a.d == b.d && a.h == b.h && a.step == b.step && (!withYear || a.y == b.y)
}

// seqState walks the data rows through the declared periods (SPEC.md §7).
// After a row that does not follow, it carries on from that row, so one
// missing or extra row gives one error instead of thousands.
type seqState struct {
	p          int   // index of the current period
	inside     bool  // at least one row of period p has been seen
	cur        stamp // the last row
	done       bool  // all periods are complete
	extra      int   // rows after the last period
	firstExtra int
	febYear    []int // year of the February rows in each period (0 = none seen)
}

func (c *checker) periodStart(p int) stamp {
	s := c.periods[p].start
	return stamp{s.y, s.m, s.d, 1, 1}
}

func (c *checker) isPeriodEnd(s stamp, p int) bool {
	e := c.periods[p].end
	return s.m == e.m && s.d == e.d && s.h == 24 && s.step == c.perHour && (e.y == 0 || s.y == e.y)
}

func (c *checker) afterPeriodEnd(s stamp, p int) bool {
	e := c.periods[p].end
	d := calDate{m: s.m, d: s.d}
	if e.y != 0 {
		d.y = s.y
	}
	return e.before(d)
}

// next returns the time step after s.
func (c *checker) next(s stamp) stamp {
	if s.step < c.perHour {
		s.step++
		return s
	}
	s.step = 1
	if s.h < 24 {
		s.h++
		return s
	}
	s.h = 1
	feb29 := c.leapYes && s.y != 0 && isLeap(s.y)
	if s.d < daysIn(s.m, feb29) {
		s.d++
		return s
	}
	s.d = 1
	if s.m < 12 {
		s.m++
		return s
	}
	s.m = 1
	if s.y != 0 {
		s.y++
	}
	return s
}

func (c *checker) closePeriod() {
	c.seq.p++
	c.seq.inside = false
	if c.seq.p >= len(c.periods) {
		c.seq.done = true
	}
}

// seqBlind moves one time step on for a row whose time could not be read, so
// that the following rows still line up.
func (c *checker) seqBlind() {
	q := &c.seq
	if c.periods == nil || q.done {
		return
	}
	if !q.inside {
		q.inside = true
		q.cur = c.periodStart(q.p)
	} else {
		q.cur = c.next(q.cur)
	}
	if c.isPeriodEnd(q.cur, q.p) {
		c.closePeriod()
	}
}

func (c *checker) seqRow(ln int, s stamp) {
	q := &c.seq
	if c.periods == nil {
		return
	}
	if q.done {
		if q.extra == 0 {
			q.firstExtra = ln
		}
		q.extra++
		return
	}
	pr := c.periods[q.p]
	withYear := pr.start.y != 0

	if !q.inside {
		q.inside = true
		if start := c.periodStart(q.p); !sameTime(s, start, withYear) {
			c.errf("D-SEQ-3", ln, -1, "", s.String(), "data period %d (%s) starts at %s, but its first row is %s",
				q.p+1, pr.label(), pr.start, s)
		}
	} else {
		exp := c.next(q.cur)
		switch {
		case sameTime(s, exp, false):
			if withYear && s.y != exp.y {
				c.errf("D-SEQ-4", ln, colYear, columns[colYear].name, fmt.Sprint(s.y),
					"year %d, expected %d (data period %d has m/d/y dates)", s.y, exp.y, q.p+1)
			} else if !withYear && s.y != q.cur.y && s.m == q.cur.m {
				c.errf("D-SEQ-4", ln, colYear, columns[colYear].name, fmt.Sprint(s.y),
					"year changes from %d to %d inside month %d; it may change only when the month changes",
					q.cur.y, s.y, s.m)
			}
		case exp.m == 2 && exp.d == 29 && s.m == 3 && s.d == 1 && s.h == 1 && s.step == 1:
			c.errf("D-SEQ-5", ln, -1, "", s.String(),
				"29 February %d is missing: the leap-year field is Yes and %d is a leap year", q.cur.y, q.cur.y)
		case q.p+1 < len(c.periods) && sameTime(s, c.periodStart(q.p+1), c.periods[q.p+1].start.y != 0):
			c.errf("D-SEQ-3", ln, -1, "", s.String(),
				"data period %d (%s) ends early: expected %s, but data period %d starts here",
				q.p+1, pr.label(), exp, q.p+2)
			c.closePeriod()
			c.seqRow(ln, s)
			return
		case c.afterPeriodEnd(s, q.p):
			c.errf("D-SEQ-3", ln, -1, "", s.String(), "row %s is after the end of data period %d (%s, ends %s)",
				s, q.p+1, pr.label(), pr.end)
			c.closePeriod()
			c.seqRow(ln, s)
			return
		default:
			c.errf("D-SEQ-3", ln, -1, "", s.String(),
				"expected %s, found %s (rows must follow each other one time step apart)", exp, s)
		}
	}
	if s.m == 2 {
		q.febYear[q.p] = s.y
	}
	q.cur = s
	if c.isPeriodEnd(s, q.p) {
		c.closePeriod()
	}
}

// periodDays counts the days in period k. Whether 29 February is included
// depends on the leap-year field and the year of the period's February rows.
func (c *checker) periodDays(k int) int {
	pr := c.periods[k]
	d := pr.start
	n := 1
	for !(d.m == pr.end.m && d.d == pr.end.d && (pr.end.y == 0 || d.y == pr.end.y)) {
		y := d.y
		if pr.start.y == 0 {
			y = c.seq.febYear[k]
		}
		feb29 := c.leapYes && y != 0 && isLeap(y)
		if d.d < daysIn(d.m, feb29) {
			d.d++
		} else if d.d, d.m = 1, d.m+1; d.m > 12 {
			d.m = 1
			d.y++
		}
		n++
		if n > 366*200 { // cannot happen once H-DP-7 has passed; guard against a loop
			break
		}
	}
	return n
}

func (c *checker) finishSequence() {
	if c.periods == nil {
		return
	}
	expected := 0
	var spans []string
	for k, pr := range c.periods {
		expected += c.periodDays(k) * 24 * c.perHour
		spans = append(spans, fmt.Sprintf("%s-%s", pr.start, pr.end))
	}
	s := &c.rep.Summary
	s.ExpectedRows = expected
	if s.DataRows == expected {
		return
	}
	msg := fmt.Sprintf("DATA PERIODS (%s, %d per hour) declares %d rows, but the file has %d data rows",
		strings.Join(spans, ", "), c.perHour, expected, s.DataRows)
	if c.seq.extra > 0 {
		msg += fmt.Sprintf("; %d rows come after the last period ends (first at line %d)", c.seq.extra, c.seq.firstExtra)
	}
	c.errf("D-SEQ-6", 0, -1, "", "", "%s", msg)
}
