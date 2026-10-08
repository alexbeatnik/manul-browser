package browser

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// MockRule answers a request from a file instead of from the network.
//
// Both backends take their decisions from the same table of these, so a MOCK
// means one thing whichever protocol carries it; only pausing a request and
// answering it are protocol work.
type MockRule struct {
	// Method is the HTTP method the rule answers, e.g. "GET".
	Method string
	// Pattern is matched against the end of the request URL, query string
	// included. A '*' stands for any run of characters, so "/api/users*"
	// also answers "/api/users?page=2".
	Pattern string
	// Body is sent as the response body, with status 200.
	Body []byte
	// ContentType is the response's Content-Type.
	ContentType string
}

// mockAnswerTimeout bounds answering one paused request, so a dead connection
// cannot strand the goroutine doing it.
const mockAnswerTimeout = 10 * time.Second

// mockAnswer is the response a paused request is given.
type mockAnswer struct {
	Status  int
	Headers [][2]string // name, value
	Body    []byte
}

type compiledMock struct {
	rule MockRule
	url  *regexp.Regexp
}

// mockTable is the set of rules one page answers from. It is read from the
// goroutines that handle paused requests while a step may be adding a rule.
type mockTable struct {
	mu    sync.Mutex
	rules []compiledMock
}

// set adds a rule, replacing one already given for the same method and pattern.
func (t *mockTable) set(rule MockRule) {
	rule.Method = strings.ToUpper(strings.TrimSpace(rule.Method))
	parts := strings.Split(rule.Pattern, "*")
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	compiled := compiledMock{rule: rule, url: regexp.MustCompile(strings.Join(parts, ".*") + "$")}

	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.rules {
		if t.rules[i].rule.Method == rule.Method && t.rules[i].rule.Pattern == rule.Pattern {
			t.rules[i] = compiled
			return
		}
	}
	t.rules = append(t.rules, compiled)
}

// patterns lists the URL patterns of every rule, for a backend that can ask
// the browser to pause only matching requests.
func (t *mockTable) patterns() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.rules))
	for _, r := range t.rules {
		out = append(out, r.rule.Pattern)
	}
	return out
}

// answer decides what a paused request is given, or returns nil to let it go
// on to the network. header looks a request header up by lower-case name.
//
// A mocked response to a cross-origin request needs the CORS headers the real
// server would have sent, or the browser discards it; and the preflight that
// precedes such a request has to be answered too, since the server it would
// otherwise reach may not exist at all.
func (t *mockTable) answer(method, url string, header func(name string) string) *mockAnswer {
	method = strings.ToUpper(method)

	t.mu.Lock()
	var hit *MockRule
	covered := false
	for i := range t.rules {
		if !t.rules[i].url.MatchString(url) {
			continue
		}
		covered = true
		if t.rules[i].rule.Method == method {
			hit = &t.rules[i].rule
			break
		}
	}
	t.mu.Unlock()

	origin := header("origin")
	var cors [][2]string
	if origin != "" {
		cors = [][2]string{
			{"Access-Control-Allow-Origin", origin},
			{"Access-Control-Allow-Credentials", "true"},
		}
	}

	if hit != nil {
		headers := append([][2]string{{"Content-Type", hit.ContentType}}, cors...)
		return &mockAnswer{Status: 200, Headers: headers, Body: hit.Body}
	}
	if covered && method == "OPTIONS" && header("access-control-request-method") != "" {
		allowed := header("access-control-request-headers")
		if allowed == "" {
			allowed = "*"
		}
		headers := append(cors,
			[2]string{"Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS"},
			[2]string{"Access-Control-Allow-Headers", allowed},
		)
		return &mockAnswer{Status: 204, Headers: headers}
	}
	return nil
}
