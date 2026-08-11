package dsl

import "testing"

// ── WAIT FOR SELECTOR ────────────────────────────────────────────────────────

func TestWaitForSelector(t *testing.T) {
	cases := map[string]string{
		`WAIT FOR SELECTOR '#late'`:              "#late",
		`WAIT FOR SELECTOR ".card > .title"`:     ".card > .title",
		`wait for selector 'ytd-video-renderer'`: "ytd-video-renderer",
		`WAIT FOR SELECTOR '[data-qa="submit"]'`: `[data-qa="submit"]`,
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			cmd := parseOne(t, line)
			if cmd.Type != CmdWaitForSelector {
				t.Fatalf("type = %v, want %v", cmd.Type, CmdWaitForSelector)
			}
			if cmd.Selector != want {
				t.Errorf("selector = %q, want %q", cmd.Selector, want)
			}
		})
	}
}

// A CSS selector must not be mistaken for a human label: the two take entirely
// different paths through the runtime.
func TestWaitForSelectorIsNotConfusedWithWaitFor(t *testing.T) {
	sel := parseOne(t, `WAIT FOR SELECTOR '#thing'`)
	if sel.Type != CmdWaitForSelector {
		t.Errorf("WAIT FOR SELECTOR parsed as %v", sel.Type)
	}
	if sel.Target != "" {
		t.Errorf("a selector must not populate Target, got %q", sel.Target)
	}

	label := parseOne(t, `WAIT FOR 'Load More' to be visible`)
	if label.Type != CmdWaitFor {
		t.Errorf("WAIT FOR parsed as %v", label.Type)
	}
	if label.Target != "Load More" || label.WaitForState != "visible" {
		t.Errorf("target=%q state=%q", label.Target, label.WaitForState)
	}
	if label.Selector != "" {
		t.Errorf("a label must not populate Selector, got %q", label.Selector)
	}
}

func TestWaitForResponseStillWins(t *testing.T) {
	cmd := parseOne(t, `WAIT FOR RESPONSE "api/items"`)
	if cmd.Type != CmdWaitForResponse {
		t.Errorf("type = %v, want %v", cmd.Type, CmdWaitForResponse)
	}
}

// ── FULL SCAN / SCAN PAGE ────────────────────────────────────────────────────

func TestFullScan(t *testing.T) {
	for _, line := range []string{"FULL SCAN", "full scan"} {
		if cmd := parseOne(t, line); cmd.Type != CmdFullScan {
			t.Errorf("%q → %v, want %v", line, cmd.Type, CmdFullScan)
		}
	}
}

func TestScanPage(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"SCAN PAGE", ""},
		{"scan page", ""},
		{"SCAN PAGE into {draft.hunt}", "draft.hunt"},
		{"SCAN PAGE to {out/draft.hunt}", "out/draft.hunt"},
		{"SCAN PAGE into 'draft.hunt'", "draft.hunt"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			cmd := parseOne(t, tc.line)
			if cmd.Type != CmdScanPage {
				t.Fatalf("type = %v, want %v", cmd.Type, CmdScanPage)
			}
			if cmd.ScanOutput != tc.want {
				t.Errorf("output = %q, want %q", cmd.ScanOutput, tc.want)
			}
		})
	}
}
