package handler

import "embed"

// webFiles holds the browser page (ADR 0016).
//
//go:embed web
var webFiles embed.FS
