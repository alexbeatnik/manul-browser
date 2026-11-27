package main

import (
	"strings"
	"testing"
)

func TestCollectRunVars_FlagsWinOverTheEnvironment(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"MANUL_VAR_site=http://env.example",
		"MANUL_VAR_password=s3cr=t", // a value may hold '='
		"MANUL_BROWSER=firefox",     // not a variable
	}
	got, err := collectRunVars([]string{"site=https://flag.example", "{user}=ada"}, env)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"site": "https://flag.example", "password": "s3cr=t", "user": "ada"}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestCollectRunVars_RefusesAFlagWithoutAName(t *testing.T) {
	for _, bad := range []string{"site", "=x", "{}=x"} {
		if _, err := collectRunVars([]string{bad}, nil); err == nil || !strings.Contains(err.Error(), "name=value") {
			t.Errorf("--var %q: want an error naming the form, got %v", bad, err)
		}
	}
}
