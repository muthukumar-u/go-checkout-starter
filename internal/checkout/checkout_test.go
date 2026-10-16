package checkout

import (
	"errors"
	"testing"
	"time"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name     string
		request  Request
		subtotal int64
		discount int64
		code     string
	}{
		{name: "one product", request: Request{Items: []Item{{"notebook", 1}}}, subtotal: 49900},
		{name: "multiple products and quantities", request: Request{Items: []Item{{"notebook", 2}, {"mug", 1}}}, subtotal: 179700},
		{name: "percentage discount", request: Request{Items: []Item{{"lamp", 1}}, CouponCode: "WELCOME10"}, subtotal: 199900, discount: 19990},
		{name: "fixed discount", request: Request{Items: []Item{{"lamp", 1}, {"notebook", 1}}, CouponCode: "SAVE200"}, subtotal: 249800, discount: 20000},
		{name: "case and spaces normalized", request: Request{Items: []Item{{"lamp", 1}}, CouponCode: "  welcome10  "}, subtotal: 199900, discount: 19990},
		{name: "blank coupon", request: Request{Items: []Item{{"notebook", 1}}, CouponCode: "  "}, subtotal: 49900},
		{name: "percentage cap", request: Request{Items: []Item{{"lamp", 10}}, CouponCode: "WELCOME10"}, subtotal: 1999000, discount: 50000},
		{name: "maximum quantity", request: Request{Items: []Item{{"lamp", 99}}}, subtotal: 19790100},
		{name: "empty cart", request: Request{}, code: "invalid_cart"},
		{name: "unknown product", request: Request{Items: []Item{{"missing", 1}}}, code: "unknown_product"},
		{name: "zero quantity", request: Request{Items: []Item{{"mug", 0}}}, code: "invalid_quantity"},
		{name: "negative quantity", request: Request{Items: []Item{{"mug", -1}}}, code: "invalid_quantity"},
		{name: "excess quantity", request: Request{Items: []Item{{"mug", 100}}}, code: "invalid_quantity"},
		{name: "duplicate product", request: Request{Items: []Item{{"mug", 1}, {"mug", 2}}}, code: "duplicate_product"},
		{name: "unknown coupon", request: Request{Items: []Item{{"lamp", 1}}, CouponCode: "NOPE"}, code: "invalid_coupon"},
		{name: "expired coupon", request: Request{Items: []Item{{"lamp", 1}}, CouponCode: "EXPIRED10"}, code: "expired_coupon"},
		{name: "percentage below minimum", request: Request{Items: []Item{{"notebook", 1}}, CouponCode: "WELCOME10"}, code: "coupon_minimum"},
		{name: "fixed below minimum", request: Request{Items: []Item{{"lamp", 1}}, CouponCode: "SAVE200"}, code: "coupon_minimum"},
		{name: "too many lines", request: Request{Items: make([]Item, MaxItems+1)}, code: "invalid_cart"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quote, err := NewService().Calculate(test.request)
			if test.code != "" {
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Code != test.code {
					t.Fatalf("want error %s; got %v", test.code, err)
				}
				if validation.Field == "" || validation.Message == "" {
					t.Fatal("validation errors need a field and readable message")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if quote.Currency != "INR" || quote.Subtotal != test.subtotal || quote.Discount != test.discount || quote.Total != test.subtotal-test.discount {
				t.Fatalf("unexpected totals: %+v", quote)
			}
			var sum int64
			for _, line := range quote.Items {
				if line.Name == "" || line.LineTotal != line.UnitPrice*int64(line.Quantity) {
					t.Fatalf("invalid line: %+v", line)
				}
				sum += line.LineTotal
			}
			if sum != quote.Subtotal {
				t.Fatalf("line totals %d do not match subtotal %d", sum, quote.Subtotal)
			}
			if test.discount > 0 && quote.CouponCode != "WELCOME10" && quote.CouponCode != "SAVE200" {
				t.Fatalf("expected normalized coupon: %q", quote.CouponCode)
			}
		})
	}
}

func TestCouponBoundariesAndRounding(t *testing.T) {
	service := NewService()
	// Synthetic internal fixtures exercise paise rounding and exact thresholds
	// independently of the hand-picked whole-rupee sample catalog.
	product := service.products["notebook"]
	product.Price = 100005
	service.products[product.ID] = product
	quote, err := service.Calculate(Request{Items: []Item{{product.ID, 1}}, CouponCode: "WELCOME10"})
	if err != nil || quote.Discount != 10001 || quote.Total != 90004 {
		t.Fatalf("half a paise must round up: %+v, %v", quote, err)
	}
	for _, price := range []int64{99999, 100000} {
		product.Price = price
		service.products[product.ID] = product
		_, err := service.Calculate(Request{Items: []Item{{product.ID, 1}}, CouponCode: "WELCOME10"})
		if (price == 99999) != (err != nil) {
			t.Fatalf("incorrect minimum behavior at %d: %v", price, err)
		}
	}
	product.Price = 200000
	service.products[product.ID] = product
	quote, err = service.Calculate(Request{Items: []Item{{product.ID, 1}}, CouponCode: "SAVE200"})
	if err != nil || quote.Total != 180000 {
		t.Fatalf("fixed coupon must work at the exact minimum: %+v, %v", quote, err)
	}
	service.coupons["BIG"] = Coupon{Code: "BIG", Amount: 999999}
	quote, err = service.Calculate(Request{Items: []Item{{product.ID, 1}}, CouponCode: "BIG"})
	if err != nil || quote.Total != 0 || quote.Discount != quote.Subtotal {
		t.Fatalf("discount cannot exceed subtotal: %+v, %v", quote, err)
	}
}

func TestCouponExpiresAtExactInstant(t *testing.T) {
	service := NewService()
	expiry := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	service.coupons["TIMED"] = Coupon{Code: "TIMED", Percent: 10, ExpiresAt: expiry}
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		service.now = func() time.Time { return expiry.Add(offset) }
		_, err := service.Calculate(Request{Items: []Item{{"lamp", 1}}, CouponCode: "TIMED"})
		if (offset >= 0) != (err != nil) {
			t.Fatalf("incorrect expiration behavior at offset %v: %v", offset, err)
		}
	}
}

func TestAvailableCoupons(t *testing.T) {
	coupons := NewService().AvailableCoupons()
	if len(coupons) != 2 || coupons[0].Code != "WELCOME10" || coupons[1].Code != "SAVE200" {
		t.Fatalf("unexpected public offers: %+v", coupons)
	}
}
