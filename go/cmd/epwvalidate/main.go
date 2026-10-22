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
	"os"

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
	reports := []fileReport{}
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
			fmt.Print(epwvalidate.FormatText(path, rep))
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exit = 2
		}
	}
	os.Exit(exit)
}
