package webui

import "embed"

// spaFS holds the compiled React SPA written to static/dist/ by `npm run build` in web/.
//
//go:embed static
var spaFS embed.FS
