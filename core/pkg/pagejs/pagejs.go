// Package pagejs holds the in-page JavaScript every browser backend runs.
//
// The DOM work behind FOCUS, FILL, CHECK, SCROLL, HIGHLIGHT and the coordinate
// lookups is JavaScript, not protocol: CDP and WebDriver BiDi differ only in how
// that source reaches the page. Keeping the source here is what stops the two
// backends from growing their own dialects of it — the engine is written once,
// and so is the JS it injects.
//
// Every builder returns a complete script whose *completion value* is the
// result, matching what both `Runtime.evaluate` (CDP) and `script.evaluate`
// (BiDi) return. Statements are therefore fine; a trailing expression is the
// return value.
package pagejs

import (
	"fmt"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/core"
)

// Focus focuses the element resolved by engine ID or XPath.
func Focus(id int, xpath string) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%d]) || document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) el.focus();
	`, id, xpath)
}

// SetInputValue sets the value of an input element resolved by ID or XPath.
// Uses the native HTMLInputElement/HTMLTextAreaElement value setter to
// bypass framework-level overrides (React, Vue, etc.) that intercept
// the value property on individual elements.
func SetInputValue(id int, xpath, value string) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) {
			var targetEl = el;
			if (el.tagName !== 'INPUT' && el.tagName !== 'TEXTAREA' && el.tagName !== 'SELECT' && el.getAttribute('contenteditable') !== 'true') {
				if (el.tagName === 'LABEL' && el.htmlFor) {
					targetEl = document.getElementById(el.htmlFor) || targetEl;
				} else {
					var child = el.querySelector('input, textarea, select');
					if (child) {
						targetEl = child;
					} else if (el.nextElementSibling) {
						var next = el.nextElementSibling.matches('input, textarea, select') ? el.nextElementSibling : el.nextElementSibling.querySelector('input, textarea, select');
						if (next) targetEl = next;
					} else if (el.parentElement && el.parentElement.nextElementSibling) {
						var nextParent = el.parentElement.nextElementSibling;
						var pChild = nextParent.matches('input, textarea, select') ? nextParent : nextParent.querySelector('input, textarea, select');
						if (pChild) targetEl = pChild;
					}
				}
			}
			el = targetEl;

			// Use the native value setter so React/Vue/Angular state updates fire.
			var proto = Object.getPrototypeOf(el);
			var nativeSetter = null;
			while (proto && proto !== Object.prototype) {
				var desc = Object.getOwnPropertyDescriptor(proto, 'value');
				if (desc && desc.set) {
					nativeSetter = desc.set;
					break;
				}
				proto = Object.getPrototypeOf(proto);
			}

			if (nativeSetter) {
				nativeSetter.call(el, %[3]q);
			} else if (el.isContentEditable) {
				// A contenteditable has no value: assigning one creates a plain
				// property and leaves the text as it was, so the step passed
				// over an editor nothing had been typed into. Replace its
				// contents the way typing would, so the editor's own input
				// handling sees it; fall back to the text itself.
				el.focus();
				var range = document.createRange();
				range.selectNodeContents(el);
				var selection = window.getSelection();
				selection.removeAllRanges();
				selection.addRange(range);
				if (!document.execCommand('insertText', false, %[3]q) || el.textContent !== %[3]q) {
					el.textContent = %[3]q;
				}
			} else {
				el.value = %[3]q;
			}
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
			// Focus the element so a subsequent PRESS Enter / Tab / etc.
			// reaches it as document.activeElement. Without this, SPA
			// sites like YouTube swallow the key on document.body and
			// the form is never submitted.
			if (typeof el.focus === 'function') {
				try { el.focus({ preventScroll: true }); } catch (_) { el.focus(); }
			}
		} else {
			// Same rule as ElementCenter: an element that is gone is a failed
			// step, not a fill that quietly did nothing.
			throw new Error("Element not found");
		}
	`, id, xpath, value)
}

// checkable is the JS that walks from a resolved element to the checkbox or
// radio it stands for: the element itself, the control a <label> points at,
// one nested inside it, or one sharing its cell or row. It is defined once
// because the step that sets a state and the step that reads it back have to
// land on the same control — when they did not, a box ticked through its
// caption was reported as never having been ticked.
const checkable = `
		var manulCheckable = function(el) {
			var boxes = 'input[type=checkbox], input[type=radio]';
			if (el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) return el;
			if (el.tagName === 'LABEL' && el.htmlFor) return document.getElementById(el.htmlFor) || el;
			var child = el.querySelector(boxes);
			if (child) return child;
			// Look in nearby siblings or parents (common in tables)
			var cell = el.closest('td, th, div');
			if (cell) {
				var cb = cell.querySelector(boxes) || (cell.parentElement && cell.parentElement.querySelector(boxes));
				if (cb) return cb;
			}
			return el;
		};
`

// CaptionedBox picks, out of the checkboxes and radios inside the element at
// ID or XPath, the one captioned with caption. Its completion value is a JSON
// string: {"count":n,"xpath":"…"}, with xpath empty when no single box is
// captioned so, or when there are fewer than two boxes to choose between.
//
// A box's caption is its <label>, or the text that follows it up to the next
// box or line break: <input type=checkbox> checkbox 1<br><input …> checkbox 2.
// No element holds such a caption alone, so the step resolved to the form
// holding both, and manulCheckable took the first box in it — UNCHECK
// 'checkbox 2' unchecked checkbox 1, and passed.
func CaptionedBox(id int, xpath, caption string) string {
	return fmt.Sprintf(`(() => {
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el || !el.querySelectorAll) return JSON.stringify({count: 0, xpath: ""});
		var boxes = Array.from(el.querySelectorAll('input[type=checkbox], input[type=radio]'));
		if (boxes.length < 2) return JSON.stringify({count: boxes.length, xpath: ""});
		var fold = function(t) { return (t || '').replace(/\s+/g, ' ').trim().toLowerCase(); };
		var want = fold(%[3]q);
		var captionOf = function(box) {
			var t = Array.from(box.labels || []).map(function(l) { return l.innerText; }).join(' ');
			for (var n = box.nextSibling; n; n = n.nextSibling) {
				if (n.nodeType === Node.ELEMENT_NODE) {
					if (n.tagName === 'BR' || n.matches('input[type=checkbox], input[type=radio]') ||
						n.querySelector('input[type=checkbox], input[type=radio]')) break;
					t += ' ' + n.innerText;
				} else if (n.nodeType === Node.TEXT_NODE) {
					t += ' ' + n.textContent;
				}
			}
			return fold(t);
		};
		var pick = function(test) {
			var hits = boxes.filter(function(b) { return test(captionOf(b)); });
			return hits.length === 1 ? hits[0] : null;
		};
		var box = pick(function(c) { return c === want; }) || pick(function(c) { return c.indexOf(want) >= 0; });
		if (!box) return JSON.stringify({count: boxes.length, xpath: ""});
		var parts = [];
		for (var node = box; node && node.nodeType === Node.ELEMENT_NODE; ) {
			var tag = node.tagName.toLowerCase(), idx = 1;
			for (var sib = node.previousElementSibling; sib; sib = sib.previousElementSibling) {
				if (sib.tagName.toLowerCase() === tag) idx++;
			}
			parts.unshift(tag + '[' + idx + ']');
			node = node.parentElement || (node.parentNode && node.parentNode.host) || null;
		}
		return JSON.stringify({count: boxes.length, xpath: '/' + parts.join('/')});
	})()`, id, xpath, caption)
}

// CheckedState reports the state of the checkbox, radio or ARIA-checkable
// control that the element at ID or XPath stands for. Its completion value is
// a JSON string: {"found":true,"checked":…}, or {"found":false} when there is
// no such control.
func CheckedState(id int, xpath string) string {
	return fmt.Sprintf(`(() => {%[3]s
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el) return JSON.stringify({found: false});
		el = manulCheckable(el);
		if (el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) {
			return JSON.stringify({found: true, checked: el.checked});
		}
		var role = (el.getAttribute('role') || '').toLowerCase();
		if (role === 'checkbox' || role === 'radio' || role === 'switch') {
			return JSON.stringify({found: true, checked: el.getAttribute('aria-checked') === 'true'});
		}
		return JSON.stringify({found: false});
	})()`, id, xpath, checkable)
}

// SetChecked sets the checked state of a checkbox, radio or ARIA-checkable
// element resolved by ID or XPath.
func SetChecked(id int, xpath string, checked bool) string {
	return fmt.Sprintf(checkable+`
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) {
			var desiredAria = %[3]v ? 'true' : 'false';
			el = manulCheckable(el);

			if (el.tagName === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) {
				if (el.checked !== %[3]v) {
					if (typeof el.scrollIntoView === 'function') {
						el.scrollIntoView({ block: 'center', inline: 'center' });
					}
					if (typeof el.focus === 'function') {
						try { el.focus({ preventScroll: true }); } catch (_) { el.focus(); }
					}

					// Let the browser perform the native toggle first.
					el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
					el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
					if (typeof el.click === 'function') {
						el.click();
					} else {
						el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
					}

					// If the native click did not produce the requested state,
					// fall back to the native checked setter and emit value events.
					if (el.checked !== %[3]v) {
						var proto = Object.getPrototypeOf(el);
						var nativeSetter = null;
						while (proto && proto !== Object.prototype) {
							var desc = Object.getOwnPropertyDescriptor(proto, 'checked');
							if (desc && desc.set) {
								nativeSetter = desc.set;
								break;
							}
							proto = Object.getPrototypeOf(proto);
						}
						if (nativeSetter) {
							nativeSetter.call(el, %[3]v);
						} else {
							el.checked = %[3]v;
						}
						el.dispatchEvent(new Event('input', { bubbles: true }));
						el.dispatchEvent(new Event('change', { bubbles: true }));
					}
				}
			} else {
				var role = (el.getAttribute('role') || '').toLowerCase();
				if (role === 'checkbox' || role === 'radio' || role === 'switch') {
					var current = el.getAttribute('aria-checked');
					if (current !== desiredAria) {
						if (typeof el.scrollIntoView === 'function') {
							el.scrollIntoView({ block: 'center', inline: 'center' });
						}
						if (typeof el.focus === 'function') {
							try { el.focus({ preventScroll: true }); } catch (_) { el.focus(); }
						}

						el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
						el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
						if (typeof el.click === 'function') {
							el.click();
						} else {
							el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
						}

						if (el.getAttribute('aria-checked') !== desiredAria) {
							el.setAttribute('aria-checked', desiredAria);
							el.dispatchEvent(new Event('input', { bubbles: true }));
							el.dispatchEvent(new Event('change', { bubbles: true }));
						}
					}
				}
			}
		}
	`, id, xpath, checked)
}

// DragCenters returns the viewport centres of two elements, measured after
// one scroll rather than two. Its completion value is a JSON string with
// x1/y1/x2/y2.
//
// ElementCenter scrolls its element into view before measuring, which makes
// it wrong to call twice for a drag: the second call scrolls to the target and
// moves the source out from under the coordinates the first call just returned.
// The press then lands wherever the source used to be. Nothing errors — the
// coordinates are real, they are simply stale — so the step passes and only a
// later verification notices nothing was dragged.
func DragCenters(srcID int, srcXPath string, dstID int, dstXPath string) string {
	return fmt.Sprintf(`
		var pick = function(id, xp) {
			return (window.__manulReg && window.__manulReg[id]) ||
				document.evaluate(xp, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		};
		var a = pick(%[1]d, %[2]q), b = pick(%[3]d, %[4]q);
		if (!a) { throw new Error("drag source not found in the page"); }
		if (!b) { throw new Error("drop target not found in the page"); }

		// Centre the source first. scrollIntoView is used rather than a computed
		// window.scrollTo because it is the primitive the rest of this file
		// relies on and it works against pages that scroll something other than
		// the window.
		a.scrollIntoView({block: 'center', inline: 'center'});

		// If that left the target off screen, split the difference: nudge by
		// half the gap so both sit inside the viewport. A source and its drop
		// target that cannot share a viewport are beyond what a coordinate drag
		// can express anyway.
		var ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect();
		var vh = window.innerHeight, vw = window.innerWidth;
		var dy = 0, dx = 0;
		if (rb.bottom > vh) { dy = Math.min(rb.bottom - vh + 8, ra.top - 8); }
		else if (rb.top < 0) { dy = Math.max(rb.top - 8, ra.bottom - vh + 8); }
		if (rb.right > vw) { dx = Math.min(rb.right - vw + 8, ra.left - 8); }
		else if (rb.left < 0) { dx = Math.max(rb.left - 8, ra.right - vw + 8); }
		if (dy !== 0 || dx !== 0) { window.scrollBy(dx, dy); }

		ra = a.getBoundingClientRect();
		rb = b.getBoundingClientRect();
		JSON.stringify({
			x1: ra.x + ra.width / 2, y1: ra.y + ra.height / 2,
			x2: rb.x + rb.width / 2, y2: rb.y + rb.height / 2
		});
	`, srcID, srcXPath, dstID, dstXPath)
}

// ScrollIntoView scrolls the element resolved by ID or XPath into the viewport.
func ScrollIntoView(id int, xpath string) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%d]) || document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) el.scrollIntoView({block: "center", inline: "center"});
	`, id, xpath)
}

// ScrollPage scrolls the page or a container element in the given direction.
// Currently only direction="up" or "down" are well supported.
func ScrollPage(direction, container string) string {
	var amount = 500
	if direction == "up" {
		amount = -500
	}
	containerLower := strings.ToLower(strings.TrimSpace(container))
	if containerLower == string(core.ScrollStrategyGenericList) {
		return fmt.Sprintf(`(() => {
			const target = document.querySelector('#dropdown') ||
				document.querySelector('[role="listbox"]') ||
				document.querySelector('[class*="dropdown"]');
			if (!target) return false;
			target.scrollBy({ top: %d, behavior: 'auto' });
			return true;
		})()`, amount)
	}
	if container == "" {
		return fmt.Sprintf(`window.scrollBy(0, %d);`, amount)
	}
	// A more robust selection: find the element, and if it's not scrollable, look for a scrollable child.
	return fmt.Sprintf(`
		(() => {
			var el = %s;
			if (!el) return;

			const isScrollable = (node) => {
				const cs = window.getComputedStyle(node);
				const hasOverflow = (cs.overflowY === 'auto' || cs.overflowY === 'scroll');
				return hasOverflow && node.scrollHeight > node.clientHeight;
			};

			// Find the scrollable element (itself or deeply nested)
			var target = el;
			if (!isScrollable(el)) {
				var all = el.querySelectorAll('*');
				for(var i=0; i<all.length; i++) {
					if (isScrollable(all[i])) {
						target = all[i];
						// Don't break! Find the MOST deeply nested scrollable child (usually the one people mean)
					}
				}
			}
			target.scrollBy({ top: %d, behavior: 'auto' });
		})()
	`, locatorExpression(container), amount)
}

// SelectOption picks an option of a native <select> by its label or its value,
// and reports what it did.
//
// Two things it does that a plain `option.text === want` does not.
//
// It compares normalised — whitespace collapsed, case folded. The label a
// caller writes is read off the page by a human or a model, not copied out of
// the DOM, so "від дешевих до дорогих" and "Від дешевих до дорогих" are the
// same option to everyone except `===`.
//
// And its completion value says whether an option actually matched, listing
// the ones that exist when none did. A select that quietly changes nothing is
// indistinguishable from success at the protocol level, which is how a wrong
// label reaches the user as a green step over an unsorted page.
func SelectOption(xpath, value string) string {
	return fmt.Sprintf(`(() => {
		const norm = s => (s || "").replace(/\s+/g, " ").trim();
		const fold = s => norm(s).toLowerCase();
		const el = document.evaluate(%[1]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el || !el.options) {
			return JSON.stringify({ matched: false, options: [] });
		}
		const options = [...el.options].map(o => norm(o.text));
		const want = fold(%[2]q);
		let hit = null;
		for (const o of el.options) {
			if (fold(o.text) === want || fold(o.value) === want) { hit = o; break; }
		}
		if (!hit) {
			return JSON.stringify({ matched: false, options: options });
		}
		// Through the native setter: frameworks keep their own record of the
		// last value they wrote, and a direct assignment leaves it untouched,
		// so the change event that follows is discarded as a no-op.
		const desc = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), "value");
		if (desc && desc.set) { desc.set.call(el, hit.value); } else { el.value = hit.value; }
		el.dispatchEvent(new Event("input", { bubbles: true }));
		el.dispatchEvent(new Event("change", { bubbles: true }));
		return JSON.stringify({ matched: true, value: hit.value, options: options });
	})()`, xpath, value)
}

// FileInput resolves the file input associated with the element at ID or XPath
// and returns it as the completion value (null when there is none). Backends
// hand the resulting node to their own file-setting command.
func FileInput(id int, xpath string) string {
	return fmt.Sprintf(`(() => {
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el) return null;

		var targetEl = el;
		if (el.tagName !== 'INPUT' || el.type !== 'file') {
			if (el.tagName === 'LABEL') {
				if (el.control && el.control.tagName === 'INPUT' && el.control.type === 'file') {
					targetEl = el.control;
				} else if (el.htmlFor) {
					targetEl = document.getElementById(el.htmlFor) || targetEl;
				}
			}
			if ((targetEl.tagName !== 'INPUT' || targetEl.type !== 'file') && el.querySelector) {
				var child = el.querySelector('input[type=file]');
				if (child) targetEl = child;
			}
			if ((targetEl.tagName !== 'INPUT' || targetEl.type !== 'file') && el.nextElementSibling) {
				var next = el.nextElementSibling.matches('input[type=file]')
					? el.nextElementSibling
					: el.nextElementSibling.querySelector && el.nextElementSibling.querySelector('input[type=file]');
				if (next) targetEl = next;
			}
			if ((targetEl.tagName !== 'INPUT' || targetEl.type !== 'file') && el.parentElement) {
				var nearby = el.parentElement.querySelector && el.parentElement.querySelector('input[type=file]');
				if (nearby) targetEl = nearby;
			}
		}

		if (!targetEl || targetEl.tagName !== 'INPUT' || targetEl.type !== 'file') {
			return null;
		}
		return targetEl;
	})()`, id, xpath)
}

// Highlight injects a temporary border highlight for debugging.
// 4px solid red border + #ffeb3b background.
func Highlight(id int, xpath string, durationMS int) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%d]) || document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) {
			el.setAttribute('data-manul-flash-old-border', el.style.border || '');
			el.setAttribute('data-manul-flash-old-bg', el.style.backgroundColor || '');
			el.style.border = '4px solid red';
			el.style.backgroundColor = '#ffeb3b';
			setTimeout(() => {
				var oldBorder = el.getAttribute('data-manul-flash-old-border');
				if (oldBorder !== null) {
					el.style.border = oldBorder;
					el.removeAttribute('data-manul-flash-old-border');
				}
				var oldBg = el.getAttribute('data-manul-flash-old-bg');
				if (oldBg !== null) {
					el.style.backgroundColor = oldBg;
					el.removeAttribute('data-manul-flash-old-bg');
				}
			}, %d);
		}
	`, id, xpath, durationMS)
}

// ClearHighlight immediately removes any active highlight from the page.
// It restores original inline styles for flash highlights and removes debug
// highlight attributes.
func ClearHighlight() string {
	return `(() => {
		document.querySelectorAll('[data-manul-debug-highlight]').forEach(
			el => el.removeAttribute('data-manul-debug-highlight')
		);
		const s = document.getElementById('manul-debug-style');
		if (s) s.remove();
		document.querySelectorAll('[data-manul-flash-old-border]').forEach(el => {
			el.style.border = el.getAttribute('data-manul-flash-old-border') || '';
			el.removeAttribute('data-manul-flash-old-border');
		});
		document.querySelectorAll('[data-manul-flash-old-bg]').forEach(el => {
			el.style.backgroundColor = el.getAttribute('data-manul-flash-old-bg') || '';
			el.removeAttribute('data-manul-flash-old-bg');
		});
	})();`
}

// ElementCenter scrolls the element at ID or XPath into view and returns its
// viewport centre as a JSON string ({"x":…,"y":…}) completion value.
func ElementCenter(id int, xpath string) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%d]) || document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el) {
			throw new Error("Element not found");
		}
		el.scrollIntoView({behavior: 'instant', block: 'center', inline: 'center'});
		var rect = el.getBoundingClientRect();
		// If it's still outside, we might need a small delay, but instant scroll usually is synchronous.
		JSON.stringify({x: rect.x + rect.width/2, y: rect.y + rect.height/2});
	`, id, xpath)
}

// CoveredBy reports whether a click at viewport point (x, y) would reach the
// element at ID or XPath. Its completion value is a JSON string:
// {"covered":false}, or {"covered":true,"by":"<div.modal>"} naming what is
// on top.
//
// A click is dispatched at coordinates, so it goes to whatever is there. When
// that is an open date picker, a modal backdrop or a cookie banner, the step
// used to pass having clicked the wrong thing. The element itself, anything
// inside it, anything it is inside, and the <label> it belongs to all count
// as reaching it — a styled checkbox sits under its own label's artwork by
// design.
func CoveredBy(id int, xpath string, x, y float64) string {
	return fmt.Sprintf(`(() => {
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		var clear = JSON.stringify({covered: false});
		if (!el) return clear;
		var hit = document.elementFromPoint(%[3]f, %[4]f);
		while (hit && hit.shadowRoot) {
			var inner = hit.shadowRoot.elementFromPoint(%[3]f, %[4]f);
			if (!inner || inner === hit) break;
			hit = inner;
		}
		if (!hit) return clear;
		// Up through shadow hosts as well as parents.
		var within = function(outer, node) {
			for (var n = node; n; n = n.parentNode || n.host) {
				if (n === outer) return true;
			}
			return false;
		};
		if (within(el, hit) || within(hit, el)) return clear;
		var label = hit.closest('label');
		if (label && (label.control === el || within(label, el))) return clear;
		var name = hit.tagName.toLowerCase();
		if (hit.id) name += '#' + hit.id;
		if (typeof hit.className === 'string' && hit.className.trim()) {
			name += '.' + hit.className.trim().split(/\s+/).slice(0, 2).join('.');
		}
		return JSON.stringify({covered: true, by: '<' + name + '>'});
	})()`, id, xpath, x, y)
}

// ElementRect scrolls the element at ID or XPath into view and returns its
// viewport box as a JSON string, with the two numbers needed to turn that box
// into screenshot pixels: the viewport's width in CSS pixels, and the device
// pixel ratio as a fallback.
func ElementRect(id int, xpath string) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%d]) || document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!el) {
			throw new Error("Element not found");
		}
		el.scrollIntoView({behavior: 'instant', block: 'center', inline: 'center'});
		var rect = el.getBoundingClientRect();
		JSON.stringify({
			x: rect.x, y: rect.y, width: rect.width, height: rect.height,
			viewportWidth: window.innerWidth, dpr: window.devicePixelRatio || 1
		});
	`, id, xpath)
}

// CurrentURL is the expression whose value is the page's current URL.
const CurrentURL = "window.location.href"

// ReadyState is the expression whose value is the document's load state.
const ReadyState = "document.readyState"

// ArmPointerWatch counts the pointer events the document receives from now on,
// on window in the capture phase, ahead of any handler of the page's that
// could stop them. PointerReceived then says whether any came.
//
// A click is dispatched at coordinates; whether it arrives is the browser's
// business. In a browser the engine did not launch, the password manager's
// prompt after a sign-in took clicks meant for the page, and the step passed
// with nothing clicked.
const ArmPointerWatch = `(() => {
	if (!window.__manulPointerHooked) {
		window.__manulPointerHooked = true;
		['pointerdown', 'mousedown', 'mouseup', 'click', 'dblclick', 'contextmenu'].forEach(function (t) {
			window.addEventListener(t, function () { if (window.__manulPointer) window.__manulPointer.n++; }, true);
		});
	}
	window.__manulPointer = {n: 0};
	return true;
})()`

// PointerReceived is 1 when the document received a pointer event since
// ArmPointerWatch, 0 when it received none, and 2 when it is another document
// — the action navigated, which is an answer of its own.
const PointerReceived = `(window.__manulPointer ? (window.__manulPointer.n > 0 ? 1 : 0) : 2)`

// RequestWatchFunction installs, once per document, what RouteState reads: a
// count of the fetch and XMLHttpRequest requests an action started that have
// not come back, and the time the document last changed.
//
// A single-page app changes its address without a new document and draws what
// the address shows when its own request comes back — and Rozetka's search
// asked the server first and changed the address only when it answered, so
// for as long as the request took nothing said a step was not done. A request
// counts when it starts while the action's window is open (from
// ArmNavigationWatch until CloseActionWindow) or after the address has moved:
// a page that polls in the background is not the action's business.
//
// It is a preload script on every page the engine opens, run before the
// page's own: an app that keeps the fetch it found at start-up, as ofetch
// does, never calls one replaced later. ArmNavigationWatch installs it too,
// for a document that was already there.
const RequestWatchFunction = `function () {
	if (window.__manulNet) return;
	const net = window.__manulNet = {cur: {n: 0, open: false, href: ""}, last: 0};
	const touch = () => { net.last = performance.now(); };
	const counted = () => {
		const a = net.cur;
		return (a.open || (a.href !== "" && location.href !== a.href)) ? a : null;
	};
	try {
		new MutationObserver(touch).observe(document, {subtree: true, childList: true, characterData: true});
	} catch (e) {}
	const fetch0 = window.fetch;
	if (typeof fetch0 === "function") {
		try {
			window.fetch = function () {
				const p = fetch0.apply(this, arguments);
				const a = counted();
				if (a) {
					a.n++;
					const done = () => { a.n--; touch(); };
					p.then(done, done);
				}
				return p;
			};
		} catch (e) {}
	}
	const X = window.XMLHttpRequest;
	if (X && X.prototype && typeof X.prototype.send === "function") {
		const send0 = X.prototype.send;
		try {
			X.prototype.send = function () {
				const a = counted();
				let open = !!a;
				if (a) a.n++;
				const done = () => { if (open) { open = false; a.n--; } touch(); };
				this.addEventListener("loadend", done);
				try { return send0.apply(this, arguments); } catch (e) { done(); throw e; }
			};
		} catch (e) {}
	}
}`

// ArmNavigationWatch marks the current document before an action, so that
// NavigationState can tell afterwards whether the action is replacing it.
//
// readyState alone cannot: the old document stays "complete" until the new
// one's response arrives, and on a slow server the next step ran on the page
// that was about to go away. beforeunload fires as the navigation starts,
// before any request is sent.
//
// It also notes the address and opens the action's window for
// RequestWatchFunction's count, installing the count when the document has
// none yet.
const ArmNavigationWatch = `(() => {
	window.__manulStay = true;
	window.__manulLeaving = false;
	if (!window.__manulWatching) {
		window.__manulWatching = true;
		window.addEventListener("beforeunload", () => { window.__manulLeaving = true; });
	}
	window.__manulHref = location.href;
	(` + RequestWatchFunction + `)();
	window.__manulNet.cur = {n: 0, open: true, href: location.href};
	window.__manulNet.last = performance.now();
	return 1;
})()`

// CloseActionWindow ends the time in which a request the page starts counts
// as the action's, unless the address moves. It is called once the action
// has been given its moment to start what it starts.
const CloseActionWindow = `(window.__manulNet ? (window.__manulNet.cur.open = false, 1) : 0)`

// RouteState says, as JSON, whether the armed document's address has changed
// (moved), how many of the requests RequestWatchFunction counted for the
// action are still in flight (pending), and for how many milliseconds neither
// its DOM nor those requests have changed (idle). A document that was never armed has not
// moved. JSON in a string, because EvalJS hands strings back unquoted.
const RouteState = `(() => {
	const net = window.__manulNet;
	if (!net || window.__manulHref === undefined) return JSON.stringify({moved: false, pending: 0, idle: 0});
	return JSON.stringify({moved: location.href !== window.__manulHref, pending: net.cur.n, idle: Math.round(performance.now() - net.last)});
})()`

// NavigationState is 0 when the armed document is still here and nothing is
// leaving it, 1 while it is being navigated away from, and 2 once another
// document has replaced it. It is a number because EvalJS hands strings back
// unquoted.
const NavigationState = `(window.__manulStay !== true ? 2 : (window.__manulLeaving === true ? 1 : 0))`

func looksLikeXPath(locator string) bool {
	locator = strings.TrimSpace(locator)
	if locator == "" {
		return false
	}
	lower := strings.ToLower(locator)
	if strings.HasPrefix(lower, "xpath=") {
		return true
	}
	return strings.HasPrefix(locator, "/") ||
		strings.HasPrefix(locator, "//") ||
		strings.HasPrefix(locator, "./") ||
		strings.HasPrefix(locator, "../") ||
		strings.HasPrefix(locator, "(")
}

// locatorExpression turns an XPath or CSS locator into the JS expression that
// resolves it to a node.
func locatorExpression(locator string) string {
	trimmed := strings.TrimSpace(locator)
	if strings.HasPrefix(strings.ToLower(trimmed), "xpath=") {
		trimmed = strings.TrimSpace(trimmed[6:])
	}
	if looksLikeXPath(trimmed) {
		return fmt.Sprintf("document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue", trimmed)
	}
	return fmt.Sprintf("document.querySelector(%q)", trimmed)
}

// FrameOffset, evaluated inside a frame, returns where that frame's viewport
// sits in the top-level one: {"ok":true,"x":…,"y":…}, adding up every
// <iframe> between them — its box, border and padding — as long as each
// parent is a document this one may look at. {"ok":false} when one is not
// (a cross-origin parent hides window.frameElement); IframeOffsetByURL asks
// the top document instead.
const FrameOffset = `(() => {
	var x = 0, y = 0, w = window;
	try {
		while (w !== w.top) {
			var fe = w.frameElement;
			if (!fe) return JSON.stringify({ok: false});
			var r = fe.getBoundingClientRect();
			var cs = w.parent.getComputedStyle(fe);
			x += r.left + fe.clientLeft + (parseFloat(cs.paddingLeft) || 0);
			y += r.top + fe.clientTop + (parseFloat(cs.paddingTop) || 0);
			w = w.parent;
		}
	} catch (_) {
		return JSON.stringify({ok: false});
	}
	return JSON.stringify({ok: true, x: x, y: y});
})()`

// frameBoxBody measures an <iframe> element `f` from the document holding it:
// where its content box starts in that document's viewport, and its size.
const frameBoxBody = `var r = f.getBoundingClientRect(), cs = getComputedStyle(f);
		var pl = parseFloat(cs.paddingLeft) || 0, pt = parseFloat(cs.paddingTop) || 0;
		var pr = parseFloat(cs.paddingRight) || 0, pb = parseFloat(cs.paddingBottom) || 0;
		var shown = cs.display !== 'none' && cs.visibility !== 'hidden';
		return JSON.stringify({x: r.left + f.clientLeft + pl, y: r.top + f.clientTop + pt,
			w: shown ? Math.max(0, f.clientWidth - pl - pr) : 0, h: shown ? Math.max(0, f.clientHeight - pt - pb) : 0});`

// allFrames lists every <iframe> and <frame> of the document, open shadow
// roots included: a web component that embeds a frame keeps it in its shadow
// root, where querySelectorAll does not look. MDN's live examples are five
// cross-site frames, none of them in the light DOM.
const allFrames = `function () {
		var out = [];
		(function walk(root) {
			root.querySelectorAll('*').forEach(function (e) {
				if (/^I?FRAME$/.test(e.tagName)) out.push(e);
				if (e.shadowRoot) walk(e.shadowRoot);
			});
		})(document);
		return out;
	}`

// FrameWindows lists the window of every frame of the document, in the order
// allFrames finds them. Over WebDriver BiDi a window serializes to its
// browsing-context id, even a cross-origin one, which is what ties a frame's
// context to the element that holds it; FrameBox(i) then measures the i-th.
const FrameWindows = `(` + allFrames + `)().map(function (f) { return f.contentWindow; })`

// FrameBoxes measures every frame of the document in one evaluation: for
// each, [its window, x, y, w, h] as FrameBox gives them. Placing every frame
// of a page one at a time cost several round trips each, and a page with ten
// ad frames spent seconds on it in every step.
const FrameBoxes = `(` + allFrames + `)().map(function (f) {
	var b = JSON.parse((function () { ` + frameBoxBody + ` })());
	return [f.contentWindow, b.x, b.y, b.w, b.h];
})`

// FrameBox measures the i-th <iframe> or <frame> of the document: where its
// content box starts in this document's viewport, and its width and height
// (0 for one not displayed). A cross-origin frame cannot measure itself —
// window.frameElement is null there — so it is measured from outside.
func FrameBox(i int) string {
	return fmt.Sprintf(`(() => {
		var f = (%s)()[%d];
		if (!f) return JSON.stringify(null);
		%s
	})()`, allFrames, i, frameBoxBody)
}

// FrameBoxOn is FrameBox for an element handed over as this — Chrome's
// Runtime.callFunctionOn, on the node DOM.getFrameOwner names.
const FrameBoxOn = `function () { var f = this; ` + frameBoxBody + ` }`

// CrossFrameAt reports the cross-origin frame at viewport point (x, y), if
// that is what is there: [its window, where its content box starts]. A
// same-origin frame answers null — a click dispatched at this document
// reaches into it — and so does anything else.
func CrossFrameAt(x, y float64) string { return frameAt(x, y, false) }

// FrameAt is CrossFrameAt for any frame, same-origin ones too: what a drag
// needs to find the element it starts on, which elementFromPoint at the top
// reports as the <iframe> itself.
func FrameAt(x, y float64) string { return frameAt(x, y, true) }

func frameAt(x, y float64, anyOrigin bool) string {
	return fmt.Sprintf(`(() => {
		var f = document.elementFromPoint(%[1]g, %[2]g);
		// Into open shadow roots: from outside, the point is over the host.
		while (f && f.shadowRoot) {
			var d = f.shadowRoot.elementFromPoint(%[1]g, %[2]g);
			if (!d || d === f) break;
			f = d;
		}
		if (!f || !/^I?FRAME$/.test(f.tagName)) return null;
		var same = false;
		try { same = !!f.contentDocument; } catch (_) {}
		if (same && !%[4]t) return null;
		var b = JSON.parse((function () { %[3]s })());
		return [f.contentWindow, b.x, b.y];
	})()`, x, y, frameBoxBody, anyOrigin)
}

// CrossFrameFocused answers the window of the cross-origin frame that holds
// the focus, or null: a key dispatched at this document goes no further.
const CrossFrameFocused = `(() => {
	var f = document.activeElement;
	while (f && f.shadowRoot && f.shadowRoot.activeElement) f = f.shadowRoot.activeElement;
	if (!f || !/^I?FRAME$/.test(f.tagName)) return null;
	try { if (f.contentDocument) return null; } catch (_) {}
	return [f.contentWindow];
})()`

// SeedFrameIDs starts a frame's element-ID counter far from every other
// frame's, before its first snapshot. Each window numbers its own elements
// from zero, so element 7 of an iframe and element 7 of the page were the
// same ID; a random base a million apart keeps them distinct while every
// lookup still happens in the frame's own registry.
const SeedFrameIDs = `(() => {
	if (window.__manulIdCounter === undefined) {
		window.__manulIdCounter = (1 + Math.floor(Math.random() * 2000)) * 1000000;
	}
	return true;
})()`

// HTML5DragAt performs an HTML5 drag and drop from viewport point (x1, y1) to
// (x2, y2) by dispatching the drag events themselves, when what is at the
// first point is, or is inside, an element with draggable="true". Its
// completion value is "1" when it did, "0" when there is no such element and
// the drag is left to the pointer.
//
// Firefox starts no drag session from WebDriver pointer actions, so on a page
// built on draggable="true" the pointer drag moved the mouse, fired nothing
// the page listens for, and the step passed with both columns where they
// were. The events follow the order a browser fires them in — dragstart and
// drag on the source, dragenter and dragover on the target, drop only if
// dragover was cancelled (a target that does not cancel it refuses the drop),
// dragend on the source — sharing one DataTransfer, so what dragstart stores
// is what drop reads.
func HTML5DragAt(x1, y1, x2, y2 float64) string {
	return fmt.Sprintf(`(() => {
		var at = document.elementFromPoint(%[1]f, %[2]f);
		var src = at && at.closest('[draggable="true"]');
		if (!src) return "0";
		var dst = document.elementFromPoint(%[3]f, %[4]f) || document.body;
		var dt = new DataTransfer();
		var fire = function(el, type, x, y) {
			var ev = new DragEvent(type, {bubbles: true, cancelable: true, composed: true,
				clientX: x, clientY: y, dataTransfer: dt});
			el.dispatchEvent(ev);
			return ev;
		};
		if (fire(src, 'dragstart', %[1]f, %[2]f).defaultPrevented) return "1";
		fire(src, 'drag', %[1]f, %[2]f);
		fire(dst, 'dragenter', %[3]f, %[4]f);
		if (fire(dst, 'dragover', %[3]f, %[4]f).defaultPrevented) {
			fire(dst, 'drop', %[3]f, %[4]f);
		} else {
			fire(dst, 'dragleave', %[3]f, %[4]f);
		}
		fire(src, 'dragend', %[3]f, %[4]f);
		return "1";
	})()`, x1, y1, x2, y2)
}
