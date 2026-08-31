// Package migrations registers the database schema upgrades embedded in this binary.
package migrations

import (
	"embed"

	"go.mau.fi/util/dbutil"
)

//go:embed *.sql
var rawUpgrades embed.FS

var Table = dbutil.BuildUpgradeTable().
	WithFS(rawUpgrades).
	Finish()
