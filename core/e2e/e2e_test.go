//go:build e2e

// Package e2e runs .hunt files against real browsers.
//
// The unit tests drive mock WebSocket servers: they prove framing, not what a
// browser does with it. Every bug in this directory's history was invisible to
// them, so these tests build the manul binary, serve testdata/site from two
// local origins and run each hunt in testdata/hunts through the CLI, the way a
// user would. Nothing here touches the network.
//
//	go test -tags e2e ./e2e/                              # every browser found
//	MANUL_E2E_BROWSERS=firefox go test -tags e2e ./e2e/   # just one, required
//	go test -tags e2e ./e2e/ -run 'TestHunts/chromium/dnd'
//
// A hunt may carry these comment lines among its headers:
//
//	# expect-fail: <text>    the hunt must fail, and a failed step's error
//	                         must contain <text>
//	# expect-output: <text>  the run's output must contain <text> (for PRINT)
//
// In a hunt and in a page, @SITE@ is the origin serving testdata/site,
// @CROSS@ another port of 127.0.0.1 serving the same files — cross-origin
// but the same site — and @XSITE@ that second server under the name
// localhost, which is another site: Chrome runs a frame from it in a process
// of its own.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// huntTimeout bounds one hunt on one browser. The slowest case here takes a
// few seconds; a hang is what this is for.
const huntTimeout = 2 * time.Minute

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "manul-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "manul")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/manul")
	build.Dir = ".."
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build manul:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// origins are the three a fixture can name; see the package doc.
type origins struct{ site, cross, xsite string }

func (o *origins) replace(s string) string {
	return strings.NewReplacer("@SITE@", o.site, "@CROSS@", o.cross, "@XSITE@", o.xsite).Replace(s)
}

// serve starts a file server for testdata/site on its own 127.0.0.1 port.
// Pages are served with the origins filled in.
func serve(t *testing.T, o *origins) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("testdata", "site")
	files := http.FileServer(http.Dir(root))
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".html") {
			files.ServeHTTP(w, r)
			return
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(o.replace(string(body))))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

// browsers is the list to run. Named in MANUL_E2E_BROWSERS, each one must be
// installed; otherwise both are tried and one that is missing is skipped.
func browsers() (names []string, required bool) {
	if env := strings.TrimSpace(os.Getenv("MANUL_E2E_BROWSERS")); env != "" {
		for _, b := range strings.Split(env, ",") {
			if b = strings.TrimSpace(b); b != "" {
				names = append(names, b)
			}
		}
		return names, true
	}
	return []string{"chromium", "firefox"}, false
}

type expectations struct {
	failWith string
	output   []string
}

func readExpectations(src string) expectations {
	var e expectations
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "# expect-fail:"); ok {
			e.failWith = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "# expect-output:"); ok {
			e.output = append(e.output, strings.TrimSpace(v))
		}
	}
	return e
}

type huntResult struct {
	Success bool `json:"success"`
	Results []struct {
		Step    string `json:"step"`
		Success bool   `json:"success"`
		Error   string `json:"error"`
	} `json:"results"`
}

func TestHunts(t *testing.T) {
	o := &origins{}
	o.site = serve(t, o)
	o.cross = serve(t, o)
	o.xsite = strings.Replace(o.cross, "127.0.0.1", "localhost", 1)

	paths, err := filepath.Glob(filepath.Join("testdata", "hunts", "*.hunt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no hunts found: %v", err)
	}

	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			for _, path := range paths {
				name := strings.TrimSuffix(filepath.Base(path), ".hunt")
				t.Run(name, func(t *testing.T) {
					runHunt(t, browser, required, path, o)
				})
			}
		})
	}
}

func runHunt(t *testing.T, browser string, required bool, path string, o *origins) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := o.replace(string(raw))
	want := readExpectations(src)

	dir := t.TempDir()
	hunt := filepath.Join(dir, filepath.Base(path))
	if err := os.WriteFile(hunt, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), huntTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", hunt,
		"--browser", browser, "--headless", "--json", "--screenshot", "none")
	cmd.Dir = dir // reports and the like stay out of the source tree
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	logs := stderr.String()

	if !required && (strings.Contains(logs, "chrome not found") || strings.Contains(logs, "firefox not found")) {
		t.Skipf("%s is not installed", browser)
	}
	if ctx.Err() != nil {
		t.Fatalf("hunt did not finish in %v\n%s", huntTimeout, logs)
	}

	var res huntResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &res); err != nil {
		t.Fatalf("no JSON result on stdout (%v; exit: %v)\nstdout:\n%s\nstderr:\n%s", err, runErr, stdout.String(), logs)
	}

	var failures []string
	for _, r := range res.Results {
		if !r.Success {
			failures = append(failures, fmt.Sprintf("%s: %s", r.Step, r.Error))
		}
	}

	if want.failWith != "" {
		if res.Success {
			t.Fatalf("hunt passed; it must fail with %q\n%s", want.failWith, logs)
		}
		for _, f := range failures {
			if strings.Contains(f, want.failWith) {
				return
			}
		}
		t.Fatalf("hunt failed, but no step's error contains %q:\n  %s", want.failWith, strings.Join(failures, "\n  "))
	}

	if !res.Success {
		t.Fatalf("hunt failed:\n  %s\n\nstderr:\n%s", strings.Join(failures, "\n  "), logs)
	}
	all := stdout.String() + logs
	for _, o := range want.output {
		if !strings.Contains(all, o) {
			t.Errorf("output does not contain %q\nstderr:\n%s", o, logs)
		}
	}
}
