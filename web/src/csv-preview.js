const DELIMITERS = [",", ";", "\t", "|", ":"];
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
const NUMBER = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i;

// Quotes may contain separators and line breaks. Incomplete edits still show
// their current field; blank physical lines do not create empty table rows.
function records(text, delimiter, limit = Infinity) {
  const rows = [];
  let row = [], field = "", quoted = false, started = false;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (quoted) {
      if (char === '"') {
        if (text[i + 1] === '"') { field += '"'; i++; }
        else quoted = false;
      } else field += char;
    } else if (char === '"' && !started) {
      quoted = true;
      started = true;
    } else if (char === delimiter) {
      row.push(field);
      field = "";
      started = false;
    } else if (char === "\n" || char === "\r") {
      if (started || row.length) rows.push([...row, field]);
      if (char === "\r" && text[i + 1] === "\n") i++;
      row = [];
      field = "";
      started = false;
      if (rows.length >= limit) return rows;
    } else {
      field += char;
      started = true;
    }
  }
  if (started || row.length) rows.push([...row, field]);
  return rows;
}

function detectDelimiter(text) {
  // Bound sniffing work even for a file consisting of one very long record.
  const sample = text.slice(0, 64 * 1024);
  let best = ",", score = 0, columns = 0;
  for (const delimiter of DELIMITERS) {
    const rows = records(sample, delimiter, 32);
    const widths = new Map();
    for (const row of rows) {
      if (row.length > 1) widths.set(row.length, (widths.get(row.length) || 0) + 1);
    }
    for (const [width, count] of widths) {
      const consistency = count / rows.length;
      if (consistency > score || (consistency === score && width > columns)) {
        best = delimiter;
        score = consistency;
        columns = width;
      }
    }
  }
  return best;
}

export function parseCSV(text) {
  text = text.replace(/^\uFEFF/, "");
  const delimiter = detectDelimiter(text);
  const recordsList = records(text, delimiter);
  const header = recordsList.shift() || [];
  let columns = header.length;
  const rows = recordsList.map((cells, index) => {
    columns = Math.max(columns, cells.length);
    return { cells, index };
  });
  return { delimiter, header, columns, rows };
}

export function nextCSVSort(sort, column) {
  if (sort.column !== column) return { column, direction: "ascending" };
  if (sort.direction === "ascending") return { column, direction: "descending" };
  return { column: null, direction: "none" };
}

export function sortCSVRows(rows, { column, direction }) {
  if (column === null || direction === "none") return rows;
  const values = new Map();
  let numeric = true;
  for (const row of rows) {
    const value = (row.cells[column] || "").trim();
    values.set(row, value);
    if (value && (!NUMBER.test(value) || !Number.isFinite(Number(value)))) numeric = false;
  }
  const sign = direction === "ascending" ? 1 : -1;
  return rows.slice().sort((a, b) => {
    const left = values.get(a), right = values.get(b);
    const comparison = !left || !right ? (left ? 1 : right ? -1 : 0)
      : numeric ? Number(left) - Number(right) : collator.compare(left, right);
    return sign * comparison || a.index - b.index;
  });
}
