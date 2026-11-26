package dsl

import (
	"os"
	"path/filepath"
	"testing"
)

// importHunt writes lib.hunt and main.hunt into a fresh directory and returns
// main parsed, with its imports resolved and expanded.
func importHunt(t *testing.T, lib, main string) *Hunt {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"lib.hunt": lib, "main.hunt": main} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h, err := ParseFile(filepath.Join(dir, "main.hunt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolveImports(h); err != nil {
		t.Fatalf("imports: %v", err)
	}
	if err := h.Expand(); err != nil {
		t.Fatalf("expand: %v", err)
	}
	return h
}

const loginLibrary = "@var: {user} = ada\n" +
	"STEP LogIn:\n" +
	"    FILL 'Username' field with '{user}'\n"

// A library's @var: used to be written into its steps when it was read, so
// the hunt importing it could not change the value — 'grace' below logged in
// as 'ada'. It is a default at the import level now, as the contract says.
func TestImportedVarsAreDefaultsNotLiterals(t *testing.T) {
	for name, line := range map[string]string{
		"named":    "@import: LogIn from 'lib.hunt'\n",
		"wildcard": "@import: * from 'lib.hunt'\n",
	} {
		h := importHunt(t, loginLibrary, line+"@var: {user} = grace\nUSE LogIn\n")
		if len(h.Commands) != 1 {
			t.Fatalf("%s: got %d commands", name, len(h.Commands))
		}
		if got := h.Commands[0].Value; got != "{user}" {
			t.Errorf("%s: library step value = %q, want the {user} placeholder", name, got)
		}
		if got := h.ImportVars["user"]; got != "ada" {
			t.Errorf("%s: ImportVars[user] = %q, want ada", name, got)
		}
		if got := h.Vars["user"]; got != "grace" {
			t.Errorf("%s: Vars[user] = %q, want the importing hunt's own grace", name, got)
		}
	}
}

func TestImportedVarsReachThroughNestedImports(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("base.hunt", "@var: {site} = http://base\n@var: {user} = base-user\nSTEP Open:\n    NAVIGATE to {site}\n")
	write("lib.hunt", "@import: Open from 'base.hunt'\n@var: {user} = lib-user\nSTEP LogIn:\n    USE Open\n")
	write("main.hunt", "@import: LogIn from 'lib.hunt'\nUSE LogIn\n")

	h, err := ParseFile(filepath.Join(dir, "main.hunt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolveImports(h); err != nil {
		t.Fatal(err)
	}
	if h.ImportVars["site"] != "http://base" {
		t.Errorf("site = %q, want it inherited from base.hunt", h.ImportVars["site"])
	}
	if h.ImportVars["user"] != "lib-user" {
		t.Errorf("user = %q, want lib.hunt's own value over the one it imported", h.ImportVars["user"])
	}
}

// A parse-time count still needs its value at parse time, library or not.
func TestImportedRepeatCountIsStillExpanded(t *testing.T) {
	lib := "@var: {n} = 3\nSTEP Loop:\n    REPEAT {n} TIMES:\n        PRINT 'x'\n"
	h := importHunt(t, lib, "@import: Loop from 'lib.hunt'\nUSE Loop\n")
	if len(h.Commands) != 1 || h.Commands[0].Type != CmdRepeat {
		t.Fatalf("got %+v", h.Commands)
	}
	if h.Commands[0].RepeatCount != 3 {
		t.Errorf("REPEAT count = %d, want 3", h.Commands[0].RepeatCount)
	}
}

// filepath.Join put an absolute source under the importing file's directory,
// so an absolute @import: never found its file.
func TestAbsoluteImportPath(t *testing.T) {
	libDir := t.TempDir()
	lib := filepath.Join(libDir, "lib.hunt")
	if err := os.WriteFile(lib, []byte(loginLibrary), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	main := filepath.Join(dir, "main.hunt")
	body := "@import: LogIn from '" + filepath.ToSlash(lib) + "'\nUSE LogIn\n"
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	h, err := ParseFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolveImports(h); err != nil {
		t.Fatalf("absolute import: %v", err)
	}
	if err := h.Expand(); err != nil {
		t.Fatal(err)
	}
	if len(h.Commands) != 1 || h.Commands[0].Type != CmdFill {
		t.Errorf("got %+v", h.Commands)
	}
}
