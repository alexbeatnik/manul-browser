([target, hint]) => {
    target = (target || '').toLowerCase();
    hint = (hint || '').toLowerCase();
    
    var ALL_TAGS = 'div, span, p, h1, h2, h3, h4, h5, h6, li, dd, dt, '
        + 'strong, b, i, em, label, a, button, td, th, article, section';
    var hintWords = hint ? hint.split(/\s+/).filter(function(w) { return w.length > 1; }) : [];
    var wordMatch = function(text, word) {
        if (word.length >= 5) return text.includes(word);
        var re = new RegExp('\\b' + word.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\b', 'i');
        return re.test(text);
    };

    // ── Form controls ─────────────────────────────────────────────────────────
    // What a field holds is not text: it is in none of the nodes the strategies
    // below read. A filled input used to come back as its own label, or as
    // whichever nearby sentence happened to contain the word.
    var FIELDS = 'input, textarea, select, [contenteditable="true"], [role="textbox"], [role="searchbox"]';
    var NOT_TEXT_INPUTS = ['checkbox', 'radio', 'button', 'submit', 'reset', 'image', 'file', 'hidden'];
    var isField = function(el) {
        if (!el || !el.matches || !el.matches(FIELDS)) return false;
        return el.tagName !== 'INPUT' || NOT_TEXT_INPUTS.indexOf((el.type || '').toLowerCase()) < 0;
    };
    // An object, not a string: an empty field is still an answer, and an empty
    // string is how every other strategy says it found nothing.
    var fieldValue = function(el) {
        var v;
        if (el.tagName === 'SELECT') {
            // What is shown as selected, not the option's value attribute.
            v = Array.from(el.selectedOptions).map(function(o) {
                return o.text.replace(/\s+/g, ' ').trim();
            }).join(', ');
        } else if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
            v = el.value;
        } else {
            v = (el.innerText || el.textContent || '').trim();
        }
        return { field: true, value: v == null ? '' : String(v) };
    };
    // "Customer name:" and "Email *" are the names "customer name" and "email".
    var asName = function(s) {
        return (s || '').replace(/\s+/g, ' ').trim().toLowerCase().replace(/[\s:*]+$/, '');
    };
    var fieldNames = function(el) {
        var names = ['aria-label', 'placeholder', 'title', 'name', 'id', 'data-qa', 'data-testid', 'data-test']
            .map(function(attr) { return el.getAttribute(attr); });
        Array.from(el.labels || []).forEach(function(lbl) {
            // A wrapping label's text includes its control's own: the options
            // of a <select>, the content of a <textarea>.
            var copy = lbl.cloneNode(true);
            Array.from(copy.querySelectorAll('input, select, textarea')).forEach(function(c) { c.remove(); });
            names.push(copy.textContent);
        });
        var scope = el.getRootNode();
        (el.getAttribute('aria-labelledby') || '').split(/\s+/).forEach(function(id) {
            var ref = id && scope.getElementById && scope.getElementById(id);
            if (ref) names.push(ref.textContent);
        });
        return names.map(asName).filter(Boolean);
    };
    var collectFields = function(root, out) {
        Array.from(root.querySelectorAll(FIELDS)).forEach(function(el) {
            if (isField(el) && el.getClientRects().length > 0) out.push(el);
        });
        Array.from(root.querySelectorAll('*')).forEach(function(el) {
            if (el.shadowRoot) collectFields(el.shadowRoot, out);
        });
        return out;
    };
    // Text that turns out to be a field's label answers with the field.
    var textOrField = function(el) {
        var lbl = el.closest('label');
        if (lbl && isField(lbl.control)) return fieldValue(lbl.control);
        return el.innerText.trim();
    };

    // Strategy 0: a form control the target names outright. Ahead of the text
    // strategies because they would find the same words in its label.
    var wanted = asName(target);
    if (wanted) {
        var spellings = [wanted, wanted.replace(/ /g, '-'), wanted.replace(/ /g, '_'), wanted.replace(/ /g, '')];
        var fields = collectFields(document, []);
        for (var fi = 0; fi < fields.length; fi++) {
            var named = fieldNames(fields[fi]).some(function(n) { return spellings.indexOf(n) >= 0; });
            if (named) return fieldValue(fields[fi]);
        }

        // Strategy 0b: a field with no name of its own, captioned by the
        // element just before it — "<p>Number</p><input>", an unbound <label>,
        // or the table cell beside the one holding the field. The caption has
        // to be the target and nothing else, and to sit beside exactly one
        // field; anything looser belongs to the text strategies.
        var captions = Array.from(document.querySelectorAll(ALL_TAGS));
        for (var pi = 0; pi < captions.length; pi++) {
            var caption = captions[pi];
            // textContent is free; innerText costs a layout, so rule out by
            // length first.
            if ((caption.textContent || '').length > wanted.length + 200) continue;
            if (asName(caption.innerText) !== wanted) continue;
            var beside = caption.nextElementSibling;
            if (!beside) continue;
            var held = isField(beside) ? [beside] : collectFields(beside, []);
            if (held.length === 1) return fieldValue(held[0]);
        }
    }

    // Gather all <th> text once for column/row word splitting
    var allTables = Array.from(document.querySelectorAll('table'));
    var allThTexts = [];
    allTables.forEach(function(t) {
        Array.from(t.querySelectorAll('th')).forEach(function(th) {
            allThTexts.push(th.innerText.toLowerCase().trim());
        });
    });

    // Split target words into column-words (found in headers) and row-words (the rest)
    var classifyWords = function(words) {
        var colWords = [];
        var rowWords = [];
        for (var i = 0; i < words.length; i++) {
            var w = words[i];
            var isCol = allThTexts.some(function(th) { return wordMatch(th, w); });
            if (isCol) { colWords.push(w); } else { rowWords.push(w); }
        }
        return { col: colWords, row: rowWords };
    };

    // Strategy 1: Search table rows for word matches
    if (target) {
        var targetWords = target.split(/\s+/).filter(function(w) { return w.length > 2; });
        var classified = classifyWords(targetWords);
        // Use row-words for row matching; fall back to all targetWords if no row-words
        var searchWords = classified.row.length > 0 ? classified.row : targetWords;

        var rows = Array.from(document.querySelectorAll('tr, [role="row"]'));
        var bestRow = null;
        var maxMatches = 0;
        var bestLen = Infinity;
        for (var ri = 0; ri < rows.length; ri++) {
            var rText = rows[ri].innerText.toLowerCase();
            // Skip header rows when we have row-specific search words
            var isHeader = rows[ri].querySelectorAll('th').length > 0 && rows[ri].querySelectorAll('td').length === 0;
            if (isHeader && classified.row.length > 0) continue;
            var matches = 0;
            if (searchWords.length === 0) {
                if (wordMatch(rText, target)) matches = 1;
            } else {
                for (var wi = 0; wi < searchWords.length; wi++) {
                    if (wordMatch(rText, searchWords[wi])) matches++;
                }
            }
            if (matches > maxMatches) {
                maxMatches = matches;
                bestLen = rText.length;
                bestRow = rows[ri];
            } else if (matches === maxMatches && maxMatches > 0) {
                if (rText.length < bestLen) { bestLen = rText.length; bestRow = rows[ri]; }
            }
        }
        if (bestRow) {
            var cells = Array.from(bestRow.querySelectorAll('td'));
            if (cells.length === 0) cells = Array.from(bestRow.querySelectorAll('[role="cell"]'));
            if (cells.length > 1) {
                // Try column header matching using column-words
                var colSearchWords = classified.col.length > 0 ? classified.col : hintWords;
                if (colSearchWords.length > 0) {
                    var table = bestRow.closest('table');
                    if (table) {
                        var ths = Array.from(table.querySelectorAll('th'));
                        for (var hw = 0; hw < colSearchWords.length; hw++) {
                            for (var ti = 0; ti < ths.length; ti++) {
                                if (wordMatch(ths[ti].innerText.toLowerCase(), colSearchWords[hw])) {
                                    if (ti < cells.length) return cells[ti].innerText.trim();
                                }
                            }
                        }
                    }
                }
                // Try hint-based cell matching
                if (hint) {
                    var hintIdx = -1;
                    for (var hi = 0; hi < cells.length; hi++) {
                        if (cells[hi].innerText.toLowerCase().includes(hint)) { hintIdx = hi; break; }
                    }
                    if (hintIdx >= 0 && hintIdx + 1 < cells.length) return cells[hintIdx + 1].innerText.trim();
                    for (var hi2 = 0; hi2 < cells.length; hi2++) {
                        if (cells[hi2].innerText.toLowerCase().includes(hint)) return cells[hi2].innerText.trim();
                    }
                }
                // Try numeric/currency cell
                for (var ci = 0; ci < cells.length; ci++) {
                    var ct = cells[ci].innerText.trim();
                    if (/[$\u20ac\u00a3%\u20b4]/.test(ct) || ct.includes('Rs.') || !isNaN(parseFloat(ct))) return ct;
                }
                return cells[cells.length - 1].innerText.trim();
            }
            if (cells.length === 1) return cells[0].innerText.trim();
            return bestRow.innerText.trim();
        }
    }
    // Strategy 2: hint-only with column resolution
    if (hintWords.length > 0) {
        var rows2 = Array.from(document.querySelectorAll('tr'));
        for (var r2i = 0; r2i < rows2.length; r2i++) {
            var row = rows2[r2i];
            var cells2 = Array.from(row.querySelectorAll('td'));
            if (cells2.length < 2) continue;
            var rowText = cells2.map(function(c) { return c.innerText.toLowerCase(); }).join(' ');
            var table2 = row.closest('table');
            var ths2 = table2 ? Array.from(table2.querySelectorAll('th')) : [];
            var colWords = hintWords.filter(function(w) { return ths2.some(function(th) { return th.innerText.toLowerCase().includes(w); }); });
            var rowWords = hintWords.filter(function(w) { return colWords.indexOf(w) < 0; });
            if (rowWords.length > 0 && rowWords.every(function(w) { return rowText.includes(w); })) {
                if (colWords.length > 0) {
                    var colIdx2 = -1;
                    for (var ti2 = 0; ti2 < ths2.length; ti2++) {
                        if (colWords.some(function(w) { return ths2[ti2].innerText.toLowerCase().includes(w); })) { colIdx2 = ti2; break; }
                    }
                    if (colIdx2 >= 0 && colIdx2 < cells2.length) return cells2[colIdx2].innerText.trim();
                }
                for (var ci2 = 0; ci2 < cells2.length; ci2++) {
                    var ct2 = cells2[ci2].innerText.trim();
                    if (/[$\u20ac\u00a3%\u20b4]/.test(ct2) || !isNaN(parseFloat(ct2))) return ct2;
                }
                return cells2[cells2.length - 1].innerText.trim();
            }
        }
    }
    // Strategy 3: broad text search
    if (target) {
        var allEls = Array.from(document.querySelectorAll(ALL_TAGS));
        for (var ei = 0; ei < allEls.length; ei++) {
            var txt = (allEls[ei].innerText || '').trim().toLowerCase();
            if (!txt || txt.length > 200) continue;
            
            var matchAllWords = true;
            if (targetWords.length === 0) matchAllWords = false;
            for (var wi = 0; wi < targetWords.length; wi++) {
                if (!txt.includes(targetWords[wi])) {
                    matchAllWords = false;
                    break;
                }
            }

            if (txt === target || txt.includes(target) || matchAllWords) {
                var kids = allEls[ei].querySelectorAll(ALL_TAGS);
                var leafKids = Array.from(kids).filter(function(k) {
                    var kt = (k.innerText || '').trim().toLowerCase();
                    if (!kt) return false;
                    var kw = true;
                    for (var ki = 0; ki < targetWords.length; ki++) {
                       if (!kt.includes(targetWords[ki])) kw = false;
                    }
                    return kt.includes(target) || kw;
                });
                if (leafKids.length > 0) return textOrField(leafKids[leafKids.length - 1]);
                
                // If the target matched the parent but no children had the target words,
                // perhaps a child holds the value we want (like "3.3%"). 
                // Return the text of the LAST child if it exists, otherwise the parent's text.
                if (kids.length > 0) {
                    var lastChildText = (kids[kids.length - 1].innerText || '').trim();
                    if (lastChildText && lastChildText.length < 50) return lastChildText;
                }
                
                return textOrField(allEls[ei]);
            }
        }
    }
    return '';
}
