// Package web embeds the checkout page into the Go executable. The executable
// can serve it from any working directory, without a separate frontend server.
package web

import "embed"

//go:embed index.html styles.css app.js assets/*.svg
var Files embed.FS
