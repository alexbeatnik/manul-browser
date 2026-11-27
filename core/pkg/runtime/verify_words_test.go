package runtime

import (
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
)

// VERIFY that 'Cart: 1' is present passed on a page that said "Cart: 10":
// the page text was searched as a string, numbers and all. A number in the
// target now stands alone; the rest is still a substring search, because
// innerText puts nothing between neighbouring inline elements.
func TestVerify_ANumberStandsAlone(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{
		regEl(1, "span", "Cart: 10"),
		regEl(2, "p", "21 items"),
	}}
	for _, c := range []struct {
		step string
		pass bool
	}{
		{"VERIFY that 'Cart: 1' is present", false},
		{"VERIFY that 'Cart: 1' is NOT present", true},
		{"VERIFY that 'Cart: 10' is present", true},
		{"VERIFY that '1 items' is NOT present", true},
		{"VERIFY that '21 items' is present", true},
	} {
		_, err := runSource(t, page, c.step+"\n")
		if (err == nil) != c.pass {
			t.Errorf("%s: err = %v, want pass=%v", c.step, err, c.pass)
		}
	}
}

func TestPageShows(t *testing.T) {
	page := foldSpace(" deep buttoninner clicked · cart: 10 · 21 items · ціна: 1 299 ₴ · workshop ")
	for target, want := range map[string]bool{
		"inner clicked": true, // two elements, nothing between them in innerText
		"cart: 1":       false,
		"cart: 10":      true,
		"1 items":       false,
		"21 items":      true,
		"1 299 ₴":       true,
		"shop":          true, // a substring search otherwise
		"":              true,
	} {
		if got := pageShows(page, target); got != want {
			t.Errorf("pageShows(%q) = %v, want %v", target, got, want)
		}
	}
}
