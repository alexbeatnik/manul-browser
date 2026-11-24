# CLAUDE.md

Working notes for this repository. Behavioural coding guidance lives in
[`core/CLAUDE.md`](core/CLAUDE.md); this file is about *Manul specifically*.

## What this is

One engine, written once, in Go. Everything else drives it.

```
core/            The engine. The only implementation of scoring, DSL and CDP.
spec/            Contracts + the stdio session protocol. Source of truth.
bindings/python/ Thin client: ships the binary, speaks the protocol.
conformance/     Fixtures every path must agree on. (not started)
```

The repository exists because the engine was previously written twice, in Go and
Python, and they drifted: eight of nine shared contracts had diverged by the time
they were merged. **Do not reintroduce a second implementation.** If a binding
needs behaviour the engine lacks, add a protocol command in `core/` — never a
local implementation in the binding.

Neither predecessor is in the working tree, and nothing outside this paragraph
refers to them. Their history is still an ancestor of `main`, so a file can be
read back without a network round-trip: `git show 5e0c5db:legacy/python/...`
(`5e0c5db` grafted it in; `c894906` is the final upstream Python commit).

## Commands

```bash
cd core && go build ./... && go test -short ./...      # engine
cd core && gofmt -l . && go vet ./...                  # both are CI gates
cd bindings/python && python -m pytest tests/ -q       # binding (no Chrome needed)
```

The Python tests run against a fake engine that speaks the protocol, so they
need neither Chrome nor a compiled binary and finish in about a second.

What needs a real browser is in `core/e2e`, behind a build tag:

```bash
cd core && go test -tags e2e ./e2e/                     # Chrome and Firefox, whichever are installed
cd core && MANUL_E2E_BROWSERS=firefox go test -tags e2e ./e2e/
```

It builds `manul`, serves `e2e/testdata/site` from two local origins and runs
every hunt in `e2e/testdata/hunts` through the CLI; CI runs it on both browsers.
A hunt that must fail says so with `# expect-fail: <text>`, and one whose PRINT
output matters with `# expect-output: <text>`. Every hunt there was checked to
fail on the engine before its fix — a fixture the old engine passes proves
nothing. A bug found on a live site gets its smallest local page here.

## Invariants

**stdout is the protocol.** In `manul serve`, stdout carries NDJSON and nothing
else. Every log goes to stderr. One stray `fmt.Println` breaks every client.

**Contracts describe shipped behaviour.** `spec/contracts/*.md` are not design
documents. Changing what the engine does means changing the contract in the same
commit. Each file embeds a JSON block that must stay parseable — check it.

**Bindings own process lifecycle and typing, nothing else.** No scoring, no DSL
parsing, no CDP framing, no probe JavaScript.

**Two protocols, one engine.** Chromium is driven over CDP (`pkg/cdp`), Firefox
over WebDriver BiDi (`pkg/bidi`) — Firefox removed CDP in version 141, so this
is not a preference. Everything above the transport is shared, and the in-page
JavaScript lives in `pkg/pagejs` precisely so the two backends cannot grow
their own dialects of FILL or CHECK. A behaviour that belongs to the page goes
there; only framing belongs in `pkg/cdp` or `pkg/bidi`.

**A declared verb must be executable.** `pkg/runtime` carries a guard test that
walks every `CommandType` and fails on "not yet implemented". `WAIT FOR` and
`HIGHLIGHT` both shipped declared-but-dead; that is what the guard is for.

## Traps that have already cost time

**`EvalJS` is not uniform.** Numbers, booleans and objects come back as JSON;
**strings come back as bare, unquoted bytes**. `json.Unmarshal` on a string
result fails. Swallowing that error turns every string the page produced into
`null`. Use `decodeEvalResult` in `pkg/serve`, or compare the raw bytes.

**The scorer always ranks something.** "A top candidate exists" is not "the
target is present" — a nonsense label still returns a best guess. Anything
asking *is this on the page* must compare against `ThresholdAmbiguous`, the bar
the rest of the engine uses. Without it, a wait for a missing element passes on
the first poll.

The threshold is not enough on its own once a type hint is involved: the hint
and the tag together score 0.30 with no text in common, so "the 'Delete' button"
clears the bar against any button. Anything about to *act* on a winner asks
`scorer.MatchesQuery` first — the one question a score cannot answer.

**Reverse calls are strictly nested.** While the engine waits for a handler's
reply it serves only `page.eval` and `page.url`. Anything else would re-enter
the step currently executing, so it is refused rather than deadlocking. A
handler must not call `session.map()`.

**A JavaScript dialog blocks the page, and nothing was listening.** `alert()`
stops the page's only thread until something answers it. Over CDP Chrome only
announces a dialog to a client that enabled the Page domain, so the click that
opened one never returned and neither did any step after it — no error, no
timeout. Firefox dismissed dialogs on its own, which hid this there and made
`confirm` answer Cancel. Both now accept: `cdp.AcceptDialogs` on every page
connection, `unhandledPromptBehavior` in `session.new`.

**A headless Chrome page is not focused.** `element.focus()` moves
`activeElement` and fires nothing; the first real mouse press then focuses the
page and every owed focus handler runs in the middle of that click. A date
picker opened under the cursor and took the click meant for the button beneath
it. `cdp.EmulateFocus` is what makes focus happen when the engine asks for it.

**Chrome accepts a relative upload path and then stops answering.**
`DOM.setFileInputFiles` with `avatar.png` succeeds; the *next* command, whatever
it is, never returns, and the process then resists being killed. Firefox refuses
the path outright. `resolveUploadPath` makes it absolute before any browser
sees it.

**Chrome reports a failed navigation as a success.** `Page.navigate` answers
without error for a host that does not resolve; the failure is the `errorText`
field of the reply, and the tab is left on `chrome-error://`. Firefox fails the
command itself. `cdp.Navigate` reads the field so both do.

**Chrome refuses plain http on a fresh profile.** From Chrome 154 the
HTTPS-First balanced mode turns itself on, and an http-only site answers
`Page.navigate` with `net::ERR_BLOCKED_BY_CLIENT` — every `NAVIGATE` to such a
site failed. `chromeArgs` disables `HttpsFirstBalancedModeAutoEnable` and
`HttpsUpgrades` in the one `--disable-features` switch Chrome honours.

**A press without a move is not a click a page expects.** CDP's
`mousePressed` at a new point delivers `mouseenter` mid-press; a link that
replaces itself on hover took the press and lost the release, and nothing was
clicked, on Chromium only. Every CDP click moves the pointer first, as BiDi's
action sequence always did.

**Every displayed frame is entered, cross-origin ones too.** The snapshot,
text VERIFY and EXTRACT read every iframe of the page; `rt.pageFor(el)`
addresses an element's own frame, and coordinates it returns are the
top-level viewport's. What bit on the way:

- CDP announces existing execution contexts *while answering*
  `Runtime.enable`, and existing iframes while answering
  `Target.setAutoAttach`, so `FrameTracker` subscribes first and `settle`s
  before the first snapshot.
- Context 0 means "the default one" to `Runtime.evaluate`, so a frame without
  a context silently evaluated in the main document; `EvalJSInFrame` refuses.
- A cross-site iframe is an OOPIF: the page's session has no frame and no
  context for it. It is a flat session (`sessionId` on the one socket), and
  its context ids start at 1 like the page's — the tracker keys contexts by
  session. The page's session reports the frame as detached with reason
  `swap` when it moves out; that is not a removal.
- A cross-origin frame cannot place itself (`frameElement` is null), so it is
  measured from its parent: `DOM.getFrameOwner` on CDP, `contentWindow` on
  BiDi, which serializes to the frame's context id. `PlaceFrames` does all of
  a page's frames at once; one at a time cost seconds per step on a page with
  ads.
- Firefox does not deliver a top-level click into a cross-site frame — the
  step passed with nothing clicked. Sent to the frame's own context with its
  own coordinates, it arrives; `BiDiPage.routePoint` does that for every
  pointer action, and `routeFocus` for keys. Chrome routes input itself.
- MDN keeps its frames in a shadow root, where `querySelectorAll` and
  `elementFromPoint` do not look; the pagejs frame helpers walk open shadow
  roots.

**Firefox starts no HTML5 drag from WebDriver actions.** A pointer drag over
`draggable="true"` fired nothing the page listens for, however slowly it moved,
and DRAG passed with nothing moved; Chromium's mouse events do start one. The
BiDi backend dispatches the drag events itself for such a source
(`pagejs.HTML5DragAt`) and keeps the pointer for everything else.

**A click goes to coordinates, not to an element.** Whatever is on top at that
point receives it, and the step used to pass. `clickPoint` asks the page first
(`pagejs.CoveredBy`) and fails the step if something else would get the click.

**A page that is leaving still says `complete`.** After a click, a sort
dropdown's change or Enter in a form, the old document keeps
`readyState === "complete"` until the new one's response arrives, so polling
readyState returns at once and the next step runs on the page that is going.
On a slow server FILL then typed into it and passed. `armNavigationWatch`
before the action and `settleNavigation` after it watch `beforeunload` instead,
which fires before the request is sent.

**An empty query matches everything.** Pass 3 ranks the controls beside a
caption with `""`, and `strings.Contains(x, "")` is true — every labelled field
got the label bonus, and the zip-code field beat the quantity box under 'Qty.'.
A scorer that compares the query against an attribute returns 0 for an empty
query.

**A new tab is listed before it has a page.** A `target="_blank"` click
returns with the tab already listed (Chrome's `openerId`, Firefox's
`originalOpener`) but still on `about:blank`, so following it at once took a
snapshot of nothing; `waitForNewTab` waits for it to leave. A popup that closes
itself fails every evaluation afterwards, which `settleNavigation` took for a
page in transit and waited out `nav_timeout` on — it asks `TabClosed` first.
Tabs are found by listing, not by events, so none of the subscribe-order traps
below apply.

**A retried step must not act twice.** An action whose target is not on the
page yet runs again until `timeout` (`pkg/runtime/wait.go`), and that is safe
only while it has changed nothing. `rt.acted` is set at every point where a
step types, clicks, chooses, scrolls or presses; a new mutation without it is
a step that can run twice — `TestWait_AStepThatHasActedIsNotRunAgain` typed
8 times without the flag.

**Snapshot caching hides change.** The cache makes resolution cheap within a
step. Any polling loop must call `invalidateSnapshot()` each iteration or it
will never see what it is waiting for.

**Firefox is not a CDP browser, whatever `--remote-debugging-port` suggests.**
The flag survives; it starts the BiDi agent. `/json/version` and `/json/list`
are gone with the rest of CDP, so endpoint discovery reads the startup banner
on **stderr** instead — and that banner names a bare origin
(`ws://127.0.0.1:9222`) while the WebSocket upgrade only succeeds at
`/session`. `bidi.NormalizeWebSocketURL` is what closes that gap; without it
the handshake fails with nothing but `bad handshake` to go on.

**Firefox starts with focus in the address bar, and BiDi cannot move it.** Until
focus is in the page, `document.hasFocus()` is false and the page gets no
`focus`, `blur`, `focusin` or `focusout` at all — fields still fill, but
anything that reacts to focus does not. Activating the tab, a pointer click,
`window.focus()` and `focusmanager.testmode` were all tried and none helps. A
tab *created* over BiDi does start with focus in its content, which is why
`LaunchFirefox` swaps the startup tab for one (`focusContent`).

**One BiDi session per Firefox.** `session.new` on a second socket fails
rather than opening a second view, so every page of one endpoint shares a
single connection (`sharedConn` in `pkg/browser/bidi_backend.go`) and
identifies itself by browsing-context id. Consequently `BiDiPage.Close` does
**not** close the socket — doing so would cut off every sibling page,
including the caller's own tab during a background `Lookup`.

**BiDi sends no events until asked.** `Conn.Subscribe` only opens a local
channel; without `session.subscribe` for the event name, nothing arrives.
Subscribe locally *first*, then ask the browser — the other order drops
anything that happens in between, which for `WAIT FOR RESPONSE` means waiting
out the timeout for a response that already came. Network events have a
further catch: Firefox reports a response only for a request that *began*
while subscribed. The request a wait is for was sent by the step before it, so
a subscription made by the wait never heard back — every `WAIT FOR RESPONSE`
on Firefox timed out. A BiDi page subscribes to `network.responseCompleted`
when it opens (`newBiDiPage`), and the wait only listens.

## Local environment

The suite is green on Windows. It was not for a long time — twelve tests failed,
and both causes turned out to be in the tests rather than the engine:

- eleven in `pkg/config` called `os.Chdir` into their own `t.TempDir()`. Windows
  refuses to delete a directory that is a process's working directory, so the
  cleanup failed after the assertions had already passed. `t.Chdir` restores the
  old directory on cleanup, and its cleanup runs before `TempDir`'s because
  cleanups are LIFO. Linux allows deleting the working directory, which is why
  this was invisible there;
- `TestCollectScheduledHunts_Subdirectories` in `pkg/daemon` asserted on the
  literal `"sub/nested.hunt"`. `filepath.Walk` produces `sub\nested.hunt` here.
  The production code was right the whole time.

If either pattern reappears, it will fail on Windows and pass in CI.

The repository **is** gofmt-clean, and CI fails if it stops being. It used to
look otherwise: `core.autocrlf=true` gives this machine a CRLF working tree
against an LF index, gofmt counts CR as content, and so `gofmt -l` named 115
files while only 19 were actually unformatted. `.gitattributes` now pins `*.go`
to `eol=lf`, which makes the local check agree with the runner. If you still see
a wall of unformatted files, your checkout predates that file — re-checkout, or
compare against the index with `git show :path/to/file.go | gofmt -d`.

## Conventions

- Go ≥ 1.26, single dependency (`gorilla/websocket`). Keep it that way.
- There is no fake for either protocol — the unit tests drive a mock
  WebSocket server, which proves framing, not behaviour. Behaviour is what
  `core/e2e` is for; a single hunt can also be run by hand with `manul run
  x.hunt --browser firefox --headless`.
- The engine version lives in `cmd/manul/main.go` as `version`, without a `v`
  prefix, and must match the contracts.
- `.hunt` verbs are matched by keyword, not strict grammar. New verbs go in the
  `parseCommand` switch **before** any prefix that would swallow them — that is
  why `WAIT FOR SELECTOR` is checked ahead of `WAIT FOR`.
