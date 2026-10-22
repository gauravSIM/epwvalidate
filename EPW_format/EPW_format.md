# EPW weather file format

## Example: first 12 lines of an EPW file

From `NOR_AK_Oslo.AP-Gardermoen.013840_TMYx.epw`. Lines 1–8 are the header records; lines 9–12 are the first four hourly data records.

```
LOCATION,Oslo.AP-Gardermoen,AK,NOR,SRC-TMYx,013840,60.20640,11.08000,1.0,207.6
DESIGN CONDITIONS,1,2025 ASHRAE Handbook -- Fundamentals - Chapter 14 Climatic Design Information,Heating,1,-17.5,-15.3,-19.5,0.7,-17.2,-17.2,0.9,-14.7,10.8,3.8,9.4,2.8,1.3,20,0.627,Cooling,7,9.0,26.3,16.6,24.5,16.0,22.5,15.1,18.2,23.4,17.2,22.1,16.3,20.8,3.5,200,16.4,11.9,19.1,15.5,11.3,18.6,14.6,10.6,17.8,51.8,23.3,48.9,22.0,46.4,20.7,21.5,Extremes,9.2,7.9,6.9,-20.4,28.4,3.4,1.8,-22.9,29.8,-24.9,30.8,-26.8,31.8,-29.3,33.2,-20.7,19.5,3.4,1.1,-23.1,20.3,-25.1,20.9,-27.0,21.5,-29.4,22.2
TYPICAL/EXTREME PERIODS,6,Summer - Week Nearest Max Temperature For Period,Extreme,7/27,8/ 2,Summer - Week Nearest Average Temperature For Period,Typical,8/17,8/23,Winter - Week Nearest Min Temperature For Period,Extreme,12/22,1/ 5,Winter - Week Nearest Average Temperature For Period,Typical,2/10,2/16,Autumn - Week Nearest Average Temperature For Period,Typical,10/27,11/ 2,Spring - Week Nearest Average Temperature For Period,Typical,4/ 5,4/11
GROUND TEMPERATURES,3,.5,,,,-4.18,-5.06,-3.50,-1.05,5.35,10.29,13.62,14.64,12.91,9.07,3.95,-0.81,2,,,,-0.93,-2.50,-2.18,-0.88,3.40,7.27,10.37,12.03,11.68,9.50,5.97,2.23,4,,,,1.75,0.14,-0.21,0.28,2.70,5.31,7.70,9.36,9.75,8.81,6.74,4.20
HOLIDAYS/DAYLIGHT SAVINGS,No,0,0,0
COMMENTS 1,"NCEI ISD/ERA5 - #years=[65] Period of Record=1940-2023; Jan=2003; Feb=2012; Mar=1994; Apr=2010; May=1975; Jun=1978; Jul=1995; Aug=1984; Sep=1960; Oct=1990; Nov=2016; Dec=1982"
COMMENTS 2,"Downloaded from Climate.Onebuilding.org -- Ground temps represent undisturbed earth temperatures - calculated from this weather data."
DATA PERIODS,1,1,Data,Sunday,1/ 1,12/31
2003,1,1,1,0,?9?9?9?9E0?9?9?9?9?9?9?9?9?9?9?9?9?9*9*9?9*9,-22.10,-24.30,81,98174,0,0,190,0,0,0,0,0,0,0,360,1.00,3,3,7000.0,77777,9,999999999,1,0.0850,51,88,0.400,0.0,0.0
2003,1,1,2,0,?9?9?9?9E0?9?9?9?9?9?9?9?9?9?9?9?9?9*9*9?9*9,-22.60,-24.90,80,98101,0,0,186,0,0,0,0,0,0,0,30,1.00,3,3,6000.0,77777,9,999999999,1,0.0850,51,88,0.400,0.0,0.0
2003,1,1,3,0,?9?9?9?9E0?9?9?9?9?9?9?9?9?9?9?9?9?9*9*9?9*9,-23.20,-25.60,79,98065,0,0,183,0,0,0,0,0,0,0,360,1.00,3,3,7000.0,77777,9,999999999,1,0.0850,51,88,0.400,0.0,0.0
2003,1,1,4,0,?9?9?9?9E0?9?9?9?9?9?9?9?9?9?9?9?9?9*9*9?9*9,-23.40,-25.80,79,97985,0,0,183,0,0,0,0,0,0,0,30,1.00,5,5,7000.0,77777,9,999999999,1,0.0850,51,88,0.400,0.0,0.0
```

Each data row (lines 9–12) has 35 comma-separated fields. The field-by-field summary table (index, name, units,
min, max, missing value) is in §3, also as CSV.

Source: EnergyPlus 26.1 *Auxiliary Programs*, Weather Converter chapter:

- <https://bigladdersoftware.com/epx/docs/26-1/auxiliary-programs/auxiliary-programs.html#energyplus-weather-file-epw-data-dictionary>
- <https://bigladdersoftware.com/epx/docs/26-1/auxiliary-programs/auxiliary-programs.html#data-sourcesuncertainty>

Any EPW file that is made should open and display correctly in the EPW viewer at
<https://klimadataforbygninger.no/EPW_viewer.html>.

---

## 1. File shape

- Plain text, comma-separated. **Semicolons do not end lines.**
- 8 header records, in this order, followed by the data records:

| # | Record keyword | Content |
|---|---|---|
| 1 | `LOCATION` | city, region, country, source, WMO, latitude, longitude, time zone, elevation |
| 2 | `DESIGN CONDITIONS` | design conditions matched by WMO number |
| 3 | `TYPICAL/EXTREME PERIODS` | typical and extreme periods found by the Weather Converter's heuristic |
| 4 | `GROUND TEMPERATURES` | monthly "undisturbed" ground temperatures at N depths |
| 5 | `HOLIDAYS/DAYLIGHT SAVING` | leap year flag, daylight saving start and end, holidays |
| 6 | `COMMENTS 1` | free text |
| 7 | `COMMENTS 2` | free text |
| 8 | `DATA PERIODS` | number of periods, records per hour, each period's name, start weekday, start and end |

### IDD conventions used in the dictionary

| Tag | Meaning |
|---|---|
| `\minimum` / `\minimum>` | value must be ≥ / > the number |
| `\maximum` / `\maximum<` | value must be ≤ / < the number |
| `\missing` | a value **≥ this number** counts as "missing", and the missing-data rules apply |
| `\default` | a blank field gets this value |
| `\units` | expected units |

### Date fields in header records

```
!ESP(r)/EnergyPlus Weather Format
!April 2002
\memo Dates in the EPW file can be several formats:
\memo <number>/<number>  (month/day)
\memo <number> Month
\memo Month <number>
\memo <number> (taken to be Julian day of year)
\memo Months are January, February, March, April, May,
\memo            June, July, August, September, October, November, December
\memo Months can be the first 3 letters of the month
```

| Field contents | Meaning | Allowed in |
|---|---|---|
| `<number>` | Julian day of year | every date field |
| `<number>/<number>` | month / day | every date field |
| `<number>/<number>/<number>` | month / day / year | DataPeriod only (special multiple-year file; see `RunPeriod:CustomRange`) |
| `<number> Month` | day and month | every date field |
| `Month <number>` | day and month | every date field |
| `<number> Weekday in Month` | numbered weekday of month | Holiday, DaylightSavingPeriod |
| `Last Weekday In Month` | last weekday of month | Holiday, DaylightSavingPeriod |

Month = January … December, or the first three letters. Weekday = Sunday … Saturday, or the first three letters.

---

## 2. Header records

### 2.1 LOCATION

Example: line 1 of the example file.

```
LOCATION,
   A1, \field city
   \type alpha
   A2, \field State Province Region
   \type alpha
   A3, \field Country
   \type alpha
   A4, \field Source
   \type alpha
   N1, \field WMO
    \note usually a 6 digit field. Used as alpha in EnergyPlus
    \type alpha
   N2 , \field Latitude
       \units deg
       \minimum -90.0
       \maximum +90.0
       \default 0.0
       \note + is North, - is South, degree minutes represented in decimal (i.e. 30 minutes is .5)
       \type real
   N3 , \field Longitude
       \units deg
       \minimum -180.0
       \maximum +180.0
       \default 0.0
       \note - is West, + is East, degree minutes represented in decimal (i.e. 30 minutes is .5)
       \type real
   N4 , \field TimeZone
       \units hr - not on standard units list???
       \minimum -12.0
       \maximum +12.0
       \default 0.0
       \note  Time relative to GMT.
       \type real
   N5 ; \field Elevation
       \units m
       \minimum -1000.0
       \maximum< +9999.9
       \default 0.0
       \type real
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
A1,City,,,,,
A2,State Province Region,,,,,
A3,Country,,,,,
A4,Source,,,,,
N1,WMO,,,,,usually a 6 digit field. Used as alpha in EnergyPlus
N2,Latitude,deg,-90.0,+90.0,0.0,"+ is North, - is South, degree minutes represented in decimal (i.e. 30 minutes is .5)"
N3,Longitude,deg,-180.0,+180.0,0.0,"- is West, + is East, degree minutes represented in decimal (i.e. 30 minutes is .5)"
N4,TimeZone,hr,-12.0,+12.0,0.0,Time relative to GMT
N5,Elevation,m,-1000.0,< +9999.9,0.0,
```

The LOCATION record duplicates the information required for the Location object.

- With only a Run Period object (a weather file), the Location object is not needed.
- With a Run Period and Design Day objects, the weather file's location is used, and it overrides any Location object.

### 2.2 DESIGN CONDITIONS

Example: line 2 of the example file.

```
DESIGN CONDITIONS,
     N1, \field Number of Design Conditions
     A1, \field Design Condition Source
         \note current sources are ASHRAE HOF 2009 US Design Conditions, Canadian Design Conditions
         \note and World Design Conditions
     A2, \field Design Condition Type (HEATING)
       \note fields here will be dependent on the source, they are shown in a header/data format
       \note in both the .rpt and .csv files that are produced by the WeatherConverter program
     ...
     An, \field Design Condition Type (COOLING)
       \note same as note on Heating Design Conditions
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
N1,Number of Design Conditions,,,,,
A1,Design Condition Source,,,,,"current sources are ASHRAE HOF 2009 US Design Conditions, Canadian Design Conditions and World Design Conditions"
A2,Design Condition Type (HEATING),,,,,"fields here will be dependent on the source, they are shown in a header/data format in both the .rpt and .csv files that are produced by the WeatherConverter program"
...,,,,,,
An,Design Condition Type (COOLING),,,,,same as note on Heating Design Conditions
```

The design conditions are matched to the location by WMO number (World Meteorological Organization station number).
The weather files carry the design conditions from the ASHRAE Handbook of Fundamentals 2009. Design Day definition
files exist for all World, Canada and United States design conditions.

### 2.3 TYPICAL/EXTREME PERIODS

Example: line 3 of the example file.

```
TYPICAL/EXTREME PERIODS,
     N1, \field Number of Typical/Extreme Periods
     A1, \field Typical/Extreme Period 1 Name
     A2, \field Typical/Extreme Period 1 Type
     A3, \field Period 1 Start Day
     A4, \field Period 1 End Day
     \note repeat (A1-A3) until number of typical periods
 -- etc to # of periods entered
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
N1,Number of Typical/Extreme Periods,,,,,
A1,Typical/Extreme Period 1 Name,,,,,
A2,Typical/Extreme Period 1 Type,,,,,
A3,Period 1 Start Day,,,,,date field
A4,Period 1 End Day,,,,,date field
...,etc to # of periods entered,,,,,repeat (A1-A3) until number of typical periods
```

The dictionary's note says "repeat (A1-A3) until number of typical periods". The record defines four fields per period
(A1–A4).

The Weather Converter finds typical and extreme periods for full-year files with a heuristic method. They are also
reported in the Weather Converter's statistics report.

### 2.4 GROUND TEMPERATURES

Example: line 4 of the example file.

```
GROUND TEMPERATURES,
       N1, Number of Ground Temperature Depths
       N2, \field Ground Temperature Depth 1
         \units m
       N3, \field Depth 1 Soil Conductivity
         \units W/m-K,
       N4, \field Depth 1 Soil Density
         \units kg/m3
       N5, \field Depth 1 Soil Specific Heat
         \units J/kg-K,
       N6, \field Depth 1 January Average Ground Temperature
        \units C
       N7, \field Depth 1 February Average Ground Temperature
        \units C
       N8, \field Depth 1 March Average Ground Temperature
        \units C
       N9, \field Depth 1 April Average Ground Temperature
        \units C
       N10, \field Depth 1 May Average Ground Temperature
        \units C
       N11, \field Depth 1 June Average Ground Temperature
        \units C
       N12, \field Depth 1 July Average Ground Temperature
        \units C
       N13, \field Depth 1 August Average Ground Temperature
        \units C
       N14, \field Depth 1 September Average Ground Temperature
        \units C
       N15, \field Depth 1 October Average Ground Temperature
        \units C
       N16, \field Depth 1 November Average Ground Temperature
        \units C
       N17, \field Depth 1 December Average Ground Temperature
        \units C
      \note repeat above (N2-N17) to number of ground temp depths indicated
-- etc to # of depths entered
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
N1,Number of Ground Temperature Depths,,,,,
N2,Ground Temperature Depth 1,m,,,,
N3,Depth 1 Soil Conductivity,W/m-K,,,,
N4,Depth 1 Soil Density,kg/m3,,,,
N5,Depth 1 Soil Specific Heat,J/kg-K,,,,
N6,Depth 1 January Average Ground Temperature,C,,,,
N7,Depth 1 February Average Ground Temperature,C,,,,
N8,Depth 1 March Average Ground Temperature,C,,,,
N9,Depth 1 April Average Ground Temperature,C,,,,
N10,Depth 1 May Average Ground Temperature,C,,,,
N11,Depth 1 June Average Ground Temperature,C,,,,
N12,Depth 1 July Average Ground Temperature,C,,,,
N13,Depth 1 August Average Ground Temperature,C,,,,
N14,Depth 1 September Average Ground Temperature,C,,,,
N15,Depth 1 October Average Ground Temperature,C,,,,
N16,Depth 1 November Average Ground Temperature,C,,,,
N17,Depth 1 December Average Ground Temperature,C,,,,
...,etc to # of depths entered,,,,,repeat above (N2-N17) to number of ground temp depths indicated
```

- The Weather Converter calculates **"undisturbed"** ground temperatures from a full year of data. The calculation is
  a heuristic, time-lagged one based on dry bulb temperature and location (Kusuda).
- Soil properties cannot be calculated from weather observations, so the record is mainly for user information.
- With the FC construction option, the 0.5 m depth values are used automatically if the user gives no values in
  `Site:GroundTemperature:FcfactorMethod`.
- These temperatures **should not be used for building losses**: they are too extreme for soil under a conditioned
  building. They are appropriate for `GroundTemperatures:Surface` and `GroundTemperatures:Deep`.
- For best results, use the Slab or Basement program to calculate monthly ground temperatures.
- Without those preprocessors, a reasonable default for typical commercial buildings in the USA is 2 °C below the
  average indoor space temperature.

### 2.5 HOLIDAYS/DAYLIGHT SAVING

Example: line 5 of the example file.

```
HOLIDAYS/DAYLIGHT SAVING,
     A1, \field LeapYear Observed
      \type choice
      \key Yes
      \key No
      \note Yes if Leap Year will be observed for this file
      \note No if Leap Year days (29 Feb) should be ignored in this file
     A2, \field Daylight Saving Start Day
     A3, \field Daylight Saving End Day
     N1, \field Number of Holidays (essentially unlimited)
     A4, \field Holiday 1 Name
     A5, \field Holiday 1 Day
     \note repeat above two fields until Number of Holidays is reached
-- etc to # of Holidays entered
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
A1,LeapYear Observed,,,,,"choice: Yes, No. Yes if Leap Year will be observed for this file; No if Leap Year days (29 Feb) should be ignored in this file"
A2,Daylight Saving Start Day,,,,,date field
A3,Daylight Saving End Day,,,,,date field
N1,Number of Holidays,,,,,essentially unlimited
A4,Holiday 1 Name,,,,,
A5,Holiday 1 Day,,,,,date field
...,etc to # of Holidays entered,,,,,repeat above two fields until Number of Holidays is reached
```

- These values are used when the matching fields of the RunPeriod object are set to "Yes".
- Weather files on the EnergyPlus web site have no special days and no daylight saving period.
- Special Day Period and Daylight Saving Period objects in the input file can add more of these.

### 2.6 COMMENTS 1 / COMMENTS 2

Example: lines 6 and 7 of the example file.

```
COMMENTS 1, A1 \field Comments_1
COMMENTS 2, A1 \field Comments_2
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
A1,Comments_1,,,,,record COMMENTS 1
A1,Comments_2,,,,,record COMMENTS 2
```

Additional information about the weather data source, or anything that does not fit the other header records.

### 2.7 DATA PERIODS

Example: line 8 of the example file.

```
DATA PERIODS,
     N1, \field Number of Data Periods
     N2, \field Number of Records per hour
     A1, \field Data Period 1 Name/Description
     A2, \field Data Period 1 Start Day of Week
       \type choice
       \key  Sunday
       \key  Monday
       \key  Tuesday
       \key  Wednesday
       \key  Thursday
       \key  Friday
       \key  Saturday
     A3, \field Data Period 1 Start Day
     A4, \field Data Period 1 End Day
     \note repeat above to number of data periods
-- etc to # of periods entered
```

The same fields as CSV (comma-delimited):

```csv
Fld,Name,Units,Min,Max,Default,Note
N1,Number of Data Periods,,,,,
N2,Number of Records per hour,,,,,
A1,Data Period 1 Name/Description,,,,,
A2,Data Period 1 Start Day of Week,,,,,"choice: Sunday, Monday, Tuesday, Wednesday, Thursday, Friday, Saturday"
A3,Data Period 1 Start Day,,,,,date field
A4,Data Period 1 End Day,,,,,date field
...,etc to # of periods entered,,,,,repeat above to number of data periods
```

- A file may contain several data periods. This is not required, and it may be detrimental.
- A file may contain several records per hour, but the count **must match the Number of Time Steps in Hour** of the
  simulation. Interval data gives the exact weather values for each time step, instead of interpolated values.
- A file may contain several consecutive years. EnergyPlus processes the extra years when the RunPeriod's Number of
  Years field is used. There is no way to start in a year in the middle of the file.
- A Run Period may not cross a data-period boundary.

---

## 3. Data records: 35 fields

Example: lines 9–12 of the example file (one row per hour).

```
! Actual data does not have a descriptor
     N1, \field Year
     N2, \field Month
     N3, \field Day
     N4, \field Hour
     N5, \field Minute
     A1, \field Data Source and Uncertainty Flags
     \note Initial day of weather file is checked by EnergyPlus for validity (as shown below)
     \note Each field is checked for "missing" as shown below. Reasonable values, calculated
     \note values or the last "good" value is substituted.
     N6, \field Dry Bulb Temperature
        \units C
        \minimum> -70
        \maximum< 70
        \missing 99.9
     N7, \field Dew Point Temperature
        \units C
        \minimum> -70
        \maximum< 70
        \missing 99.9
     N8, \field Relative Humidity
         \missing 999.
         \minimum 0
         \maximum 110
     N9, \field Atmospheric Station Pressure
        \units Pa
        \missing 999999.
        \minimum> 31000
        \maximum< 120000
     N10, \field Extraterrestrial Horizontal Radiation
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N11, \field Extraterrestrial Direct Normal Radiation
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N12, \field Horizontal Infrared Radiation Intensity
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N13, \field Global Horizontal Radiation
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N14, \field Direct Normal Radiation
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N15, \field Diffuse Horizontal Radiation
        \units Wh/m2
        \missing 9999.
        \minimum 0
     N16, \field Global Horizontal Illuminance
        \units lux
        \missing 999999.
        \note will be missing if > = 999900
        \minimum 0
     N17, \field Direct Normal Illuminance
        \units lux
        \missing 999999.
        \note will be missing if > = 999900
        \minimum 0
     N18, \field Diffuse Horizontal Illuminance
        \units lux
        \missing 999999.
        \note will be missing if > = 999900
        \minimum 0
     N19, \field Zenith Luminance
        \units Cd/m2
        \missing 9999.
        \note will be missing if > = 9999
        \minimum 0
     N20, \field Wind Direction
        \units degrees
        \missing 999.
        \minimum 0
        \maximum 360
     N21, \field Wind Speed
        \units m/s
        \missing 999.
        \minimum 0
        \maximum 40
     N22, \field Total Sky Cover
        \missing 99
        \minimum 0
        \maximum 10
     N23, \field Opaque Sky Cover (used if Horizontal IR Intensity missing)
        \missing 99
        \minimum 0
        \maximum 10
     N24, \field Visibility
        \units km
        \missing 9999
     N25, \field Ceiling Height
        \units m
        \missing 99999
     N26, \field Present Weather Observation
     N27, \field Present Weather Codes
     N28, \field Precipitable Water
        \units mm
        \missing 999
     N29, \field Aerosol Optical Depth
        \units thousandths
        \missing .999
     N30, \field Snow Depth
        \units cm
        \missing 999
     N31, \field Days Since Last Snowfall
        \missing 99
     N32, \field Albedo
        \missing 999
     N33, \field Liquid Precipitation Depth
          \units mm
        \missing 999
     N34; \field Liquid Precipitation Quantity
          \units hr
        \missing 99
```

Summary of the same fields:

| Idx | Fld | Name | Units | Min | Max | Missing (≥) |
|---|---|---|---|---|---|---|
| 0 | N1 | Year |  |  |  |  |
| 1 | N2 | Month |  | 1 | 12 | cannot be missing |
| 2 | N3 | Day |  | 1 | depends on month | cannot be missing |
| 3 | N4 | Hour |  | 1 | 24 | cannot be missing |
| 4 | N5 | Minute |  | 1 | 60 |  |
| 5 | A1 | Data Source and Uncertainty Flags | text |  |  |  |
| 6 | N6 | Dry Bulb Temperature | C | > −70 | < 70 | 99.9 |
| 7 | N7 | Dew Point Temperature | C | > −70 | < 70 | 99.9 |
| 8 | N8 | Relative Humidity | % | 0 | 110 | 999 |
| 9 | N9 | Atmospheric Station Pressure | Pa | > 31000 | < 120000 | 999999 |
| 10 | N10 | Extraterrestrial Horizontal Radiation | Wh/m2 | 0 |  | 9999 |
| 11 | N11 | Extraterrestrial Direct Normal Radiation | Wh/m2 | 0 |  | 9999 |
| 12 | N12 | Horizontal Infrared Radiation Intensity | Wh/m2 | 0 |  | 9999 |
| 13 | N13 | Global Horizontal Radiation | Wh/m2 | 0 |  | 9999 |
| 14 | N14 | Direct Normal Radiation | Wh/m2 | 0 |  | 9999 |
| 15 | N15 | Diffuse Horizontal Radiation | Wh/m2 | 0 |  | 9999 |
| 16 | N16 | Global Horizontal Illuminance | lux | 0 |  | 999999. Missing if ≥ 999900 |
| 17 | N17 | Direct Normal Illuminance | lux | 0 |  | 999999. Missing if ≥ 999900 |
| 18 | N18 | Diffuse Horizontal Illuminance | lux | 0 |  | 999999. Missing if ≥ 999900 |
| 19 | N19 | Zenith Luminance | Cd/m2 | 0 |  | 9999 |
| 20 | N20 | Wind Direction | degrees | 0 | 360 | 999 |
| 21 | N21 | Wind Speed | m/s | 0 | 40 | 999 |
| 22 | N22 | Total Sky Cover | tenths | 0 | 10 | 99 |
| 23 | N23 | Opaque Sky Cover | tenths | 0 | 10 | 99 |
| 24 | N24 | Visibility | km |  |  | 9999 |
| 25 | N25 | Ceiling Height | m |  |  | 99999 |
| 26 | N26 | Present Weather Observation |  |  |  |  |
| 27 | N27 | Present Weather Codes | 9-digit text |  |  |  |
| 28 | N28 | Precipitable Water | mm |  |  | 999 |
| 29 | N29 | Aerosol Optical Depth | thousandths |  |  | .999 |
| 30 | N30 | Snow Depth | cm |  |  | 999 |
| 31 | N31 | Days Since Last Snowfall |  |  |  | 99 |
| 32 | N32 | Albedo | ratio (unitless) |  |  | 999 |
| 33 | N33 | Liquid Precipitation Depth | mm |  |  | 999 |
| 34 | N34 | Liquid Precipitation Quantity | hr |  |  | 99 |

The same table as CSV (comma-delimited):

```csv
Idx,Fld,Name,Units,Min,Max,Missing (≥)
0,N1,Year,,,,
1,N2,Month,,1,12,cannot be missing
2,N3,Day,,1,depends on month,cannot be missing
3,N4,Hour,,1,24,cannot be missing
4,N5,Minute,,1,60,
5,A1,Data Source and Uncertainty Flags,text,,,
6,N6,Dry Bulb Temperature,C,> −70,< 70,99.9
7,N7,Dew Point Temperature,C,> −70,< 70,99.9
8,N8,Relative Humidity,%,0,110,999
9,N9,Atmospheric Station Pressure,Pa,> 31000,< 120000,999999
10,N10,Extraterrestrial Horizontal Radiation,Wh/m2,0,,9999
11,N11,Extraterrestrial Direct Normal Radiation,Wh/m2,0,,9999
12,N12,Horizontal Infrared Radiation Intensity,Wh/m2,0,,9999
13,N13,Global Horizontal Radiation,Wh/m2,0,,9999
14,N14,Direct Normal Radiation,Wh/m2,0,,9999
15,N15,Diffuse Horizontal Radiation,Wh/m2,0,,9999
16,N16,Global Horizontal Illuminance,lux,0,,999999. Missing if ≥ 999900
17,N17,Direct Normal Illuminance,lux,0,,999999. Missing if ≥ 999900
18,N18,Diffuse Horizontal Illuminance,lux,0,,999999. Missing if ≥ 999900
19,N19,Zenith Luminance,Cd/m2,0,,9999
20,N20,Wind Direction,degrees,0,360,999
21,N21,Wind Speed,m/s,0,40,999
22,N22,Total Sky Cover,tenths,0,10,99
23,N23,Opaque Sky Cover,tenths,0,10,99
24,N24,Visibility,km,,,9999
25,N25,Ceiling Height,m,,,99999
26,N26,Present Weather Observation,,,,
27,N27,Present Weather Codes,9-digit text,,,
28,N28,Precipitable Water,mm,,,999
29,N29,Aerosol Optical Depth,thousandths,,,.999
30,N30,Snow Depth,cm,,,999
31,N31,Days Since Last Snowfall,,,,99
32,N32,Albedo,ratio (unitless),,,999
33,N33,Liquid Precipitation Depth,mm,,,999
34,N34,Liquid Precipitation Quantity,hr,,,99
```

Notes from the dictionary:

- The initial day of the weather file is checked by EnergyPlus for validity.
- Each field is checked for "missing". In its place, EnergyPlus substitutes a reasonable value, a calculated value or
  the last "good" value.
- Hour 1 is 00:01 to 01:00.
- Dry bulb and dew point are full numeric fields (`23.6`), not integers in tenths.
- Station pressure limits come from the standard barometric pressure for all elevations of the world.
- Radiation and illuminance values are amounts received **during the number of minutes before** the time indicated.
- Illuminance is the average in hundreds of lux. Zenith luminance is the average in tens of Cd/m2.
- Wind direction: North = 0.0, East = 90.0, South = 180.0, West = 270.0. If calm, the direction is zero.
- Total sky cover: 1 means 1/10 covered and 10 means total coverage.
- Ceiling height: 77777 = unlimited ceiling height, 88888 = cirroform ceiling.
- Precipitable water is not rain. It is not used because its reporting is unreliable.
- The field descriptions are taken from the IWEC manual.

---

## 4. Horizontal infrared radiation when missing

When N12 is missing, it is calculated from the Opaque Sky Cover field (Walton 1983; Clark & Allen 1978):

```
Horizontal_IR = ε · σ · T_drybulb⁴        [W/m²], σ = 5.6697e-8 W/m²-K⁴, T_drybulb in K
ε = (0.787 + 0.764 · ln(T_dewpoint / 273)) · (1 + 0.0224 N − 0.0035 N² + 0.00028 N³)
T_dewpoint in K, N = opaque sky cover in tenths
```

Example: clear sky (N = 0), T_drybulb = 273 + 20 = 293 K, T_dewpoint = 273 + 10 = 283 K:
ε = 0.787 + 0.764 × 0.036 = 0.815, and Horizontal_IR = 0.815 × 5.6697e-8 × 293⁴ = 340.6 W/m².

References:

- Walton, G. N. 1983. *Thermal Analysis Research Program Reference Manual*. NBSSIR 83-2655. National Bureau of
  Standards, p. 21.
- Clark, G. and C. Allen. "The Estimation of Atmospheric Radiation for Clear and Cloudy Skies." *Proceedings 2nd
  National Passive Solar Conference (AS/ISES)*, 1978, pp. 675–678.

---

## 5. Data Source and Uncertainty Flags (field A1)

The EPW format puts all the source and uncertainty flags into one field. Each data element has a source flag and an
uncertainty flag, and its position in the string matches the element's place in the data record. Most sources use the
TMY2 fields and values.

| Data element | Source flag values | Uncertainty flag values |
|---|---|---|
| Dry Bulb Temperature | A–F | 0–9 |
| Dew Point Temperature | A–F | 0–9 |
| Relative Humidity | A–F | 0–9 |
| Atmospheric Station Pressure | A–F | 0–9 |
| Horizontal Infrared Radiation | A–H, ? | 0–9 |
| Global Horizontal Radiation | A–H, ? | 0–9 |
| Direct Normal Radiation | A–H, ? | 0–9 |
| Diffuse Horizontal Radiation | A–H, ? | 0–9 |
| Global Horizontal Illuminance | I, ? | 0–9 |
| Direct Normal Illuminance | I, ? | 0–9 |
| Diffuse Horizontal Illuminance | I, ? | 0–9 |
| Zenith Luminance | I, ? | 0–9 |
| Wind Direction | A–F | 0–9 |
| Wind Speed | A–F | 0–9 |
| Total Sky Cover | A–F | 0–9 |
| Opaque Sky Cover | A–F | 0–9 |
| Visibility | A–F, ? | 0–9 |
| Ceiling Height | A–F, ? | 0–9 |
| Precipitable Water | A–F | 0–9 |
| Broadband Aerosol Optical Depth | A–F | 0–9 |
| Snow Depth | A–F, ? | 0–9 |
| Days Since Last Snowfall | A–F, ? | 0–9 |

**Solar radiation and illuminance data source flags**

| Flag | Definition |
|---|---|
| A | Post-1976 measured solar radiation data as received from NCDC or other sources |
| B | Same as "A" except the global horizontal data underwent a calibration correction |
| C | Pre-1976 measured global horizontal data (direct and diffuse were not measured before 1976), adjusted from solar to local time, usually with a calibration correction |
| D | Data derived from the other two elements of solar radiation using the relationship global = diffuse + direct × cosine(zenith) |
| E | Modeled solar radiation data using inputs of observed sky cover (cloud amount) and aerosol optical depths derived from direct normal data collected at the same location |
| F | Modeled solar radiation data using interpolated sky cover and aerosol optical depths derived from direct normal data collected at the same location |
| G | Modeled solar radiation data using observed sky cover and aerosol optical depths estimated from geographical relationships |
| H | Modeled solar radiation data using interpolated sky cover and estimated aerosol optical depths |
| I | Modeled illuminance or luminance data derived from measured or modeled solar radiation data |
| ? | Source does not fit any of the above categories. Used for nighttime values and missing data |

**Solar radiation and illuminance data uncertainty flags**

| Flag | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 0 |
|---|---|---|---|---|---|---|---|---|---|---|
| Uncertainty range (%) | not used | 2–4 | 4–6 | 6–9 | 9–13 | 13–18 | 18–25 | 25–35 | 35–50 | not applicable |

**Meteorological data source flags**

| Flag | Definition |
|---|---|
| A | Data as received from NCDC, converted to SI units |
| B | Linearly interpolated |
| C | Non-linearly interpolated to fill data gaps from 6 to 47 hours in length |
| D | Not used |
| E | Modeled or estimated, except: precipitable water, calculated from radiosonde data; dew point temperature calculated from dry bulb temperature and relative humidity; and relative humidity calculated from dry bulb temperature and dew point temperature |
| F | Precipitable water, calculated from surface vapor pressure; aerosol optical depth, estimated from geographic correlation |
| ? | Source does not fit any of the above. Used mostly for missing data |

**Meteorological uncertainty flags**

| Flag | Definition |
|---|---|
| 1–6 | Not used |
| 7 | Uncertainty consistent with NWS practices and the instrument or observation used to obtain the data |
| 8 | Greater uncertainty than 7 because values were interpolated or estimated |
| 9 | Greater uncertainty than 8 or unknown |
| 0 | Not definable |

---

## 6. Rain and snow

**Present Weather Observation (N26)**

| Value | Meaning |
|---|---|
| 0 | Weather observation made. The weather codes are taken from the next field |
| 9 | Weather observation not made, or missing |

These two fields are used mainly for rain and wet surfaces. A missing observation or a missing weather code means
**no rain**.

**Present Weather Codes (N27)**

- They follow the TMY2 conventions. The field may look numeric, but it is **a text field of 9
  single digits**.
- Formats that do not follow this convention (for example TMY) are converted to it as far as possible by the
  Weather Converter.
- The most important columns are the ones for liquid precipitation, when the building surfaces would be wet.
- EnergyPlus uses Snow Depth, not these codes, to decide whether snow is on the ground.

In every column, **9** = none if the observation indicator is 0. If the indicator is 9, then 9 = unknown or missing.

| Col | Element | Values |
|---|---|---|
| 1 | Thunderstorm, tornado, or squall | 0 thunderstorm: lightning and thunder, wind gusts < 25.7 m/s, hail (if any) < 1.9 cm · 1 heavy or severe thunderstorm: frequent intense lightning and thunder, wind gusts > 25.7 m/s, hail (if any) ≥ 1.9 cm · 2 report of tornado or waterspout · 4 moderate squall: wind speed suddenly rises by at least 8.2 m/s, reaching 11.3 m/s or more, for at least 1 minute · 6 water spout (from Jan 1984) · 7 funnel cloud (from Jan 1984) · 8 tornado (from Jan 1984) |
| 2 | Rain, rain showers, or freezing rain | 0 light rain · 1 moderate rain · 2 heavy rain · 3 light rain showers · 4 moderate rain showers · 5 heavy rain showers · 6 light freezing rain · 7 moderate freezing rain · 8 heavy freezing rain. Light = up to 0.25 cm/h, moderate = 0.28–0.76 cm/h, heavy = more than 0.76 cm/h |
| 3 | Rain squalls, drizzle, or freezing drizzle | 0 light rain squalls · 1 moderate rain squalls · 3 light drizzle · 4 moderate drizzle · 5 heavy drizzle · 6 light freezing drizzle · 7 moderate freezing drizzle · 8 heavy freezing drizzle. With other weather: light = up to 0.025 cm/h, moderate = 0.025–0.051 cm/h, heavy = more than 0.051 cm/h. Alone: light = visibility ≥ 1 km, moderate = 0.5–1 km, heavy = ≤ 0.5 km |
| 4 | Snow, snow pellets, or ice crystals | 0 light snow · 1 moderate snow · 2 heavy snow · 3 light snow pellets · 4 moderate snow pellets · 5 heavy snow pellets · 6 light ice crystals · 7 moderate ice crystals · 8 heavy ice crystals. From April 1963, any ice crystals are recorded as 7 |
| 5 | Snow showers, snow squalls, or snow grains | 0 light snow · 1 moderate snow showers · 2 heavy snow showers · 3 light snow squall · 4 moderate snow squall · 5 heavy snow squall · 6 light snow grains · 7 moderate snow grains |
| 6 | Sleet, sleet showers, or hail | 0 light ice pellet showers · 1 moderate ice pellet showers · 2 heavy ice pellet showers · 4 hail. Before April 1970, ice pellets were coded as sleet. From April 1970, sleet and small hail are ice pellets, coded 0, 1 or 2 |
| 7 | Fog, blowing dust, or blowing sand | 0 fog · 1 ice fog · 2 ground fog · 3 blowing dust · 4 blowing sand · 5 heavy fog · 6 glaze (from 1984) · 7 heavy ice fog (from 1984) · 8 heavy ground fog (from 1984). Recorded only when visibility < 11 km |
| 8 | Smoke, haze, smoke and haze, blowing snow, blowing spray, or dust | 0 smoke · 1 haze · 2 smoke and haze · 3 dust · 4 blowing snow · 5 blowing spray · 6 dust storm (from 1984) · 7 volcanic ash. Recorded only when visibility < 11 km |
| 9 | Ice pellets | 0 light ice pellets · 1 moderate ice pellets · 2 heavy ice pellets |

Example: observation `0` with codes `929999999` means heavy rain in that data period. The period is usually an hour,
but it depends on the intervals-per-hour field in DATA PERIODS.

**Liquid Precipitation Depth (N33)**

- It is the liquid precipitation (mm) observed at the time indicated, over the period given in Liquid Precipitation
  Quantity (N34).
- If N33 is not missing, it is used and overrides the rain indicator (IsRain): rain is set when the hourly liquid
  precipitation is **no less than 0.8 mm**.
- If the rain indicator shows rain and N33 is missing or zero, N33 is set to **2.0 mm**.

**Snow Depth (N30)** tells EnergyPlus when snow is on the ground, which can change ground reflectance.
