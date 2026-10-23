package epwvalidate

// The simien profile (SPEC.md §11.1): can this EPW become a SIMIEN hourly
// climate file? That is a different question from "is this a correct EPW", so
// these rules are opt-in and never change the default report.
//
// The target is a 12-column CSV holding one full year. Two of its columns,
// solar altitude and azimuth, are computed from the LOCATION coordinates and
// are not carried by the EPW. The rest are read straight across — which is why
// a missing value matters here and not in an EPW: the CSV has no missing code,
// so an unfilled hour is read as a real number.

const (
	// simienRows is one full year of hourly values: 365 × 24.
	simienRows = 8760
	// simienDefaultAlbedo is what a missing albedo is filled with. The
	// standard SIMIEN station files carry this constant for the whole year.
	simienDefaultAlbedo = "0.20"
)

func (c *checker) simienProfile() bool { return c.opt.Profile == ProfileSimien }

// finishSimien runs after the data rows have been read, so it can report on
// the file as a whole. It is called from finishData.
func (c *checker) finishSimien() {
	if !c.simienProfile() {
		return
	}

	for i, col := range columns {
		n := c.missing[i]
		if n == 0 || !col.simien {
			continue
		}
		c.errf("S-COL-1", c.firstMissing[i], i, col.name, "",
			"%s is missing on %d of %d rows (first at line %d); a SIMIEN climate file has no missing code, "+
				"so the missing hours would be read as real values",
			col.name, n, c.rowsChecked, c.firstMissing[i])
	}

	// Albedo is a ground property, not weather, and the standard station files
	// carry one constant for the year — so a missing value has an answer, and
	// refusing a year of good weather over it would be the rule biting the
	// wrong case.
	if n := c.missing[colAlbedo]; n > 0 {
		c.warnf("S-ALB-1", c.firstMissing[colAlbedo], colAlbedo, columns[colAlbedo].name, "",
			"albedo is missing on %d of %d rows (first at line %d); it will be filled with %s",
			n, c.rowsChecked, c.firstMissing[colAlbedo], simienDefaultAlbedo)
	}

	if c.feb29 > 0 {
		c.errf("S-LEAP-1", c.firstFeb29, 2, "Day", "29",
			"the file has %d rows for 29 February (first at line %d); a SIMIEN climate file numbers days 1-365",
			c.feb29, c.firstFeb29)
	}

	if c.perHour != 1 {
		c.errf("S-RATE-1", 0, -1, "", "",
			"DATA PERIODS declares %d records per hour; a SIMIEN climate file is hourly, and EPW radiation is "+
				"Wh/m2 per record, which equals W/m2 only at one record per hour", c.perHour)
	}

	// Counted in rows rather than periods: how the year is split across
	// DATA PERIODS is the EPW rules' business (§7), not this profile's.
	if rows := c.rep.Summary.DataRows; rows != simienRows {
		c.errf("S-ROWS-1", 0, -1, "", "",
			"the file has %d data rows; a SIMIEN climate file is one full year, %d hourly rows",
			rows, simienRows)
	}
}
