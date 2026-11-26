package scorer

import (
	"reflect"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
)

func TestSignificantWords_FiltersStopWordsAndShortWords(t *testing.T) {
	got := SignificantWords("the price of a coffee")
	want := []string{"price", "coffee"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SignificantWords = %v, want %v", got, want)
	}
}

func TestSignificantWords_CountsRunesNotBytes(t *testing.T) {
	// Single-letter Cyrillic function words are 2 bytes but 1 rune — they must
	// be dropped just like single-letter Latin words, or word-overlap scoring
	// on Ukrainian queries is diluted by «і»/«в»/«з»/«у».
	got := SignificantWords("пошук і фільтри в каталозі")
	want := []string{"пошук", "фільтри", "каталозі"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SignificantWords = %v, want %v", got, want)
	}
	// Two-letter Cyrillic words stay.
	got = SignificantWords("на головну")
	want = []string{"на", "головну"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SignificantWords = %v, want %v", got, want)
	}
}

// MatchesQuery accepts word overlap; ContainsQuery is the stricter question a
// wait asks — is the thing itself still here.
func TestContainsQuery_WantsThePhraseNotAWordOfIt(t *testing.T) {
	sentence := dom.ElementSnapshot{Tag: "p", VisibleText: "Elements (e.g., checkbox, input field) change."}
	sentence.Normalize()
	if !MatchesQuery("A checkbox", &sentence) {
		t.Fatal("precondition: the sentence is a word-overlap match")
	}
	if ContainsQuery("A checkbox", &sentence) {
		t.Error("a sentence that mentions checkboxes is not 'A checkbox'")
	}

	row := dom.ElementSnapshot{Tag: "div", VisibleText: "  A   checkbox "}
	row.Normalize()
	field := dom.ElementSnapshot{Tag: "input", HTMLId: "user-name"}
	field.Normalize()
	if !ContainsQuery("a checkbox", &row) || !ContainsQuery("User name", &field) {
		t.Error("the phrase itself, or the id it spells, must match")
	}
}

// A column whose only card is 'Write tests' scores exactly what the card does.
// DOM order put the column first, and DRAG pressed the mouse on empty column.
func TestRank_TieGoesToTheDescendant(t *testing.T) {
	const col = "/html[1]/body[1]/main[1]/div[1]"
	elements := []dom.ElementSnapshot{
		{ID: 1, Tag: "div", XPath: col, VisibleText: "Write tests", IsVisible: true},
		{ID: 2, Tag: "div", XPath: col + "/div[1]", VisibleText: "Write tests", IsVisible: true},
		{ID: 3, Tag: "div", XPath: "/html[1]/body[1]/main[1]/div[11]", VisibleText: "Write tests", IsVisible: true}, // shares a prefix, not a parent
	}
	ranked := Rank("Write tests", "", "clickable", elements, 3, nil)
	if got := []int{ranked[0].Element.ID, ranked[1].Element.ID, ranked[2].Element.ID}; !reflect.DeepEqual(got, []int{2, 1, 3}) {
		t.Errorf("rank order = %v, want the card, then its column, then the unrelated div", got)
	}
}

func TestIsXPathAncestor(t *testing.T) {
	for _, c := range []struct {
		anc, desc string
		want      bool
	}{
		{"/html[1]/body[1]/div[1]", "/html[1]/body[1]/div[1]/span[1]", true},
		{"/html[1]/body[1]/div[1]", "/html[1]/body[1]/div[11]", false},
		{"/html[1]/body[1]/div[1]", "/html[1]/body[1]/div[1]", false},
		{"", "/html[1]", false},
	} {
		if got := IsXPathAncestor(c.anc, c.desc); got != c.want {
			t.Errorf("IsXPathAncestor(%q, %q) = %v, want %v", c.anc, c.desc, got, c.want)
		}
	}
}

// A cart link that is only a picture is named by its alt text. Without that
// name, 'Cart' went to the first "ADD TO CART" button, which added a product.
func TestRank_PictureLinkIsNamedByItsAlt(t *testing.T) {
	elements := []dom.ElementSnapshot{
		{ID: 1, Tag: "button", XPath: "/html[1]/body[1]/div[2]/button[1]", VisibleText: "ADD TO CART", IsVisible: true},
		{ID: 2, Tag: "a", XPath: "/html[1]/body[1]/header[1]/a[1]", VisibleText: "1", AccessibleName: "Cart", IsVisible: true},
	}
	for i := range elements {
		elements[i].Normalize()
	}
	ranked := Rank("Cart", "link", "clickable", elements, 2, nil)
	if ranked[0].Element.ID != 2 {
		t.Errorf("winner = %q, want the cart link", ranked[0].Element.VisibleText)
	}
}
