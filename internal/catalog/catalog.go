// Package catalog contains the server-owned product list. Prices are in paise:
// 100 paise = ₹1. Never accept prices supplied by a browser.
package catalog

type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int64  `json:"price"`
	Image       string `json:"image"`
}

// Products returns a fresh slice so callers cannot change the catalog.
func Products() []Product {
	return []Product{
		{ID: "notebook", Name: "The Daily Notebook", Description: "Sage green · 160 dotted pages", Price: 49900, Image: "/assets/notebook.svg"},
		{ID: "mug", Name: "Slow Morning Mug", Description: "Terracotta · 350 ml ceramic", Price: 79900, Image: "/assets/mug.svg"},
		{ID: "lamp", Name: "Little Desk Lamp", Description: "Warm sand · soft, focused light", Price: 199900, Image: "/assets/lamp.svg"},
		{ID: "tote", Name: "Everyday Canvas Tote", Description: "Natural cotton · made to carry more", Price: 69900, Image: "/assets/tote.svg"},
	}
}
