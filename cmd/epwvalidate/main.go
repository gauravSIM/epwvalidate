// Command epwvalidate checks EPW weather files.
//
//	epwvalidate [--json] file.epw [more.epw ...]
//
// Exit code: 0 = all files valid, 1 = at least one file invalid,
// 2 = a file could not be read or no file was given.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"epwvalidate"
)

type fileReport struct {
	File string `json:"file"`
	epwvalidate.Report
}

func main() {
	asJSON := flag.Bool("json", false, "print the reports as JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: epwvalidate [--json] file.epw [more.epw ...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	exit := 0
	var reports []fileReport
	for _, path := range flag.Args() {
		rep, err := epwvalidate.ValidateFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exit = 2
			continue
		}
		if !rep.Valid && exit == 0 {
			exit = 1
		}
		if *asJSON {
			reports = append(reports, fileReport{File: path, Report: rep})
		} else {
			printReport(os.Stdout, path, rep)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exit = 2
		}
	}
	os.Exit(exit)
}

func printReport(w io.Writer, path string, rep epwvalidate.Report) {
	s := rep.Summary
	status := "VALID"
	if !rep.Valid {
		status = "INVALID"
	}
	fmt.Fprintf(w, "%s: %s - %d data rows", path, status, s.DataRows)
	if s.ExpectedRows >= 0 {
		fmt.Fprintf(w, " (DATA PERIODS declares %d)", s.ExpectedRows)
	}
	fmt.Fprintf(w, ", %d errors, %d warnings\n", s.ErrorCount, s.WarningCount)
	for _, is := range rep.Errors {
		fmt.Fprintf(w, "  error    %s\n", is)
	}
	for _, is := range rep.Warnings {
		fmt.Fprintf(w, "  warning  %s\n", is)
	}
	var more []string
	for rule, n := range s.RuleCounts {
		if n > epwvalidate.MaxIssuesPerRule {
			more = append(more, rule)
		}
	}
	sort.Strings(more)
	for _, rule := range more {
		fmt.Fprintf(w, "  ... and %d more [%s]\n", s.RuleCounts[rule]-epwvalidate.MaxIssuesPerRule, rule)
	}
}
