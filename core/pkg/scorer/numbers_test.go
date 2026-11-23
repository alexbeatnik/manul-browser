package scorer

import (
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
)

// Word overlap drops one-letter words, digits among them, so 'Under 4
// minutes' matched "Under 3 minutes" completely and YouTube's filter for a
// different duration was clicked.
func TestMatchesQuery_NumbersMustAgree(t *testing.T) {
	el := func(text string) *dom.ElementSnapshot {
		e := &dom.ElementSnapshot{Tag: "a", VisibleText: text, IsVisible: true}
		e.Normalize()
		return e
	}
	cases := []struct {
		query, text string
		want        bool
	}{
		{"Under 4 minutes", "Under 3 minutes", false},
		{"Under 3 minutes", "Under 3 minutes", true},
		{"Page 1", "Page 10", false},
		{"Page 1", "Page 1 of 10", true},
		{"$24.50", "Total: $24.50", true},
		{"Add to cart", "Add to cart (2)", true},
		{"Under 4 minutes", "Under 3 minutes 3 - 20 minutes Over 20 minutes 4K 360° VR180", false},
		{"button2", "button2", true},
	}
	for _, c := range cases {
		if got := MatchesQuery(c.query, el(c.text)); got != c.want {
			t.Errorf("MatchesQuery(%q, %q) = %v, want %v", c.query, c.text, got, c.want)
		}
	}
}

func TestRank_NumberDecidesBetweenNearTwins(t *testing.T) {
	three := dom.ElementSnapshot{ID: 1, Tag: "a", VisibleText: "Under 3 minutes", IsVisible: true, XPath: "/a[1]"}
	four := dom.ElementSnapshot{ID: 2, Tag: "a", VisibleText: "Under 4 minutes", IsVisible: true, XPath: "/a[2]"}
	three.Normalize()
	four.Normalize()
	ranked := Rank("Under 4 minutes", "link", "clickable", []dom.ElementSnapshot{three, four}, 2, nil)
	if len(ranked) == 0 || ranked[0].Element.ID != 2 {
		t.Fatalf("winner = %+v, want 'Under 4 minutes'", ranked)
	}
}
