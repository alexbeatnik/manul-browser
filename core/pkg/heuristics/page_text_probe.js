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
    // Append shadow-root text the flat innerText misses, from every shadow
    // root however deep: querySelectorAll does not enter one, and pages of
    // web components nest them. Each root's text is read a top-level child at
    // a time — innerText covers the rest of that tree — skipping <style> and
    // the like, whose innerText is their source when they are not rendered.
    const roots = [root];
    for (let i = 0; i < roots.length; i++) {
        roots[i].querySelectorAll('*').forEach(el => {
            if (el.shadowRoot) roots.push(el.shadowRoot);
        });
    }
    // Transparent text is not on the page for anyone reading it, and VERIFY
    // already says so: innerText keeps opacity:0, and opacity is not
    // inherited, so the topmost transparent elements are found and each one's
    // text taken out once. The same bar as VERIFY and the snapshot.
    const transparent = (el) => parseFloat(getComputedStyle(el).opacity) < 0.01;
    if (root.nodeType === 1 && transparent(root)) return "";
    const faded = [];
    roots.forEach(r => r.querySelectorAll('*').forEach(el => {
        if (transparent(el) && !faded.some(f => f.contains(el))) faded.push(el);
    }));
    roots.slice(1).forEach(sr => {
        const shadow = Array.from(sr.children)
            .filter(c => !/^(STYLE|SCRIPT|TEMPLATE|LINK)$/.test(c.tagName) && !faded.includes(c))
            .map(c => (c.innerText || "")).join(' ');
        if (shadow.trim()) t += "\n" + shadow;
    });
    // Once, case and lines kept: the same words shown elsewhere stay. A block
    // whose text is not found exactly as innerText laid it out stays too.
    faded.forEach(el => {
        const s = (el.innerText || "").trim();
        if (!s) return;
        const at = t.indexOf(s);
        if (at >= 0) t = t.slice(0, at) + t.slice(at + s.length);
    });
    return t.replace(/\n{3,}/g, "\n\n");
}
