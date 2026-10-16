// Package httpapi translates HTTP/JSON into calls to the checkout service.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"checkout-api/internal/catalog"
	"checkout-api/internal/checkout"
	"checkout-api/web"
)

const maxBodyBytes = 16 * 1024

type errorResponse struct {
	Error *checkout.ValidationError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// All callers pass JSON-safe structs. A write failure means the client left;
	// writing a second response would corrupt the original one.
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	writeJSON(w, status, errorResponse{Error: &checkout.ValidationError{Code: code, Message: message, Field: field}})
}

func NewHandler() http.Handler {
	service := checkout.NewService()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/products", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"currency": checkout.Currency, "products": catalog.Products()})
	})
	mux.HandleFunc("GET /api/coupons", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"currency": checkout.Currency, "coupons": service.AvailableCoupons()})
	})
	mux.HandleFunc("POST /api/checkout", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Send the request with Content-Type: application/json.", "body")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request checkout.Request
		err = decoder.Decode(&request)
		if err == nil {
			// Require exactly one JSON document, including rejecting trailing data.
			var extra any
			err = decoder.Decode(&extra)
			if errors.Is(err, io.EOF) {
				err = nil
			} else if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Request body must be 16 KiB or smaller.", "body")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_json", "Send one JSON object with items and an optional coupon_code. Check field names and value types.", "body")
			}
			return
		}
		quote, err := service.Calculate(request)
		if err != nil {
			var validation *checkout.ValidationError
			if errors.As(err, &validation) {
				writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: validation})
			} else {
				writeError(w, http.StatusInternalServerError, "internal_error", "Unable to calculate your cart.", "")
			}
			return
		}
		writeJSON(w, http.StatusOK, quote)
	})
	// Keep unknown API URLs from falling through to the static file server.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health", "/api/products", "/api/coupons", "/api/checkout":
			if r.URL.Path == "/api/checkout" {
				w.Header().Set("Allow", "POST")
			} else {
				w.Header().Set("Allow", "GET, HEAD")
			}
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "This endpoint does not support that HTTP method.", "method")
		default:
			writeError(w, http.StatusNotFound, "not_found", "API endpoint not found.", "path")
		}
	})
	files := http.FileServer(http.FS(web.Files))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Expose only public assets, never embedded source such as embed.go.
		switch r.URL.Path {
		case "/", "/styles.css", "/app.js", "/assets/notebook.svg", "/assets/mug.svg", "/assets/lamp.svg", "/assets/tote.svg":
			files.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
