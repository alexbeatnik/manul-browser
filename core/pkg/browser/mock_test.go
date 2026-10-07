package browser

import (
	"strings"
	"testing"
)

func headers(pairs ...string) func(string) string {
	return func(name string) string {
		for i := 0; i+1 < len(pairs); i += 2 {
			if strings.EqualFold(pairs[i], name) {
				return pairs[i+1]
			}
		}
		return ""
	}
}

func headerOf(a *mockAnswer, name string) string {
	for _, h := range a.Headers {
		if strings.EqualFold(h[0], name) {
			return h[1]
		}
	}
	return ""
}

// The pattern is matched against the end of the URL, with '*' as a wildcard —
// the rule the predecessor's `**<pattern>` glob gave, and the one WAIT FOR
// RESPONSE uses. The patch over window.fetch this replaces compared the
// pattern with the whole URL for equality, so it never matched one.
func TestMockTable_MatchesTheEndOfTheURL(t *testing.T) {
	var table mockTable
	table.set(MockRule{Method: "get", Pattern: "/todos/1", Body: []byte("one"), ContentType: "application/json"})
	table.set(MockRule{Method: "GET", Pattern: "/users*", Body: []byte("users"), ContentType: "text/plain"})

	for _, tc := range []struct {
		method, url, want string
	}{
		{"GET", "https://api.test/todos/1", "one"},
		{"get", "https://api.test/v2/todos/1", "one"},
		{"GET", "https://api.test/todos/1?x=1", ""}, // the query is part of the URL
		{"GET", "https://api.test/todos/11", ""},
		{"POST", "https://api.test/todos/1", ""}, // another method is another request
		{"GET", "https://api.test/users", "users"},
		{"GET", "https://api.test/users?page=2", "users"},
		{"GET", "https://api.test/accounts", ""},
	} {
		a := table.answer(tc.method, tc.url, headers())
		got := ""
		if a != nil {
			got = string(a.Body)
		}
		if got != tc.want {
			t.Errorf("%s %s: answered %q, want %q", tc.method, tc.url, got, tc.want)
		}
	}
}

func TestMockTable_ALaterRuleForTheSameRequestReplacesTheFirst(t *testing.T) {
	var table mockTable
	table.set(MockRule{Method: "GET", Pattern: "/cart", Body: []byte("empty")})
	table.set(MockRule{Method: "GET", Pattern: "/cart", Body: []byte("full")})

	if got := len(table.patterns()); got != 1 {
		t.Fatalf("rules = %d, want 1", got)
	}
	if a := table.answer("GET", "https://shop.test/cart", headers()); a == nil || string(a.Body) != "full" {
		t.Errorf("answer = %+v, want the later body", a)
	}
}

// A pattern is text, not a regular expression: only '*' means anything.
func TestMockTable_PatternIsLiteralApartFromTheStar(t *testing.T) {
	var table mockTable
	table.set(MockRule{Method: "GET", Pattern: "/a.b?c=(1)", Body: []byte("x")})

	if table.answer("GET", "https://x.test/a.b?c=(1)", headers()) == nil {
		t.Error("the pattern did not match itself")
	}
	if table.answer("GET", "https://x.test/aXb?c=(1)", headers()) != nil {
		t.Error("'.' was treated as a wildcard")
	}
}

// A mocked response to a cross-origin request is discarded by the browser
// unless it carries the headers the real server would have sent, and the
// preflight before it would otherwise go to a server that may not exist.
func TestMockTable_AnswersCrossOriginRequestsAndTheirPreflight(t *testing.T) {
	var table mockTable
	table.set(MockRule{Method: "POST", Pattern: "/orders", Body: []byte("{}"), ContentType: "application/json"})

	a := table.answer("POST", "https://api.test/orders", headers("Origin", "https://shop.test"))
	if a == nil || a.Status != 200 {
		t.Fatalf("answer = %+v", a)
	}
	if headerOf(a, "Content-Type") != "application/json" || headerOf(a, "Access-Control-Allow-Origin") != "https://shop.test" {
		t.Errorf("headers = %v", a.Headers)
	}

	pre := table.answer("OPTIONS", "https://api.test/orders", headers(
		"Origin", "https://shop.test",
		"Access-Control-Request-Method", "POST",
		"Access-Control-Request-Headers", "content-type",
	))
	if pre == nil || pre.Status != 204 {
		t.Fatalf("preflight = %+v", pre)
	}
	if headerOf(pre, "Access-Control-Allow-Origin") != "https://shop.test" || headerOf(pre, "Access-Control-Allow-Headers") != "content-type" {
		t.Errorf("preflight headers = %v", pre.Headers)
	}

	// An OPTIONS request that is not a preflight, and a preflight for a URL no
	// rule covers, are none of the table's business.
	if table.answer("OPTIONS", "https://api.test/orders", headers()) != nil {
		t.Error("answered a plain OPTIONS request")
	}
	if table.answer("OPTIONS", "https://api.test/other", headers("Access-Control-Request-Method", "POST")) != nil {
		t.Error("answered a preflight for a URL no rule covers")
	}
}
