package migrations

import "embed"

// Files contains the SQL migrations embedded into the release binary.
//
//go:embed *.sql
var Files embed.FS

const Dir = "."
