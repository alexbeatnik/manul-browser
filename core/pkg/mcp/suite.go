package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
)

// suiteGuide is how to build a test suite on Manul: the layout, the
// configuration, shared blocks, environments and secrets, data, running and
// CI. It is compiled in, so it describes the engine it ships with; its
// example project is parsed by the unit tests and run by the e2e suite.
//
//go:embed suite_guide.md
var suiteGuide string

var suiteGuideTool = tool{
	Name:  "suite_guide",
	Title: "How to build a test suite",
	Description: `How to build a test suite on Manul that lives in a repository and runs again in CI: the project layout, ` +
		`manul.config.json, shared blocks, environments and secrets, data-driven tests, running and the CI job, with a whole example project. ` +
		`Read it before writing .hunt files for someone's project. It opens no browser.`,
	InputSchema: schema(nil, map[string]any{}),
	Annotations: annotations{Title: "How to build a test suite", ReadOnlyHint: true, IdempotentHint: true},
	offline:     true,
	run: func(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
		var none struct{}
		if err := decode(raw, &none); err != nil {
			return errorResult("bad arguments: " + err.Error()), nil
		}
		return okResult(suiteGuide), nil
	},
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Arguments   []promptArgument `json:"arguments"`

	render func(args map[string]string) string `json:"-"`
}

// prompts are what a person picks to start a piece of work: Claude Code
// offers them as /mcp__manul-browser__<name>, Claude Desktop in its menu of
// the connector. The model never calls them itself; suite_guide is the
// tool it reaches for.
var prompts = []prompt{
	{
		Name:  "build_test_suite",
		Title: "Build a test suite",
		Description: "Build a Manul test suite for a web app: explore it with the browser, write the tests, " +
			"the shared blocks, the configuration and a CI job, and run them until they pass.",
		Arguments: []promptArgument{
			{Name: "url", Description: "Address of the app to test, e.g. http://localhost:3000.", Required: true},
			{Name: "journeys", Description: "What to cover, e.g. sign in, search, checkout. Empty: the main journeys you find."},
		},
		render: func(args map[string]string) string {
			var b strings.Builder
			b.WriteString("Build a Manul test suite for the app at " + args["url"] + ".")
			if j := strings.TrimSpace(args["journeys"]); j != "" {
				b.WriteString(" Cover: " + j + ".")
			}
			b.WriteString(" Follow the guide below. Explore the app with the manul-browser tools (step and map) before writing any step, " +
				"and copy labels exactly from map. If you can write files and run commands here, create the files in this repository " +
				"and run each test with manul run until it passes, then the whole suite; otherwise give me the files. " +
				"Ask me for test accounts, secrets and the CI address when you need them.\n\n")
			b.WriteString(suiteGuide)
			return b.String()
		},
	},
}

func promptByName(name string) (prompt, bool) {
	for _, p := range prompts {
		if p.Name == name {
			return p, true
		}
	}
	return prompt{}, false
}
