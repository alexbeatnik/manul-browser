package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexbeatnik/Manul/core/pkg/config"
	"github.com/alexbeatnik/Manul/core/pkg/serve"
	"github.com/alexbeatnik/Manul/core/pkg/utils"
)

// cmdServe runs the persistent session server. Language bindings spawn this and
// speak NDJSON over stdio rather than paying process + attach cost per step —
// and, more importantly, so DSL variables survive between steps.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	stdio := fs.Bool("stdio", true, "speak the protocol over stdin/stdout (currently the only transport)")
	verbose := fs.Bool("verbose", false, "enable verbose logging on stderr")
	cdpEndpoint := fs.String("cdp", "", "CDP endpoint to attach to; implies --attach")
	attach := fs.Bool("attach", false, "drive an already-running Chrome")
	launch := fs.Bool("launch", false, "start a new Chrome (default)")
	headless := fs.Bool("headless", false, "run a launched Chrome headless")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: manul serve [--stdio] [--attach|--launch] [--cdp <url>]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if _, err := parseInterleaved(fs, args); err != nil {
		return err
	}
	if !*stdio {
		return fmt.Errorf("serve: only --stdio is supported")
	}
	if *attach && *launch {
		return fmt.Errorf("serve: --attach and --launch are mutually exclusive")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("serve: load config: %w", err)
	}
	if *cdpEndpoint != "" {
		cfg.CDPEndpoint = *cdpEndpoint
	}
	switch {
	case *attach:
		cfg.BrowserMode = config.ModeAttach
	case *launch:
		cfg.BrowserMode = config.ModeLaunch
	}
	if *headless {
		cfg.Headless = true
	}
	cfg.Verbose = cfg.Verbose || *verbose

	// stdout carries the protocol and nothing else, so logs go to stderr.
	level := utils.LogLevelInfo
	if cfg.Verbose {
		level = utils.LogLevelDebug
	}
	logger := utils.NewLoggerTo(os.Stderr, nil).WithLevel(level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return serve.Serve(ctx, os.Stdin, os.Stdout, serve.Options{
		EngineVersion: version,
		Schema:        engineSchema,
		Config:        cfg,
		Logger:        logger,
	})
}
