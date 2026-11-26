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

// FrameLocator is implemented by a backend that can measure a frame from the
// document that holds it, which is the only way to place a cross-origin one:
// its own window.frameElement is null.
type FrameLocator interface {
	// FrameOrigin is where frame frameIndex's content box starts in the
	// top-level viewport, and its size. A frame that is not displayed has
	// size 0.
	FrameOrigin(ctx context.Context, frameIndex int) (x, y, w, h float64, err error)
}

// Placement is where a frame's content box starts in the top-level viewport,
// and its size.
type Placement struct{ X, Y, W, H float64 }

// FramePlacer is implemented by a backend that can place every frame of the
// page at once, which costs a round trip or two where measuring frame by
// frame cost several per frame.
type FramePlacer interface {
	// PlaceFrames places every frame it can, by frame index. A frame it
	// could not measure is missing from the map.
	PlaceFrames(ctx context.Context) (map[int]Placement, error)
}

// FrameOffset measures where frame frameIndex of p sits in the top-level
// viewport. A same-origin frame measures itself through frameElement, in one
// evaluation. A cross-origin one is measured from outside when the backend
// can (FrameLocator); a frame with no size — a tracking pixel, a hidden
// widget — is reported as an error, since there is nothing in it to act on.
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
	if json.Unmarshal(raw, &o) == nil && o.OK {
		return o.X, o.Y, nil
	}
	loc, ok := p.(FrameLocator)
	if !ok {
		return 0, 0, fmt.Errorf("frame %d is cross-origin to the page and cannot be placed", frameIndex)
	}
	x, y, w, h, err := loc.FrameOrigin(ctx, frameIndex)
	if err != nil {
		return 0, 0, fmt.Errorf("frame %d: %w", frameIndex, err)
	}
	if w*h < 4 {
		return 0, 0, fmt.Errorf("frame %d is not displayed", frameIndex)
	}
	return x, y, nil
}

// frameBox decodes pagejs.FrameBox.
func frameBox(raw []byte) (x, y, w, h float64, err error) {
	var b *struct{ X, Y, W, H float64 }
	if err := json.Unmarshal(raw, &b); err != nil || b == nil {
		return 0, 0, 0, 0, fmt.Errorf("frame element not found (%s)", strings.TrimSpace(string(raw)))
	}
	return b.X, b.Y, b.W, b.H, nil
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

// frameFiles is a backend that can set the files of a field inside a frame.
// Both hand the browser a node reference, and a reference belongs to the
// execution context it was taken from — the main document's is no use for a
// field in a frame.
type frameFiles interface {
	setFileInputInFrame(ctx context.Context, frameIndex, id int, xpath string, filePaths []string) error
}

func (f *framedPage) SetFileInput(ctx context.Context, id int, xpath string, filePaths []string) error {
	if ff, ok := f.Page.(frameFiles); ok {
		return ff.setFileInputInFrame(ctx, f.frame, id, xpath, filePaths)
	}
	return fmt.Errorf("UPLOAD into a field inside an <iframe> is not supported by this backend")
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
