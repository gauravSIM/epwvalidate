'use strict';
// Tests for epwvalidate.js. Run from this folder: node --test
//
// The Go version is the reference: for every test file, this version must give
// exactly the result stored in ../testdata/expected.json, and its text output
// for the broken files must match ../testdata/broken_results.txt.

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');
const epw = require('./epwvalidate');

const ROOT = path.join(__dirname, '..');
const expected = JSON.parse(fs.readFileSync(path.join(ROOT, 'testdata', 'expected.json'), 'utf8'));

test('every test file gives the expected result', () => {
  const files = Object.keys(expected);
  assert.ok(files.length > 100);
  for (const f of files) {
    assert.deepStrictEqual(epw.validateFile(path.join(ROOT, f)), expected[f], f);
  }
});

test('the simien profile gives the expected result', () => {
  // SPEC.md section 11.1. Go is the reference here too.
  const want = JSON.parse(fs.readFileSync(path.join(ROOT, 'testdata', 'expected_simien.json'), 'utf8'));
  const files = Object.keys(want);
  assert.ok(files.length > 100);
  for (const f of files) {
    assert.deepStrictEqual(epw.validateFile(path.join(ROOT, f), { profile: epw.PROFILE_SIMIEN }), want[f], f);
  }
});

test('parseProfile', () => {
  assert.strictEqual(epw.parseProfile(''), epw.PROFILE_NONE);
  assert.strictEqual(epw.parseProfile('  SIMIEN '), epw.PROFILE_SIMIEN);
  assert.strictEqual(epw.parseProfile('energyplus'), null);
});

test('text output matches broken_results.txt', () => {
  const files = Object.keys(expected).filter((f) => f.startsWith('testdata/broken/')).sort();
  let text = '';
  for (const f of files) text += epw.formatText(f.slice('testdata/'.length), epw.validateFile(path.join(ROOT, f))) + '\n';
  const want = fs.readFileSync(path.join(ROOT, 'testdata', 'broken_results.txt'), 'utf8').replace(/\r\n/g, '\n');
  assert.strictEqual(text, want);
});

test('validate accepts bytes and strings', () => {
  const data = fs.readFileSync(path.join(ROOT, 'EPW_format', 'epw_examples', 'NOR_AK_Hurdal.NOR076_NORCERRA.epw'));
  assert.strictEqual(epw.validate(data).valid, true);
  assert.strictEqual(epw.validate(new Uint8Array(data)).valid, true);
  assert.strictEqual(epw.validate(data.toString('utf8')).valid, true);
});

test('empty input and missing file', () => {
  const rep = epw.validate('');
  assert.strictEqual(rep.valid, false);
  assert.strictEqual(epw.formatIssue(rep.errors[0]), 'file: the file is empty [F-1]');
  assert.throws(() => epw.validateFile(path.join(ROOT, 'testdata', 'does-not-exist.epw')));
});

test('command line', () => {
  const f = path.join(ROOT, 'testdata', 'broken', 'D-RNG-1__dry_bulb_75.epw');
  let out = '';
  const write = process.stdout.write;
  process.stdout.write = (s) => ((out += s), true);
  let code;
  try {
    code = epw.main(['--json', f]);
  } finally {
    process.stdout.write = write;
  }
  assert.strictEqual(code, 1);
  const reports = JSON.parse(out);
  assert.strictEqual(reports[0].file, f);
  assert.strictEqual(reports[0].errors[0].rule, 'D-RNG-1');
});
