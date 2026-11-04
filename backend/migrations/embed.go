// Package migrations embeds the SQL migrations run by golang-migrate at API start-up.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
