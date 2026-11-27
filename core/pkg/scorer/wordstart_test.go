package scorer

import (
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
)

// On YouTube, CLICK the 'All' tab on a page with no such tab clicked the
// search box and passed: the box held "… behind the counter in a small town",
// and "all" is inside "small". A query is matched where a word begins, so an
// inflected word still finds its longer form and a word inside another does
// not.
func TestMatchesQuery_AWordInsideAnotherIsNotAMatch(t *testing.T) {
	search := dom.ElementSnapshot{Tag: "input", InputType: "text", Name: "search_query",
		Value: "Pearl Jam Elderly Woman Behind the Counter in a Small Town"}
	search.Normalize()
	for _, c := range []struct {
		query, text string
		want        bool
	}{
		{"All", "", false},            // the search box above
		{"Install", "Install", true},  // the word itself
		{"all", "Install now", false}, // at the end of a word
		{"Product", "Products", true}, // an inflected form: a word that begins with it
		{"товар", "Товари зі знижкою", true},
		{"вар", "Товари зі знижкою", false},
		{"Small Town", "", true}, // a phrase that begins where a word does
		{"mall Town", "", false},
		{"$24.50", "Price: $24.50", true}, // a query that starts with a sign
		{"e-mail", "Your e-mail", true},
		{"mail", "Your e-mail", true}, // after a hyphen a word begins
		{"购物车", "加入购物车", true},        // no spaces in Chinese: any character begins one
		{"カート", "カートに入れる", true},
	} {
		el := &search
		if c.text != "" {
			e := dom.ElementSnapshot{Tag: "button", VisibleText: c.text}
			e.Normalize()
			el = &e
		}
		if got := MatchesQuery(c.query, el); got != c.want {
			t.Errorf("MatchesQuery(%q, %q) = %v, want %v", c.query, el.VisibleText+el.Value, got, c.want)
		}
	}
}

// What tells "is it still on the page" goes by the same rule: a wait for
// 'All' to disappear must not be held up by the word "small".
func TestContainsQuery_AWordInsideAnotherIsNotThere(t *testing.T) {
	p := dom.ElementSnapshot{Tag: "p", VisibleText: "A small town"}
	p.Normalize()
	if ContainsQuery("all", &p) {
		t.Error("ContainsQuery: 'all' found inside 'small'")
	}
	if !ContainsQuery("small", &p) {
		t.Error("ContainsQuery: 'small' not found")
	}
}
