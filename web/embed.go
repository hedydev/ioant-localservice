package web

import "embed"

//go:embed index.html app.js style.css workspace.css js/*.js
var Files embed.FS
