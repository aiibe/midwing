package midwing

import (
	"regexp"
	"testing"
)

var tokenFixture = Service{
	ID:          "sample-api",
	Name:        "Sample API",
	Description: "Profile and inventory items.",
	Origin:      "https://api.example.test",
	AuthKind:    "token",
	Header:      "Authorization",
	Prefix:      "Bearer ",
	Permissions: []Permission{
		{ID: "profile", Label: "Profile", Description: "Read your Sample API profile"},
		{ID: "items", Label: "Items", Description: "Read inventory items"},
	},
	Examples: []string{"/profile", "/items"},
	Endpoints: []Endpoint{
		{
			Paths:      []string{"/profile"},
			Permission: "profile",
			Pattern:    regexp.MustCompile(`^/profile$`),
			Queries:    map[string]int{"page": 0, "per_page": 100},
		},
		{
			Paths:      []string{"/items", "/items/ITEM_ID"},
			Permission: "items",
			Pattern:    regexp.MustCompile(`^/items(/[A-Za-z0-9_.-]+)?$`),
			Queries:    map[string]int{"page": 0, "per_page": 100},
		},
	},
}

func registerTokenFixture(t *testing.T) {
	t.Helper()
	if _, err := findService(services, "sample-api"); err == nil {
		return
	}
	registerServiceFixture(t, tokenFixture)
}

func registerServiceFixture(t *testing.T, definition Service) {
	t.Helper()
	original := services
	services = append(append([]Service{}, services...), definition)
	t.Cleanup(func() { services = original })
}
