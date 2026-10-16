// Package checkout validates carts and calculates totals. It knows nothing about
// HTTP or HTML, so its business rules can be tested without starting a server.
package checkout

import (
	"fmt"
	"strings"
	"time"

	"checkout-api/internal/catalog"
)

const (
	MaxItems    = 20 // Maximum number of distinct products in one request.
	MaxQuantity = 99 // Maximum quantity per product.
	Currency    = "INR"
)

type Item struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Request struct {
	Items      []Item `json:"items"`
	CouponCode string `json:"coupon_code"`
}

type LineItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
	LineTotal int64  `json:"line_total"`
}

type Quote struct {
	Currency   string     `json:"currency"`
	Items      []LineItem `json:"items"`
	Subtotal   int64      `json:"subtotal"`
	Discount   int64      `json:"discount"`
	Total      int64      `json:"total"`
	CouponCode string     `json:"coupon_code,omitempty"`
}

// ValidationError is safe to show to the user. Code is stable for API clients;
// Message is readable text, and Field tells the client which input to correct.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

func (e *ValidationError) Error() string { return e.Message }

type Coupon struct {
	Code        string    `json:"code"`
	Description string    `json:"description"`
	MinSubtotal int64     `json:"min_subtotal"`
	Percent     int64     `json:"percent,omitempty"`
	Amount      int64     `json:"amount,omitempty"`
	MaxDiscount int64     `json:"max_discount,omitempty"`
	ExpiresAt   time.Time `json:"-"`
}

// Service holds immutable sample data. Concurrent requests only read it.
type Service struct {
	products map[string]catalog.Product
	coupons  map[string]Coupon
	now      func() time.Time
}

func NewService() *Service {
	products := make(map[string]catalog.Product)
	for _, product := range catalog.Products() {
		products[product.ID] = product
	}
	return &Service{
		products: products,
		coupons: map[string]Coupon{
			"WELCOME10": {Code: "WELCOME10", Description: "10% off orders of ₹1,000 or more, up to ₹500", MinSubtotal: 100000, Percent: 10, MaxDiscount: 50000},
			"SAVE200":   {Code: "SAVE200", Description: "₹200 off orders of ₹2,000 or more", MinSubtotal: 200000, Amount: 20000},
			"EXPIRED10": {Code: "EXPIRED10", Description: "Expired demonstration coupon", Percent: 10, ExpiresAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		now: time.Now,
	}
}

// AvailableCoupons returns only currently active offers, in a predictable order.
func (s *Service) AvailableCoupons() []Coupon {
	result := make([]Coupon, 0, 2)
	for _, code := range []string{"WELCOME10", "SAVE200"} {
		coupon := s.coupons[code]
		if coupon.ExpiresAt.IsZero() || s.now().Before(coupon.ExpiresAt) {
			result = append(result, coupon)
		}
	}
	return result
}

func invalid(code, message, field string) error {
	return &ValidationError{Code: code, Message: message, Field: field}
}

// Calculate creates a quote; it does not place an order or collect payment.
// Cart sizes and quantities are bounded before multiplication, which also keeps
// arithmetic safely within int64 for this fixed, server-owned catalog.
func (s *Service) Calculate(request Request) (Quote, error) {
	if len(request.Items) == 0 || len(request.Items) > MaxItems {
		return Quote{}, invalid("invalid_cart", "Add between 1 and 20 distinct products to your cart.", "items")
	}

	quote := Quote{Currency: Currency, Items: make([]LineItem, 0, len(request.Items))}
	seen := make(map[string]bool)
	for index, item := range request.Items {
		field := fmt.Sprintf("items[%d]", index)
		product, exists := s.products[item.ProductID]
		if !exists {
			return Quote{}, invalid("unknown_product", "This product is not in the catalog.", field+".product_id")
		}
		if seen[item.ProductID] {
			return Quote{}, invalid("duplicate_product", "Combine quantities into one entry per product.", field+".product_id")
		}
		if item.Quantity < 1 || item.Quantity > MaxQuantity {
			return Quote{}, invalid("invalid_quantity", "Quantity must be a whole number between 1 and 99.", field+".quantity")
		}
		seen[item.ProductID] = true
		lineTotal := product.Price * int64(item.Quantity)
		quote.Items = append(quote.Items, LineItem{ProductID: product.ID, Name: product.Name, Quantity: item.Quantity, UnitPrice: product.Price, LineTotal: lineTotal})
		quote.Subtotal += lineTotal
	}

	code := strings.ToUpper(strings.TrimSpace(request.CouponCode))
	if code != "" {
		coupon, exists := s.coupons[code]
		if !exists {
			return Quote{}, invalid("invalid_coupon", "That coupon code was not found. Try WELCOME10 or SAVE200.", "coupon_code")
		}
		if !coupon.ExpiresAt.IsZero() && !s.now().Before(coupon.ExpiresAt) {
			return Quote{}, invalid("expired_coupon", "This coupon has expired. Please try another code.", "coupon_code")
		}
		if quote.Subtotal < coupon.MinSubtotal {
			return Quote{}, invalid("coupon_minimum", fmt.Sprintf("%s needs a subtotal of at least ₹%d.", code, coupon.MinSubtotal/100), "coupon_code")
		}
		quote.Discount = coupon.Amount
		if coupon.Percent > 0 {
			// Round half up to the nearest paise using integer arithmetic.
			quote.Discount = (quote.Subtotal*coupon.Percent + 50) / 100
		}
		if coupon.MaxDiscount > 0 && quote.Discount > coupon.MaxDiscount {
			quote.Discount = coupon.MaxDiscount
		}
		if quote.Discount > quote.Subtotal {
			quote.Discount = quote.Subtotal
		}
		quote.CouponCode = code
	}
	quote.Total = quote.Subtotal - quote.Discount
	return quote, nil
}
