package hooks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/lifecycle"
	"github.com/alexbeatnik/manul-browser/core/pkg/runtime"
	"github.com/alexbeatnik/manul-browser/core/pkg/serve"
)

// The hook script these tests spawn is this same test binary, re-executed with
// scriptModeEnv set. That keeps a genuine process boundary — real pipes, real
// exit codes, the thing the feature is actually about — without needing Python
// or Node on the machine running the tests.
const scriptModeEnv = "MANUL_TEST_HOOK_MODE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(scriptModeEnv); mode != "" {
		os.Exit(runFakeScript(mode))
	}
	os.Exit(m.Run())
}

// runFakeScript is the child half: it declares what it owns, announces ready,
// then answers reverse calls until its input closes.
func runFakeScript(mode string) int {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 4096), 1024*1024)
	out := json.NewEncoder(os.Stdout)

	// readReply consumes the engine's answer to a command we sent.
	readReply := func() bool { return in.Scan() }

	switch mode {
	case "silent":
		// Exits without ever announcing ready — the handshake must not hang.
		return 3

	case "garbage":
		fmt.Println("this is not JSON")
		readReply()
		return 0

	case "early":
		// Tries to drive the session instead of declaring hooks.
		_ = out.Encode(map[string]any{"id": 1, "cmd": "run-step", "args": map[string]string{"step": "CLICK"}})
		readReply()
		return 0
	}

	_ = out.Encode(map[string]any{
		"id":  1,
		"cmd": "register",
		"args": map[string]any{
			"calls": []string{"test.echo"},
			"hooks": []map[string]string{
				{"kind": "before_all"},
				{"kind": "before_group", "tag": "smoke"},
			},
		},
	})
	if !readReply() {
		return 1
	}

	_ = out.Encode(map[string]any{"id": 2, "cmd": "ready"})
	if !readReply() {
		return 1
	}

	// Serve reverse calls until stdin closes.
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req struct {
			Invoke int      `json:"invoke"`
			Kind   string   `json:"kind"`
			Hook   string   `json:"hook"`
			Tag    string   `json:"tag"`
			Name   string   `json:"name"`
			Args   []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return 1
		}

		var result any
		switch req.Kind {
		case "hook":
			if req.Hook == "before_all" {
				result = map[string]string{"token": "seeded-by-hook"}
			}
		case "call":
			result = strings.Join(req.Args, "|")
		}

		_ = out.Encode(map[string]any{"invoke": req.Invoke, "ok": true, "result": result})
	}
	return 0
}

// startScript spawns this test binary as a hook script in the given mode.
func startScript(t *testing.T, ctx context.Context, mode string) (*Host, error) {
	t.Helper()
	t.Setenv(scriptModeEnv, mode)
	runtime.ResetRuntimeRegistries()
	lifecycle.Reset()
	t.Cleanup(func() {
		runtime.ResetRuntimeRegistries()
		lifecycle.Reset()
	})
	return Start(ctx, os.Args[0], serve.Options{})
}

func TestStartRegistersWhatTheScriptDeclares(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, err := startScript(t, ctx, "normal")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer host.Close()

	// This is the whole point of the feature: before --hooks, the stock binary
	// could never make this false.
	if lifecycle.IsEmpty() {
		t.Fatal("lifecycle registry is empty; the script's hooks did not register")
	}
	if _, ok := runtime.GetGoCall("test.echo"); !ok {
		t.Fatal("CALL handler test.echo did not register")
	}
}

func TestBeforeAllPublishesVariablesBackIntoTheSuite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, err := startScript(t, ctx, "normal")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer host.Close()

	gctx := lifecycle.NewGlobalContext()
	if err := lifecycle.RunBeforeAll(ctx, gctx); err != nil {
		t.Fatalf("RunBeforeAll: %v", err)
	}
	if got := gctx.Vars()["token"]; got != "seeded-by-hook" {
		t.Fatalf("token = %q, want %q", got, "seeded-by-hook")
	}
}

func TestCallHandlerRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, err := startScript(t, ctx, "normal")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer host.Close()

	handler, ok := runtime.GetGoCall("test.echo")
	if !ok {
		t.Fatal("test.echo not registered")
	}
	got, err := handler(ctx, runtime.GoCallInvocation{
		Name: "test.echo",
		Args: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got != "a|b" {
		t.Fatalf("result = %v, want %q", got, "a|b")
	}
}

// The hazard the design carries: group hooks fire from every goroutine in the
// worker pool, and there is exactly one pipe to the script. Run under -race.
func TestConcurrentHooksSerialiseOverOnePipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	host, err := startScript(t, ctx, "normal")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer host.Close()

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lifecycle.RunBeforeGroup(ctx, []string{"smoke"}, lifecycle.NewGlobalContext()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent before-group hook: %v", err)
	}
}

func TestHandshakeFailsWhenScriptNeverAnnouncesReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := startScript(t, ctx, "silent")
	if err == nil {
		t.Fatal("expected an error when the script exits before ready")
	}
	if !strings.Contains(err.Error(), "before it announced ready") {
		t.Fatalf("error = %q, want it to name the missing ready", err)
	}
}

func TestHandshakeRejectsSessionCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := startScript(t, ctx, "early")
	if err == nil {
		t.Fatal("expected an error when the script sends run-step during the handshake")
	}
	if !strings.Contains(err.Error(), "run-step") {
		t.Fatalf("error = %q, want it to name the refused command", err)
	}
}

func TestHandshakeRejectsNonJSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := startScript(t, ctx, "garbage")
	if err == nil {
		t.Fatal("expected an error on a non-JSON line")
	}
	if !strings.Contains(err.Error(), "not a request") {
		t.Fatalf("error = %q, want it to say the line was not a request", err)
	}
}

func TestStartRejectsMissingScript(t *testing.T) {
	ctx := context.Background()
	if _, err := Start(ctx, filepath.Join(t.TempDir(), "nope.py"), serve.Options{}); err == nil {
		t.Fatal("expected an error for a script that does not exist")
	}
}

func TestCommandForChoosesAnInterpreterByExtension(t *testing.T) {
	dir := t.TempDir()

	// A .py path resolves to whatever Python this machine has; if it has none,
	// the error has to say so rather than trying to execute the file.
	py := filepath.Join(dir, "hooks.py")
	if err := os.WriteFile(py, []byte("# hooks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	argv, err := commandFor(py)
	if err != nil {
		if !strings.Contains(err.Error(), "needs Python") {
			t.Fatalf("error = %q, want it to name Python", err)
		}
	} else if len(argv) != 2 || argv[1] != py {
		t.Fatalf("argv = %v, want the interpreter followed by the script", argv)
	}

	// An unknown extension is not guessed at.
	odd := filepath.Join(dir, "hooks.rb")
	if err := os.WriteFile(odd, []byte("# hooks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := commandFor(odd); err == nil {
		t.Fatal("expected an error for an extension with no known interpreter")
	}
}

func TestCloseIsSafeTwice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, err := startScript(t, ctx, "normal")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
