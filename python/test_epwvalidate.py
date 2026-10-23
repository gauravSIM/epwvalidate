"""Tests for epwvalidate.py. Run from this folder:

    python -m unittest -v

The Go version is the reference: for every test file, this version must give
exactly the result stored in ../testdata/expected.json, and its text output
for the broken files must match ../testdata/broken_results.txt.
"""

import contextlib
import io
import json
import os
import unittest

import epwvalidate

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")


def _read_text(path):
    with open(path, encoding="utf-8", newline="") as f:
        return f.read().replace("\r\n", "\n")


class ConformanceTest(unittest.TestCase):
    def test_expected_json(self):
        with open(os.path.join(ROOT, "testdata", "expected.json"), encoding="utf-8") as f:
            expected = json.load(f)
        self.assertGreater(len(expected), 100)
        for path, want in expected.items():
            with self.subTest(path=path):
                got = epwvalidate.validate_file(os.path.join(ROOT, path)).to_dict()
                self.assertEqual(want, got)

    def test_expected_simien_json(self):
        """The simien profile (SPEC.md section 11.1) must match Go too."""
        with open(os.path.join(ROOT, "testdata", "expected_simien.json"), encoding="utf-8") as f:
            expected = json.load(f)
        self.assertGreater(len(expected), 100)
        opt = epwvalidate.Options(epwvalidate.PROFILE_SIMIEN)
        for path, want in expected.items():
            with self.subTest(path=path):
                got = epwvalidate.validate_file(os.path.join(ROOT, path), opt).to_dict()
                self.assertEqual(want, got)

    def test_parse_profile(self):
        self.assertEqual(epwvalidate.PROFILE_NONE, epwvalidate.parse_profile(""))
        self.assertEqual(epwvalidate.PROFILE_SIMIEN, epwvalidate.parse_profile("  SIMIEN "))
        self.assertIsNone(epwvalidate.parse_profile("energyplus"))

    def test_space_matches_go_unicode_isspace(self):
        """_SPACE must be exactly the set Go's unicode.IsSpace accepts.

        It is the one place this version restates a Go built-in rather than
        mirroring code, so nothing else would notice if it drifted: Python
        would simply trim differently, and only files with an exotic space in
        a field would disagree with Go. The characters are invisible, which
        makes the line easy to corrupt by accident and impossible to review by
        eye -- hence this test. Generated with:

            for r := rune(0); r <= unicode.MaxRune; r++ { if unicode.IsSpace(r) ... }
        """
        want = [
            0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0x85, 0xA0, 0x1680,
            0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006,
            0x2007, 0x2008, 0x2009, 0x200A, 0x2028, 0x2029, 0x202F,
            0x205F, 0x3000,
        ]
        got = [ord(c) for c in epwvalidate._SPACE]
        self.assertEqual(
            [f"U+{c:04X}" for c in want],
            [f"U+{c:04X}" for c in got],
            "_SPACE no longer matches Go's unicode.IsSpace; Python would trim differently",
        )

    def test_broken_results_text(self):
        with open(os.path.join(ROOT, "testdata", "expected.json"), encoding="utf-8") as f:
            paths = sorted(p for p in json.load(f) if p.startswith("testdata/broken/"))
        text = ""
        for path in paths:
            rep = epwvalidate.validate_file(os.path.join(ROOT, path))
            text += epwvalidate.format_text(path[len("testdata/"):], rep) + "\n"
        self.assertEqual(_read_text(os.path.join(ROOT, "testdata", "broken_results.txt")), text)


class ApiTest(unittest.TestCase):
    def test_validate_text_and_bytes(self):
        path = os.path.join(ROOT, "EPW_format", "epw_examples", "NOR_AK_Hurdal.NOR076_NORCERRA.epw")
        with open(path, "rb") as f:
            data = f.read()
        self.assertTrue(epwvalidate.validate(data).valid)
        self.assertTrue(epwvalidate.validate(data.decode("utf-8")).valid)

    def test_empty(self):
        rep = epwvalidate.validate(b"")
        self.assertFalse(rep.valid)
        self.assertEqual("F-1", rep.errors[0].rule)
        self.assertEqual("file: the file is empty [F-1]", str(rep.errors[0]))

    def test_missing_file(self):
        with self.assertRaises(OSError):
            epwvalidate.validate_file(os.path.join(ROOT, "testdata", "does-not-exist.epw"))

    def test_cli(self):
        path = os.path.join(ROOT, "testdata", "broken", "D-RNG-1__dry_bulb_75.epw")
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = epwvalidate.main(["--json", path])
        self.assertEqual(1, code)
        reports = json.loads(out.getvalue())
        self.assertEqual(path, reports[0]["file"])
        self.assertEqual("D-RNG-1", reports[0]["errors"][0]["rule"])
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(2, epwvalidate.main([]))


if __name__ == "__main__":
    unittest.main()
