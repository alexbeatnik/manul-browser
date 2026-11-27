package main

import (
	"fmt"
	"strings"
)

// runVarPrefix names the environment variables that set a variable for every
// hunt of a run: MANUL_VAR_password=… is {password}. The rest of the name is
// the variable's name as written, so a secret in CI reaches a hunt without
// being written into it.
const runVarPrefix = "MANUL_VAR_"

// runVarFlag collects --var name=value, which may be given more than once.
type runVarFlag []string

func (f *runVarFlag) String() string { return strings.Join(*f, ",") }

func (f *runVarFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

// collectRunVars reads MANUL_VAR_* from env and then --var, so a flag wins
// over the environment. A name may be written with its braces: {site}=….
func collectRunVars(flags []string, env []string) (map[string]string, error) {
	vars := map[string]string{}
	for _, kv := range env {
		if !strings.HasPrefix(kv, runVarPrefix) {
			continue
		}
		name, value, _ := strings.Cut(strings.TrimPrefix(kv, runVarPrefix), "=")
		if name != "" {
			vars[name] = value
		}
	}
	for _, kv := range flags {
		name, value, ok := strings.Cut(kv, "=")
		name = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(name), "{"), "}")
		if !ok || name == "" {
			return nil, fmt.Errorf("--var %q: want name=value, e.g. --var site=https://staging.example.com", kv)
		}
		vars[name] = value
	}
	return vars, nil
}
