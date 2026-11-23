package dsl

import "testing"

func TestParse_SwitchTab(t *testing.T) {
	cases := []struct {
		line, target, value string
	}{
		{"SWITCH TO TAB 2", "", "2"},
		{"switch to tab 1", "", "1"},
		{"SWITCH TAB 3", "", "3"},
		{"SWITCH TO TAB 'Checkout'", "Checkout", ""},
		{"SWITCH TO TAB {n}", "", "{n}"},
	}
	for _, c := range cases {
		cmd := parseCommandLine(c.line)
		if cmd.Type != CmdSwitchTab {
			t.Errorf("%q parsed as %s", c.line, cmd.Type)
			continue
		}
		if cmd.Target != c.target || cmd.Value != c.value {
			t.Errorf("%q: target %q value %q, want %q %q", c.line, cmd.Target, cmd.Value, c.target, c.value)
		}
	}
	if cmd := parseCommandLine("SWITCH TABLE"); cmd.Type == CmdSwitchTab {
		t.Errorf("SWITCH TABLE parsed as SWITCH TAB")
	}
}
