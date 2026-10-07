package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/lifecycle"
	"github.com/alexbeatnik/manul-browser/core/pkg/runtime"
)

func navigateHunt(t *testing.T, url string) *dsl.Hunt {
	t.Helper()
	h, err := dsl.Parse(strings.NewReader(
		fmt.Sprintf("STEP 1: nav\n    NAVIGATE to '%s'\nDONE.\n", url)))
	if err != nil {
		t.Fatalf("parse hunt: %v", err)
	}
	return h
}

// perWorkerFactory returns a WorkerFactory where each worker invocation
// receives its own fresh MockPage, preventing cross-goroutine data races.
func perWorkerFactory() WorkerFactory {
	var mu sync.Mutex
	cursor := 0
	return func(ctx context.Context, opts Options) (*Worker, error) {
		mu.Lock()
		i := cursor
		cursor++
		mu.Unlock()
		page := &runtime.MockPage{
			URL:   fmt.Sprintf("https://example.test/w%d", i),
			Title: fmt.Sprintf("worker-%d", i),
		}
		return AdoptWorker(opts.ID, config.Default(), page, nil), nil
	}
}

// errFactory returns a WorkerFactory that always fails with the given message.
func errFactory(msg string) WorkerFactory {
	return func(_ context.Context, _ Options) (*Worker, error) {
		return nil, errors.New(msg)
	}
}

// TestPool_Run_OrderPreserved verifies that Pool.Run returns one result per
// input hunt, in the same order as the input slice, regardless of which
// worker processed it.
func TestPool_Run_OrderPreserved(t *testing.T) {
	const N = 7
	hunts := make([]*dsl.Hunt, N)
	for i := range hunts {
		hunts[i] = navigateHunt(t, fmt.Sprintf("https://example.test/%d", i))
	}

	pool, err := NewPool(PoolOptions{
		Concurrency: 3,
		Allocator:   NewPortAllocator(1, 10),
		Factory:     perWorkerFactory(),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, runErr := pool.Run(ctx, hunts)
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if len(results) != N {
		t.Fatalf("expected %d results, got %d", N, len(results))
	}
	for i, r := range results {
		if r.Hunt != hunts[i] {
			t.Fatalf("results[%d].Hunt mismatch: result contains wrong hunt pointer", i)
		}
		if r.Err != nil {
			t.Fatalf("results[%d].Err = %v", i, r.Err)
		}
	}
}

// TestPool_Run_AllSpawnFail verifies that when every worker fails to spawn,
// Run returns a non-nil error and every result carries a non-nil error.
// This exercises the post-wg.Wait backfill for hunts that were never processed.
func TestPool_Run_AllSpawnFail(t *testing.T) {
	hunts := []*dsl.Hunt{
		navigateHunt(t, "https://a.test"),
		navigateHunt(t, "https://b.test"),
		navigateHunt(t, "https://c.test"),
	}
	pool, err := NewPool(PoolOptions{
		Concurrency: 2,
		Allocator:   NewPortAllocator(1, 10),
		Factory:     errFactory("chrome failed to start"),
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, runErr := pool.Run(ctx, hunts)
	if runErr == nil {
		t.Fatal("expected non-nil error when all workers fail to spawn")
	}
	if len(results) != len(hunts) {
		t.Fatalf("expected %d results, got %d", len(hunts), len(results))
	}
	for i, r := range results {
		if r.Err == nil {
			t.Fatalf("results[%d].Err should be non-nil after all-spawn-failure", i)
		}
		if r.Hunt != hunts[i] {
			t.Fatalf("results[%d].Hunt mismatch after backfill", i)
		}
	}
}

// TestPool_Run_EmptyInput verifies that Run with no hunts returns immediately
// without error.
func TestPool_Run_EmptyInput(t *testing.T) {
	pool, _ := NewPool(PoolOptions{
		Concurrency: 2,
		Allocator:   NewPortAllocator(1, 10),
		Factory:     errFactory("should not be called"),
	})
	results, err := pool.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("empty run: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// TestPool_Run_FailFast verifies that FailFast cancels in-flight hunts on the
// first failure.
func TestPool_Run_FailFast(t *testing.T) {
	// Create a hunt that will fail (bad command type)
	badHunt := &dsl.Hunt{
		Commands: []dsl.Command{
			{Type: dsl.CommandType("INVALID_COMMAND"), Raw: "INVALID"},
		},
	}
	goodHunt := navigateHunt(t, "https://example.test/good")

	hunts := []*dsl.Hunt{badHunt, goodHunt, goodHunt}

	pool, err := NewPool(PoolOptions{
		Concurrency: 3,
		Allocator:   NewPortAllocator(1, 10),
		Factory:     perWorkerFactory(),
		FailFast:    true,
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, runErr := pool.Run(ctx, hunts)
	if runErr == nil {
		t.Fatal("expected non-nil error with FailFast")
	}
	if len(results) != len(hunts) {
		t.Fatalf("expected %d results, got %d", len(hunts), len(results))
	}
	// At least the bad hunt should have an error
	if results[0].Err == nil {
		t.Fatal("expected bad hunt to fail")
	}
}

// TestPool_Run_PartialSpawnFail verifies that when some workers fail to spawn
// but others succeed, the successful workers still process their hunts.
func TestPool_Run_PartialSpawnFail(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	factory := func(_ context.Context, _ Options) (*Worker, error) {
		mu.Lock()
		calls++
		c := calls
		mu.Unlock()
		if c == 1 {
			return nil, errors.New("first worker fails")
		}
		page := &runtime.MockPage{URL: "https://example.test", Title: "ok"}
		return AdoptWorker(c, config.Default(), page, nil), nil
	}

	hunts := []*dsl.Hunt{
		navigateHunt(t, "https://a.test"),
		navigateHunt(t, "https://b.test"),
	}
	pool, err := NewPool(PoolOptions{
		Concurrency: 2,
		Allocator:   NewPortAllocator(1, 10),
		Factory:     factory,
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, runErr := pool.Run(ctx, hunts)
	// Should have at least one success and one error (either spawn error or context cancelled)
	if runErr == nil {
		t.Fatal("expected non-nil error with partial spawn failure")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// At least one result should have succeeded
	foundSuccess := false
	for _, r := range results {
		if r.Err == nil {
			foundSuccess = true
			break
		}
	}
	if !foundSuccess {
		t.Fatal("expected at least one successful result with partial spawn failure")
	}
}

// A worker runs many hunts on one page. What the first one SET must not be
// visible to the second — at row scope it would shadow the second file's own
// @var, and which file inherits it would depend on scheduling.
func TestPool_Run_HuntsDoNotInheritEachOthersVariables(t *testing.T) {
	parse := func(src string) *dsl.Hunt {
		h, err := dsl.Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return h
	}
	hunts := []*dsl.Hunt{
		parse("SET {who} = first\nPRINT 'who={who}'\n"),
		parse("@var: {who} = second\nPRINT 'who={who}'\n"),
		parse("PRINT 'who={who}'\n"),
	}

	pool, err := NewPool(PoolOptions{
		Concurrency: 1, // one worker, so the hunts share it in order
		Allocator:   NewPortAllocator(40100, 40110),
		Factory:     perWorkerFactory(),
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err := pool.Run(context.Background(), hunts)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"who=first", "who=second", "who={who}"}
	for i, r := range results {
		steps := r.Result.Results
		if got := steps[len(steps)-1].ActionValue; got != want[i] {
			t.Errorf("hunt %d printed %q, want %q", i, got, want[i])
		}
	}
}

// A before-group hook publishes for the hunt it brackets. The pool used to
// seed globals once, when the worker started — before any such hook had run.
func TestPool_Run_HuntSeesWhatItsGroupHookPublished(t *testing.T) {
	lifecycle.Reset()
	t.Cleanup(lifecycle.Reset)
	if err := lifecycle.RegisterBeforeGroup("smoke", func(_ context.Context, g *lifecycle.GlobalContext) error {
		g.SetVar("token", "abc123")
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	hunt, err := dsl.Parse(strings.NewReader("@tags: smoke\nPRINT 'token={token}'\n"))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := NewPool(PoolOptions{
		Concurrency: 1,
		Allocator:   NewPortAllocator(40100, 40110),
		Factory:     perWorkerFactory(),
		Lifecycle:   lifecycle.NewGlobalContext(),
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err := pool.Run(context.Background(), []*dsl.Hunt{hunt})
	if err != nil {
		t.Fatal(err)
	}
	if got := results[0].Result.Results[0].ActionValue; got != "token=abc123" {
		t.Errorf("printed %q", got)
	}
}

// The pool used to run a data-driven hunt once, with no row at all — `@data:`
// only meant anything in a sequential run.
func TestPool_Run_DataDrivenHuntRunsOncePerRow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("user\nann\nbob\ncid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hunt, err := dsl.Parse(strings.NewReader("@data: users.csv\nPRINT 'hello {user}'\n"))
	if err != nil {
		t.Fatal(err)
	}
	hunt.SourcePath = filepath.Join(dir, "greet.hunt")
	plain, _ := dsl.Parse(strings.NewReader("PRINT 'no data'\n"))

	pool, err := NewPool(PoolOptions{
		Concurrency: 2,
		Allocator:   NewPortAllocator(40100, 40110),
		Factory:     perWorkerFactory(),
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err := pool.Run(context.Background(), []*dsl.Hunt{hunt, plain})
	if err != nil {
		t.Fatal(err)
	}

	var printed []string
	for _, row := range results[0].Rows {
		printed = append(printed, row.Results[0].ActionValue)
	}
	if got := strings.Join(printed, ","); got != "hello ann,hello bob,hello cid" {
		t.Errorf("rows printed %q", got)
	}
	if results[0].Result == nil || !results[0].Result.Success {
		t.Errorf("summary result = %+v", results[0].Result)
	}
	if results[1].Rows != nil {
		t.Errorf("a hunt without @data: reported rows: %v", results[1].Rows)
	}
}

// --retries in parallel mode: a hunt that fails once and then passes is a
// flaky pass, on a Runtime that does not remember the first attempt.
func TestPool_Run_RetriesAFailedHunt(t *testing.T) {
	runtime.ResetRuntimeRegistries()
	t.Cleanup(runtime.ResetRuntimeRegistries)

	var mu sync.Mutex
	calls := 0
	if err := runtime.RegisterGoCall("flaky.once", func(context.Context, runtime.GoCallInvocation) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return nil, errors.New("first attempt fails")
		}
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}

	parse := func() *dsl.Hunt {
		h, err := dsl.Parse(strings.NewReader("SET {seen} = yes\nCALL GO flaky.once into {r}\nPRINT 'r={r}'\n"))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	run := func(retries int) PoolResult {
		cfg := config.Default()
		cfg.Retries = retries
		pool, err := NewPool(PoolOptions{
			Concurrency: 1,
			Config:      cfg,
			Allocator:   NewPortAllocator(40100, 40110),
			Factory:     perWorkerFactory(),
		})
		if err != nil {
			t.Fatal(err)
		}
		results, _ := pool.Run(context.Background(), []*dsl.Hunt{parse()})
		return results[0]
	}

	if r := run(0); r.Err == nil || r.Result.Success {
		t.Errorf("without retries the failure stands: err=%v", r.Err)
	}

	mu.Lock()
	calls = 0
	mu.Unlock()
	r := run(2)
	if r.Err != nil || !r.Result.Success || !r.Result.Flaky || r.Result.Attempts != 2 {
		t.Errorf("err=%v success=%v flaky=%v attempts=%d", r.Err, r.Result.Success, r.Result.Flaky, r.Result.Attempts)
	}
}
