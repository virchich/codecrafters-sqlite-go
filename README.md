# SQLite reader in Go

A read-only SQLite file reader built for the [CodeCrafters “Build Your Own SQLite” challenge](https://codecrafters.io/challenges/sqlite).

The project reads database pages directly: it decodes records, traverses table B-trees, follows overflow chains, and evaluates a small subset of SQL. SQL syntax parsing uses `github.com/rqlite/sql`; storage access and query execution are implemented here.

## Supported behavior

- `.dbinfo`: database page size and user-table count.
- `.tables`: space-separated user table names. Both commands exclude `sqlite_` tables.
- Single-table `SELECT` with named columns, `*`, or `COUNT(*)`.
- One optional `WHERE` comparison: `=`, `!=`, `<`, `<=`, `>`, or `>=`, with a column on the left and a literal on the right.
- Table scans across interior and leaf pages, including overflow records and integer-primary-key rowid aliases.

Rows are printed with `|` separators and no column headings. NULL values appear as empty fields; an empty count prints `0`.

## Run

Requires Go 1.27 or later, as specified in `go.mod`.

```sh
./your_program.sh sample.db .dbinfo
./your_program.sh sample.db .tables
./your_program.sh sample.db "SELECT id, name FROM apples"
./your_program.sh sample.db "SELECT COUNT(*) FROM apples WHERE name = 'Fuji'"
```

The included `sample.db` contains the user tables `apples` and `oranges`. Larger challenge fixtures can be downloaded with `./download_sample_databases.sh`; they are not needed for tests.

## Code layout

| Package | Responsibility |
| --- | --- |
| `app/cmd` | Commands, filtering, projection, aggregation, and output |
| `app/sql` | SQL parsing and extraction of supported query/schema metadata |
| `app/parser` | Database headers, pages, cells, records, overflow assembly, and table cursors |
| `app/table` | Column lookup, rowid resolution, and schema-entry interpretation |

A table cursor returns a decoded record and its rowid. Command code applies a schema and evaluates the query without handling page bytes.

## Tests

```sh
go test ./...
go vet ./...
```

Tests use small synthetic database fixtures. They cover binary decoding, page boundaries, traversal, overflow-chain failures, SQL parsing, and command output without requiring the SQLite CLI.

## Scope and remaining work

Queries currently use full table scans. Index metadata and cell parsing are present; index traversal, lookup by rowid, and index selection remain to be implemented.

This is a learning implementation, not a replacement for SQLite. It supports UTF-8 rowid tables and reads only the main database file. Use database files that are not being modified and do not depend on an unapplied WAL or journal. Writes, recovery, `WITHOUT ROWID` tables, joins, sorting, and general SQL expressions are outside the current scope. Comparisons do not implement SQLite type affinity or custom collations.

Records are assembled in memory before decoding, so large records can require substantial memory.
