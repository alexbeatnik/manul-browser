# Loops & Page Objects

> Loop constructs and the page-name registry, as the runtime implements them.

## Loops

### `REPEAT N TIMES:`

Execute a block of commands a fixed number of times.

```hunt
REPEAT 3 TIMES:
    CLICK the 'Next' button
    VERIFY that 'Step {i}' is present
```

The loop counter is automatically available as `{i}` (0-based).

### `FOR EACH {var} IN {collection}:`

Iterate over a comma-separated list stored in a variable.

```hunt
@var: {products} = Laptop, Headphones, Mouse

FOR EACH {product} IN {products}:
    FILL 'Search' field with '{product}'
    CLICK the 'Add to Cart' button NEAR '{product}'
```

### `WHILE condition:`

Repeat while a condition is true (max 100 iterations for safety).

```hunt
WHILE button 'Load More' exists:
    CLICK the 'Load More' button
    WAIT 1
```

### Nested loops

Loops can be nested inside each other and inside `IF` blocks:

```hunt
REPEAT 2 TIMES:
    FOR EACH {item} IN {items}:
        IF {item} != 'Skip':
            CLICK '{item}' button
```

---

## Page Objects (pages/ directory)

Manul Browser uses a `pages/` directory next to your hunt files to map URLs to human-readable page names.

### Fragment format

Each site gets its own JSON file: `pages/<safe-netloc>.json`

**Lean form** (recommended):
```json
{
    "site": "https://example.com/",
    "Domain": "Example",
    "https://example.com/login": "Login Page",
    "https://example.com/dashboard": "Dashboard"
}
```

**Wrapped form** (backward-compatible):
```json
{
    "https://example.com/": {
        "Domain": "Example",
        "/login": "Login Page"
    }
}
```

### Matching logic

1. **Longest-prefix site match** — `https://app.example.com/` shadows `https://example.com/`
2. **Exact URL match**, then the URL without its `?query` and `#fragment` — the same page
3. **The most specific matching key** among the rest — the longest, and of keys as long the first in order — so `/login/sso` beats `/login` for `/login/sso`, every time:
   - a **plain path** (`/login`, `/index.html`: starts with `/`, no regex characters) matches the path from its start, a whole segment at a time — `/admin` names `/admin/users`, not `/blog/admin-tips`; a dot is a dot
   - any other key is a **regular expression** (`.*/products/\d+`), or failing that a **substring**, of the URL
4. **`"Domain"` key** as final fallback

### Auto-populate

An address on a site with no fragment is added as an `Auto:` placeholder — the site named after its host, the page after its path, neither with its query:

```json
{
    "site": "https://new-site.io/",
    "Domain": "Auto: new-site.io",
    "https://new-site.io/profile": "Auto: new-site.io/profile"
}
```

This happens on the first `NAVIGATE` to an unmapped site when `auto_annotate` is on — the only time the registry is consulted during a run. The fragment is written to a temporary file and renamed into place, so parallel workers never read one half written.

### A fragment for any site

A suite that runs against localhost and staging names its pages once, by path, with `"site": "*"`. Its keys hold on every http(s) site that no fragment of its own covers; a fragment for a particular site comes first.

```json
{
    "site": "*",
    "/login.html": "Login",
    "/shop.html": "Shop"
}
```

### Using page names in hunts

```hunt
NAVIGATE to 'Shop'
VERIFY that the page is 'Shop'
VERIFY that the page is NOT 'Login'
IF the page is 'Login':
    USE LogIn
```

- `NAVIGATE to '<name>'` goes to the address `pages/` gives the name: a whole-address key, a plain path under its site, or the site itself for its `Domain`. A path of the `*` fragment is put on the site open now, or from a blank page on the hunt's `{site}`. A name with several addresses goes to the one on the site open now; when that does not decide it, the step fails and lists them. A target `pages/` does not name is an address, as before.
- `VERIFY that the page is '<name>'` passes when the page's name in `pages/` — or, without one, its `document.title` — is the one given, case and spacing aside. It waits like a text VERIFY, for a route still on its way.
- `IF` / `WHILE` take `[the] page is [not] '<name>'`, at one look.
- Every step's JSON result carries `page_name`, and the agent and MCP answers say `the page is now Shop (https://…/shop.html)`. An `Auto:` placeholder is never shown as a name.

### Using page names in custom controls

Custom controls can target specific pages:

```go
runtime.RegisterCustomControl("Checkout Page", "React Datepicker", handler)
```

The page label a control is looked up by is the page's `document.title`. Only when the title is empty does the engine derive one from the URL: the last path segment with `-`, `_` and `.` turned into spaces, plus " page" unless it already says so (`/checkout.html` → `checkout html page`), or the host name when the path is empty. The `pages/` registry is **not** consulted — it names pages for `auto_annotate` log lines, not for controls. Register with `"*"` to match every page.

