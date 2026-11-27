package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// The contract describes shipped behaviour, and a contract nobody checks
// drifts: this holds tools/list and the protocol versions to it.
func TestContract_MatchesTheServer(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "spec", "contracts", "MANUL_MCP_CONTRACT.md"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile("(?s)```json\n(.*?)```").FindSubmatch(raw)
	if block == nil {
		t.Fatal("no JSON block in the contract")
	}
	var contract struct {
		ProtocolVersions []string `json:"protocolVersions"`
		Tools            []struct {
			Name       string `json:"name"`
			Title      string `json:"title"`
			Parameters map[string]struct {
				Type     string `json:"type"`
				Required bool   `json:"required"`
			} `json:"parameters"`
			Annotations struct {
				ReadOnlyHint    bool `json:"readOnlyHint"`
				DestructiveHint bool `json:"destructiveHint"`
				IdempotentHint  bool `json:"idempotentHint"`
				OpenWorldHint   bool `json:"openWorldHint"`
			} `json:"annotations"`
		} `json:"tools"`
		Prompts []struct {
			Name      string `json:"name"`
			Title     string `json:"title"`
			Arguments map[string]struct {
				Required bool `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(block[1], &contract); err != nil {
		t.Fatalf("the contract's JSON does not parse: %v", err)
	}

	if !reflect.DeepEqual(contract.ProtocolVersions, protocolVersions) {
		t.Errorf("protocol versions: contract %v, server %v", contract.ProtocolVersions, protocolVersions)
	}
	if len(contract.Tools) != len(toolList()) {
		t.Fatalf("contract has %d tools, the server %d", len(contract.Tools), len(toolList()))
	}
	for i, want := range contract.Tools {
		got := toolList()[i]
		if got.Name != want.Name || got.Title != want.Title || got.Annotations.Title != want.Title {
			t.Errorf("tool %d: server %q %q, contract %q %q", i, got.Name, got.Title, want.Name, want.Title)
			continue
		}
		a := got.Annotations
		if a.ReadOnlyHint != want.Annotations.ReadOnlyHint || a.DestructiveHint != want.Annotations.DestructiveHint ||
			a.IdempotentHint != want.Annotations.IdempotentHint || a.OpenWorldHint != want.Annotations.OpenWorldHint {
			t.Errorf("%s: annotations %+v, contract %+v", got.Name, a, want.Annotations)
		}
		props, _ := got.InputSchema["properties"].(map[string]any)
		var required []string
		if r, ok := got.InputSchema["required"].([]string); ok {
			required = r
		}
		var wantRequired []string
		for name, p := range want.Parameters {
			sp, ok := props[name].(map[string]any)
			if !ok {
				t.Errorf("%s: the contract's parameter %q is not in the schema", got.Name, name)
				continue
			}
			if sp["type"] != p.Type {
				t.Errorf("%s.%s: type %v, contract %s", got.Name, name, sp["type"], p.Type)
			}
			if p.Required {
				wantRequired = append(wantRequired, name)
			}
		}
		if len(props) != len(want.Parameters) {
			t.Errorf("%s: %d parameters in the schema, %d in the contract", got.Name, len(props), len(want.Parameters))
		}
		sort.Strings(required)
		sort.Strings(wantRequired)
		if !reflect.DeepEqual(required, wantRequired) && (len(required) > 0 || len(wantRequired) > 0) {
			t.Errorf("%s: required %v, contract %v", got.Name, required, wantRequired)
		}
	}

	if len(contract.Prompts) != len(prompts) {
		t.Fatalf("contract has %d prompts, the server %d", len(contract.Prompts), len(prompts))
	}
	for i, want := range contract.Prompts {
		got := prompts[i]
		if got.Name != want.Name || got.Title != want.Title {
			t.Errorf("prompt %d: server %q %q, contract %q %q", i, got.Name, got.Title, want.Name, want.Title)
			continue
		}
		if len(got.Arguments) != len(want.Arguments) {
			t.Errorf("%s: %d arguments, contract %d", got.Name, len(got.Arguments), len(want.Arguments))
		}
		for _, a := range got.Arguments {
			w, ok := want.Arguments[a.Name]
			if !ok || w.Required != a.Required {
				t.Errorf("%s.%s: required=%v, contract %+v (present %v)", got.Name, a.Name, a.Required, w, ok)
			}
		}
	}
}
