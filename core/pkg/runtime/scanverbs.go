package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alexbeatnik/Manul/core/pkg/dsl"
	"github.com/alexbeatnik/Manul/core/pkg/scan"
	"github.com/alexbeatnik/Manul/core/pkg/scorer"
)

// In-hunt scanning and highlighting.
//
// `manul scan` already covers the offline case — point it at a URL, get a draft.
// These verbs cover the other one: a page you can only reach by driving there,
// behind a login or three steps into a flow, where the interesting question is
// "what is on screen right now".

// highlightTarget flashes a border around a resolved element.
//
// Purely diagnostic: it changes nothing on the page beyond a temporary outline,
// and a target that does not resolve is an error rather than a silent no-op —
// a highlight nobody can see is not worth passing.
func (rt *Runtime) highlightTarget(ctx context.Context, cmd dsl.Command) error {
	target := rt.resolveVariables(cmd.Target)
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("HIGHLIGHT: missing target")
	}

	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("HIGHLIGHT: %w", err)
	}

	mode := string(cmd.InteractionMode)
	if mode == "" {
		mode = string(dsl.ModeNone)
	}
	ranked := scorer.Rank(target, cmd.TypeHint, mode, elements, 1, nil)
	if len(ranked) == 0 || ranked[0].Explain.Score.Total <= 0 {
		return fmt.Errorf("HIGHLIGHT: could not resolve %q", target)
	}

	winner := ranked[0].Element
	rt.logger.ActionDetail("🖍", "Highlighting %q → <%s>", target, winner.Tag)
	return rt.page.HighlightElement(ctx, winner.ID, winner.XPath, 2000)
}

// fullScan renders every interactive control on the page as Markdown, grouped
// by landmark.
//
// Markdown rather than JSON because the audience is a person reading a terminal
// or a model reading a prompt, and both do better with a table than with a
// nested object.
func (rt *Runtime) fullScan(ctx context.Context) (string, error) {
	groups, err := scan.ScanCurrentPageFull(ctx, rt.page)
	if err != nil {
		return "", fmt.Errorf("FULL SCAN: %w", err)
	}

	url, _ := rt.page.CurrentURL(ctx)

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n# Full scan — %s\n", url)
	if len(groups) == 0 {
		sb.WriteString("\n(no interactive controls found)\n")
		return sb.String(), nil
	}

	// Landmark order is not meaningful, but a stable order is: a scan diffed
	// against an earlier one should show real changes, not reshuffling.
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	total := 0
	for _, name := range names {
		elements := groups[name]
		if len(elements) == 0 {
			continue
		}
		total += len(elements)
		fmt.Fprintf(&sb, "\n## %s\n\n", name)
		sb.WriteString("| role | label | tag | editable | locator |\n")
		sb.WriteString("|---|---|---|---|---|\n")
		for _, el := range elements {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
				mdCell(el.Role),
				mdCell(el.Label),
				mdCell(el.Tag),
				yesNo(el.Editable),
				mdCell(el.Locator))
		}
	}
	fmt.Fprintf(&sb, "\n%d control(s) in %d group(s).\n", total, len(names))
	return sb.String(), nil
}

// scanPageDraft renders the current page as a draft .hunt, optionally writing
// it to a file.
func (rt *Runtime) scanPageDraft(ctx context.Context, outputFile string) (string, error) {
	groups, err := scan.ScanCurrentPageFull(ctx, rt.page)
	if err != nil {
		return "", fmt.Errorf("SCAN PAGE: %w", err)
	}

	url, _ := rt.page.CurrentURL(ctx)
	if url == "" {
		url = "about:current"
	}
	draft := scan.BuildHuntFull(url, groups)

	if name := strings.TrimSpace(outputFile); name != "" {
		if err := os.WriteFile(name, []byte(draft), 0o644); err != nil {
			return "", fmt.Errorf("SCAN PAGE: write %s: %w", name, err)
		}
		rt.logger.ActionDetail("💾", "Draft written to %s", name)
	}
	return draft, nil
}

// mdCell keeps a value from breaking the table it sits in.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.Join(strings.Fields(s), " ")
	const max = 60
	if len(s) > max {
		return s[:max-1] + "…"
	}
	if s == "" {
		return "—"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
