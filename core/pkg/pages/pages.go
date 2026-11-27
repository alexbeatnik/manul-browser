// Package pages implements the Manul Browser page-name registry.
//
// The registry on disk:
//   - Per-site JSON fragments under <project>/pages/<safe_netloc>.json
//   - Lean shape:   { "site": "https://example.com/", "Domain": "Example", "/login": "Login Page" }
//   - Wrapped shape:{ "https://example.com/": { "Domain": "Example", "/login": "Login Page" } }
//   - Auto-populate unknown URLs as "Auto: domain/path" placeholders.
package pages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Registry maps site_root_url → { pattern_or_"Domain": page_name }.
type Registry struct {
	mu   sync.RWMutex
	data map[string]map[string]string // site_root → {pattern → name}
	dir  string
	// seen is the fragments' names, sizes and times when they were last
	// read: a step asks for its page's name, and reading every fragment
	// again for each one is waste when none has changed.
	seen string
}

// NewRegistry creates a Registry backed by *dir*.
// If dir is empty it defaults to "pages" under the current working directory.
func NewRegistry(dir string) *Registry {
	if dir == "" {
		cwd, _ := os.Getwd()
		dir = filepath.Join(cwd, "pages")
	}
	r := &Registry{dir: dir, data: map[string]map[string]string{}}
	r.Reload()
	return r
}

// Reload merges every pages/*.json fragment into the in-memory registry. It
// reads them only when one has been added, removed or changed since the last
// time; a registry with no directory is empty.
func (r *Registry) Reload() {
	r.mu.Lock()
	defer r.mu.Unlock()

	merged := map[string]map[string]string{}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		r.data, r.seen = merged, ""
		return
	}
	var sig strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if info, infoErr := e.Info(); infoErr == nil {
			fmt.Fprintf(&sig, "%s|%d|%d\n", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	if sig.String() == r.seen && r.data != nil {
		return
	}
	r.seen = sig.String()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var raw map[string]any
		if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
			continue
		}
		for siteKey, siteVal := range normalizeFragment(raw) {
			existing, ok := merged[siteKey]
			if !ok {
				existing = map[string]string{}
				merged[siteKey] = existing
			}
			for k, v := range siteVal {
				existing[k] = v
			}
		}
	}
	r.data = merged
}

// LookupPageName matches *url* against the registry and returns the mapped page name.
//
// Matching logic:
//  1. Reload registry from disk on every call so manual edits are picked up.
//  2. Find best-matching site block by longest-prefix against URL scheme+host+path.
//  3. Within site block, the most specific of:
//     a. The address itself, then the address without its query and fragment.
//     b. Every other key that matches: a plain path ("/login") matches the
//     path from its start, a segment at a time; any other key matches as a
//     regular expression, or failing that as a substring, of the address.
//     The longest such key wins, and of keys as long, the first in order.
//     c. The special "Domain" key as fallback.
//  4. If no site block matches, auto-populate a placeholder.
//
// Two keys that both matched one address used to name it at random — Go
// walks a map in no fixed order — and a plain path matched anywhere in the
// address, so '/admin' named a post about admin tips.
func (r *Registry) LookupPageName(rawURL string) string {
	r.Reload()
	if name, ok := r.lookup(rawURL); ok {
		return name
	}
	return r.autoPopulate(rawURL)
}

func (r *Registry) lookup(rawURL string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 2. Find best (longest-prefix) site block.
	var bestSite string
	for siteRoot := range r.data {
		if belongsToSite(rawURL, siteRoot) {
			if bestSite == "" || len(siteRoot) > len(bestSite) || (len(siteRoot) == len(bestSite) && siteRoot < bestSite) {
				bestSite = siteRoot
			}
		}
	}
	if bestSite == "" {
		return "", false
	}

	// 3. Match within site block.
	pages := r.data[bestSite]
	bare := withoutQuery(rawURL)
	if name, ok := pages[rawURL]; ok {
		return name, true
	}
	if name, ok := pages[bare]; ok {
		return name, true
	}
	var best, bestName string
	for pattern, name := range pages {
		if pattern == "Domain" || pattern == "site" || !patternMatches(pattern, rawURL, bare) {
			continue
		}
		if best == "" || len(pattern) > len(best) || (len(pattern) == len(best) && pattern < best) {
			best, bestName = pattern, name
		}
	}
	if best != "" {
		return bestName, true
	}
	if domainName := pages["Domain"]; domainName != "" {
		return domainName, true
	}
	return "", false
}

// PageName is the name a person gave the page at rawURL in pages/, or "" when
// there is none — an "Auto:" placeholder is not a name. Unlike
// LookupPageName it writes nothing: it is asked on every step.
func (r *Registry) PageName(rawURL string) string {
	if rawURL == "" || strings.HasPrefix(rawURL, "about:") {
		return ""
	}
	r.Reload()
	name, ok := r.lookup(rawURL)
	if !ok || strings.HasPrefix(name, "Auto:") {
		return ""
	}
	return name
}

// PagesNamed is every address pages/ gives the name: a key that is a whole
// address, a plain path under its site, or the site itself for its "Domain"
// name. Keys that are patterns name many addresses and so none to go to. A
// plain path of the AnySite fragment comes back as the path itself, for the
// caller to put on the site it means.
// The name is compared as a person reads it: case and runs of spaces aside.
// The result is sorted, so the same registry gives the same answer.
func (r *Registry) PagesNamed(name string) []string {
	want := foldName(name)
	if want == "" || strings.HasPrefix(want, "auto:") {
		return nil
	}
	r.Reload()
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for site, keys := range r.data {
		root := strings.TrimSuffix(site, "/")
		for key, n := range keys {
			if foldName(n) != want {
				continue
			}
			var addr string
			switch {
			case site == AnySite && key == "Domain":
				continue
			case site == AnySite && isPlainPath(key):
				addr = key
			case key == "Domain":
				addr = site
			case key == "site":
				continue
			case strings.Contains(key, "://"):
				addr = key
			case isPlainPath(key):
				addr = root + key
			default:
				continue
			}
			if !seen[addr] {
				seen[addr] = true
				out = append(out, addr)
			}
		}
	}
	sort.Strings(out)
	return out
}

// SameSite reports whether two addresses are on one site: scheme and host.
func SameSite(a, b string) bool {
	pa, pb := parseURL(a), parseURL(b)
	return pa.host != "" && pa.scheme == pb.scheme && pa.host == pb.host
}

func foldName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// patternMatches reports whether a registry key names rawURL.
func patternMatches(pattern, rawURL, bare string) bool {
	if isPlainPath(pattern) {
		path := parseURL(bare).path
		root := strings.TrimSuffix(pattern, "/")
		return path == pattern || path == root || strings.HasPrefix(path, root+"/")
	}
	if re, err := regexp.Compile(pattern); err == nil && re.MatchString(rawURL) {
		return true
	}
	return strings.Contains(rawURL, pattern)
}

// isPlainPath reports whether a key is a path and nothing more: it starts
// with '/' and has none of the characters a regular expression is made of.
// A dot in it is a dot.
func isPlainPath(pattern string) bool {
	return strings.HasPrefix(pattern, "/") && !strings.ContainsAny(pattern, `^$*+?()[]{}|\`)
}

// withoutQuery is rawURL without its query and fragment: the same page.
func withoutQuery(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

// autoPopulate creates a placeholder entry for an address on a site the
// registry does not know, persists it, and returns the page's placeholder.
//
// The site is named after its host and the page after its path, neither with
// a query: the site used to take the first address it was seen at, query and
// all ("Auto: other.test/p/1?ref=a"), and every other page of it was then
// reported under that name. The fragment is written to a temporary file and
// renamed into place, so parallel workers never read one half written.
func (r *Registry) autoPopulate(rawURL string) string {
	bare := withoutQuery(rawURL)
	parsed := parseURL(bare)

	siteRoot := fmt.Sprintf("%s://%s/", parsed.scheme, parsed.host)
	sitePlaceholder := fmt.Sprintf("Auto: %s", parsed.host)
	if parsed.host == "" {
		sitePlaceholder = fmt.Sprintf("Auto: %s", bare)
	}
	placeholder := sitePlaceholder
	if slug := strings.Trim(parsed.host+parsed.path, "/"); slug != "" && strings.Trim(parsed.path, "/") != "" {
		placeholder = fmt.Sprintf("Auto: %s", slug)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fragmentPath := filepath.Join(r.dir, safeSiteFilename(siteRoot))

	// Read current on-disk fragment.
	siteBlock := map[string]string{}
	fragmentSite := siteRoot
	if data, readErr := os.ReadFile(fragmentPath); readErr == nil {
		var raw map[string]any
		if jsonErr := json.Unmarshal(data, &raw); jsonErr == nil {
			if allDict(raw) && len(raw) > 0 {
				// Wrapped form.
				if _, ok := raw[siteRoot]; ok {
					siteBlock = dictStringString(raw[siteRoot])
				} else {
					for k, v := range raw {
						fragmentSite = k
						siteBlock = dictStringString(v)
						break
					}
				}
			} else {
				// Lean form.
				if s, ok := raw["site"].(string); ok && s != "" {
					fragmentSite = s
				}
				siteBlock = dictStringString(raw)
			}
		}
	}

	// Merge: never overwrite existing keys.
	updated := false
	if _, ok := siteBlock["Domain"]; !ok {
		siteBlock["Domain"] = sitePlaceholder
		updated = true
	}
	if _, ok := siteBlock[bare]; !ok {
		siteBlock[bare] = placeholder
		updated = true
	}

	// Update in-memory registry.
	siteEntry, ok := r.data[fragmentSite]
	if !ok {
		siteEntry = map[string]string{}
		r.data[fragmentSite] = siteEntry
	}
	for k, v := range siteBlock {
		siteEntry[k] = v
	}

	// Persist back to disk.
	if updated {
		_ = os.MkdirAll(r.dir, 0755)
		payload := map[string]any{"site": fragmentSite}
		for k, v := range siteBlock {
			payload[k] = v
		}
		if data, marshalErr := json.MarshalIndent(payload, "", "    "); marshalErr == nil {
			if tmp, err := os.CreateTemp(r.dir, ".fragment-*.tmp"); err == nil {
				_, werr := tmp.Write(append(data, '\n'))
				cerr := tmp.Close()
				if werr == nil && cerr == nil {
					_ = os.Chmod(tmp.Name(), 0644)
					if os.Rename(tmp.Name(), fragmentPath) != nil {
						_ = os.Remove(tmp.Name())
					}
				} else {
					_ = os.Remove(tmp.Name())
				}
			}
		}
	}

	return siteBlock[bare]
}

// Dir returns the directory backing this registry.
func (r *Registry) Dir() string {
	return r.dir
}

// ── internal helpers ──────────────────────────────────────────────────────────

// normalizeFragment turns a single JSON payload into {site_root: {pattern: name}}.
func normalizeFragment(raw map[string]any) map[string]map[string]string {
	out := map[string]map[string]string{}
	if len(raw) == 0 {
		return out
	}

	// Wrapped form: every top-level value is a dict.
	if allDict(raw) {
		for siteKey, siteVal := range raw {
			sk := fmt.Sprint(siteKey)
			if sk == "" {
				continue
			}
			out[sk] = dictStringString(siteVal)
		}
		return out
	}

	// Lean form: site root comes from "site" field.
	site := ""
	if s, ok := raw["site"].(string); ok {
		site = s
	}
	if site == "" {
		return out
	}
	fields := map[string]string{}
	for k, v := range raw {
		if k == "site" {
			continue
		}
		fields[k] = fmt.Sprint(v)
	}
	if len(fields) > 0 {
		out[site] = fields
	}
	return out
}

func allDict(m map[string]any) bool {
	for _, v := range m {
		if _, ok := v.(map[string]any); !ok {
			return false
		}
	}
	return len(m) > 0
}

func dictStringString(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v2 := range m {
		out[k] = fmt.Sprint(v2)
	}
	return out
}

// AnySite is the site of a fragment whose pages are on every site: "site":
// "*". A suite run against localhost and against staging names its pages
// once, by path. A fragment for a particular site comes first.
const AnySite = "*"

func belongsToSite(candidateURL, siteRoot string) bool {
	if siteRoot == AnySite {
		return strings.HasPrefix(candidateURL, "http://") || strings.HasPrefix(candidateURL, "https://")
	}
	c := parseURL(candidateURL)
	s := parseURL(siteRoot)
	if c.scheme != s.scheme || c.host != s.host {
		return false
	}
	cPath := strings.Trim(c.path, "/")
	sPath := strings.Trim(s.path, "/")
	if sPath == "" {
		return true
	}
	return cPath == sPath || strings.HasPrefix(cPath, sPath+"/")
}

type urlParts struct {
	scheme string
	host   string
	path   string
}

// parseURL is a lightweight URL parser sufficient for page-name matching.
func parseURL(raw string) urlParts {
	// Handle scheme.
	scheme := "https"
	rest := raw
	if idx := strings.Index(rest, "://"); idx >= 0 {
		scheme = strings.ToLower(rest[:idx])
		rest = rest[idx+3:]
	}
	// Split host and path.
	host := rest
	path := ""
	if idx := strings.Index(rest, "/"); idx >= 0 {
		host = rest[:idx]
		path = rest[idx:]
	}
	return urlParts{scheme: scheme, host: host, path: path}
}

func safeSiteFilename(siteRoot string) string {
	p := parseURL(siteRoot)
	netloc := p.host
	if netloc == "" {
		netloc = siteRoot
	}
	re := regexp.MustCompile(`[^0-9A-Za-z._-]`)
	safe := strings.Trim(re.ReplaceAllString(netloc, "_"), "_")
	if safe == "" {
		safe = "site"
	}
	return safe + ".json"
}

// setdefault helper for maps.
type stringMapMap map[string]map[string]string

func (m stringMapMap) setdefault(key string, value map[string]string) map[string]string {
	if existing, ok := m[key]; ok {
		return existing
	}
	m[key] = value
	return value
}

// MigrateLegacyJSON reads a legacy pages.json (wrapped or lean) and writes
// per-site fragments into <outDir>/<safe_netloc>.json.
func MigrateLegacyJSON(legacyPath, outDir string) error {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("read legacy pages.json: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse legacy pages.json: %w", err)
	}

	fragments := normalizeFragment(raw)
	if len(fragments) == 0 {
		return fmt.Errorf("no site blocks found in %s", legacyPath)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create pages dir: %w", err)
	}

	for siteRoot, fields := range fragments {
		payload := map[string]any{"site": siteRoot}
		for k, v := range fields {
			payload[k] = v
		}
		data, err := json.MarshalIndent(payload, "", "    ")
		if err != nil {
			continue
		}
		path := filepath.Join(outDir, safeSiteFilename(siteRoot))
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("write fragment %s: %w", path, err)
		}
	}
	return nil
}
