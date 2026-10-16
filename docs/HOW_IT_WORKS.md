# How the checkout API works

This application takes a list of products, their quantities, and an optional
coupon code. It checks the inputs and returns a **quote**: item prices, subtotal,
discount, and final total.

The browser page and API run on the same Go server. The browser handles buttons
and display; Go owns product prices, validates the request, and calculates money.

## 1. Where to find each part

| File | Responsibility |
| --- | --- |
| [cmd/server/main.go](../cmd/server/main.go) | Starts the server and shuts it down on Ctrl+C. |
| [internal/catalog/catalog.go](../internal/catalog/catalog.go) | Defines the available products and their trusted prices. |
| [internal/checkout/checkout.go](../internal/checkout/checkout.go) | Validates carts, checks coupons, and calculates quotes. |
| [internal/httpapi/router.go](../internal/httpapi/router.go) | Chooses routes, reads JSON requests, and writes HTTP responses. |
| [web/embed.go](../web/embed.go) | Includes the page and its assets inside the Go executable. |
| [web/index.html](../web/index.html) | Defines the page's structure and controls. |
| [web/styles.css](../web/styles.css) | Styles the page for desktop and mobile screens. |
| [web/app.js](../web/app.js) | Manages the browser's cart and calls the API. |

`cmd` contains the runnable application. Go's `internal` folder contains packages
that are private to this project. A package is a group of Go files with a shared
purpose.

## 2. What happens when the server starts

Run this from the project root:

```sh
go run ./cmd/server
```

`main()` reads the optional `ADDR` environment variable. If it is unset, the
server listens at `127.0.0.1:8080`.

It calls `httpapi.NewHandler()`, which creates the checkout service and registers
the routes. `checkout.NewService()` loads products and coupons into maps—lookup
tables indexed by product ID or coupon code. This data lives in server memory;
there is no database.

The server then accepts requests. Pressing Ctrl+C stops new connections and gives
active requests up to five seconds to finish.

## 3. What happens when the page loads

Open the address printed by the Go server in your browser.

1. `GET /` returns the embedded HTML page.
2. The browser loads its CSS and JavaScript. Relative asset links work both at
   the Go server's root and when previewing the page under `/web/`.
3. JavaScript's `start()` requests `/api/products` and `/api/coupons` in parallel.
4. The page displays the product collection and available coupon offers.
5. It sends the initial cart to `POST /api/checkout` and displays the returned
   quote.

The initial cart contains one notebook, one mug, and one lamp. Its total before
discounts is ₹3,297.00.

**Why use the Go server instead of Live Server?** Live Server can serve HTML, CSS,
and JavaScript, but it does not implement `/api/products`, `/api/coupons`, or
`/api/checkout`. The complete checkout needs those endpoints on the same origin
(the same scheme, host, and port). Use the URL printed by Go for a working cart.

## 4. Follow one checkout request

Suppose the customer chooses one lamp and applies `WELCOME10`.

### The browser sends JSON

`requestQuote()` in `web/app.js` makes this request:

```http
POST /api/checkout
Content-Type: application/json
```

```json
{
  "items": [
    {"product_id": "lamp", "quantity": 1}
  ],
  "coupon_code": "WELCOME10"
}
```

The browser sends no prices. A customer could edit a browser request, so the
server must look up the real price itself.

### The HTTP handler reads the request

The handler in `internal/httpapi/router.go`:

1. Requires `Content-Type: application/json`.
2. Limits the request body to 16 KiB.
3. Decodes JSON into a `checkout.Request` struct—a Go value with named fields.
4. Rejects unknown fields, incorrect value types, and extra data after the JSON.
5. Passes the request to `service.Calculate()`.

For example, sending `"price": 1` inside an item is rejected as an unknown field.
Sending a quantity of `1.5` fails because quantities must decode as integers.

### The checkout service validates the cart

`Calculate()` checks the following rules before applying a coupon:

| Rule | Reason |
| --- | --- |
| Between 1 and 20 product entries | Reject empty or excessively large carts. |
| Every product ID exists | Only known catalog products can be quoted. |
| No duplicate product IDs | A product's quantity belongs in one entry. |
| Each quantity is between 1 and 99 | Reject zero, negative, and excessive quantities. |

The sample catalog currently has four products. The 20-entry limit allows the
catalog to grow without changing the request limit.

For each valid entry, the service calculates:

```text
line_total = catalog price × quantity
subtotal   = sum of all line totals
```

It builds a new quote for each request. It does not change the catalog or save
the customer's cart on the server.

### The service checks the coupon

The service trims spaces and converts the code to uppercase, so
`" welcome10 "` becomes `WELCOME10`. An omitted or blank code skips discounts.

For a supplied code, the service checks that it exists, has not expired, and
meets its minimum subtotal. A coupon is expired at or after its expiry instant.

| Code | Rule |
| --- | --- |
| `WELCOME10` | 10% off a subtotal of at least ₹1,000, capped at ₹500. |
| `SAVE200` | ₹200 off a subtotal of at least ₹2,000. |
| `EXPIRED10` | Expired demonstration code, used to exercise the error path. |

Only one coupon can be applied per request. The public offers endpoint advertises
the active codes listed by `AvailableCoupons()`; it excludes the expired example.

### The service calculates money in paise

All API money fields are integer paise: **100 paise = ₹1**. For example, `199900`
means ₹1,999.00. Using integers avoids floating point errors in money arithmetic.

For this lamp request:

| Calculation | Paise | Rupees |
| --- | --- | --- |
| Subtotal: one lamp | 199900 | ₹1,999.00 |
| Discount: 10% of subtotal | 19990 | ₹199.90 |
| Total: subtotal minus discount | 179910 | ₹1,799.10 |

Percentage discounts round half up to the nearest paise. The Go calculation is:

```go
discount = (subtotal*percent + 50) / 100
```

Integer division drops the remainder; adding `50` makes a fractional result of
half a paise or more round upward. For example, 10% of `100005` paise is
`10000.5` paise, which rounds to `10001` paise.

The service then applies any maximum discount and limits the discount to the
subtotal. The final total cannot be negative.

### The handler returns the quote

The successful response has HTTP status **200**:

```json
{
  "currency": "INR",
  "items": [
    {
      "product_id": "lamp",
      "name": "Little Desk Lamp",
      "quantity": 1,
      "unit_price": 199900,
      "line_total": 199900
    }
  ],
  "subtotal": 199900,
  "discount": 19990,
  "total": 179910,
  "coupon_code": "WELCOME10"
}
```

`coupon_code` is omitted when no coupon is applied. The browser divides paise by
100 and formats the amounts as INR for display. The quote contains no separate
shipping or tax fields; the demo page displays free delivery.

## 5. How the browser handles changes

The `state` object in `web/app.js` holds the loaded products, selected product
IDs and quantities, applied coupon, latest quote, and a busy flag. This state is
temporary: refreshing the page restores the initial demo cart.

| Action | What happens |
| --- | --- |
| Add, remove, or change a quantity | `updateItem()` updates the selection, clears the old quote, and requests a new one. Decreasing quantity from 1 removes the item. |
| Apply a coupon | `refreshQuote()` submits the candidate code and updates the applied coupon only on success. |
| Try an invalid replacement code | The page displays the error and keeps the previously valid quote and applied coupon. |
| Remove a coupon | The page requests a fresh quote with an empty coupon code. |
| Make an applied coupon ineligible by changing the cart | The page clears the coupon, requests an undiscounted quote, and displays the coupon error. |
| Remove every item | The page shows an empty cart and ₹0.00 locally, without sending an invalid empty-cart request. Coupon and review controls are disabled. |
| Review the order | The page requests a fresh quote before showing the review dialog. |
| Lose the connection during an update | The page clears the quote and displays an error. Applying a coupon or reviewing the cart can retry the request. |

While a quote request runs, cart and coupon actions are disabled to avoid
overlapping updates. API calls have a 12-second client timeout. Server text is
inserted with `textContent`, so messages are displayed as text rather than HTML.

## 6. How errors reach the user

The checkout service returns a `ValidationError` with three fields:

- `code`: a stable identifier that client code can inspect.
- `message`: a readable explanation for the customer.
- `field`: the input to correct. Array indexes start at zero, so `items[0]` means
  the first item.

The HTTP handler turns that error into JSON:

```json
{
  "error": {
    "code": "coupon_minimum",
    "message": "SAVE200 needs a subtotal of at least ₹2000.",
    "field": "coupon_code"
  }
}
```

| Status | Meaning |
| --- | --- |
| 400 | JSON cannot be decoded, includes unknown fields, or has trailing data. |
| 404 | API endpoint does not exist. |
| 405 | HTTP method is unsupported; the `Allow` header lists supported methods. |
| 413 | Request body exceeds 16 KiB. |
| 415 | Content type is missing or is not JSON. |
| 422 | Decoded input fails cart or coupon validation. |

The browser's `api()` helper reads the response and exposes its error message and
field to the page. Coupon errors appear near the coupon input; connection errors
appear above the cart. Unknown static-file URLs use ordinary HTTP 404 responses.

## 7. How to verify or extend it

Business-rule tests in
[checkout_test.go](../internal/checkout/checkout_test.go) call `Calculate()`
directly. They cover totals, invalid carts, coupon minimums, caps, rounding, and
expiry boundaries. HTTP tests in
[router_test.go](../internal/httpapi/router_test.go) exercise request decoding,
status codes, routes, and embedded files through Go's `httptest` helpers.

Run both sets without starting a server:

```sh
go test ./...
```

To add a product, update `catalog.Products()`, add its image under `web/assets/`,
and allow its public asset path in `httpapi.NewHandler()`. The page loads products
from the API, so it needs no copied product markup.

To add a coupon, update the coupon map in `checkout.NewService()`. Choose either
`Percent` or `Amount`, express money limits in paise, and add its code to
`AvailableCoupons()` if it should appear as an offer. Add a test for the new rule.

Restart the Go process after changing code or frontend assets: the page files are
embedded when the program is built.

The current application calculates quotes. Reviewing a quote does not create an
order, reserve inventory, or collect payment. A real order/payment flow would need
separate endpoints and persistent storage.
