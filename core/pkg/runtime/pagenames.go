package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/pages"
)

// PageName is the name pages/ gives the page at url, or "" when it gives
// none: for a step's result, the map and an agent's answer.
func (rt *Runtime) PageName(url string) string {
	return rt.pages.PageName(url)
}

// pageNow is what the page open now is called: its name in pages/, its title
// and its address.
func (rt *Runtime) pageNow(ctx context.Context) (name, title, url string, err error) {
	if url, err = rt.page.CurrentURL(ctx); err != nil {
		return "", "", "", err
	}
	name = rt.pages.PageName(url)
	// A string comes back from EvalJS unquoted; stringified, it is JSON.
	if raw, e := rt.page.EvalJS(ctx, "JSON.stringify(document.title)"); e == nil {
		_ = json.Unmarshal(raw, &title)
	}
	return name, title, url, nil
}

// pageIs reports whether a page called name (in pages/) and titled title is
// the page asked for, compared as a person reads them.
func pageIs(want, name, title string) bool {
	w := foldSpace(want)
	return w != "" && (foldSpace(name) == w || foldSpace(title) == w)
}

// verifyPage is VERIFY that the page is [NOT] '<name>'.
//
// The name is the one pages/ gives the address, or else the page's title, so
// a suite with no registry checks the title and one with a registry checks
// what it calls its pages. It waits, as a text VERIFY does, for the step's
// timeout: a single-page app's route may still be on its way.
func (rt *Runtime) verifyPage(ctx context.Context, cmd dsl.Command, res *explain.ExecutionResult) error {
	want := rt.resolveVariables(cmd.VerifyText)
	res.TargetQuery = want
	if strings.TrimSpace(want) == "" {
		return fmt.Errorf("VERIFY that the page is: no page name given")
	}
	deadline := time.Now().Add(rt.cfg.DefaultTimeout)
	var name, title, url string
	var lastErr error
	for {
		rt.pollTick(ctx)
		n, ti, u, err := rt.pageNow(ctx)
		if err == nil {
			name, title, url, lastErr = n, ti, u, nil
			if pageIs(want, name, title) != cmd.VerifyNegated {
				return nil
			}
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		if err := rt.page.Wait(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
	res.FailureReason = explain.ReasonVerifyFailed
	if lastErr != nil && url == "" {
		return fmt.Errorf("verification failed: could not tell which page is open: %w", lastErr)
	}
	if cmd.VerifyNegated {
		return fmt.Errorf("verification failed: the page is '%s' (%s), but expected NOT to be", want, describePage(name, title, url))
	}
	return fmt.Errorf("verification failed: the page is not '%s': it is %s", want, describePage(name, title, url))
}

// describePage says what a page is called, for a failure.
func describePage(name, title, url string) string {
	var parts []string
	if name != "" {
		parts = append(parts, fmt.Sprintf("'%s' in pages/", name))
	}
	if title != "" {
		parts = append(parts, fmt.Sprintf("titled '%s'", title))
	}
	parts = append(parts, "at "+url)
	return strings.Join(parts, ", ")
}

// pageAddress resolves NAVIGATE to '<page name>' through pages/. It answers
// ok false when pages/ has no page of that name: the target is an address,
// as it always was. A name pages/ gives several addresses — the same page
// on localhost and on staging — goes to the one on the site open now, and
// is an error when that does not decide it.
func (rt *Runtime) pageAddress(ctx context.Context, target string) (addr string, ok bool, err error) {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "about:") {
		return "", false, nil
	}
	addrs := rt.pages.PagesNamed(target)
	// A path of the any-site fragment is on the site open now, or, from a
	// blank page, on the hunt's {site}.
	for i, a := range addrs {
		if !strings.HasPrefix(a, "/") {
			continue
		}
		base := ""
		if current, cerr := rt.page.CurrentURL(ctx); cerr == nil && (strings.HasPrefix(current, "http://") || strings.HasPrefix(current, "https://")) {
			base = origin(current)
		} else if site := rt.resolveVariables("{site}"); site != "{site}" && site != "" {
			base = strings.TrimSuffix(site, "/")
		}
		if base == "" {
			return "", false, fmt.Errorf("NAVIGATE: pages/ puts %q at %s on any site; open a page of the site first, or give the hunt {site}", target, a)
		}
		addrs[i] = base + a
	}
	addrs = dedupe(addrs)
	switch len(addrs) {
	case 0:
		if strings.ContainsAny(strings.TrimSpace(target), " \t") {
			return "", false, fmt.Errorf("NAVIGATE: %q is not an address, and no page in pages/ has that name", target)
		}
		return "", false, nil
	case 1:
		return addrs[0], true, nil
	}
	if current, cerr := rt.page.CurrentURL(ctx); cerr == nil {
		var here []string
		for _, a := range addrs {
			if pages.SameSite(a, current) {
				here = append(here, a)
			}
		}
		if len(here) == 1 {
			return here[0], true, nil
		}
	}
	return "", false, fmt.Errorf("NAVIGATE: pages/ gives %q several addresses (%s), and the page open now does not say which; NAVIGATE to the address", target, strings.Join(addrs, ", "))
}

// origin is the scheme and host of an address: https://shop.test.
func origin(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return u
	}
	if j := strings.IndexAny(u[i+3:], "/?#"); j >= 0 {
		return u[:i+3+j]
	}
	return u
}

func dedupe(ss []string) []string {
	seen := map[string]bool{}
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
