package epwvalidate

// The Go version is the reference. TestExpected writes its results for every
// test file to ../testdata/expected.json and ../testdata/broken_results.txt
// (with -update) or checks that they are unchanged. The Python, Node and .NET
// versions must reproduce both files exactly.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// conformanceFiles lists the files covered by expected.json, as paths
// relative to the repository root.
func conformanceFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"EPW_format/epw_examples/*.epw", "testdata/partial/*.epw", "testdata/broken/*.epw"} {
		matches, err := filepath.Glob(filepath.Join("..", pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			rel, _ := filepath.Rel("..", m)
			files = append(files, filepath.ToSlash(rel))
		}
	}
	sort.Strings(files)
	return files
}

func TestExpected(t *testing.T) {
	files := conformanceFiles(t)
	reports := map[string]Report{}
	var text strings.Builder
	for _, f := range files {
		rep, err := ValidateFile(filepath.Join("..", f))
		if err != nil {
			t.Fatal(err)
		}
		reports[f] = rep
		if name, ok := strings.CutPrefix(f, "testdata/"); ok && strings.HasPrefix(name, "broken/") {
			text.WriteString(FormatText(name, rep) + "\n")
		}
	}

	var js bytes.Buffer
	enc := json.NewEncoder(&js)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(reports); err != nil {
		t.Fatal(err)
	}

	jsonPath := filepath.Join("..", "testdata", "expected.json")
	textPath := filepath.Join("..", "testdata", "broken_results.txt")
	if *update {
		if err := os.WriteFile(jsonPath, js.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(textPath, []byte(text.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	disk, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("%v; run: go test -run 'TestFixtures|TestExpected' -update", err)
	}
	var want, got any
	if err := json.Unmarshal(disk, &want); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(js.Bytes(), &got)
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s is out of date; run: go test -run 'TestFixtures|TestExpected' -update", jsonPath)
	}
	if diskText, err := os.ReadFile(textPath); err != nil || string(diskText) != text.String() {
		t.Errorf("%s is out of date; run: go test -run 'TestFixtures|TestExpected' -update", textPath)
	}
}
