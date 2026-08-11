package dsl

import (
	"strings"
	"testing"
)

// The same command under all three spellings must parse identically: a .hunt
// written for the Python engine keeps working, and new scripts can say HOST.
func TestCallHostSpellingsAreEquivalent(t *testing.T) {
	lines := map[string]string{
		"go":     `CALL GO compute_total with args: "12" into {total}`,
		"python": `CALL PYTHON compute_total with args: "12" into {total}`,
		"host":   `CALL HOST compute_total with args: "12" into {total}`,
	}
	for name, line := range lines {
		t.Run(name, func(t *testing.T) {
			hunt, err := Parse(strings.NewReader(line))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(hunt.Commands) != 1 {
				t.Fatalf("got %d commands", len(hunt.Commands))
			}
			cmd := hunt.Commands[0]
			if cmd.Type != CmdCallGo {
				t.Errorf("type = %v, want %v", cmd.Type, CmdCallGo)
			}
			if cmd.GoCallName != "compute_total" {
				t.Errorf("name = %q, want compute_total", cmd.GoCallName)
			}
			if len(cmd.GoCallArgs) != 1 || cmd.GoCallArgs[0] != "12" {
				t.Errorf("args = %#v, want [12]", cmd.GoCallArgs)
			}
			if cmd.GoCallResultVar != "total" {
				t.Errorf("result var = %q, want total", cmd.GoCallResultVar)
			}
		})
	}
}
