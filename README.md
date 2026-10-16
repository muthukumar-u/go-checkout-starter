# Checkout API starter

A beginner-friendly Go checkout API with a small HTML, CSS, and JavaScript page.
The API validates a cart and an optional coupon, then returns an itemized quote.

Read [How it works](docs/HOW_IT_WORKS.md) for a beginner-friendly walkthrough of
the request flow, validation, coupon calculations, and frontend behavior.

## Run

Requires **Go 1.22 or newer**. No database, npm install, or API keys are needed.

```sh
go run ./cmd/server
```

Open **http://127.0.0.1:8080**. Press **Ctrl+C** to stop.

Go serves both the frontend and the API. Use the Go server's URL for the complete
checkout. Live Server at `http://127.0.0.1:5500/web/` can preview the page's styles,
but it does not serve the `/api/` endpoints needed for products, coupons, and totals.
Opening `web/index.html` directly also does not provide the API.

If port 8080 is in use:

```sh
# macOS / Linux
ADDR=127.0.0.1:8090 go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:ADDR = "127.0.0.1:8090"
go run ./cmd/server
```

Then open **http://127.0.0.1:8090**.

## Folder structure

```text
checkout-api/
├── cmd/server/main.go          Server startup and graceful shutdown
├── internal/
│   ├── catalog/catalog.go      Products and server-owned prices
│   ├── checkout/               Cart validation, coupon rules, and their tests
│   └── httpapi/                HTTP routes, JSON responses, and their tests
├── web/
│   ├── embed.go                Embeds the frontend in the Go executable
│   ├── index.html              Page structure
│   ├── styles.css              Responsive styling
│   ├── app.js                  Cart interactions and API calls
│   └── assets/                 Product illustrations
├── docs/HOW_IT_WORKS.md        Step-by-step explanation of the application
├── go.mod                      Module name and minimum Go version
└── README.md
```

**Suggested reading order:** `catalog.go` → `checkout.go` → `router.go` →
`main.go` → `app.js`.

A checkout request follows this flow:

1. The browser sends product IDs, quantities, and an optional coupon.
2. `httpapi` decodes the JSON and calls the checkout service.
3. `checkout` validates the cart, looks up trusted prices, and calculates totals.
4. `httpapi` returns JSON; the browser displays the quote.

## API

| Method | Endpoint | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Check whether the API is running |
| GET | `/api/products` | Get the catalog |
| GET | `/api/coupons` | Get available offers |
| POST | `/api/checkout` | Validate a cart and calculate a quote |

All money values are **integer paise**: `49900` means **₹499.00**. Currency is INR.
The server owns prices; the client sends only product IDs and quantities.

```sh
curl -s http://127.0.0.1:8080/api/checkout \
  -H 'Content-Type: application/json' \
  -d '{
    "items": [{"product_id": "lamp", "quantity": 1}],
    "coupon_code": "WELCOME10"
  }'
```

This returns a subtotal of `199900`, a discount of `19990`, and a total of
`179910` paise (**₹1,799.10**), plus item details. Omit `coupon_code` for a quote
without a discount. Change the URL's port if you started on a different port.

| Coupon | Discount | Minimum subtotal |
| --- | --- | --- |
| `WELCOME10` | 10%, up to ₹500 | ₹1,000 |
| `SAVE200` | ₹200 | ₹2,000 |
| `EXPIRED10` | Expired code for testing; rejected | — |

Codes are trimmed and converted to uppercase. One coupon is allowed per cart.
Percentage discounts round half up to the nearest paise.

A cart accepts 1–20 distinct product entries, with quantities from 1 to 99.
Unknown products, duplicate entries, invalid quantities, and invalid coupons
return **422**. Malformed JSON or unknown fields return **400**, oversized bodies
return **413**, and missing/incorrect JSON content types return **415**.

API errors include a stable code, a readable message, and the affected field:

```json
{
  "error": {
    "code": "invalid_quantity",
    "message": "Quantity must be a whole number between 1 and 99.",
    "field": "items[0].quantity"
  }
}
```

## Test and build

```sh
go test ./...
go test -race ./...
go vet ./...
go build -o bin/checkout ./cmd/server
```

The race detector requires a supported platform and a C compiler. Tests use
in-process HTTP handlers and do not need a running server. The built executable
includes the frontend files, so it can run without a separate web server.

## Customize

- **Products:** edit `internal/catalog/catalog.go`. Add image files under
  `web/assets/` and allow their public paths in `internal/httpapi/router.go`.
- **Coupons:** edit the coupon map in `internal/checkout/checkout.go`. Add public
  offers to `AvailableCoupons()` and add tests for new rules.
- **Page:** edit `web/index.html`, `web/styles.css`, and `web/app.js`.

Restart Go after changes so embedded frontend files are rebuilt.

This starter calculates quotes. It does not persist carts, create orders,
reserve stock, or collect payment. Refreshing restores the demo cart. Delivery
is free, and no separate tax calculation is included.
