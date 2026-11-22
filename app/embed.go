// Package app is the top-level application package for kumacore.
package app

import "embed"

// Templates holds all templates baked into the binary at compile time: shared layouts, shared components, each module's
// page templates, and per-module component fragments (modules/<name>/components/*.html).
//
//go:embed web/templates/layouts/*.html web/templates/components/*.html modules/errorscreen/*.html modules/*/components/*.html modules/home/*.html modules/auth/*.html
var Templates embed.FS
