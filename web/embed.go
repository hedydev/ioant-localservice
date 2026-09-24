package web

import "embed"

//go:embed index.html app.js builds.js style.css
var Files embed.FS
