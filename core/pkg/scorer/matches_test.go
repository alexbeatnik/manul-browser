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

// "button" names a kind of control, not a particular one. 'Cross button'
// overlapped 'Host button' by exactly that word, and the host's button was
// clicked for one in a frame the engine does not enter.
func TestMatchesQuery_RoleWordAloneIsNotAMatch(t *testing.T) {
	el := func(tag, text string) *dom.ElementSnapshot {
		e := &dom.ElementSnapshot{Tag: tag, VisibleText: text, IsVisible: true}
		e.Normalize()
		return e
	}
	cases := []struct {
		query, tag, text string
		want             bool
	}{
		{"Cross button", "button", "Host button", false},
		{"Cross button", "button", "Cross button", true},
		{"Cross button", "button", "Cross", true},
		{"Help link", "a", "Privacy link", false},
		{"Help link", "a", "Help", true},
		{"button", "button", "Host button", true},
		{"CPU of Chrome", "td", "Chrome CPU", true},
	}
	for _, c := range cases {
		if got := MatchesQuery(c.query, el(c.tag, c.text)); got != c.want {
			t.Errorf("MatchesQuery(%q, %q) = %v, want %v", c.query, c.text, got, c.want)
		}
	}
}

// A caption in a <p> spans the viewport. Measured from its middle, the field
// right under it was 600px away and scored no proximity at all.
func TestScoreNear_WideAnchorIsMeasuredFromItsNearestPoint(t *testing.T) {
	caption := &AnchorContext{
		Rect:  dom.Rect{Left: 8, Top: 16, Width: 1350, Height: 18},
		XPath: "/html[1]/body[1]/p[1]",
	}
	field := &dom.ElementSnapshot{Tag: "input", XPath: "/html[1]/body[1]/input[1]",
		Rect: dom.Rect{Left: 8, Top: 50, Width: 153, Height: 21}}
	far := &dom.ElementSnapshot{Tag: "input", XPath: "/html[1]/body[1]/input[2]",
		Rect: dom.Rect{Left: 8, Top: 450, Width: 153, Height: 21}}
	near, away := scoreNear(field, caption), scoreNear(far, caption)
	if near < 0.5 {
		t.Errorf("field under the caption: proximity %.2f, want >= 0.5", near)
	}
	if away >= near {
		t.Errorf("field 400px further down scored %.2f, not below %.2f", away, near)
	}
}

// Half the words was a match: 'Add to cart' shares only "cart" with the
// paragraph "Cart: 0", and with the products not loaded yet CLICK clicked the
// paragraph and passed. More than half is the bar, as it was for labels.
func TestMatchesQuery_HalfTheWordsIsNotAMatch(t *testing.T) {
	el := func(text string) *dom.ElementSnapshot {
		e := &dom.ElementSnapshot{Tag: "p", VisibleText: text, IsVisible: true}
		e.Normalize()
		return e
	}
	cases := []struct {
		query, text string
		want        bool
	}{
		{"Add to cart", "Cart: 0", false},
		{"Add to cart", "Add to cart", true},
		{"Submit your application today", "Submit your application", true},
		{"Submit your application today", "Submit", false},
		{"CPU of Chrome", "Chrome CPU 7.4%", true},
	}
	for _, c := range cases {
		if got := MatchesQuery(c.query, el(c.text)); got != c.want {
			t.Errorf("MatchesQuery(%q, %q) = %v, want %v", c.query, c.text, got, c.want)
		}
	}
}
