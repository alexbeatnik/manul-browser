// Package hooks runs an out-of-process extension script for `manul run`.
//
// Custom controls, CALL HOST handlers and suite-level hooks were reachable two
// ways: by embedding the engine in a Go program, or by driving it through a
// binding over the session protocol. The stock binary could reach neither, so
// `lifecycle.IsEmpty()` was always true in cmdRun and the suite wiring there
// could never fire. `--hooks` is the third way, and it deliberately adds no new
// wire format: the script speaks the same NDJSON reverse-call protocol a
// binding already speaks.
//
// What changes is only who spawns whom. A binding starts `manul serve --stdio`
// and talks down the pipe it owns; here the engine starts the script and talks
// down the pipe *it* owns. Both sides then exchange the identical `register` /
// `invoke` / reply lines, which is why this package is mostly process handling
// and almost no protocol.
//
// The script's stdout is the protocol channel and carries nothing else. Its
// stderr is passed through to the engine's stderr, so a hook can print, log and
// crash visibly without corrupting the stream — the same rule the engine
// imposes on itself in serve mode.
package hooks

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/serve"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// shutdownGrace is how long a script gets to notice its input closed and exit
// before it is killed. Generous, because a hook's last act is often flushing a
// report or closing a database handle, and short enough that a wedged script
// cannot hang a CI run indefinitely.
const shutdownGrace = 10 * time.Second

// Host is a running hook script and the reverse-call server bound to it.
type Host struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	log   *utils.Logger
}

// Start launches the script, completes the declaration handshake, and returns
// with every control, call and hook it declared registered in the engine.
//
// It blocks until the script announces `ready`. That is the contract: the
// extension registries are read by worker goroutines without further
// synchronisation, so registration must be finished before the first hunt
// begins rather than racing it.
func Start(ctx context.Context, script string, opts serve.Options) (*Host, error) {
	path, err := filepath.Abs(script)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", script, err)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("hook script: %w", err)
	}

	argv, err := commandFor(path)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// The script runs beside itself, not beside the hunt files, so a hook can
	// import its own helpers with a relative path.
	cmd.Dir = filepath.Dir(path)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("hook script stdout: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("hook script stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start hook script %s: %w", strings.Join(argv, " "), err)
	}

	h := &Host{cmd: cmd, stdin: stdin, log: opts.Logger}
	if h.log == nil {
		h.log = utils.NewLoggerTo(io.Discard, nil)
	}

	peer := serve.NewPeer(stdout, stdin, opts)
	if err := peer.Handshake(ctx); err != nil {
		// The script is still running and still holds the pipes; tearing it down
		// here keeps a failed handshake from leaving an orphan behind.
		h.terminate()
		return nil, fmt.Errorf("hook script %s: %w", filepath.Base(path), err)
	}

	return h, nil
}

// Close shuts the script down and waits for it.
//
// Closing stdin is the signal to exit: the script is blocked reading the next
// invocation, sees EOF, and returns. Only a script that ignores that gets
// killed, and its exit status is reported rather than swallowed — a hook that
// died during teardown is exactly the thing a suite needs told about.
func (h *Host) Close() error {
	if h == nil || h.cmd == nil {
		return nil
	}
	_ = h.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- h.cmd.Wait() }()

	select {
	case err := <-done:
		h.cmd = nil
		if err != nil {
			h.log.Warn("hook script exited: %v", err)
		}
		return err
	case <-time.After(shutdownGrace):
		h.log.Warn("hook script did not exit after its input closed; killing it")
		h.terminate()
		<-done
		h.cmd = nil
		return fmt.Errorf("hook script had to be killed")
	}
}

func (h *Host) terminate() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
}

// commandFor decides how to run the script.
//
// Interpreter choice lives here, in the engine, and not in any binding: the
// binary is what a person types, so it is what has to know that a `.py` file
// needs Python. A binding only ever supplies the library the script imports.
//
// An executable file with no recognised extension is run directly, which is how
// a compiled hook host or a shell script gets in.
func commandFor(path string) ([]string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		// Order matters, and it differs per platform. On Windows `python3` is
		// usually the Microsoft Store's App Execution Alias — a stub that prints
		// an advert and exits 9009, which reaches us as a script that died
		// before saying anything. `python` is the real interpreter there. Every
		// other platform is the other way round, where a bare `python` may still
		// be Python 2.
		candidates := []string{"python3", "python"}
		if runtime.GOOS == "windows" {
			candidates = []string{"python", "python3"}
		}
		exe, err := lookInterpreter("MANUL_PYTHON", candidates...)
		if err != nil {
			return nil, fmt.Errorf("hook script %s needs Python: %w", filepath.Base(path), err)
		}
		return []string{exe, path}, nil

	case ".js", ".mjs", ".cjs":
		exe, err := lookInterpreter("MANUL_NODE", "node")
		if err != nil {
			return nil, fmt.Errorf("hook script %s needs Node: %w", filepath.Base(path), err)
		}
		return []string{exe, path}, nil

	case ".exe":
		return []string{path}, nil
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err == nil && info.Mode()&0o111 != 0 {
			return []string{path}, nil
		}
		return nil, fmt.Errorf(
			"cannot run hook script %s: no known extension (.py, .js) and not executable",
			filepath.Base(path))
	}
	return nil, fmt.Errorf(
		"cannot run hook script %s: no known extension (.py, .js, .exe)",
		filepath.Base(path))
}

// lookInterpreter honours an explicit override before searching PATH. The
// override takes a full path or a bare name, because a virtualenv is named by
// path and a system interpreter by name.
func lookInterpreter(envVar string, candidates ...string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		exe, err := exec.LookPath(v)
		if err != nil {
			return "", fmt.Errorf("%s=%q: %w", envVar, v, err)
		}
		return exe, nil
	}
	for _, name := range candidates {
		if exe, err := exec.LookPath(name); err == nil {
			return exe, nil
		}
	}
	return "", fmt.Errorf("none of %s found on PATH (set %s to choose one)",
		strings.Join(candidates, ", "), envVar)
}
