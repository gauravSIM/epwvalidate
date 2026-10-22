# epwvalidate

Checks whether an EPW (EnergyPlus Weather) file is correct:
- all header records are present and complete;
- all data rows are well-formed;
- all values are in range;
- the rows match the declared time periods.

The rules and their IDs (e.g. `D-RNG-1`) are defined in [SPEC.md](SPEC.md).

Files do not need to cover a full year. A day, a week, a month, or several periods from different years are all
fine, as long as the `DATA PERIODS` header record declares them.

The validator exists in four languages. They give exactly the same result, the same messages and the same JSON for
every file. Each one is a single source file with no dependencies, meant to be copied into another codebase.

| Language | Copy this file | Needs |
|---|---|---|
| Go | the `.go` files in [go/](go/) (package `epwvalidate`) | Go 1.22+ |
| Python | [python/epwvalidate.py](python/epwvalidate.py) | Python 3.9+ |
| Node / TypeScript | [node/epwvalidate.js](node/epwvalidate.js) and [node/epwvalidate.d.ts](node/epwvalidate.d.ts) | Node 18+ |
| C# / .NET | [dotnet/EpwValidate/EpwValidator.cs](dotnet/EpwValidate/EpwValidator.cs) | .NET 8+ |

## Repository layout

```
SPEC.md              the rules, shared by all languages
EPW_format/          the EPW format document and 13 real example files
testdata/
  partial/           partial-year files that must be valid
  broken/            broken files that must be invalid, named <RULE>__<description>.epw
  expected.json      the exact report for every test file (from the Go version)
  broken_results.txt the text output for every broken file
go/                  Go version (the reference) and the test-file generator
python/              Python version and tests
node/                Node version, TypeScript types and tests
dotnet/              C# version, a command-line project and a test project
```

## Using it in another codebase

Every version has the same three operations:
- validate a file;
- validate bytes or text already in memory;
- format a report as text.

A report has the same shape everywhere, matching the JSON output:

```json
{
  "valid": false,
  "errors": [
    { "severity": "error", "rule": "D-RNG-1", "line": 79, "column": 6, "field": "Dry Bulb Temperature",
      "value": "75.0", "message": "Dry Bulb Temperature = 75.0 is outside the allowed range > -70 and < 70 (missing values are >= 99.9)" }
  ],
  "warnings": [ ... ],
  "summary": { "lines": 176, "dataRows": 168, "expectedRows": 168, "periods": 1, "recordsPerHour": 1,
               "errorCount": 1, "warningCount": 1, "missing": { ... }, "ruleCounts": { "D-RNG-1": 1, "D-ROW-6": 1 } }
}
```

- `valid` is true when there are no errors. Warnings do not make a file invalid.
- `line` is 1-based, with 0 meaning the whole file. `column` is the 0-based field index, with -1 meaning the whole
  line.
- `field` and `value` are left out when they don't apply.
- At most 20 issues per rule are listed. `summary.ruleCounts`, `errorCount` and `warningCount` hold the full counts.
- Reading the file can fail (missing file, no permission). That is an I/O error (exception or `error`), not an
  issue in the report.

### Python

```python
from epwvalidate import validate_file, validate, format_text

report = validate_file("file.epw")         # raises OSError if the file cannot be read
if not report.valid:
    for issue in report.errors:
        print(issue)                       # line 79, Dry Bulb Temperature: ... [D-RNG-1]
report_from_upload = validate(data)        # data: bytes or str
as_json = report.to_dict()                 # the JSON shape above, ready for json.dumps
```

### Node / TypeScript

```js
const { validateFile, validate, formatIssue, formatText } = require('./epwvalidate');

const report = validateFile('file.epw');   // throws if the file cannot be read
if (!report.valid) for (const issue of report.errors) console.log(formatIssue(issue));
const fromUpload = validate(buffer);        // Buffer, Uint8Array or string
res.json(report);                           // the report object is already the JSON shape above
```

It works with `require` and with `import { validateFile } from './epwvalidate.js'` in ES modules. TypeScript picks
up the types from `epwvalidate.d.ts`.

### C# / .NET

```csharp
using EpwValidate;

var report = EpwValidator.ValidateFile("file.epw");  // throws IOException if the file cannot be read
if (!report.Valid)
    foreach (var issue in report.Errors)
        Console.WriteLine(issue);                    // line 79, Dry Bulb Temperature: ... [D-RNG-1]
var fromUpload = EpwValidator.Validate(bytes);       // byte[] or string
string json = report.ToJson();                       // the JSON shape above
```

### Go

```go
import "epwvalidate"

rep, err := epwvalidate.ValidateFile("file.epw") // err is only for I/O problems
if !rep.Valid {
    for _, is := range rep.Errors {
        fmt.Println(is) // line 79, Dry Bulb Temperature: ... [D-RNG-1]
    }
}
```

`Validate(io.Reader)` and `ValidateBytes([]byte)` do the same for data already in memory. To use the package from
another Go module without copying it, point to the folder in that module's `go.mod`:

```
require epwvalidate v0.0.0
replace epwvalidate => ../path/to/epwvalidate/go
```

## Command line

Every version is also a command-line tool with the same options and output:

```bash
python python/epwvalidate.py file.epw
```

```bash
node node/epwvalidate.js file.epw
```

```bash
dotnet run --project dotnet/EpwValidate.Cli -- file.epw
```

```bash
cd go && go run ./cmd/epwvalidate ../file.epw
```

- Several files can be given at once.
- `--json` prints the reports as a JSON array; each report also has a `file` property.
- Exit code: `0` all files valid, `1` at least one file invalid, `2` a file could not be read or no file was given.

## What the output looks like

Each file gets a summary line: `VALID` or `INVALID`, the number of data rows, how many rows `DATA PERIODS`
declares, and the error and warning counts. Each error and warning follows on its own line with the line number,
field name, message and rule ID in brackets.

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

## Tests

The Go version is the reference. The other three must reproduce its results exactly: every one of them checks all
132 test files against `testdata/expected.json`, and its text output against `testdata/broken_results.txt`. The test
files are the 13 examples, 18 partial-year files and 101 broken files.

```bash
cd go && go test ./...
```

```bash
cd python && python -m unittest -v
```

```bash
cd node && node --test
```

```bash
dotnet run --project dotnet/EpwValidate.Tests
```

GitHub runs all four on every push (`.github/workflows/test.yml`).

The Go tests also check:
- every header row deleted, cut short or blanked;
- warnings;
- that errors don't cascade;
- date and number parsing.

**Changing the rules:**
1. Update `SPEC.md`.
2. Change the Go version.
3. Regenerate the test files and expected results:
   ```bash
   cd go && go test -run 'TestFixtures|TestExpected' -update
   ```
4. Make the Python, Node and .NET versions pass again.

Test files can be added to `testdata/partial/` or `testdata/broken/` by hand. The Go tests pick them up by name,
and `-update` adds them to `expected.json`.

To check every `.epw` file under another folder with the Go tests as well:

```bash
cd go && EPWVALIDATE_SAMPLES=/path/to/epw/folder go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
