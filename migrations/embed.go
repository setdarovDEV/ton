// Package migrations embeds the SQL schema migrations so that both binaries
// (api and indexer) can apply them without shipping files next to the binary.
package migrations

import "embed"

// FS holds the golang-migrate compatible *.sql files.
//
//go:embed *.sql
var FS embed.FS
