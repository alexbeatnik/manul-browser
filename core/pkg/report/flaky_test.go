package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
)

// run_history.json is read by other tools, and "flaky" is one of the statuses
// its contract lists. It was never written: nothing retried, so nothing could
// be flaky.
func TestAppendRunHistory_Statuses(t *testing.T) {
	dir := t.TempDir()
	results := []*explain.HuntResult{
		{HuntFile: "a.hunt", Success: true},
		{HuntFile: "b.hunt", Success: false},
		{HuntFile: "c.hunt", Success: true, SoftErrors: []string{"soft"}},
		{HuntFile: "d.hunt", Success: true, Flaky: true, Attempts: 2},
		{HuntFile: "e.hunt", Success: true, Flaky: true, Attempts: 3, SoftErrors: []string{"soft"}},
	}
	for _, r := range results {
		if err := AppendRunHistory(dir, r); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, "run_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var entry RunHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unreadable history line %q: %v", line, err)
		}
		got = append(got, entry.Status)
	}
	if want := "pass,fail,warning,flaky,flaky"; strings.Join(got, ",") != want {
		t.Errorf("statuses = %v, want %s", got, want)
	}
}

func TestGenerateHTML_SaysWhenAHuntWasFlaky(t *testing.T) {
	dir := t.TempDir()
	path, err := GenerateHTML(&explain.HuntResult{Title: "checkout", Success: true, Flaky: true, Attempts: 2}, dir)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(path)
	if !strings.Contains(string(page), "Flaky: passed on attempt 2") {
		t.Error("the report does not mention the retry")
	}
}
