package ui

import "embed"

// dist is produced by a frontend build that hasn't run.
//
//go:embed dist
var dist embed.FS

// Title is the page title.
func Title() string { return "fx" }
