// Package migrations holds the SQL schema history.
//
// The .sql files are embedded into the binary so that deploying JobPulse is
// "copy the binary and run it" — no migrate CLI, no mounted migrations
// directory, no init container. New migrations are plain numbered files next
// to these; nothing else needs updating.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
