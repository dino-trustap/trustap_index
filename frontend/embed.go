// Package frontend embeds the built merchant dashboard SPA. Build it with
// `npm run build` in this directory (the dist/ output is committed so plain
// `make api` keeps working without Node).
package frontend

import "embed"

//go:embed all:dist
var Dist embed.FS
