package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// Framed is a Page that stands for one iframe of another: what it is asked
// about an element it answers from inside the frame, and the coordinates it
// returns are the top-level viewport's, ready for Click and the other mouse
// methods, which act on the whole page.
type Framed interface {
	Page
	// FrameOffset is where the frame's viewport sits in the top-level one,
	// measured now: scrolling the page moves it.
	FrameOffset(ctx context.Context) (dx, dy float64, err error)
}

// InFrame returns the view of p that addresses frame frameIndex, as numbered
// by p.Frames. Frame 0 is the page itself.
func InFrame(p Page, frameIndex int) Page {
	if frameIndex == 0 {
		return p
	}
	return &framedPage{Page: p, frame: frameIndex}
}

// FrameOffset measures where frame frameIndex of p sits in the top-level
// viewport, from inside the frame. It fails for a frame with a cross-origin
// document between it and the top, which hides window.frameElement: such a
// frame is not entered at all (see the DSL contract), because a click Firefox
// dispatches from the top level never reaches it — the step passed with
// nothing clicked — and Chromium runs it in a process of its own.
func FrameOffset(ctx context.Context, p Page, frameIndex int) (float64, float64, error) {
	if frameIndex == 0 {
		return 0, 0, nil
	}
	raw, err := p.EvalJSInFrame(ctx, frameIndex, pagejs.FrameOffset)
	if err != nil {
		return 0, 0, err
	}
	var o struct {
		OK   bool
		X, Y float64
	}
	if json.Unmarshal(raw, &o) != nil || !o.OK {
		return 0, 0, fmt.Errorf("frame %d is cross-origin to the page; it is not entered", frameIndex)
	}
	return o.X, o.Y, nil
}

type framedPage struct {
	Page
	frame int
}

func (f *framedPage) FrameOffset(ctx context.Context) (float64, float64, error) {
	return FrameOffset(ctx, f.Page, f.frame)
}

func (f *framedPage) EvalJS(ctx context.Context, expr string) ([]byte, error) {
	return f.Page.EvalJSInFrame(ctx, f.frame, expr)
}

func (f *framedPage) CallProbe(ctx context.Context, fn string, arg any) ([]byte, error) {
	return f.Page.CallProbeInFrame(ctx, f.frame, fn, arg)
}

func (f *framedPage) Focus(ctx context.Context, id int, xpath string) error {
	_, err := f.EvalJS(ctx, pagejs.Focus(id, xpath))
	return err
}

func (f *framedPage) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	_, err := f.EvalJS(ctx, pagejs.SetInputValue(id, xpath, value))
	return err
}

func (f *framedPage) SetChecked(ctx context.Context, id int, xpath string, checked bool) error {
	_, err := f.EvalJS(ctx, pagejs.SetChecked(id, xpath, checked))
	return err
}

func (f *framedPage) ScrollIntoView(ctx context.Context, id int, xpath string) error {
	_, err := f.EvalJS(ctx, pagejs.ScrollIntoView(id, xpath))
	return err
}

func (f *framedPage) HighlightElement(ctx context.Context, id int, xpath string, durationMS int) error {
	_, err := f.EvalJS(ctx, pagejs.Highlight(id, xpath, durationMS))
	return err
}

func (f *framedPage) ClearHighlight(ctx context.Context) error {
	_, err := f.EvalJS(ctx, pagejs.ClearHighlight())
	return err
}

// SetFileInput is not routed into frames: both backends hand the browser a
// node reference taken from the main document's evaluation.
func (f *framedPage) SetFileInput(ctx context.Context, id int, xpath string, filePaths []string) error {
	return fmt.Errorf("UPLOAD into a field inside an <iframe> is not supported")
}

// GetElementCenter scrolls the element into view — which scrolls the page
// holding its frame too — and only then measures the frame, so the two
// numbers describe the same moment.
func (f *framedPage) GetElementCenter(ctx context.Context, id int, xpath string) (float64, float64, error) {
	raw, err := f.EvalJS(ctx, pagejs.ElementCenter(id, xpath))
	if err != nil {
		return 0, 0, err
	}
	var c struct{ X, Y float64 }
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0, 0, fmt.Errorf("element centre: %w (%s)", err, strings.TrimSpace(string(raw)))
	}
	dx, dy, err := f.FrameOffset(ctx)
	if err != nil {
		return 0, 0, err
	}
	return c.X + dx, c.Y + dy, nil
}

func (f *framedPage) GetDragCenters(ctx context.Context, srcID int, srcXPath string, dstID int, dstXPath string) (float64, float64, float64, float64, error) {
	raw, err := f.EvalJS(ctx, pagejs.DragCenters(srcID, srcXPath, dstID, dstXPath))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	var c struct{ X1, Y1, X2, Y2 float64 }
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("drag centres: %w", err)
	}
	dx, dy, err := f.FrameOffset(ctx)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return c.X1 + dx, c.Y1 + dy, c.X2 + dx, c.Y2 + dy, nil
}
