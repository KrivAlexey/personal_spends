package handler

import (
	"embed"
	"net/http"
)

// webFiles holds the browser page (ADR 0016).
//
//go:embed web
var webFiles embed.FS

// webFile serves the embedded file name as contentType. The CSP lets the page
// load only this server's script, styles and API: no inline or foreign code,
// so a file name can't become script even if it reached the DOM as markup.
func (h *Handler) webFile(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := webFiles.ReadFile(name)
		if err != nil {
			h.logger.ErrorContext(r.Context(), "read web file failed",
				"method", r.Method, "route", r.Pattern, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, err := w.Write(body); err != nil {
			h.logger.WarnContext(r.Context(), "write response failed",
				"method", r.Method, "route", r.Pattern, "error", err)
		}
	}
}
