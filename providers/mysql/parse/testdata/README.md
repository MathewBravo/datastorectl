# parser fixtures

Fixture pairs (`.sql` + `.json`) for the CREATE USER and GRANT parsers.
Each `.sql` holds raw output captured from a live `mysql:8.0` or
`mysql:8.4` container. Each `.json` is the expected result of parsing
that DDL — written by the fixture test itself, not by hand.

## Layout

```
testdata/
├── capture.sh
├── 8.0/
│   ├── users/
│   │   ├── basic_user.sql
│   │   ├── basic_user.json
│   │   ├── user_with_limits.sql
│   │   └── ...
│   └── grants/
│       ├── global_grant.sql
│       └── ...
└── 8.4/
    ├── users/
    └── grants/
```

## Running the tests

```
go test ./providers/mysql/parse/...
```

Fixture tests iterate every `.sql` file, parse it, and assert the
output matches the sibling `.json`. A mismatch means the parser
changed — intentionally or not.

## Regenerating after intentional parser changes

```
go test ./providers/mysql/parse/... -update-fixtures
```

Only run this after deliberately changing parser behavior or the
output struct shape. Commit the resulting `.json` diffs.

## Regenerating from fresh server captures

```
./providers/mysql/parse/testdata/capture.sh
go test ./providers/mysql/parse/... -update-fixtures
```

The capture script spins up `mysql:8.0` and `mysql:8.4` containers in
sequence, creates a matrix of users and grants covering every
supported clause and scope, dumps the `SHOW CREATE USER` and
`SHOW GRANTS` output, and tears the containers down. Each run
produces different `AuthString` bytes (fresh salts), so the `.sql`
files will show diff; running `-update-fixtures` afterward regenerates
the `.json` files to match.

Docker is required for `capture.sh`. The fixture tests themselves
need only the committed files and run without Docker.
