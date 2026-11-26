package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// `SCAN PAGE into {reports/draft.hunt}` is the natural way to keep drafts out
// of the way, and reports/ does not exist until something creates it. The write
// used to fail there with "no such file or directory".
func TestScanPageCreatesTheOutputDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	mock := &MockPage{URL: "https://example.com/", Title: "Example"}
	mock.EvalResult = func(expr string) ([]byte, bool) {
		return []byte(`{"Page":[{"role":"button","label":"Go","tag":"button","editable":false,"locator":"#go"}]}`), true
	}

	rt := New(config.Config{}, mock, utils.NewLogger(nil))
	out := filepath.Join("reports", "drafts", "page.hunt")
	draft, err := rt.scanPageDraft(context.Background(), out)
	if err != nil {
		t.Fatalf("SCAN PAGE into a missing directory failed: %v", err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("draft not written: %v", err)
	}
	if string(written) != draft || !strings.Contains(draft, "https://example.com/") {
		t.Fatalf("written draft does not match the returned one:\n%s", written)
	}
}
