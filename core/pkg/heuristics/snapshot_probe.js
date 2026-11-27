(() => {
    // ── Constants ──────────────────────────────────────────────────────────────

    // Tags to prune entirely (their subtrees carry no targeting signal).
    const PRUNE = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'SVG', 'TEMPLATE', 'HEAD']);

    // ── Utilities ──────────────────────────────────────────────────────────────

    const normalize = (v) => (v || '').replace(/\s+/g, ' ').trim();

    // A native <select> made transparent and laid over a styled facade is
    // still the thing a click lands on — a sort dropdown at opacity .001 over
    // the label it shows. Counting it as invisible left that dropdown out of
    // the snapshot, and SELECT had nothing to choose from. Text fields are not
    // let through the same way: a transparent one is as often a bot trap.
    const isVisible = (el) => {
        const cs = window.getComputedStyle(el);
        const rect = el.getBoundingClientRect();
        return cs.display !== 'none'
            && cs.visibility !== 'hidden'
            && (parseFloat(cs.opacity) >= 0.01 || el.tagName === 'SELECT')
            && rect.width > 0
            && rect.height > 0;
    };

    const isInteractive = (el) => {
        const tag = el.tagName.toLowerCase();
        if (['button', 'a', 'input', 'select', 'textarea', 'option', 'label',
             'summary', 'details', 'img'].includes(tag)) return true;
        const role = (el.getAttribute('role') || '').toLowerCase();
        if (['button', 'link', 'menuitem', 'tab', 'checkbox', 'radio',
             'switch', 'slider', 'spinbutton', 'textbox', 'combobox',
             'listbox', 'option', 'treeitem', 'gridcell', 'columnheader',
             'rowheader', 'row', 'cell'].includes(role)) return true;
        if (el.getAttribute('contenteditable') === 'true') return true;
        // Table data cells — needed for EXTRACT and disambiguation.
        if (tag === 'td' || tag === 'th' || tag === 'li' || tag === 'span' ||
            tag === 'p' || tag === 'div' || tag === 'section') return true;
        // Headings: the usual NEAR anchor ("Add to cart" NEAR 'Manul Plush').
        // Without them a product title was no element at all, and the anchor
        // fell to whatever container held its text — often the whole list.
        if (/^h[1-6]$/.test(tag)) return true;
        if (el.tagName.includes('-')) return true; // custom elements
        return false;
    };

    // ── Label resolution ───────────────────────────────────────────────────────

    const getLabelText = (el) => {
        // 1. Explicit <label for="id"> — every one of them. A page can point
        // two labels at one field; querySelector took the first, and the
        // order form's name field was labelled "Total: 790" rather than "Name:".
        if (el.id) {
            const forLabels = el.labels
                ? [...el.labels].filter(l => !l.contains(el))
                : [...document.querySelectorAll(`label[for="${CSS.escape(el.id)}"]`)];
            const text = forLabels.map(l => normalize(l.innerText || l.textContent)).filter(Boolean).join(' ');
            if (text) return text;
        }
        // 2. Wrapping <label>
        const parent = el.closest('label');
        if (parent) {
            const clone = parent.cloneNode(true);
            for (const child of clone.querySelectorAll('input,select,textarea')) child.remove();
            return normalize(clone.innerText || clone.textContent);
        }
        // 3. aria-labelledby
        const lblIds = el.getAttribute('aria-labelledby');
        if (lblIds) {
            const text = lblIds.split(/\s+/)
                .map(id => document.getElementById(id))
                .filter(Boolean)
                .map(e => normalize(e.innerText || e.textContent))
                .join(' ');
            if (text) return text;
        }
        // 4. A checkbox or radio with no label is captioned by the text that
        // follows it, up to the next box or <br> — "<input type=checkbox>
        // checkbox 2". Without this the box had no text at all and was found
        // only when a container around it happened to be in the snapshot.
        // pagejs.CaptionedBox reads captions by the same rule.
        if (el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) {
            let t = '';
            for (let n = el.nextSibling; n; n = n.nextSibling) {
                if (n.nodeType === Node.ELEMENT_NODE) {
                    if (n.tagName === 'BR' || n.matches('input,select,textarea,button') ||
                        n.querySelector('input,select,textarea,button')) break;
                    t += ' ' + (n.innerText || n.textContent || '');
                } else if (n.nodeType === Node.TEXT_NODE) {
                    t += ' ' + n.textContent;
                }
            }
            t = normalize(t);
            if (t) return t;
        }
        // 5. Fieldset legend context for grouped controls.
        if (['INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName)) {
            const fieldset = el.closest('fieldset');
            if (fieldset) {
                const legend = Array.from(fieldset.children || []).find(child => child.tagName === 'LEGEND')
                    || fieldset.querySelector('legend');
                if (legend) {
                    const legendText = normalize(legend.innerText || legend.textContent);
                    if (legendText) return legendText;
                }
            }
        }
        // 6. Table column header context for <td>
        if (el.tagName === 'TD' || el.tagName === 'TH') {
            const tr = el.closest('tr');
            if (tr) {
                const cells = Array.from(tr.cells || tr.querySelectorAll('td,th'));
                const texts = [];
                for (const c of cells) {
                    if (c === el) continue;
                    const ct = normalize(c.innerText || c.textContent);
                    if (ct.length > 0 && ct.length < 100) texts.push(ct);
                }
                const table = tr.closest('table');
                if (table) {
                    const colIdx = Array.from(tr.cells || tr.children).indexOf(el);
                    if (colIdx >= 0) {
                        const hrow = table.querySelector('thead tr')
                            || (() => {
                                const fr = table.querySelector('tr');
                                return (fr && fr !== tr && fr.querySelector('th')) ? fr : null;
                            })();
                        if (hrow) {
                            const hCells = Array.from(hrow.cells || hrow.children);
                            if (colIdx < hCells.length) {
                                const ht = normalize(hCells[colIdx].innerText || hCells[colIdx].textContent);
                                if (ht && ht.length < 100) texts.unshift(ht);
                            }
                        }
                    }
                }
                if (texts.length > 0) return texts.join(' ');
            }
        }
        return '';
    };

    // ── Accessible name ────────────────────────────────────────────────────────

    const CONTROL_SELECTOR = 'a, button, [role="button"], [role="link"]';
    const NAMED_BY_CONTENT = /^(A|BUTTON)$/;
    const NAMED_BY_CONTENT_ROLE = /^(link|button|menuitem|tab)$/;

    const getAccessibleName = (el) => {
        const ariaLabel = el.getAttribute('aria-label');
        if (ariaLabel) return normalize(ariaLabel);
        const labelText = getLabelText(el);
        if (labelText) return labelText;
        const title = el.getAttribute('title');
        if (title) return normalize(title);
        const alt = el.getAttribute('alt');
        if (alt) return normalize(alt);
        // A link or button that is only a picture is named by the picture:
        // <a class="cart-icon"><img alt="Cart"> 1</a> is the "Cart" link.
        // Without this it had no name at all, and 'Cart' went to the first
        // "ADD TO CART" button on the page.
        if (NAMED_BY_CONTENT.test(el.tagName) || NAMED_BY_CONTENT_ROLE.test(el.getAttribute('role') || '')) {
            const alts = [...el.querySelectorAll('img[alt]')].map(img => normalize(img.alt)).filter(Boolean);
            if (alts.length) return alts.join(' ');
        }
        return '';
    };

    // ── XPath builder ──────────────────────────────────────────────────────────

    const buildXPath = (el) => {
        const parts = [];
        let node = el;
        while (node && (node.nodeType === Node.ELEMENT_NODE || node.nodeType === Node.DOCUMENT_FRAGMENT_NODE)) {
            if (node.nodeType === Node.DOCUMENT_FRAGMENT_NODE) {
                if (node.host) {
                    node = node.host;
                    continue;
                } else {
                    break;
                }
            }
            const tag = node.tagName.toLowerCase();
            let idx = 1;
            let sibling = node.previousElementSibling;
            while (sibling) {
                if (sibling.tagName.toLowerCase() === tag) idx++;
                sibling = sibling.previousElementSibling;
            }
            parts.unshift(`${tag}[${idx}]`);
            node = node.parentElement || (node.parentNode && node.parentNode.nodeType === Node.DOCUMENT_FRAGMENT_NODE ? node.parentNode : null);
        }
        return '/' + parts.join('/');
    };

    // The text a person sees in el: its innerText without the text drawn at
    // font-size 0. Allo writes "hex" into each colour swatch at size 0, and
    // Comfy's rating link repeats its score in a star at size 0; the map
    // listed 'hex' five times and '4.9 4.9 218'. It goes text node by text
    // node, because a container at size 0 whose children set a size of their
    // own — the way inline-blocks are packed without gaps — shows them all.
    // The same function is in pkg/scan's map probe, so a label the map gives
    // is the text a step matches.
    const __manulSizes = new WeakMap();
    function shownText(el) {
        const text = el.innerText || el.textContent || '';
        if (!text.trim() || text.length > 300) return text;
        let out = text;
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        let node, seen = 0;
        while ((node = walker.nextNode()) && seen++ < 64) {
            const t = node.nodeValue.trim();
            const p = node.parentElement;
            if (!t || !p) continue;
            let size = __manulSizes.get(p);
            if (size === undefined) {
                const f = parseFloat(getComputedStyle(p).fontSize);
                size = isNaN(f) ? 16 : f;
                __manulSizes.set(p, size);
            }
            if (size < 1) {
                const i = out.indexOf(t);
                if (i >= 0) out = out.slice(0, i) + out.slice(i + t.length);
            }
        }
        return out;
    }

    // ── Element ID registry ────────────────────────────────────────────────────

    // Use expando property to avoid mutating the live DOM with attributes.
    if (window.__manulIdCounter === undefined) window.__manulIdCounter = 0;
    const reg = window.__manulReg = window.__manulReg || {};

    // ── Collection ─────────────────────────────────────────────────────────────

    const seen = new Set();
    const elements = [];

    const processElement = (el, inShadow) => {
        if (seen.has(el)) return;
        seen.add(el);
        if (!isInteractive(el)) return;

        const tag = el.tagName.toLowerCase();
        const vis = isVisible(el);

        // Special inputs (checkbox, radio, file) collected even when hidden.
        const isSpecial = tag === 'input' && (el.type === 'checkbox' || el.type === 'radio' || el.type === 'file');
        if (!vis && !isSpecial) return;

        const rect = el.getBoundingClientRect();
        // Collect all interactive elements, even if small (might be hidden inputs)

        let eid = el.__manulId;
        if (eid === undefined) {
            eid = ++window.__manulIdCounter;
            el.__manulId = eid;
        }
        reg[eid] = el;

        const visibleText = normalize(shownText(el).slice(0, 300));
        const labelText   = getLabelText(el);
        const accessibleName = getAccessibleName(el);

        // Every placeholder a probe has seen this element show, the map's
        // scan included (pkg/scan records into the same expando). Comfy types
        // example searches into its search box's placeholder a letter at a
        // time, so the label the map showed had changed before the step that
        // used it ran, and nothing on the page matched it any more.
        const placeholder = el.getAttribute('placeholder') || '';
        if (placeholder.trim()) {
            const seenPh = el.__manulPh || (el.__manulPh = []);
            if (seenPh.indexOf(placeholder.trim()) === -1) {
                seenPh.push(placeholder.trim());
                if (seenPh.length > 32) seenPh.shift();
            }
        }

        // Extract icon classes from <i>, <span>, or <svg> children — but not
        // from inside a link or button of their own. Those icons name that
        // control; collected here they made the header around a.cart-icon a
        // better match for 'cart-icon' than the link itself.
        const iconClasses = (() => {
            const icons = el.querySelectorAll("i, span, svg, [class*='icon'], [class*='fa-']");
            const classes = new Set();
            icons.forEach(icon => {
                const owner = icon.closest(CONTROL_SELECTOR);
                if (owner && owner !== el && el.contains(owner)) return;
                if (typeof icon.className === 'string' && icon.className) {
                    icon.className.split(/\s+/).forEach(c => {
                        if (c.includes('icon') || c.includes('fa-')) classes.add(c);
                    });
                }
            });
            return Array.from(classes).join(' ');
        })();

        // Capture up to 8 levels of ancestor tags.
        const ancestors = [];
        let p = el.parentElement;
        while (p && ancestors.length < 8) {
            ancestors.push(p.tagName.toLowerCase());
            p = p.parentElement;
        }

        // Build canonical name: context -> core_name [METADATA]
        const name = (() => {
            const core = accessibleName || visibleText || el.id || el.getAttribute('name') || tag;
            let n = core.slice(0, 80);
            if (!vis) n += ' [HIDDEN]';
            if (inShadow) n += ' [SHADOW_DOM]';
            if (el.disabled) n += ' [DISABLED]';
            return n;
        })();

        elements.push({
            id:             eid,
            name:           name,
            xpath:          buildXPath(el),
            tag:            tag,
            input_type:     typeof el.type === 'string' ? el.type : '',
            visible_text:   visibleText,
            aria_label:     el.getAttribute('aria-label') || '',
            placeholder:    placeholder,
            placeholders_seen: (el.__manulPh && el.__manulPh.length > 1) ? el.__manulPh.slice() : undefined,
            title:          el.getAttribute('title') || '',
            data_qa:        el.getAttribute('data-qa') || '',
            data_testid:    el.getAttribute('data-testid') || el.getAttribute('data-test') || '',
            label_text:     labelText,
            name_attr:      el.getAttribute('name') || '',
            html_id:        el.getAttribute('id') || '',
            class_name:     (typeof el.className === 'string' ? el.className : '') || '',
            icon_classes:   iconClasses,
            role:           el.getAttribute('role') || '',
            value:          typeof el.value === 'string' ? el.value : (typeof el.value === 'number' && el.value !== 0 ? String(el.value) : ''),
            accessible_name: accessibleName,
            ancestors:      ancestors,
            is_visible:     vis,
            is_disabled:    el.disabled === true || el.getAttribute('aria-disabled') === 'true',
            is_hidden:      !vis,
            is_select:      tag === 'select',
            is_contenteditable: el.contentEditable === 'true' || el.getAttribute('contenteditable') === 'true',
            is_editable:    el.disabled !== true && el.readOnly !== true &&
                            ((tag === 'input' && !['radio','checkbox','submit','button','image','reset','file','hidden'].includes((typeof el.type === 'string' ? el.type : '').toLowerCase())) ||
                             tag === 'textarea' ||
                             el.getAttribute('contenteditable') === 'true' ||
                             (el.getAttribute('role') || '') === 'textbox'),
            // Read as true or false, never as whatever the element holds:
            // YouTube's tabs keep the selected tab's index in `selected`, a
            // component may keep `disabled` as a word, and a number where a
            // boolean belongs failed the snapshot of the whole page. So do
            // value and type, which a custom element may hold as anything.
            is_checked:     el.checked === true || el.getAttribute('aria-checked') === 'true',
            is_selected:    el.selected === true || el.getAttribute('aria-selected') === 'true',
            is_in_shadow:   inShadow,
            rect: {
                top:    rect.top,
                left:   rect.left,
                bottom: rect.bottom,
                right:  rect.right,
                width:  rect.width,
                height: rect.height,
            },
        });
    };

    // ── TreeWalker DOM traversal (with shadow DOM support) ─────────────────────

    const walk = (root, inShadow) => {
        const tw = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT, {
            acceptNode(n) {
                if (PRUNE.has(n.tagName)) return NodeFilter.FILTER_REJECT;
                return NodeFilter.FILTER_ACCEPT;
            }
        });
        let n;
        while ((n = tw.nextNode())) {
            // Pierce shadow roots — handles Web Components.
            if (n.shadowRoot) walk(n.shadowRoot, true);
            processElement(n, inShadow);
        }
    };
    walk(document.body || document.documentElement, false);

    // ── Full visible page text (for VERIFY) ────────────────────────────────────

    const pageText = (() => {
        let t = (document.body ? document.body.innerText : '') + ' ';
        document.querySelectorAll('[aria-label],[placeholder],[title],[aria-valuetext],input,textarea,select').forEach(el => {
            const cs = window.getComputedStyle(el);
            if (cs.display === 'none' || cs.visibility === 'hidden') return;
            t += ' ' + (el.getAttribute('aria-label') || '');
            t += ' ' + (el.getAttribute('placeholder') || '');
            t += ' ' + (el.getAttribute('title') || '');
            t += ' ' + (el.getAttribute('aria-valuetext') || '');
            if (typeof el.value === 'string') t += ' ' + el.value;
        });
        return t.toLowerCase();
    })();

    return {
        url:         window.location.href,
        title:       document.title || '',
        visible_text: pageText,
        elements:    elements
    };
})()
