(() => {
    let t = (document.body.innerText || "") + " ";
    // Every shadow root, however deep. document.querySelectorAll stops at the
    // first one, and a page built of web components nests them: archive.org
    // is app-root > collection-page > collection-browser > …, and the whole
    // of its text read as "skip to main content".
    const roots = [document];
    for (let i = 0; i < roots.length; i++) {
        roots[i].querySelectorAll('*').forEach(el => {
            if (el.shadowRoot) roots.push(el.shadowRoot);
        });
    }
    // Transparent is not shown. innerText leaves out display:none and
    // visibility:hidden but keeps opacity:0, and opacity is not inherited —
    // the text inside a faded-out block has an opacity of 1 of its own — so
    // VERIFY that 'X' is NOT present failed on text nobody could see. The
    // same bar as the snapshot's: under 0.01 is invisible.
    const transparent = (st) => parseFloat(st.opacity) < 0.01;
    const hidden = (el) => {
        const st = window.getComputedStyle(el);
        return st.display === 'none' || st.visibility === 'hidden' || transparent(st);
    };
    const faded = [];
    const inFaded = (el) => faded.some(f => f.contains(el));
    roots.forEach((root, i) => {
        if (i > 0) {
            // A shadow root's own text, a top-level child at a time:
            // innerText of an element covers what is below it in the same
            // tree. One that is not rendered answers with its textContent,
            // which for a <style> is CSS, so those are skipped.
            Array.from(root.children).forEach(c => {
                if (/^(STYLE|SCRIPT|TEMPLATE|LINK)$/.test(c.tagName) || hidden(c)) return;
                t += (c.innerText || "") + " ";
            });
        }
        root.querySelectorAll('*').forEach(el => {
            const st = window.getComputedStyle(el);
            if (transparent(st) && !inFaded(el)) faded.push(el);
            if (st.display === 'none' || st.visibility === 'hidden' || inFaded(el)) return;
            if (el.title)       t += el.title + " ";
            if (el.value && typeof el.value === 'string') t += el.value + " ";
            if (el.placeholder) t += el.placeholder + " ";
            const ariaLabel = el.getAttribute && el.getAttribute('aria-label');
            if (ariaLabel) t += ariaLabel + " ";
            const ariaValText = el.getAttribute && el.getAttribute('aria-valuetext');
            if (ariaValText) t += ariaValText + " ";
        });
    });
    // Each transparent block takes its own text out once. Once, not every
    // occurrence: the same words shown elsewhere on the page stay present.
    const fold = (s) => s.replace(/\s+/g, ' ').trim().toLowerCase();
    t = ' ' + fold(t) + ' ';
    faded.forEach(el => {
        const s = fold(el.innerText || '');
        if (!s) return;
        const at = t.indexOf(s);
        if (at >= 0) t = t.slice(0, at) + ' ' + t.slice(at + s.length);
    });
    return t;
})()
