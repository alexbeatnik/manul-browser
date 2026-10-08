(sel) => {
    // Returns the human-readable visible text of the page, case-preserved and
    // shadow-DOM aware. Unlike the visible-text presence probe, this is meant
    // for an agent (or LLM) to READ — so it keeps original casing and joins
    // shadow content rather than flattening everything for matching.
    //
    // sel is an optional CSS selector scoping extraction to one region; when
    // empty, the whole document body is used.
    //
    // A selector that matches nothing reads as nothing. It used to fall back
    // to the body, so asking for '#result' on a page without one returned the
    // whole page as if that were the result. A selector that cannot parse is
    // answered with an object, which no text can be mistaken for.
    sel = sel || "";

    let root = document.body;
    if (sel) {
        try {
            root = document.querySelector(sel);
        } catch (_) {
            return { invalidSelector: true };
        }
    }
    if (!root) return "";

    let t = root.innerText || "";
    // Append shadow-root text the flat innerText misses.
    root.querySelectorAll('*').forEach(el => {
        if (el.shadowRoot) {
            const shadow = Array.from(el.shadowRoot.querySelectorAll('*'))
                .map(e => (e.innerText || "")).join(' ');
            if (shadow) t += "\n" + shadow;
        }
    });
    return t;
}
