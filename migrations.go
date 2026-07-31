// Package fernweh exposes repo-level embedded assets (SQL migrations) so any
// binary in the monorepo can run them.
package fernweh

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
