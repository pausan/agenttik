# CSV previews

`.csv` and `.tsv` files offer Preview beside Edit and Diff. The table reads
the tab's current text, including unsaved edits. The first nonblank record is
the header. All columns render, including columns present only in later rows;
missing cells are empty and unnamed headers use `Column N`. Wide tables scroll
horizontally in the file pane. Headers stay visible during vertical scrolling.
Values render as text, including markup, spaces and embedded line breaks.

Separator detection tries commas, semicolons, tabs, pipes and colons. It samples
at most 64 KiB and 32 records per candidate, preferring the most consistent
multi-column width, then the larger width. Remaining ties prefer candidates
in that order. Comma is the fallback for single-column files. Parsing supports
double-quoted fields, doubled quote escapes, quoted line breaks, LF, CRLF and
CR endings, a leading UTF-8 BOM and trailing empty fields. Blank physical
lines are skipped. Unclosed quoted fields remain visible during editing.

Clicking a header cycles ascending, descending and original file order.
Choosing another column starts ascending. A column whose nonempty cells are
finite decimal numbers (including signs and exponents) sorts numerically;
other columns use locale-aware, case-insensitive natural text order. Empty
cells sort first ascending and last descending. Ties retain file order in
both directions. Sorting changes only the preview. Changed text resets sorting
and pagination.

Pages contain at most 200 data rows or 10,000 cells, with at least one row.
Previous/Next controls appear for larger tables; all rows remain accessible.
Sorting applies to the full loaded file and returns to the first page. File
loading retains the existing 2 MiB text limit and partial-file notice.

Unit tests cover detection, quoted and incomplete fields, record widths,
numeric/text sorting, stable ties and the three-click cycle. Browser tests
cover actual header clicks, plain-text rendering, unsaved edits, empty files,
all columns, paging and sorting across pages.
