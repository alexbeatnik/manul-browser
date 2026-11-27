package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
)

// The guide is for building a suite before any page is open: reading it must
// not start a browser.
func TestSuiteGuide_OpensNoBrowser(t *testing.T) {
	c := start(t, openFake(&fakeBackend{}))
	out, isErr := c.call("suite_guide", map[string]any{})
	if isErr || !strings.HasPrefix(out, "# Building a test suite on Manul") {
		t.Fatalf("isError=%v:\n%.200s", isErr, out)
	}
	if c.opens != 0 {
		t.Errorf("suite_guide opened the browser %d times", c.opens)
	}
	if out, isErr := c.call("suite_guide", map[string]any{"topic": "ci"}); !isErr || !strings.Contains(out, "bad arguments") {
		t.Errorf("an argument it does not take: isError=%v %s", isErr, out)
	}
}

func TestPrompts_BuildTestSuite(t *testing.T) {
	c := start(t, openFake(&fakeBackend{}))

	c.request("prompts/list", map[string]any{})
	var list struct {
		Prompts []prompt `json:"prompts"`
	}
	if err := json.Unmarshal(c.read().Result, &list); err != nil || len(list.Prompts) != 1 || list.Prompts[0].Name != "build_test_suite" {
		t.Fatalf("prompts/list: %+v %v", list, err)
	}

	c.request("prompts/get", map[string]any{"name": "build_test_suite", "arguments": map[string]string{"url": "http://localhost:3000", "journeys": "checkout"}})
	var got struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(c.read().Result, &got); err != nil || len(got.Messages) != 1 {
		t.Fatalf("prompts/get: %+v %v", got, err)
	}
	m := got.Messages[0]
	for _, want := range []string{"http://localhost:3000", "Cover: checkout.", "# Building a test suite on Manul"} {
		if m.Role != "user" || m.Content.Type != "text" || !strings.Contains(m.Content.Text, want) {
			t.Errorf("the message does not carry %q", want)
		}
	}

	for _, bad := range []map[string]any{
		{"name": "build_test_suite"},                              // no url
		{"name": "write_poems", "arguments": map[string]string{}}, // no such prompt
	} {
		c.request("prompts/get", bad)
		if r := c.read(); r.Error == nil || r.Error.Code != codeInvalidParams {
			t.Errorf("%v: want -32602, got %+v", bad, r.Error)
		}
	}
	if c.opens != 0 {
		t.Errorf("the prompts opened the browser %d times", c.opens)
	}
}

// guideFile is a fenced block of the guide that names its path:
// ```hunt tests/login.hunt.
var guideFile = regexp.MustCompile("(?s)```[a-z]+ ([^\\s`]+)\n(.*?)```")

// The guide's example project is a project: every hunt in it parses with its
// imports resolved, its data file is where @data: looks, and its config uses
// only keys the engine reads — an unknown key is ignored without a word, so
// a guide that invented one would be believed. The e2e suite runs it.
func TestSuiteGuide_ExampleProjectIsAProject(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for _, m := range guideFile.FindAllStringSubmatch(suiteGuide, -1) {
		files[m[1]] = m[2]
		p := filepath.Join(dir, filepath.FromSlash(m[1]))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(m[2]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"manul.config.json", "lib/env.hunt", "lib/account.hunt", "data/users.csv", ".github/workflows/manul.yml"} {
		if files[want] == "" {
			t.Errorf("the guide has no %s", want)
		}
	}

	tests := 0
	for path := range files {
		if !strings.HasSuffix(path, ".hunt") {
			continue
		}
		h, err := dsl.ParseFileWithVars(filepath.Join(dir, path), map[string]string{"password": "from-ci"})
		if err == nil {
			err = dsl.ResolveImports(h)
		}
		if err == nil {
			err = h.Expand()
		}
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if strings.HasPrefix(path, "tests/") {
			tests++
			if h.Title == "" || h.Context == "" || len(h.Tags) == 0 {
				t.Errorf("%s: a test without @title, @context or @tags", path)
			}
			if h.DataFile != "" {
				if _, err := os.Stat(filepath.Join(dir, "tests", h.DataFile)); err != nil {
					t.Errorf("%s: @data: %s is not where the file says: %v", path, h.DataFile, err)
				}
			}
		}
	}
	if tests < 3 {
		t.Errorf("%d tests in the example project", tests)
	}

	known := map[string]bool{}
	ct := reflect.TypeOf(config.Config{})
	for i := 0; i < ct.NumField(); i++ {
		if tag := strings.Split(ct.Field(i).Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
			known[tag] = true
		}
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(files["manul.config.json"]), &cfg); err != nil {
		t.Fatalf("manul.config.json: %v", err)
	}
	for k := range cfg {
		if !known[k] {
			t.Errorf("manul.config.json: %q is not a key the engine reads", k)
		}
	}
}
