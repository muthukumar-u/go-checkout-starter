package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"checkout-api/internal/checkout"
)

func TestCheckoutHTTP(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
		code        string
	}{
		{"valid", `{"items":[{"product_id":"lamp","quantity":1}],"coupon_code":"WELCOME10"}`, "application/json", 200, ""},
		{"no coupon", `{"items":[{"product_id":"notebook","quantity":1}]}`, "application/json; charset=utf-8", 200, ""},
		{"empty body", ``, "application/json", 400, "invalid_json"},
		{"broken JSON", `{"items":`, "application/json", 400, "invalid_json"},
		{"unknown top-level field", `{"items":[],"price":1}`, "application/json", 400, "invalid_json"},
		{"client price rejected", `{"items":[{"product_id":"lamp","quantity":1,"price":1}]}`, "application/json", 400, "invalid_json"},
		{"fractional quantity", `{"items":[{"product_id":"lamp","quantity":1.5}]}`, "application/json", 400, "invalid_json"},
		{"string quantity", `{"items":[{"product_id":"lamp","quantity":"1"}]}`, "application/json", 400, "invalid_json"},
		{"overflow quantity", `{"items":[{"product_id":"lamp","quantity":999999999999999999999}]}`, "application/json", 400, "invalid_json"},
		{"multiple documents", `{"items":[]} {}`, "application/json", 400, "invalid_json"},
		{"trailing garbage", `{"items":[]} xyz`, "application/json", 400, "invalid_json"},
		{"array root", `[]`, "application/json", 400, "invalid_json"},
		{"null root", `null`, "application/json", 422, "invalid_cart"},
		{"null items", `{"items":null}`, "application/json", 422, "invalid_cart"},
		{"empty cart", `{"items":[]}`, "application/json", 422, "invalid_cart"},
		{"missing quantity", `{"items":[{"product_id":"lamp"}]}`, "application/json", 422, "invalid_quantity"},
		{"invalid coupon", `{"items":[{"product_id":"lamp","quantity":1}],"coupon_code":"NOPE"}`, "application/json", 422, "invalid_coupon"},
		{"expired coupon", `{"items":[{"product_id":"lamp","quantity":1}],"coupon_code":"EXPIRED10"}`, "application/json", 422, "expired_coupon"},
		{"missing media type", `{}`, "", 415, "unsupported_media_type"},
		{"wrong media type", `{}`, "text/plain", 415, "unsupported_media_type"},
		{"large body", `{"coupon_code":"` + strings.Repeat("a", maxBodyBytes) + `"}`, "application/json", 413, "body_too_large"},
		{"large trailing whitespace", `{"items":[]}` + strings.Repeat(" ", maxBodyBytes), "application/json", 413, "body_too_large"},
	}
	handler := NewHandler()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/checkout", strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("want status %d, got %d: %s", test.status, response.Code, response.Body)
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
				t.Fatal("API response must be JSON")
			}
			if test.code != "" {
				var payload errorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Error == nil || payload.Error.Code != test.code {
					t.Fatalf("want error %s, got %s", test.code, response.Body)
				}
			} else {
				var quote checkout.Quote
				if err := json.Unmarshal(response.Body.Bytes(), &quote); err != nil || quote.Total <= 0 || len(quote.Items) != 1 {
					t.Fatalf("invalid quote: %s", response.Body)
				}
				if test.name == "valid" && (quote.Total != 179910 || quote.Discount != 19990) {
					t.Fatalf("incorrect server totals: %+v", quote)
				}
			}
		})
	}
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		method, path string
		status       int
		contains     string
	}{
		{"GET", "/api/health", 200, `"status":"ok"`},
		{"GET", "/api/products", 200, `"price":49900`},
		{"GET", "/api/coupons", 200, `WELCOME10`},
		{"GET", "/api/checkout", 405, `method_not_allowed`},
		{"PUT", "/api/products", 405, `method_not_allowed`},
		{"OPTIONS", "/api/checkout", 405, `method_not_allowed`},
		{"POST", "/api/missing", 404, `not_found`},
		{"GET", "/api/missing", 404, `not_found`},
		{"GET", "/", 200, `Your bag · Form & Field`},
		{"GET", "/styles.css", 200, `--paper`},
		{"GET", "/app.js", 200, `requestQuote`},
		{"GET", "/assets/notebook.svg", 200, `<svg`},
		{"GET", "/missing", 404, `404`},
		{"GET", "/embed.go", 404, `404`},
	}
	handler := NewHandler()
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("want status %d containing %q; got %d: %s", test.status, test.contains, response.Code, response.Body)
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing security headers")
			}
			if test.status == 405 && response.Header().Get("Allow") == "" {
				t.Fatal("method errors must declare supported methods")
			}
		})
	}
}
