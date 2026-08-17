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
		}
	`, id, xpath, value)
}

// SetChecked sets the checked state of a checkbox, radio or ARIA-checkable
// element resolved by ID or XPath.
func SetChecked(id int, xpath string, checked bool) string {
	return fmt.Sprintf(`
		var el = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (el) {
			var targetEl = el;
			var desiredAria = %[3]v ? 'true' : 'false';
			// Refinement: find checkbox/radio if el is not one
			if (el.tagName !== 'INPUT' || (el.type !== 'checkbox' && el.type !== 'radio')) {
				if (el.tagName === 'LABEL' && el.htmlFor) {
					targetEl = document.getElementById(el.htmlFor) || targetEl;
				} else {
					var child = el.querySelector('input[type=checkbox], input[type=radio]');
					if (child) {
						targetEl = child;
					} else {
						// Look in nearby siblings or parents (common in tables)
						var cell = el.closest('td, th, div');
						if (cell) {
							var cb = cell.querySelector('input[type=checkbox], input[type=radio]') ||
							         cell.parentElement.querySelector('input[type=checkbox], input[type=radio]');
							if (cb) targetEl = cb;
						}
					}
				}
			}
			el = targetEl;

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

// CurrentURL is the expression whose value is the page's current URL.
const CurrentURL = "window.location.href"

// ReadyState is the expression whose value is the document's load state.
const ReadyState = "document.readyState"

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
