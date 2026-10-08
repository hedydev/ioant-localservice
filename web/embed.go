package web

import "embed"

//go:embed index.html app.js style.css workspace.css release-cards.css build-jobs.css confirm-dialog.css js/*.js
var Files embed.FS
