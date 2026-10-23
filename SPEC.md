# epwvalidate — specification

This document defines what `epwvalidate` accepts as a correct EPW (EnergyPlus Weather) file. Every rule has an ID
(e.g. `D-RNG-1`) that appears in the validator's output.

The EPW format itself (every header record and data field, with units, ranges and missing-value codes) is
described in [EPW_format/EPW_format.md](EPW_format/EPW_format.md). Example files are in
[EPW_format/epw_examples/](EPW_format/epw_examples/).

## 0. Purpose and scope

`epwvalidate` answers one question: **is this EPW file acceptable?**

- Every header record is present and complete.
- Every data row is well-formed.
- Every value is inside its allowed range, or marked as missing.
- The rows match the time periods declared in `DATA PERIODS`.

The validator checks **structure and values**. It does not judge climate quality (for example whether a typical
month is representative).

### Files do not have to cover a full year

A file may hold any amount of data, as long as `DATA PERIODS` declares it correctly:

| Example | DATA PERIODS | Rows (hourly) |
|---|---|---|
| Full year | `DATA PERIODS,1,1,Data,Sunday,1/ 1,12/31` | 8760 |
| One day | `DATA PERIODS,1,1,Day,Monday,7/15,7/15` | 24 |
| One week | `DATA PERIODS,1,1,Week,Monday,1/ 8,1/14` | 168 |
| One month | `DATA PERIODS,1,1,January,Sunday,1/ 1,1/31` | 744 |
| Two months from different years | `DATA PERIODS,2,1,Jan,Sunday,1/ 1,1/31,Jul,Friday,7/ 1,7/31` | 744 + 744 |
| Typical year (each month from a different year) | `DATA PERIODS,1,1,Data,Sunday,1/ 1,12/31` | 8760; the `Year` column changes between months |

---

## 1. The EPW file at a glance

- Plain text, comma-separated, one record per line.
- Lines 1–8 are header records, in this order:

| Line | Record | Content |
|---|---|---|
| 1 | `LOCATION` | city, region, country, source, WMO, latitude, longitude, time zone, elevation |
| 2 | `DESIGN CONDITIONS` | ASHRAE design conditions, or `0` for none |
| 3 | `TYPICAL/EXTREME PERIODS` | typical and extreme weeks |
| 4 | `GROUND TEMPERATURES` | monthly ground temperatures at one or more depths |
| 5 | `HOLIDAYS/DAYLIGHT SAVINGS` | leap-year flag, daylight saving, holidays |
| 6 | `COMMENTS 1` | free text |
| 7 | `COMMENTS 2` | free text |
| 8 | `DATA PERIODS` | number of periods, records per hour, and each period's name, start weekday, start and end date |

- From line 9 on, every line is one data row of 35 fields (§6–§9).

Example (first 9 lines of a full-year file, long lines shortened with `...`):

```
LOCATION,Oslo.AP-Gardermoen,AK,NOR,SRC-TMYx,013840,60.20640,11.08000,1.0,207.6
DESIGN CONDITIONS,1,2025 ASHRAE Handbook -- Fundamentals - Chapter 14 Climatic Design Information,Heating,1,-17.5,...,Cooling,7,9.0,...,Extremes,9.2,...
TYPICAL/EXTREME PERIODS,6,Summer - Week Nearest Max Temperature For Period,Extreme,7/27,8/ 2,...
GROUND TEMPERATURES,3,.5,,,,-4.18,-5.06,-3.50,-1.05,5.35,10.29,13.62,14.64,12.91,9.07,3.95,-0.81,2,...
HOLIDAYS/DAYLIGHT SAVINGS,No,0,0,0
COMMENTS 1,"NCEI ISD/ERA5 - #years=[65] Period of Record=1940-2023; Jan=2003; Feb=2012; ..."
COMMENTS 2,"Ground temps represent undisturbed earth temperatures - calculated from this weather data."
DATA PERIODS,1,1,Data,Sunday,1/ 1,12/31
2003,1,1,1,0,?9?9?9?9E0?9?9?9?9?9?9?9?9?9?9?9?9?9*9*9?9*9,-22.10,-24.30,81,98174,0,0,190,0,0,0,0,0,0,0,360,1.00,3,3,7000.0,77777,9,999999999,1,0.0850,51,88,0.400,0.0,0.0
```

---

## 2. Result

Every finding is an **error** or a **warning**.

- **Error**: the file is not acceptable. A file is valid only when it has no errors.
- **Warning**: the file is acceptable, but something looks wrong or is unusual.

Each finding reports the line number, the field, the value, a message and the rule ID. At most 20 findings per rule
are listed; the full count per rule is always reported.

The validator exists in Go, Python, Node and C#. All four must give exactly the same findings, with the same
messages, for every file; §12 says how this is checked. Their APIs, the JSON shape of a report and the command-line
tools are described in [README.md](README.md).

---

## 3. File-level rules (F)

| ID | Sev | Rule |
|---|---|---|
| F-1 | E | The file is not empty and is valid UTF-8 (plain ASCII is valid UTF-8). |
| F-2 | E | No UTF-8 byte-order mark (BOM) at the start. With a BOM the first keyword reads as `﻿LOCATION`. |
| F-3 | — | Line endings may be CRLF or LF. Both are accepted. |
| F-4 | E | No empty lines between the first header line and the last data row. Empty lines at the very end are allowed. |
| F-5 | E | No line ends with `;`. Semicolons do not end lines in EPW. |
| F-6 | E | Double-quoted fields are allowed only in free-text header fields (comments, period names, holiday names, design-condition source). `LOCATION` and data rows must contain no `"` characters, because many EPW readers split those lines on bare commas. |
| F-7 | W | Mixed line endings (some CRLF, some LF). |

## 4. Header order (H-ORD)

| ID | Sev | Rule |
|---|---|---|
| H-ORD-1 | E | Lines 1–8 hold the 8 header records of §1, in that order. A missing record is reported once by name ("the COMMENTS 2 record is missing"), and the following lines are matched against the records after it, so the rest of the file is still checked. A record in the wrong place is reported as out of order. A file that ends before all 8 records is reported too. |
| H-ORD-2 | — | Keywords are matched case-insensitively. `HOLIDAYS/DAYLIGHT SAVING` (without the final S) is accepted as well. |
| H-ORD-3 | E | No header keyword appears among the data rows. |

## 5. Header records (H)

**Field counts.** Each header record must have exactly the number of fields given below. Empty fields at the very
end of a record are ignored (warning H-TRAIL), because some writers add trailing commas.

**No missing parts.** Every header field must be filled in. A record that is cut short fails its field-count rule,
and an empty field fails the rule for that field. The only fields that may be empty are:
- the soil properties in GROUND TEMPERATURES (H-GT-4);
- one empty field that some writers put after the DESIGN CONDITIONS source.

This is tested by deleting every header row, cutting every header row short at every field, and blanking every
header field one at a time; each change must make the file invalid.

### 5.1 LOCATION

`LOCATION, city, region, country, source, WMO, latitude, longitude, time zone, elevation`

| ID | Sev | Rule |
|---|---|---|
| H-LOC-1 | E | Exactly 10 fields. |
| H-LOC-2 | E | City is empty. |
| H-LOC-3 | E | Region, country, source or WMO is empty. |
| H-LOC-4 | E | Latitude is a number, −90 ≤ lat ≤ 90 (degrees, + = North). |
| H-LOC-5 | E | Longitude is a number, −180 ≤ lon ≤ 180 (degrees, + = East). |
| H-LOC-6 | E | Time zone is a number, −12 ≤ tz ≤ 12 (hours from GMT). |
| H-LOC-7 | E | Elevation is a number, −1000 ≤ elev < 9999.9 (m). |
| H-LOC-8 | W | Time zone differs from `round(longitude / 15)` by more than 3 h (a likely sign swap or wrong zone). |

### 5.2 DESIGN CONDITIONS

`DESIGN CONDITIONS, number, source, [empty,] Heating, values..., Cooling, values..., Extremes, values...`, or
`DESIGN CONDITIONS,0` for none.

| ID | Sev | Rule |
|---|---|---|
| H-DC-1 | E | The number of design conditions is a whole number ≥ 0. |
| H-DC-2 | E | If the number is 0, the record has no further fields. |
| H-DC-3 | E | If the number is ≥ 1, the source is not empty. |
| H-DC-4 | E | If the number is ≥ 1, the record continues with a `Heating`, a `Cooling` and an `Extremes` section, in that order. One empty field is allowed between the source and `Heating`. Each section has at least one value. Every value is a number or `N/A` (ASHRAE's "not available"). An empty value is an error. |
| H-DC-5 | E | For known ASHRAE Handbook editions, each section has its exact number of values. 2021 ASHRAE Handbook: 16 Heating, 32 Cooling, 15 Extremes. 2025 ASHRAE Handbook: 16 Heating, 32 Cooling, 27 Extremes. Other sources are checked by H-DC-4 only. |

### 5.3 TYPICAL/EXTREME PERIODS

`TYPICAL/EXTREME PERIODS, N`, then for each period: `name, type, start day, end day`.

| ID | Sev | Rule |
|---|---|---|
| H-TEP-1 | E | N is a whole number ≥ 0. |
| H-TEP-2 | E | Field count = 2 + 4·N. |
| H-TEP-3 | W | The period type is not `Typical` or `Extreme` (case-insensitive). |
| H-TEP-4 | E | Start and end days are valid date fields (§5.9). A period may wrap past the year end (e.g. `12/22` → `1/ 5`). |
| H-TEP-5 | E | A period name or period type is empty. |

### 5.4 GROUND TEMPERATURES

`GROUND TEMPERATURES, N`, then for each depth: `depth (m), soil conductivity (W/m-K), soil density (kg/m³), soil
specific heat (J/kg-K), 12 monthly temperatures (°C, January to December)`.

| ID | Sev | Rule |
|---|---|---|
| H-GT-1 | E | N is a whole number ≥ 0. |
| H-GT-2 | E | Field count = 2 + 16·N. |
| H-GT-3 | E | Each depth is a number > 0. |
| H-GT-4 | — | Soil conductivity, density and specific heat may be empty. If given, they must be numbers > 0 (error). |
| H-GT-5 | E | All 12 monthly temperatures are numbers, −70 < T < 70. |
| H-GT-6 | W | Depths are not in increasing order. |

### 5.5 HOLIDAYS/DAYLIGHT SAVINGS

`HOLIDAYS/DAYLIGHT SAVINGS, leap year, daylight saving start, daylight saving end, N`, then for each holiday:
`name, day`.

| ID | Sev | Rule |
|---|---|---|
| H-HOL-1 | E | Leap year is `Yes` or `No` (case-insensitive). `Yes` means 29 February is part of the data in leap years (D-SEQ-5). |
| H-HOL-2 | E | Daylight-saving start and end are `0` (none) or valid date fields, including the weekday forms (§5.9). |
| H-HOL-3 | E | N is a whole number ≥ 0, and field count = 5 + 2·N. |
| H-HOL-4 | E | Each holiday name is not empty, and each holiday day is a valid date field. |

### 5.6 COMMENTS 1 / COMMENTS 2

`COMMENTS 1, text` and `COMMENTS 2, text`.

| ID | Sev | Rule |
|---|---|---|
| H-COM-1 | E | The comment text is missing or empty. The text is otherwise free: it may be quoted and may contain commas. |

### 5.7 DATA PERIODS

`DATA PERIODS, P, R`, then for each period: `name, start day of week, start day, end day`.
P is the number of periods, R the number of records per hour.

| ID | Sev | Rule |
|---|---|---|
| H-DP-1 | E | P is a whole number ≥ 1. |
| H-DP-2 | E | R is one of 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60 (so that 60/R is a whole number of minutes). |
| H-DP-3 | E | Field count = 3 + 4·P. |
| H-DP-4 | E | A period name is empty. |
| H-DP-5 | E | The start day of week is a weekday name: Sunday … Saturday, or its first three letters (case-insensitive). It is **not** compared with the real calendar, because typical-year files use made-up weekdays. |
| H-DP-6 | E | Start and end days are valid date fields. Only `m/d`, `m/d/y`, day of year, `<day> Month` and `Month <day>` are allowed here (no weekday forms). |
| H-DP-7 | E | Within one period the end is not before the start, and either both dates have a year or neither does. A period cannot wrap past 31 December: data running from December into January must be split into two periods, or use `m/d/y` dates. |
| H-DP-8 | E | Periods do not overlap. Periods without years are compared by month and day; periods with years by the real date. |
| H-DP-9 | W | Periods are not listed in time order. |

### 5.8 Trailing fields

| ID | Sev | Rule |
|---|---|---|
| H-TRAIL | W | A header record has extra empty fields at the end. They are ignored for the field counts. |

### 5.9 Date fields in header records

Spaces inside a date are ignored (`8/ 2` = `8/2`).

| Form | Example | Meaning | Allowed in |
|---|---|---|---|
| `m/d` | `1/ 1`, `12/31` | month / day | all date fields |
| `m/d/y` | `1/1/2019` | month / day / year | DATA PERIODS only |
| day of year | `32` | day 1–366 (non-leap calendar; 366 = 31 December) | all date fields |
| `<day> Month`, `Month <day>` | `5 Jan`, `January 5` | day and month | all date fields |
| `<n> Weekday in Month` | `2 Sunday in March` | n-th weekday (n = 1–5) of the month | holidays and daylight saving only |
| `Last Weekday in Month` | `Last Sunday in October` | last weekday of the month | holidays and daylight saving only |

Month and weekday names may be written in full or as their first three letters, in any case. The day must exist in
that month. 29 February is accepted in a date field; whether rows for it may exist is rule D-SEQ-5.

---

## 6. Data rows: structure (D-ROW)

Each data row has 35 fields:

| Idx | Field | Idx | Field |
|---|---|---|---|
| 0 | Year | 18 | Diffuse Horizontal Illuminance |
| 1 | Month | 19 | Zenith Luminance |
| 2 | Day | 20 | Wind Direction |
| 3 | Hour | 21 | Wind Speed |
| 4 | Minute | 22 | Total Sky Cover |
| 5 | Data Source and Uncertainty Flags | 23 | Opaque Sky Cover |
| 6 | Dry Bulb Temperature | 24 | Visibility |
| 7 | Dew Point Temperature | 25 | Ceiling Height |
| 8 | Relative Humidity | 26 | Present Weather Observation |
| 9 | Atmospheric Station Pressure | 27 | Present Weather Codes |
| 10 | Extraterrestrial Horizontal Radiation | 28 | Precipitable Water |
| 11 | Extraterrestrial Direct Normal Radiation | 29 | Aerosol Optical Depth |
| 12 | Horizontal Infrared Radiation Intensity | 30 | Snow Depth |
| 13 | Global Horizontal Radiation | 31 | Days Since Last Snowfall |
| 14 | Direct Normal Radiation | 32 | Albedo |
| 15 | Diffuse Horizontal Radiation | 33 | Liquid Precipitation Depth |
| 16 | Global Horizontal Illuminance | 34 | Liquid Precipitation Quantity |
| 17 | Direct Normal Illuminance | | |

Hour 1 is the period from 00:00 to 01:00; hour 24 ends at midnight.

| ID | Sev | Rule |
|---|---|---|
| D-ROW-1 | E | The row does not have exactly 35 fields. |
| D-ROW-2 | E | Fields 0–4 (year, month, day, hour, minute) are not whole numbers. |
| D-ROW-3 | E | Fields 6–26 and 28–34 are not plain decimal numbers (optional sign, decimal point `.`, optional exponent). Empty, `NaN`, `Inf` and hex are errors. |
| D-ROW-4 | E | Field 27 (Present Weather Codes) is not exactly 9 digits. |
| D-ROW-5 | E | Field 26 (Present Weather Observation) is not `0` (observation made) or `9` (not made). |
| D-ROW-6 | W | Field 5 (flags) does not have the classic form: 44 characters, 22 pairs of a source letter (`A`–`I` or `?`) and an uncertainty digit. This is never an error, because real files differ: TMYx files use `*` as well, and NORCERRA files use 43 characters with `_` and `*`. Reported once per file with a count. |
| D-ROW-7 | E | Field 5 (flags) is empty. |

## 7. Data rows: time sequence (D-SEQ)

These rules allow partial-year files. Rows are matched to the declared periods in order: all rows of period 1, then
all rows of period 2, and so on.

| ID | Sev | Rule |
|---|---|---|
| D-SEQ-1 | E | Month is not 1–12, the day does not exist in the month, or the hour is not 1–24. |
| D-SEQ-2 | E | Minute is wrong. If R = 1 (hourly), any whole number 0–60 is accepted (real files use 0, 30 and 60). If R > 1, the minutes inside each hour must run 60/R, 2·60/R, …, 60 (e.g. R = 4: 15, 30, 45, 60). |
| D-SEQ-3 | E | The rows do not follow the periods. The first row of each period must be its start date, hour 1, first time step. The last row must be its end date, hour 24, last time step. Each row must be exactly one time step after the previous one, with no gaps or repeats. After a row that does not follow, checking continues from that row, so one missing or extra row gives one error, not one per following row. |
| D-SEQ-4 | E | The year changes in the wrong place. In a period without years, the `Year` may change only when the month changes (this is how typical-year files mix years). In a period with `m/d/y` dates, the year must follow the calendar. |
| D-SEQ-5 | E | 29 February is wrong. Rows for 29 February may exist only if the leap-year field is `Yes` and the row's year is a leap year. If the field is `Yes`, 29 February must be present when the year is a leap year and the period covers that date. |
| D-SEQ-6 | E | The number of rows differs from what the periods declare. Reported once as "expected N rows, found M", with the number of rows after the last period if there are any. |
| D-SEQ-7 | W | The year is outside 1900–2100. |

## 8. Data rows: value ranges (D-RNG)

A value is **missing** if it is ≥ the column's missing code. Missing values are allowed (see §9). Every value that is
not missing must be inside its range; `>` and `<` are strict.

| Idx | Field (unit) | Valid range | Missing if ≥ |
|---|---|---|---|
| 6 | Dry Bulb Temperature (°C) | > −70 and < 70 | 99.9 |
| 7 | Dew Point Temperature (°C) | > −70 and < 70 | 99.9 |
| 8 | Relative Humidity (%) | 0–110 | 999 |
| 9 | Atmospheric Station Pressure (Pa) | > 31000 and < 120000 | 999999 |
| 10 | Extraterrestrial Horizontal Radiation (Wh/m²) | ≥ 0 | 9999 |
| 11 | Extraterrestrial Direct Normal Radiation (Wh/m²) | ≥ 0 | 9999 |
| 12 | Horizontal Infrared Radiation Intensity (Wh/m²) | ≥ 0 | 9999 |
| 13 | Global Horizontal Radiation (Wh/m²) | ≥ 0 | 9999 |
| 14 | Direct Normal Radiation (Wh/m²) | ≥ 0 | 9999 |
| 15 | Diffuse Horizontal Radiation (Wh/m²) | ≥ 0 | 9999 |
| 16 | Global Horizontal Illuminance (lux) | ≥ 0 | 999900 |
| 17 | Direct Normal Illuminance (lux) | ≥ 0 | 999900 |
| 18 | Diffuse Horizontal Illuminance (lux) | ≥ 0 | 999900 |
| 19 | Zenith Luminance (Cd/m²) | ≥ 0 | 9999 |
| 20 | Wind Direction (degrees; 0 = North, 90 = East) | 0–360 | 999 |
| 21 | Wind Speed (m/s) | 0–40 | 999 |
| 22 | Total Sky Cover (tenths) | 0–10 | 99 |
| 23 | Opaque Sky Cover (tenths) | 0–10 | 99 |
| 24 | Visibility (km) | any number | 9999 |
| 25 | Ceiling Height (m) | any number | 99999 |
| 28 | Precipitable Water (mm) | any number | 999 |
| 29 | Aerosol Optical Depth | any number | 0.999 |
| 30 | Snow Depth (cm) | any number | 999 |
| 31 | Days Since Last Snowfall | any number | 99 |
| 32 | Albedo | any number | 999 |
| 33 | Liquid Precipitation Depth (mm) | any number | 999 |
| 34 | Liquid Precipitation Quantity (hr) | any number | 99 |

| ID | Sev | Rule |
|---|---|---|
| D-RNG-1 | E | A value that is not missing is outside its range in the table above. |
| D-RNG-2 | W | A value in fields 24, 25 or 28–34 is negative. These fields have no minimum, but a negative value is almost certainly wrong. |

## 9. Data rows: missing data and consistency (D-MIS, D-CON)

The **core** columns are dry bulb (6), relative humidity (8), station pressure (9), global horizontal (13), direct
normal (14), diffuse horizontal (15), wind direction (20) and wind speed (21).

| ID | Sev | Rule |
|---|---|---|
| D-MIS-1 | E | A core column is missing on every row. A file without it cannot be used for simulation. |
| D-MIS-2 | W | A core column has some missing values. The count and the first line are reported. Simulation tools fill these in, but many viewers plot missing codes (99.9, 999, …) as real values. |
| D-MIS-3 | W | A non-core column is missing on every row. This is normal for some sources: NORCERRA files, for example, have no extraterrestrial radiation or illuminance. |
| D-CON-1 | W | Dew point is more than 0.5 °C above dry bulb. |
| D-CON-2 | W | Relative humidity is above 100 % (allowed up to 110). |

A night-time radiation check (global horizontal > 0 while extraterrestrial horizontal = 0) was tried and dropped. It
fires hundreds of times per real file, in sunrise and sunset hours whose values are correct.

---

## 10. Where real files differ from the classic EPW definition

The classic EPW data dictionary is stricter than the files in use today. The validator follows real files on these
points:

| # | Classic definition | Real files | Validator |
|---|---|---|---|
| 1 | Minute 1–60 | Typical-year files use `0`; some use `30` | Accepts 0–60 for hourly files (D-SEQ-2) |
| 2 | Keyword `HOLIDAYS/DAYLIGHT SAVING` | Most files use `HOLIDAYS/DAYLIGHT SAVINGS` | Accepts both (H-ORD-2) |
| 3 | Flags: source letters A–I or `?`, 44 characters | `*` and `_` are common; NORCERRA uses 43 characters | Warning only (D-ROW-6) |
| 4 | "Repeat (A1–A3)" for typical/extreme periods | 4 fields per period | 4 fields per period (H-TEP-2) |
| 5 | Usually a full year | Partial files are useful too | Any length that matches DATA PERIODS (§7) |
| 6 | Design-condition values are numbers | ASHRAE writes `N/A` for "not available" | Accepts `N/A`, rejects empty (H-DC-4) |

---

## 11. Profiles (S)

A profile is an **opt-in** set of extra rules for one consumer of the file. Without a profile the validator answers
only the question in §0 — is this a correct EPW file — and a report is exactly what §1–§10 describe. Choosing a
profile adds its rules on top; it never removes or weakens an EPW rule, and it never changes the default report.

Profiles exist because "a correct EPW" and "usable by a given tool" are different questions. A one-week file is a
correct EPW (§0) and no tool can run a year from it. Reporting that as an EPW error would be wrong; saying nothing
leaves the caller to find out later. A profile lets the one validator answer both, without the default answer
drifting.

Profile rule IDs start with `S-`. A profile may report something the EPW rules allow, and may restate as an error
something they report as a warning. It must not report the same column twice: where a profile rule covers what an
EPW warning covers, the profile rule replaces it for the columns it applies to.

### 11.1 `simien` — usable as a SIMIEN climate file

SIMIEN's hourly climate file is a 12-column CSV holding one full year: day of year, hour, solar altitude, solar
azimuth, dry bulb, relative humidity, global horizontal, direct normal and diffuse horizontal radiation, wind
direction, wind speed and albedo. Solar altitude and azimuth are computed from the `LOCATION` coordinates, so the
EPW does not carry them. The CSV has no missing-value codes: every cell is read as a real number.

The columns the profile reads are dry bulb (6), relative humidity (8), global horizontal (13), direct normal (14),
diffuse horizontal (15), wind direction (20), wind speed (21) and albedo (32). Station pressure (9) is a core column
in §9 but is **not** read here, so a file without it is still usable.

| ID | Sev | Rule |
|---|---|---|
| S-COL-1 | E | A column the profile reads, other than albedo, has missing values on any row. The count and the first line are reported. Replaces D-MIS-1 and D-MIS-2 for those columns: the target format cannot carry a missing code, so a missing hour would be read as a real value — 9999 W/m² of radiation, say, in a column whose range is 0–1400. |
| S-ROWS-1 | E | The file does not hold 8760 data rows. A SIMIEN climate file is one full year, hour by hour. |
| S-RATE-1 | E | `DATA PERIODS` declares more than one record per hour. The target format is hourly, and EPW radiation is Wh/m² per record, which equals W/m² only at one record per hour. |
| S-LEAP-1 | E | The data holds rows for 29 February. The target format numbers days 1–365. |
| S-ALB-1 | W | Albedo (32) has missing values. Albedo is a ground property and the standard SIMIEN station files carry one constant for the whole year, so a missing value is filled with 0.20 rather than refused. The count and the first line are reported. |

`S-ROWS-1` counts rows, not periods: a file whose `DATA PERIODS` declares two half-years passes if the rows add up
to 8760 and the sequence rules (§7) are satisfied.

---

## 12. Tests

The Go version is the reference. Its tests check the points below and record its exact report for every test file
in `testdata/expected.json`, and its text output for the broken files in `testdata/broken_results.txt`. The Python,
Node and C# tests must reproduce both files exactly, so all four versions agree on every rule, message, line number
and count.

The Go tests (`cd go && go test ./...`) check:

1. **Real files** in `EPW_format/epw_examples/` (13 files) are valid. They include:
   - a typical-year file (TMYx) with 2025 ASHRAE design conditions;
   - a reanalysis file (NORCERRA) with columns missing on every row;
   - minute `30` in every row;
   - 2021 ASHRAE design conditions with the empty field after the source;
   - `N/A` design-condition values.
2. **Partial files** in `testdata/partial/` are valid, with exactly the declared number of rows. They cover:
   - one day, one week, one month, and three months;
   - two periods from different years, and four representative months from four years;
   - a leap year with 29 February, and the leap flag `Yes` in a year that is not a leap year;
   - 15-minute data;
   - `m/d/y` dates, including a period across New Year;
   - day-of-year and month-name dates;
   - LF line endings, trailing blank lines, the `SAVING` keyword, and holiday/daylight-saving dates.
3. **Broken files** in `testdata/broken/` are invalid. Each is named `<RULE>__<description>.epw` and must produce
   that error. The validator's output for all of them is in `testdata/broken_results.txt`.
4. **Header completeness:** every header row is deleted, cut short at every field, and blanked field by field; every
   change must make the file invalid (§5).
5. **No cascades:** a deleted, duplicated or swapped row gives at most a few sequence errors.
6. **Warnings:** each warning rule is triggered by a small change and leaves the file valid.
7. **Unit tests** for date parsing and number parsing.

Files added to `testdata/partial/` or `testdata/broken/` are picked up by name. Setting `EPWVALIDATE_SAMPLES` to a
folder also checks every `.epw` file below it.

When a rule changes: update this document, change the Go version, regenerate the test files and expected results
with `cd go && go test -run 'TestFixtures|TestExpected' -update`, and make the other three versions pass again.

## 13. Open questions

1. **Core columns (D-MIS-1).** Is the list right? Should relative humidity *or* dew point be enough, since one can be
   calculated from the other?
2. **Sub-hourly files (R > 1).** They are supported. Should they be rejected if no producer is expected to make
   them?
3. **Year changes (D-SEQ-4).** Should the year be allowed to change at any day boundary (e.g. for representative
   weeks), not only between months?
4. **Strict mode.** Should there be an option where warnings also fail? §11 adds an options mechanism a strict
   mode could reuse.
5. **More profiles (§11).** Only `simien` exists. Is the shape right for a second consumer?
