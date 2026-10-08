// Package migrations holds versioned SQL for the SQLite schema.
package migrations

import "embed"

// FS contains the SQL migrations in lexical order.
//
//go:embed *.sql
var FS embed.FS
