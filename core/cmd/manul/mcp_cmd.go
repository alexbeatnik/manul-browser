package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/mcp"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// cmdMCP serves the engine to MCP clients over stdio: Claude Desktop, Claude
// Code, or any other client of the Model Context Protocol. The browser starts
// with the first tool call and closes when the client goes away.
func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	browserName := fs.String("browser", "", "browser engine to launch: chromium (CDP, default) or firefox (WebDriver BiDi)")
	headless := fs.Bool("headless", false, "run a launched browser without a window")
	cdpEndpoint := fs.String("cdp", "", "drive an already-running browser at this endpoint instead of launching one; implies --attach")
	attach := fs.Bool("attach", false, "drive an already-running browser (at --cdp, or the configured endpoint)")
	channel := fs.String("channel", "", "browser channel to launch (chrome, chrome-beta, chromium, msedge, firefox, firefox-esr, …)")
	executablePath := fs.String("executable-path", "", "absolute path of the browser to launch")
	userDataDir := fs.String("user-data-dir", "", "browser profile directory, kept between sessions (default: a fresh temp profile each time)")
	locale := fs.String("locale", "", "language the browser uses and asks pages for, e.g. en-US or uk-UA (default en-US)")
	verbose := fs.Bool("verbose", false, "verbose logging on stderr")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: manul mcp [flags]\n\n"+
			"  Serves the engine to MCP clients (Claude Desktop, Claude Code, …) over stdio.\n"+
			"  The browser starts with the first tool call and closes when the client goes away.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if _, err := parseInterleaved(fs, args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}
	if *browserName != "" {
		if _, err := browser.NormalizeEngine(*browserName); err != nil {
			return err
		}
		cfg.Browser = *browserName
	}
	if *headless {
		cfg.Headless = true
	}
	if *cdpEndpoint != "" {
		cfg.CDPEndpoint = *cdpEndpoint
		cfg.BrowserMode = config.ModeAttach
	}
	if *attach {
		cfg.BrowserMode = config.ModeAttach
	}
	if *channel != "" {
		c := *channel
		cfg.Channel = &c
	}
	if *executablePath != "" {
		p := *executablePath
		cfg.ExecutablePath = &p
	}
	if *locale != "" {
		cfg.Locale = *locale
	}
	cfg.Verbose = cfg.Verbose || *verbose
	// stdin and stdout belong to the protocol: the debugger, which prompts on
	// one and reads the other, would corrupt both.
	cfg.DebugMode = false
	cfg.BreakLines = nil
	// Nothing is written beside the client's working directory, which for a
	// desktop app may be anywhere — or not writable at all.
	cfg.HTMLReport = false
	cfg.Screenshot = "none"

	level := utils.LogLevelInfo
	if cfg.Verbose {
		level = utils.LogLevelDebug
	}
	logger := utils.NewLoggerTo(os.Stderr, nil).WithLevel(level)

	open := func(ctx context.Context) (mcp.Backend, error) {
		opts := agent.Options{
			Headless:       cfg.Headless,
			ExecutablePath: derefString(cfg.ExecutablePath),
			UserDataDir:    *userDataDir,
			Browser:        cfg.Browser,
			Config:         &cfg,
			Logger:         logger,
		}
		if cfg.ResolveBrowserMode() == config.ModeAttach {
			endpoint := cfg.AttachEndpoint()
			logger.Info("mcp: attaching to the browser at %s", endpoint)
			return agent.Attach(ctx, endpoint, "", opts)
		}
		engine, _ := browser.NormalizeEngine(cfg.Browser)
		logger.Info("mcp: launching %s", engine)
		return agent.Launch(ctx, opts)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mcp.Serve(ctx, os.Stdin, os.Stdout, mcp.Options{
		EngineVersion: version,
		Open:          open,
		Logger:        logger,
	})
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
