// Package webassets embeds the BlindEnv dashboard assets into the binary so
// the UI works without any network access.
package webassets

import "embed"

// FS contains index.html, app.js and the compiled Tailwind stylesheet.
//
//go:embed index.html app.js dist
var FS embed.FS
