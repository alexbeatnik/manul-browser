package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// MockPage is a test-only browser.Page implementation that operates on
// pre-defined DOM snapshots instead of a real browser.
type MockPage struct {
	URL      string
	Title    string
	Elements []dom.ElementSnapshot

	// Record of interaction calls
	Clicks       []Point
	Inputs       map[string]string // xpath -> value
	FileInputs   map[string][]string
	Mocks        []browser.MockRule
	LastNavigate string
	ProbeCalls   int
	EvalCalls    []string

	// EvalResult lets a test answer an injected script the way a real page
	// would. Returning handled=false falls through to the default behaviour.
	// A command that acts through JS — SELECT is the one that does — can only
	// be tested for what it does with the page's answer if the test can give
	// it one.
	EvalResult func(expr string) (raw []byte, handled bool)

	// FrameOffsets is where each iframe's viewport sits on the page's, by
	// frame index. Element rects in Elements are on the page's viewport.
	FrameOffsets map[int][2]float64
	// Frames lists the iframes this page has, by index (0, the page, is
	// always there): their snapshot elements, on the frame's own viewport,
	// and their visible text. A CrossOrigin frame reports no offset.
	FrameDocs map[int]MockFrame
}

// MockFrame is one iframe of a MockPage.
type MockFrame struct {
	Elements    []dom.ElementSnapshot
	Text        string
	CrossOrigin bool
}

type Point struct {
	X, Y float64
}

func (m *MockPage) Navigate(ctx context.Context, url string) error {
	m.URL = url
	m.LastNavigate = url
	return nil
}

func (m *MockPage) EvalJS(ctx context.Context, expr string) ([]byte, error) {
	m.EvalCalls = append(m.EvalCalls, expr)
	if m.EvalResult != nil {
		if raw, handled := m.EvalResult(expr); handled {
			return raw, nil
		}
	}
	if strings.Contains(expr, "document.title") {
		return json.Marshal(m.Title)
	}
	if strings.Contains(expr, "matched:") {
		// The SELECT script, which reports whether an option matched. The
		// mock page is one that has the option asked for; a test that needs
		// a miss says so through EvalResult.
		return []byte(`{"matched":true}`), nil
	}
	return nil, nil
}

func (m *MockPage) CallProbe(ctx context.Context, fn string, arg any) ([]byte, error) {
	m.ProbeCalls++
	// If it's a data extraction probe, we simulate the logic:
	// Find element matching the target text from arg and return its value/text.
	if strings.Contains(fn, "classified") && strings.Contains(fn, "allTables") { // Simple detection for extract_data.js
		params, _ := arg.([]string)
		if len(params) > 0 {
			target := strings.ToLower(params[0])
			for _, el := range m.Elements {
				if strings.Contains(strings.ToLower(el.VisibleText), target) || strings.ToLower(el.Tag) == target {
					if el.Value != "" {
						return []byte(el.Value), nil
					}
					return []byte(el.VisibleText), nil
				}
			}
		}
		return []byte("null"), nil
	}

	// Default: return the all-elements snapshot
	res := dom.PageSnapshot{
		URL:         m.URL,
		VisibleText: "Mock Page Content",
		Elements:    m.Elements,
	}
	return json.Marshal(res)
}

func (m *MockPage) Click(ctx context.Context, x, y float64) error {
	m.Clicks = append(m.Clicks, Point{x, y})
	return nil
}

func (m *MockPage) Focus(ctx context.Context, id int, xpath string) error { return nil }

func (m *MockPage) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	if m.Inputs == nil {
		m.Inputs = make(map[string]string)
	}
	m.Inputs[xpath] = value
	return nil
}

func (m *MockPage) SetChecked(ctx context.Context, id int, xpath string, checked bool) error {
	for i := range m.Elements {
		if m.Elements[i].XPath == xpath || m.Elements[i].ID == id {
			m.Elements[i].IsChecked = checked
			return nil
		}
	}
	return nil
}

func (m *MockPage) ScrollIntoView(ctx context.Context, id int, xpath string) error { return nil }

func (m *MockPage) ScrollPage(ctx context.Context, direction, container string) error { return nil }

func (m *MockPage) DoubleClick(ctx context.Context, x, y float64) error { return nil }

func (m *MockPage) RightClick(ctx context.Context, x, y float64) error { return nil }

func (m *MockPage) Hover(ctx context.Context, x, y float64) error { return nil }

func (m *MockPage) DragAndDrop(ctx context.Context, fX, fY, tX, tY float64) error { return nil }

func (m *MockPage) GetDragCenters(ctx context.Context, srcID int, srcXPath string, dstID int, dstXPath string) (float64, float64, float64, float64, error) {
	return 0, 0, 0, 0, nil
}

func (m *MockPage) Mock(ctx context.Context, rule browser.MockRule) error {
	m.Mocks = append(m.Mocks, rule)
	return nil
}

func (m *MockPage) SetFileInput(ctx context.Context, id int, xpath string, paths []string) error {
	if m.FileInputs == nil {
		m.FileInputs = make(map[string][]string)
	}
	copyPaths := append([]string(nil), paths...)
	m.FileInputs[xpath] = copyPaths
	return nil
}

func (m *MockPage) Screenshot(ctx context.Context) ([]byte, error) { return nil, nil }

func (m *MockPage) WaitForResponse(ctx context.Context, pattern string, timeout time.Duration) error {
	return nil
}

func (m *MockPage) HighlightElement(ctx context.Context, id int, xpath string, duration int) error {
	return nil
}

func (m *MockPage) ClearHighlight(ctx context.Context) error {
	return nil
}

func (m *MockPage) GetElementCenter(ctx context.Context, id int, xpath string) (float64, float64, error) {
	for _, el := range m.Elements {
		if el.XPath == xpath || el.ID == id {
			return el.Rect.Left + el.Rect.Width/2, el.Rect.Top + el.Rect.Height/2, nil
		}
	}
	return 0, 0, fmt.Errorf("element not found: %s (ID=%d)", xpath, id)
}

func (m *MockPage) DispatchKey(ctx context.Context, key string, mods int) error { return nil }

func (m *MockPage) CurrentURL(ctx context.Context) (string, error) { return m.URL, nil }

func (m *MockPage) WaitForLoad(ctx context.Context) error { return nil }

func (m *MockPage) Wait(ctx context.Context, d time.Duration) error { return nil }

// Frames reports a single main frame (the mock has no real iframes).
func (m *MockPage) Frames(ctx context.Context) ([]browser.FrameRef, error) {
	out := []browser.FrameRef{{Index: 0}}
	for i := 1; i <= len(m.FrameDocs); i++ {
		if _, ok := m.FrameDocs[i]; ok {
			out = append(out, browser.FrameRef{Index: i, URL: fmt.Sprintf("http://frame%d.test/", i)})
		}
	}
	return out, nil
}

// EvalJSInFrame answers, for an element whose FrameIndex is not 0, the two
// questions a frame is asked when it is acted in: where it sits on the page
// (FrameOffsets, zero by default) and where the element's centre is on the
// frame's own viewport. Everything else goes to EvalJS.
func (m *MockPage) EvalJSInFrame(ctx context.Context, frameIndex int, expr string) ([]byte, error) {
	if frameIndex == 0 {
		return m.EvalJS(ctx, expr)
	}
	if m.EvalResult != nil {
		if raw, handled := m.EvalResult(expr); handled {
			m.EvalCalls = append(m.EvalCalls, expr)
			return raw, nil
		}
	}
	off := m.FrameOffsets[frameIndex]
	if expr == pagejs.FrameOffset {
		if m.FrameDocs[frameIndex].CrossOrigin {
			return []byte(`{"ok":false}`), nil
		}
		return json.Marshal(map[string]any{"ok": true, "x": off[0], "y": off[1]})
	}
	if strings.Contains(expr, "JSON.stringify({x: rect.x") {
		// A FrameDocs element is on its frame's viewport already.
		for _, el := range m.FrameDocs[frameIndex].Elements {
			if strings.Contains(expr, fmt.Sprintf("__manulReg[%d]", el.ID)) {
				return json.Marshal(map[string]float64{"x": el.Rect.Left + el.Rect.Width/2, "y": el.Rect.Top + el.Rect.Height/2})
			}
		}
		for _, el := range m.Elements {
			if el.FrameIndex == frameIndex && strings.Contains(expr, fmt.Sprintf("__manulReg[%d]", el.ID)) {
				return json.Marshal(map[string]float64{
					"x": el.Rect.Left + el.Rect.Width/2 - off[0],
					"y": el.Rect.Top + el.Rect.Height/2 - off[1],
				})
			}
		}
	}
	return m.EvalJS(ctx, expr)
}

// CallProbeInFrame answers a frame of FrameDocs with its own snapshot or
// text, and anything else as CallProbe does.
func (m *MockPage) CallProbeInFrame(ctx context.Context, frameIndex int, fn string, arg any) ([]byte, error) {
	doc, ok := m.FrameDocs[frameIndex]
	if frameIndex == 0 || !ok {
		return m.CallProbe(ctx, fn, arg)
	}
	if strings.Contains(fn, "elements:") { // the snapshot probe
		return json.Marshal(dom.PageSnapshot{Elements: doc.Elements})
	}
	if strings.Contains(fn, "t.toLowerCase()") { // the visible-text probe
		return []byte(strings.ToLower(doc.Text)), nil
	}
	return nil, nil
}

func (m *MockPage) Close() error { return nil }
