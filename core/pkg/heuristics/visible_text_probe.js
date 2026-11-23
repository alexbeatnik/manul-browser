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
    const hidden = (el) => {
        const st = window.getComputedStyle(el);
        return st.display === 'none' || st.visibility === 'hidden' || st.opacity === '0';
    };
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
            if (hidden(el)) return;
            if (el.title)       t += el.title + " ";
            if (el.value && typeof el.value === 'string') t += el.value + " ";
            if (el.placeholder) t += el.placeholder + " ";
            const ariaLabel = el.getAttribute && el.getAttribute('aria-label');
            if (ariaLabel) t += ariaLabel + " ";
            const ariaValText = el.getAttribute && el.getAttribute('aria-valuetext');
            if (ariaValText) t += ariaValText + " ";
        });
    });
    return t.toLowerCase();
})()
