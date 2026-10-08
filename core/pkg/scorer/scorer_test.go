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
