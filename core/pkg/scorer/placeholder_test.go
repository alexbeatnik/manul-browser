package scorer

import (
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
)

// Comfy types example searches into its search box's placeholder a letter at
// a time. The map named the field 'Пошук товарів'; by the time FILL ran the
// placeholder said 'павер', nothing on the page matched, and the step failed
// with a list of paragraphs as the closest labels.
func TestPlaceholder_AFieldIsFoundByAHintItShowedBefore(t *testing.T) {
	search := dom.ElementSnapshot{
		ID: 1, Tag: "input", InputType: "text", IsVisible: true, IsEditable: true, XPath: "/input[1]",
		Placeholder:      "павер",
		PlaceholdersSeen: []string{"Пошук товарів", "павер"},
	}
	email := dom.ElementSnapshot{
		ID: 2, Tag: "input", InputType: "text", IsVisible: true, IsEditable: true, XPath: "/input[2]",
		Placeholder: "email@email.com",
	}
	para := dom.ElementSnapshot{ID: 3, Tag: "p", IsVisible: true, XPath: "/p[1]",
		VisibleText: "Comfy пропонує величезний перелік товарів"}
	for _, e := range []*dom.ElementSnapshot{&search, &email, &para} {
		e.Normalize()
	}

	if !MatchesQuery("Пошук товарів", &search) {
		t.Error("MatchesQuery: the field that showed 'Пошук товарів' does not match it")
	}
	ranked := Rank("Пошук товарів", "field", "input", []dom.ElementSnapshot{para, email, search}, 3, nil)
	if len(ranked) == 0 || ranked[0].Element.ID != 1 {
		t.Fatalf("winner = %+v, want the search field", ranked)
	}
	if got := ranked[0].Explain.Score.Total; got < 0.5 {
		t.Errorf("score %.2f: the hint it showed should count as a placeholder match", got)
	}
}

// The hint it shows now still counts, and a hint it never showed does not.
func TestPlaceholder_OnlyHintsTheFieldShowed(t *testing.T) {
	f := dom.ElementSnapshot{Tag: "input", Placeholder: "блендер", PlaceholdersSeen: []string{"Пошук товарів", "блендер"}}
	f.Normalize()
	if s := scorePlaceholder("блендер", &f); s != 0.7 {
		t.Errorf("current hint: %.2f, want 0.7", s)
	}
	if s := scorePlaceholder("пошук", &f); s != 0.4 {
		t.Errorf("part of an earlier hint: %.2f, want 0.4", s)
	}
	if s := scorePlaceholder("мультиварка", &f); s != 0 {
		t.Errorf("a hint it never showed: %.2f, want 0", s)
	}
}
