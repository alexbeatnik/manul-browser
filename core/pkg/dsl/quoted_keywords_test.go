package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A label is free text. These are the cases where a word inside the quotes is
// also a word some verb splits on — each one used to cut the command in the
// wrong place, usually without any error.

func parseLine(t *testing.T, line string) Command {
	t.Helper()
	h, err := Parse(strings.NewReader(line))
	if err != nil {
		t.Fatalf("Parse(%q): %v", line, err)
	}
	if len(h.Commands) != 1 {
		t.Fatalf("Parse(%q): want 1 command, got %d", line, len(h.Commands))
	}
	return h.Commands[0]
}

func TestQualifierWordsInsideLabelAreNotQualifiers(t *testing.T) {
	for _, line := range []string{
		"CLICK 'Stores near me' link",
		"CLICK 'Sign on now' button",
		"CLICK 'Look inside the box' button",
	} {
		cmd := parseLine(t, line)
		if cmd.NearAnchor != "" || cmd.OnRegion != "" || cmd.InsideContainer != "" {
			t.Errorf("%q: near=%q on=%q inside=%q, want none",
				line, cmd.NearAnchor, cmd.OnRegion, cmd.InsideContainer)
		}
	}

	cmd := parseLine(t, "CLICK 'Stores near me' link NEAR 'Find us'")
	if cmd.Target != "Stores near me" || cmd.NearAnchor != "Find us" {
		t.Errorf("target=%q near=%q", cmd.Target, cmd.NearAnchor)
	}
}

func TestSplitWordsInsideLabel(t *testing.T) {
	cmd := parseLine(t, "FILL 'Pay with card' field with '4111'")
	if cmd.Target != "Pay with card" || cmd.Value != "4111" {
		t.Errorf("FILL: target=%q value=%q", cmd.Target, cmd.Value)
	}

	cmd = parseLine(t, "TYPE 'drop into bucket' into 'Notes'")
	if cmd.Value != "drop into bucket" || cmd.Target != "Notes" {
		t.Errorf("TYPE: value=%q target=%q", cmd.Value, cmd.Target)
	}

	cmd = parseLine(t, "SELECT 'Made from wood' from the 'Material' dropdown")
	if cmd.Value != "Made from wood" || cmd.Target != "Material" {
		t.Errorf("SELECT: value=%q target=%q", cmd.Value, cmd.Target)
	}

	cmd = parseLine(t, "CHECK 'Pay for me' checkbox")
	if cmd.Target != "Pay for me" {
		t.Errorf("CHECK: target=%q", cmd.Target)
	}

	cmd = parseLine(t, "WAIT FOR 'Ready to be shipped' to be visible")
	if cmd.Target != "Ready to be shipped" || cmd.WaitForState != "visible" {
		t.Errorf("WAIT FOR: target=%q state=%q", cmd.Target, cmd.WaitForState)
	}

	cmd = parseLine(t, "UPLOAD 'path to file.txt' to 'Avatar'")
	if cmd.UploadFilePath != "path to file.txt" || cmd.Target != "Avatar" {
		t.Errorf("UPLOAD: file=%q target=%q", cmd.UploadFilePath, cmd.Target)
	}

	cmd = parseLine(t, "DRAG 'Salt and pepper' and drop it into 'Cart'")
	if cmd.DragSource != "Salt and pepper" || cmd.DragTarget != "Cart" {
		t.Errorf("DRAG: source=%q target=%q", cmd.DragSource, cmd.DragTarget)
	}

	cmd = parseLine(t, "EXTRACT the 'Deposit into account' into {x}")
	if cmd.Target != "Deposit into account" || cmd.ExtractVar != "x" {
		t.Errorf("EXTRACT: target=%q var=%q", cmd.Target, cmd.ExtractVar)
	}

	cmd = parseLine(t, "PRESS Enter on 'Sign on'")
	if cmd.PressKey != "Enter" || cmd.PressTarget != "Sign on" {
		t.Errorf("PRESS: key=%q target=%q", cmd.PressKey, cmd.PressTarget)
	}
}

func TestVerifyWordsInsideLabel(t *testing.T) {
	cmd := parseLine(t, "VERIFY that 'This is not a drill' is present")
	if cmd.Type != CmdVerify || cmd.VerifyNegated {
		t.Errorf("type=%s negated=%v, want a plain positive VERIFY", cmd.Type, cmd.VerifyNegated)
	}

	cmd = parseLine(t, "VERIFY that 'Item is hidden gem' is present")
	if cmd.Type != CmdVerify || cmd.VerifyState != "" {
		t.Errorf("type=%s state=%q, want a text VERIFY", cmd.Type, cmd.VerifyState)
	}

	cmd = parseLine(t, "VERIFY that 'It has text inside' is present")
	if cmd.Type != CmdVerify {
		t.Errorf("type=%s, want VERIFY", cmd.Type)
	}

	// The real forms still parse.
	cmd = parseLine(t, "VERIFY that 'This is not a drill' is NOT present")
	if !cmd.VerifyNegated {
		t.Error("negation outside the quotes was lost")
	}
	cmd = parseLine(t, "VERIFY 'Total' field has value 'has text x'")
	if cmd.Type != CmdVerifyField || cmd.VerifyFieldKind != "value" || cmd.Value != "has text x" {
		t.Errorf("type=%s kind=%q value=%q", cmd.Type, cmd.VerifyFieldKind, cmd.Value)
	}
}

func TestTypeHintInsideLabelIsNotAHint(t *testing.T) {
	cmd := parseLine(t, "CLICK 'My radio show'")
	if cmd.TypeHint != "" || cmd.InteractionMode != ModeClickable {
		t.Errorf("hint=%q mode=%q, want no hint", cmd.TypeHint, cmd.InteractionMode)
	}
	cmd = parseLine(t, "CLICK the 'My radio show' link")
	if cmd.TypeHint != "link" {
		t.Errorf("hint=%q, want link", cmd.TypeHint)
	}
}

func TestApostropheInsideLabel(t *testing.T) {
	cmd := parseLine(t, "FILL 'Name' field with 'O'Brien'")
	if cmd.Target != "Name" || cmd.Value != "O'Brien" {
		t.Errorf("target=%q value=%q", cmd.Target, cmd.Value)
	}
}

func TestElseInsideSetupAndTeardown(t *testing.T) {
	h, err := Parse(strings.NewReader(`
IF {a} == '1':
    PRINT 'mission'

[SETUP]
IF {b} == '1':
    PRINT 'one'
ELSE:
    PRINT 'other'
[END SETUP]

[TEARDOWN]
IF {c} == '1':
    PRINT 'x'
ELIF {c} == '2':
    PRINT 'y'
ELSE:
    PRINT 'z'
[END TEARDOWN]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Commands) != 1 || len(h.Commands[0].Branches) != 1 {
		t.Errorf("mission IF picked up a branch that belongs elsewhere: %+v", h.Commands)
	}
	if len(h.SetupCommands) != 1 || len(h.SetupCommands[0].Branches) != 2 {
		t.Errorf("setup: want one IF with 2 branches, got %d command(s)", len(h.SetupCommands))
	}
	if len(h.TeardownCommands) != 1 || len(h.TeardownCommands[0].Branches) != 3 {
		t.Errorf("teardown: want one IF with 3 branches, got %d command(s)", len(h.TeardownCommands))
	}
}

func TestRepeatNamesItsLoopVariable(t *testing.T) {
	if cmd := parseLine(t, "REPEAT 5 TIMES as {n}:"); cmd.RepeatCount != 5 || cmd.RepeatVar != "n" {
		t.Errorf("count=%d var=%q", cmd.RepeatCount, cmd.RepeatVar)
	}
	if cmd := parseLine(t, "REPEAT 3 TIMES:"); cmd.RepeatVar != "i" {
		t.Errorf("default var=%q, want i", cmd.RepeatVar)
	}
}

func TestInlineComments(t *testing.T) {
	cmd := parseLine(t, "FILL 'Email' field with 'a@b.c'  # the test account")
	if cmd.Value != "a@b.c" {
		t.Errorf("value=%q", cmd.Value)
	}

	// A '#' that is part of the text is not a comment.
	for line, want := range map[string]string{
		"NAVIGATE to https://example.com/#/login": "https://example.com/#/login",
		"NAVIGATE to 'https://example.com/ # x'":  "https://example.com/ # x",
	} {
		if got := parseLine(t, line).URL; got != want {
			t.Errorf("%q: url=%q, want %q", line, got, want)
		}
	}
	if cmd := parseLine(t, "SET {colour} = #fff"); cmd.SetValue != "#fff" {
		t.Errorf("SET value=%q", cmd.SetValue)
	}
	if cmd := parseLine(t, "WAIT FOR SELECTOR #main"); cmd.Selector != "#main" {
		t.Errorf("selector=%q", cmd.Selector)
	}
	if cmd := parseLine(t, "PRINT 'Issue # 5'"); cmd.PrintText != "Issue # 5" {
		t.Errorf("print=%q", cmd.PrintText)
	}
}

func TestNumberedCommands(t *testing.T) {
	h, err := Parse(strings.NewReader("1. NAVIGATE to https://example.com\n2. CLICK the 'Login' button\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Commands) != 2 || h.Commands[0].Type != CmdNavigate || h.Commands[1].Type != CmdClick {
		t.Fatalf("got %+v", h.Commands)
	}
	if h.Commands[1].Target != "Login" {
		t.Errorf("target=%q", h.Commands[1].Target)
	}
}

func TestContractSpellings(t *testing.T) {
	if cmd := parseLine(t, "DEBUG"); cmd.Type != CmdPause {
		t.Errorf("DEBUG parsed as %s", cmd.Type)
	}
	cmd := parseLine(t, "CHOOSE 'Red' from the 'Colour' dropdown")
	if cmd.Type != CmdSelect || cmd.Value != "Red" || cmd.Target != "Colour" {
		t.Errorf("CHOOSE: type=%s value=%q target=%q", cmd.Type, cmd.Value, cmd.Target)
	}
}

func TestStepHeaders(t *testing.T) {
	// A command that mentions "step" is still a command.
	for _, line := range []string{"USE Checkout step two", "PRINT next step done"} {
		if cmd := parseLine(t, line); cmd.StepBlock != "" {
			t.Errorf("%q ran as part of step %q", line, cmd.StepBlock)
		}
	}

	// A header may quote the thing it describes.
	h, err := Parse(strings.NewReader("STEP 2: Fill the 'Email' field\n    PRINT 'x'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Commands) != 1 || h.Commands[0].StepBlock != "STEP 2: Fill the 'Email' field" {
		t.Errorf("got %+v", h.Commands)
	}
}

func TestScrollWithNothingAfterIt(t *testing.T) {
	// {dir} expands to nothing; this used to index an empty slice.
	h, err := Parse(strings.NewReader("@var: {dir} =\nSCROLL {dir}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Commands) != 1 {
		t.Fatalf("got %d commands", len(h.Commands))
	}
}

func TestNamedImportIsUsableAsWritten(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("auth.hunt", "STEP 1: Login\n    PRINT 'in login'\n")

	for name, body := range map[string]string{
		"plain.hunt": "@import: Login from 'auth.hunt'\nUSE Login\n",
		"alias.hunt": "@import: Login as QuickLogin from 'auth.hunt'\nUSE QuickLogin\n",
	} {
		h, err := ParseFile(write(name, body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := ResolveImports(h); err != nil {
			t.Fatalf("%s: imports: %v", name, err)
		}
		if err := h.Expand(); err != nil {
			t.Fatalf("%s: expand: %v", name, err)
		}
		if len(h.Commands) != 1 || h.Commands[0].Type != CmdPrint {
			t.Errorf("%s: got %+v", name, h.Commands)
		}
	}
}

func TestMaskQuoted(t *testing.T) {
	for in, want := range map[string]string{
		`CLICK 'Sign on' button`:          `CLICK '_______' button`,
		`FILL "a 'b' c" with 'x'`:         `FILL "_______" with '_'`,
		`CLICK 'Don't show again' button`: `CLICK '________________' button`,
		`CLICK 'Save'! NEAR 'Form'`:       `CLICK '____'! NEAR '____'`,
		`IF {role} == 'admin':`:           `IF {role} == '_____':`,
		`CLICK Don't show`:                `CLICK Don't show`,
		`CLICK 'never closed`:             `CLICK 'never closed`,
		`CLICK 'п'ять' NEAR 'x y'`:        `CLICK '_________' NEAR '___'`, // masked per byte, so indexes carry over
		`SET {x} = ''`:                    `SET {x} = ''`,
	} {
		if got := maskQuoted(in); got != want {
			t.Errorf("maskQuoted(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}
