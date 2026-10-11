import assert from "node:assert/strict";
import { test } from "node:test";
import { nextCSVSort, parseCSV, sortCSVRows } from "./csv-preview.js";

for (const delimiter of [",", ";", "\t", ":", "|"]) {
  test(`detects ${JSON.stringify(delimiter)} while ignoring quoted separators`, () => {
    const data = parseCSV(`Name${delimiter}Count${delimiter}Note\r\nAlice${delimiter}12${delimiter}"commas, semicolons; tabs\t colons: pipes|"\r\nBob${delimiter}2${delimiter}plain\r\n`);
    assert.equal(data.delimiter, delimiter);
    assert.deepEqual(data.header, ["Name", "Count", "Note"]);
    assert.equal(data.columns, 3);
    assert.deepEqual(data.rows.map(row => row.cells), [["Alice", "12", "commas, semicolons; tabs\t colons: pipes|"], ["Bob", "2", "plain"]]);
  });
}

test("timestamps and decimal commas do not override the actual separator", () => {
  const data = parseCSV("Name;Price;Time\nAlice;1,50;12:34:56\nBob;2,75;09:20:30\nCarol;3,25;11:22:33");
  assert.equal(data.delimiter, ";");
  assert.deepEqual(data.rows[0].cells, ["Alice", "1,50", "12:34:56"]);
});

test("handles BOM, escaped quotes, multiline values, empty fields and blank lines", () => {
  const data = parseCSV('\uFEFFName,Note,Last\r\n\r\n"Alice","said ""hello""\r\nagain",\r\nBob,,end\r\n,,\r\n');
  assert.deepEqual(data.header, ["Name", "Note", "Last"]);
  assert.deepEqual(data.rows.map(row => row.cells), [["Alice", 'said "hello"\r\nagain', ""], ["Bob", "", "end"], ["", "", ""]]);
});

test("retains extra columns and short records", () => {
  const data = parseCSV("A,B\n1,2\n3\n4,5,6,");
  assert.equal(data.delimiter, ",");
  assert.equal(data.columns, 4);
  assert.deepEqual(data.rows.map(row => row.cells), [["1", "2"], ["3"], ["4", "5", "6", ""]]);
});

test("empty, single-column, header-only and incomplete edited files remain usable", () => {
  assert.deepEqual(parseCSV("\uFEFF\r\n\n").header, []);
  assert.deepEqual(parseCSV("Name\nAlice\nBob").rows.map(row => row.cells), [["Alice"], ["Bob"]]);
  assert.deepEqual(parseCSV("A;B").header, ["A", "B"]);
  assert.deepEqual(parseCSV('A,B\n1,"unfinished\nvalue').rows[0].cells, ["1", "unfinished\nvalue"]);
});

test("three clicks sort numbers ascending, descending, then restore file order", () => {
  const { rows } = parseCSV("Name,Value\nA,10\nB,-2\nC,2.5\nD,2.5\nE,1e2\nF,");
  let sort = { column: null, direction: "none" };
  const order = () => sortCSVRows(rows, sort).map(row => row.cells[0]);
  sort = nextCSVSort(sort, 1);
  assert.equal(sort.direction, "ascending");
  assert.deepEqual(order(), ["F", "B", "C", "D", "A", "E"]);
  sort = nextCSVSort(sort, 1);
  assert.equal(sort.direction, "descending");
  assert.deepEqual(order(), ["E", "A", "C", "D", "B", "F"]);
  sort = nextCSVSort(sort, 1);
  assert.deepEqual(sort, { column: null, direction: "none" });
  assert.deepEqual(order(), ["A", "B", "C", "D", "E", "F"]);
  assert.equal(sortCSVRows(rows, sort), rows);
});

test("switching columns starts ascending; text uses stable natural ordering", () => {
  const { rows } = parseCSV("Name,Value\nitem10,2\nitem2,1\nITEM2,3\nitem1,x");
  const sort = nextCSVSort({ column: 1, direction: "descending" }, 0);
  assert.equal(sort.direction, "ascending");
  assert.deepEqual(sortCSVRows(rows, sort).map(row => row.index), [3, 1, 2, 0]);
  assert.deepEqual(sortCSVRows(rows, { column: 1, direction: "ascending" }).map(row => row.index), [1, 0, 2, 3]);
  assert.deepEqual(rows.map(row => row.index), [0, 1, 2, 3]);
});

test("sorting missing cells and overflowing numeric values is deterministic", () => {
  const { rows } = parseCSV("Name,Value\nA,1e999\nB\nC,2\nD,");
  assert.deepEqual(sortCSVRows(rows, { column: 1, direction: "ascending" }).map(row => row.index), [1, 3, 0, 2]);
});
