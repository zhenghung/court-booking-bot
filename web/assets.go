package webassets

import "embed"

//go:embed index.html app.js style.css favicon.png favicon.ico apple-touch-icon.png icon-512.png favicon-16.png
var FS embed.FS
