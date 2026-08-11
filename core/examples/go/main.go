// Command example shows how to drive Manul from Go by embedding the engine
// directly — no subprocess, no protocol. This is the Go half of the same story
// bindings/python tells for Python.
//
//	go run ./examples/go                     # launch a fresh Chrome
//	go run ./examples/go -attach             # drive a Chrome already running
//
// Compiled as part of ./... so it cannot quietly rot.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
)

func main() {
	attach := flag.Bool("attach", false, "drive an already-running Chrome instead of launching one")
	cdp := flag.String("cdp", "http://127.0.0.1:9222", "CDP endpoint to attach to")
	headless := flag.Bool("headless", true, "run a launched Chrome headless")
	target := flag.String("url", "https://example.com", "page to open")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Launch owns the browser and closes it; Attach does not — it did not open
	// it. That is the whole difference between the two modes.
	var (
		sess *agent.Session
		err  error
	)
	if *attach {
		sess, err = agent.Attach(ctx, *cdp, "", agent.Options{})
	} else {
		sess, err = agent.Launch(ctx, agent.Options{Headless: *headless})
	}
	if err != nil {
		log.Fatalf("open session: %v", err)
	}
	defer sess.Close()

	if !*attach {
		if out, err := sess.Step(ctx, "NAVIGATE to "+*target); err != nil || !out.OK {
			log.Fatalf("navigate: %v (%s)", err, out.Reason)
		}
	}

	state, err := sess.PageState(ctx)
	if err != nil {
		log.Fatalf("page state: %v", err)
	}
	fmt.Printf("on %q (%s)\n", state.Title, state.URL)

	// map is the cheap way to show a page to an LLM: far fewer tokens than raw
	// HTML for the same context.
	pageMap, err := sess.Map(ctx, agent.MapBudget{MaxPerGroup: 5})
	if err != nil {
		log.Fatalf("map: %v", err)
	}
	for _, group := range pageMap.Groups {
		fmt.Printf("  %s:", group.Name)
		for _, el := range group.Elements {
			fmt.Printf(" %q", el.Label)
		}
		fmt.Println()
	}

	// Variables written by EXTRACT persist for the life of the session — the
	// reason to hold a Session rather than issue one-shot commands.
	if _, err := sess.Step(ctx, "EXTRACT the 'More information' into {link}"); err == nil {
		if vars, err := sess.Vars(ctx); err == nil && vars["link"] != "" {
			fmt.Printf("extracted {link} = %q\n", vars["link"])
		}
	}

	os.Exit(0)
}
