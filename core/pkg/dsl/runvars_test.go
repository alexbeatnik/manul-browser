package dsl

import (
	"strings"
	"testing"
)

const envHunt = `@title: login
@var: {site} = http://localhost:8000
@var: {login} = {site}/login.html

STEP 1: Sign in
    NAVIGATE to {login}
    FILL 'Password' field with '{password}'
DONE.
`

// A suite run against staging and locally from one set of files: the address
// of the site and a password from CI are given for the run, and a hunt's own
// @var is the default. @var values are written into the steps as the file is
// parsed, so a value given afterwards would have changed nothing.
func TestParseWithVars_TheRunsValueReplacesTheFilesDefault(t *testing.T) {
	h, err := ParseWithVars(strings.NewReader(envHunt), map[string]string{
		"site":     "https://staging.example.com",
		"password": "from-ci",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Commands[0].URL; got != "https://staging.example.com/login.html" {
		t.Errorf("NAVIGATE to %q: a @var built from {site} should use the run's site", got)
	}
	if got := h.Commands[1].Value; got != "from-ci" {
		t.Errorf("FILL with %q: a name the file never declares takes the run's value", got)
	}
	if h.Vars["site"] != "https://staging.example.com" {
		t.Errorf("Vars[site] = %q", h.Vars["site"])
	}
}

func TestParseWithVars_WithoutAValueTheDefaultStands(t *testing.T) {
	h, err := ParseWithVars(strings.NewReader(envHunt), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Commands[0].URL; got != "http://localhost:8000/login.html" {
		t.Errorf("NAVIGATE to %q", got)
	}
}
