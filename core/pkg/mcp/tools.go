package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
)

// instructions is what the client is told about the server as a whole. It is
// sent once per session, so it says how the tools fit together and nothing a
// tool's own description already says.
const instructions = `Manul drives a real browser with plain-English steps and finds elements by the text a person sees — no selectors and no element references. ` +
	`Work in a loop: call map to see the labelled controls of the page, then step with one line that names one of them exactly, ` +
	`such as CLICK the 'Sign in' button or FILL 'Email' field with 'ada@example.com'. ` +
	`Labels do not go stale when the page changes the way references do, so map again only when you are on a new page or cannot find what you need. ` +
	`Check with step VERIFY that '<text>' is present rather than reading the page again. ` +
	`Use read for one value, read_text for prose, run_hunt for several steps at once. ` +
	`Iframes and new tabs need nothing special: a tab a step opens is where the next step runs.`

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

func text(s string) content { return content{Type: "text", Text: s} }

func okResult(s string) toolResult    { return toolResult{Content: []content{text(s)}} }
func errorResult(s string) toolResult { return toolResult{Content: []content{text(s)}, IsError: true} }

type annotations struct {
	Title           string `json:"title"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

type tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations annotations    `json:"annotations"`

	run func(ctx context.Context, b Backend, args json.RawMessage) (toolResult, error) `json:"-"`
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

// readOnly is a tool that only looks. acting is one that can change the page,
// and with it whatever the page changes: a click can submit a form, place an
// order or delete a record, so the client is told it may be destructive.
var (
	readOnly = func(title string) annotations {
		return annotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: true}
	}
	acting = func(title string) annotations {
		return annotations{Title: title, DestructiveHint: true, OpenWorldHint: true}
	}
)

var tools = []tool{
	{
		Name:  "step",
		Title: "Run a step",
		Description: `Run one step in the browser, written as one line of Manul's plain-English language. ` +
			`The element is found by the text a person sees, quoted: copy it exactly from map. Examples: ` +
			`NAVIGATE to https://example.com | CLICK the 'Sign in' button | CLICK the 'Pricing' link | ` +
			`FILL 'Email' field with 'ada@example.com' | SELECT 'Blue' from the 'Colour' dropdown | ` +
			`CHECK the checkbox for 'Remember me' | PRESS Enter | HOVER over the 'Menu' | SCROLL DOWN | ` +
			`DRAG 'Card' and drop it into 'Done' | UPLOAD '/abs/path/file.pdf' to 'Attachment' | ` +
			`VERIFY that 'Welcome' is present | VERIFY that 'Error' is NOT present | VERIFY 'Email' field has value 'ada@example.com' | ` +
			`WAIT FOR 'Results' to be visible | SWITCH TO TAB 1 | SWITCH TO TAB 'Checkout'. ` +
			`When several elements share a label, narrow it: CLICK the 'Edit' button NEAR 'Ada Lovelace', ` +
			`CLICK the 'Delete' button INSIDE 'Users' row with 'Ada', CLICK the 'Login' button ON HEADER. ` +
			`An element still loading is waited for. A failed step says why and, when it can, lists the closest real labels to use instead.`,
		InputSchema: schema([]string{"step"}, map[string]any{
			"step": prop("string", "One line of the language, e.g. CLICK the 'Sign in' button."),
		}),
		Annotations: acting("Run a step"),
		run:         runStep,
	},
	{
		Name:  "map",
		Title: "See the page",
		Description: `List the labelled interactive elements of the current page — links, buttons, fields, dropdowns, checkboxes — ` +
			`grouped by landmark (header, main, a form, a dialog …), each with its exact label and the verb that acts on it. ` +
			`Iframes and shadow DOM are included. At most max_per_group elements a group are listed; a group that has more says how many.`,
		InputSchema: schema(nil, map[string]any{
			"max_per_group":     prop("integer", "Elements to list per landmark group. Default 8; raise it when what you need is in the '+N more'."),
			"include_unlabeled": prop("boolean", "Also list elements with no visible label. Default false."),
		}),
		Annotations: readOnly("See the page"),
		run:         runMap,
	},
	{
		Name:  "read",
		Title: "Read a value",
		Description: `Read one value off the page by the label a person sees next to it — a price, a total, a status, ` +
			`or what a form field currently holds ('Order total', 'Email'). A table cell is named by its row and column ('Chrome CPU').`,
		InputSchema: schema([]string{"target"}, map[string]any{
			"target": prop("string", "The label of the value, e.g. Order total."),
		}),
		Annotations: readOnly("Read a value"),
		run:         runRead,
	},
	{
		Name:  "read_text",
		Title: "Read the text",
		Description: `Read the visible text of the page, or of one region of it by CSS selector — an article, a search result, an answer panel. ` +
			`Text a person cannot see is left out. Long text is cut at max_chars.`,
		InputSchema: schema(nil, map[string]any{
			"selector":  prop("string", "CSS selector of the region. Empty for the whole page."),
			"max_chars": prop("integer", "Longest text to return. Default 20000."),
		}),
		Annotations: readOnly("Read the text"),
		run:         runReadText,
	},
	{
		Name:  "run_hunt",
		Title: "Run several steps",
		Description: `Run several steps at once: a .hunt script, one step per line, in the same language as step. ` +
			`STEP 1: <name> headings, IF/ELSE, REPEAT, FOR EACH and @var: {name} = value are allowed. ` +
			`The run stops at the first failed step and says which one, so fix that step instead of running again the ones that passed.`,
		InputSchema: schema([]string{"script"}, map[string]any{
			"script": prop("string", "The steps, one per line."),
		}),
		Annotations: acting("Run several steps"),
		run:         runHunt,
	},
}

func toolList() []tool { return tools }

func toolByName(name string) (tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return tool{}, false
}

// decode reads a tool's arguments. Arguments that do not fit the schema are
// a mistake the model can correct, so they come back as a tool error with
// the reason, not as a protocol error.
func decode(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("bad arguments: %v", err)
	}
	return nil
}

func runStep(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
	var a struct {
		Step string `json:"step"`
	}
	if err := decode(raw, &a); err != nil {
		return errorResult(err.Error()), nil
	}
	if strings.TrimSpace(a.Step) == "" {
		return errorResult("step is empty: give one line, e.g. CLICK the 'Sign in' button."), nil
	}
	if strings.Contains(strings.TrimSpace(a.Step), "\n") {
		return errorResult("step takes one line; use run_hunt for several."), nil
	}
	out, err := b.Step(ctx, a.Step)
	if out.Step == "" {
		out.Step = strings.TrimSpace(a.Step)
	}
	return toolResult{Content: []content{text(out.DescribeForLLM())}, IsError: !out.OK}, err
}

func runHunt(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
	var a struct {
		Script string `json:"script"`
	}
	if err := decode(raw, &a); err != nil {
		return errorResult(err.Error()), nil
	}
	if strings.TrimSpace(a.Script) == "" {
		return errorResult("script is empty."), nil
	}
	out, err := b.Run(ctx, a.Script)
	if err != nil && out.TotalSteps == 0 {
		return errorResult(err.Error()), err
	}
	return toolResult{Content: []content{text(out.RenderForLLM())}, IsError: !out.OK}, err
}

func runMap(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
	var a struct {
		MaxPerGroup      int  `json:"max_per_group"`
		IncludeUnlabeled bool `json:"include_unlabeled"`
	}
	if err := decode(raw, &a); err != nil {
		return errorResult(err.Error()), nil
	}
	if a.MaxPerGroup < 0 {
		return errorResult("max_per_group must not be negative."), nil
	}
	pm, err := b.Map(ctx, agent.MapBudget{MaxPerGroup: a.MaxPerGroup, IncludeUnlabeled: a.IncludeUnlabeled})
	if err != nil {
		return errorResult("Could not read the page: " + err.Error()), err
	}
	out := pm.RenderForLLM(0)
	if out == "" {
		where := ""
		if pm.URL != "" {
			where = " (" + pm.URL + ")"
		}
		return okResult("No labelled interactive elements on this page" + where + ". It may still be loading, or empty: NAVIGATE somewhere, or read_text to see what is there."), nil
	}
	return okResult(out), nil
}

func runRead(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
	var a struct {
		Target string `json:"target"`
	}
	if err := decode(raw, &a); err != nil {
		return errorResult(err.Error()), nil
	}
	if strings.TrimSpace(a.Target) == "" {
		return errorResult("target is empty: give the label of the value, e.g. Order total."), nil
	}
	v, err := b.Read(ctx, a.Target)
	if err != nil {
		return errorResult("Could not read the page: " + err.Error()), err
	}
	if !v.Found {
		return errorResult(fmt.Sprintf("Nothing on the page is labelled %q. Use map to see the labels that are there.", a.Target)), nil
	}
	if v.Text == "" {
		return okResult("(empty)"), nil
	}
	return okResult(v.Text), nil
}

// defaultMaxChars bounds read_text when the caller sets no limit: about five
// thousand tokens, enough for an article, short of a whole Wikipedia page.
const defaultMaxChars = 20000

func runReadText(ctx context.Context, b Backend, raw json.RawMessage) (toolResult, error) {
	var a struct {
		Selector string `json:"selector"`
		MaxChars *int   `json:"max_chars"`
	}
	if err := decode(raw, &a); err != nil {
		return errorResult(err.Error()), nil
	}
	limit := defaultMaxChars
	if a.MaxChars != nil {
		limit = *a.MaxChars
	}
	if limit < 0 {
		return errorResult("max_chars must not be negative."), nil
	}
	t, err := b.ReadText(ctx, a.Selector)
	if errors.Is(err, agent.ErrBadSelector) {
		return errorResult(fmt.Sprintf("%q is not a valid CSS selector.", a.Selector)), nil
	}
	if err != nil {
		return errorResult("Could not read the page: " + err.Error()), err
	}
	if t == "" {
		if a.Selector != "" {
			return errorResult(fmt.Sprintf("Nothing on the page matches %q, or it has no visible text.", a.Selector)), nil
		}
		return okResult("(the page has no visible text)"), nil
	}
	return okResult(agent.TruncateText(t, limit)), nil
}
