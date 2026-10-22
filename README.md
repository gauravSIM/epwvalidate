# epwvalidate

Checks whether an EPW (EnergyPlus Weather) file is correct: all header records present and complete, all data rows
well-formed, all values in range, and the rows matching the declared time periods. The rules and their IDs (e.g.
`D-RNG-1`) are defined in [SPEC.md](SPEC.md).

Files do not need to cover a full year: a day, a week, a month, or several periods from different years are all
fine, as long as the `DATA PERIODS` header record declares them.

Needs Go 1.22 or newer. Uses only the Go standard library.

## Command line

```bash
go build -o epwvalidate ./cmd/epwvalidate
```

```bash
epwvalidate file.epw
```

- The output is described in [What the output looks like](#what-the-output-looks-like).
- Several files can be given at once.
- `--json` prints machine-readable reports.
- Exit code: `0` all files valid, `1` at least one file invalid, `2` a file could not be read.
- Errors make a file invalid. Warnings do not.
- At most 20 issues per rule are listed. The total per rule is in the summary.

## What the output looks like

Each file gets a summary line: `VALID` or `INVALID`, the number of data rows, how many rows `DATA PERIODS`
declares, and the error and warning counts. Each error and warning follows on its own line with the line number,
field name, message and rule ID in brackets. The rule IDs are explained in [SPEC.md](SPEC.md).

### Valid files

A full-year TMYx file. The one warning is normal for TMYx files and does not make the file invalid:

```
NOR_AK_Oslo.AP-Gardermoen.013840_TMYx.2007-2021.epw: VALID - 8760 data rows (DATA PERIODS declares 8760), 0 errors, 1 warnings
  warning  line 9, Data Source and Uncertainty Flags: 8760 of 8760 rows have flags that are not the classic 44 characters (a source letter A-I or ? followed by a digit, 22 times); this is common and allowed [D-ROW-6]
```

A full-year NORCERRA file. Some columns are not in this source at all; that is reported, but allowed:

```
NOR_AK_Hurdal.NOR076_NORCERRA.epw: VALID - 8760 data rows (DATA PERIODS declares 8760), 0 errors, 9 warnings
  warning  line 9, Data Source and Uncertainty Flags: 8760 of 8760 rows have flags that are not the classic 44 characters ... [D-ROW-6]
  warning  file, Extraterrestrial Horizontal Radiation: Extraterrestrial Horizontal Radiation is missing on every row [D-MIS-3]
  warning  file, Global Horizontal Illuminance: Global Horizontal Illuminance is missing on every row [D-MIS-3]
  ...
```

A partial file with two periods (January 2019 and July 2021):

```
partial/two_periods_jan2019_jul2021.epw: VALID - 1488 data rows (DATA PERIODS declares 1488), 0 errors, 1 warnings
  warning  line 9, Data Source and Uncertainty Flags: 1488 of 1488 rows have flags that are not the classic 44 characters ... [D-ROW-6]
```

### Broken files

A sample of the broken test files. The D-ROW-6 warning that every one of them also shows is left out here.

A header row is missing:

```
broken/H-ORD-1__comments_2_missing.epw: INVALID - 168 data rows (DATA PERIODS declares 168), 1 errors, 1 warnings
  error    line 7: the COMMENTS 2 record is missing: line 7 should hold it, but holds the DATA PERIODS record [H-ORD-1]
```

Part of a header row is missing:

```
broken/H-LOC-3__empty_country.epw: INVALID - 168 data rows (DATA PERIODS declares 168), 1 errors, 1 warnings
  error    line 1, Country: Country is empty [H-LOC-3]

broken/H-DC-4__design_conditions_cut_short.epw: INVALID - 168 data rows (DATA PERIODS declares 168), 1 errors, 1 warnings
  error    line 2, DESIGN CONDITIONS: expected the Extremes section at field 41, found "" [H-DC-4]
```

A value out of range, and a data row with a field missing:

```
broken/D-RNG-1__dry_bulb_75.epw: INVALID - 168 data rows (DATA PERIODS declares 168), 1 errors, 1 warnings
  error    line 79, Dry Bulb Temperature: Dry Bulb Temperature = 75.0 is outside the allowed range > -70 and < 70 (missing values are >= 99.9) [D-RNG-1]

broken/D-ROW-1__row_with_34_fields.epw: INVALID - 168 data rows (DATA PERIODS declares 168), 1 errors, 1 warnings
  error    line 29: row has 34 fields, expected 35 [D-ROW-1]
```

One hourly row deleted, and a file whose DATA PERIODS says January but which holds one week:

```
broken/D-SEQ-3__one_row_deleted.epw: INVALID - 167 data rows (DATA PERIODS declares 168), 2 errors, 1 warnings
  error    line 109: expected 1/12/2009 hour 5, found 1/12/2009 hour 6 (rows must follow each other one time step apart) [D-SEQ-3]
  error    file: DATA PERIODS (1/8-1/14, 1 per hour) declares 168 rows, but the file has 167 data rows [D-SEQ-6]

broken/D-SEQ-6__declares_month_has_week.epw: INVALID - 168 data rows (DATA PERIODS declares 744), 2 errors, 1 warnings
  error    line 9: data period 1 (January) starts at 1/1, but its first row is 1/8/2009 hour 1 [D-SEQ-3]
  error    file: DATA PERIODS (1/1-1/31, 1 per hour) declares 744 rows, but the file has 168 data rows [D-SEQ-6]
```

Rows for 29 February in a file whose leap-year field is No. At most 20 issues are listed per rule; the rest are
counted:

```
broken/D-SEQ-5__feb29_rows_with_leap_no.epw: INVALID - 816 data rows (DATA PERIODS declares 792), 25 errors, 1 warnings
  error    line 681, Day: row for 29 February, but the leap-year field is No [D-SEQ-5]
  error    line 682, Day: row for 29 February, but the leap-year field is No [D-SEQ-5]
  ...
  error    file: DATA PERIODS (2/1-3/5, 1 per hour) declares 792 rows, but the file has 816 data rows [D-SEQ-6]
  ... and 4 more [D-SEQ-5]
```

The output for all 101 broken test files is in [testdata/broken_results.txt](testdata/broken_results.txt).

## Go

```go
import "epwvalidate"

rep, err := epwvalidate.ValidateFile("file.epw") // err is only for I/O problems
if !rep.Valid {
    for _, is := range rep.Errors {
        fmt.Println(is) // "line 79, Dry Bulb Temperature: ... [D-RNG-1]"
    }
}
```

`Validate(io.Reader)` and `ValidateBytes([]byte)` do the same for data already in memory.

To use the package from another Go module, point to this folder in that module's `go.mod`:

```
require epwvalidate v0.0.0
replace epwvalidate => ../path/to/epwvalidate
```

## Tests

```bash
go test ./...
```

- `EPW_format/epw_examples/`: 13 real files (TMYx and NORCERRA). All must be valid.
- `testdata/partial/`: partial-year files (one day, one week, one month, several periods from different years, leap
  year, 15-minute data, ...). All must be valid.
- `testdata/broken/`: files with one problem each, named `<RULE>__<description>.epw`. Each must be invalid with that
  rule. You can add your own files to either folder; the tests pick them up by name.
- The partial and broken files are generated from two of the example files by `fixtures_test.go`. After changing it, run
  `go test -run TestFixtures -update`.
- To check every `.epw` file under a folder as well, set `EPWVALIDATE_SAMPLES`:

```bash
EPWVALIDATE_SAMPLES=/path/to/epw/folder go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
