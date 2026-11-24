package browser

import (
	"context"
	"fmt"

	"github.com/alexbeatnik/manul-browser/core/pkg/bidi"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// Cross-origin frames over WebDriver BiDi.
//
// Every frame is a browsing context of its own, so evaluating in one needs
// nothing special. Two things do. A cross-origin frame cannot say where it
// is — window.frameElement is null — so it is measured from the document
// that holds it, matched by its window, which serializes to the frame's
// context id. And Firefox does not deliver a pointer action dispatched at the
// top-level context into a cross-site frame: the click lands nowhere and the
// step passes. Sent to the frame's own context, with the frame's own
// coordinates, it arrives. So every pointer action is routed: while the point
// is over a cross-origin frame, the action moves into that frame.

// maxFrameDepth bounds the descent through nested frames.
const maxFrameDepth = 8

type bidiFrameNode struct {
	id     string
	parent int // index into the flat list; -1 for the page itself
}

// frameNodes flattens the page's context tree depth-first, main frame first,
// in the order Frames numbers them, keeping each frame's parent.
func (p *BiDiPage) frameNodes(ctx context.Context) ([]bidiFrameNode, error) {
	tree, err := bidi.GetTree(ctx, p.conn, p.contextID)
	if err != nil {
		return nil, err
	}
	var flat []bidiFrameNode
	var walk func(nodes []bidi.Context, parent int)
	walk = func(nodes []bidi.Context, parent int) {
		for _, n := range nodes {
			flat = append(flat, bidiFrameNode{id: n.ID, parent: parent})
			walk(n.Children, len(flat)-1)
		}
	}
	walk(tree, -1)
	return flat, nil
}

// FrameOrigin implements FrameLocator.
func (p *BiDiPage) FrameOrigin(ctx context.Context, frameIndex int) (x, y, w, h float64, err error) {
	nodes, err := p.frameNodes(ctx)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if frameIndex <= 0 || frameIndex >= len(nodes) {
		return 0, 0, 0, 0, fmt.Errorf("no frame %d (%d frames)", frameIndex, len(nodes))
	}
	first := true
	for k := frameIndex; k > 0; k = nodes[k].parent {
		parent := nodes[nodes[k].parent].id
		bx, by, bw, bh, err := p.childBox(ctx, parent, nodes[k].id)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		x, y = x+bx, y+by
		if first {
			w, h, first = bw, bh, false
		}
	}
	return x, y, w, h, nil
}

// childBox measures, in the parent context, the frame element whose window is
// the child context.
func (p *BiDiPage) childBox(ctx context.Context, parent, child string) (x, y, w, h float64, err error) {
	val, err := bidi.Evaluate(ctx, p.conn, parent, pagejs.FrameWindows)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	windows, _ := val.([]interface{})
	for i, win := range windows {
		if windowContext(win) != child {
			continue
		}
		raw, err := bidi.Evaluate(ctx, p.conn, parent, pagejs.FrameBox(i))
		if err != nil {
			return 0, 0, 0, 0, err
		}
		s, _ := raw.(string)
		return frameBox([]byte(s))
	}
	return 0, 0, 0, 0, fmt.Errorf("the frame's element was not found in its parent document")
}

func windowContext(v interface{}) string {
	m, _ := v.(map[string]interface{})
	id, _ := m["context"].(string)
	return id
}

// routePoint finds the context a pointer action at top-level point (x, y)
// has to be sent to, and the point in that context's viewport: the page
// itself, unless the point is over a cross-origin frame.
func (p *BiDiPage) routePoint(ctx context.Context, x, y float64) (string, float64, float64) {
	contextID := p.contextID
	for depth := 0; depth < maxFrameDepth; depth++ {
		val, err := bidi.Evaluate(ctx, p.conn, contextID, pagejs.CrossFrameAt(x, y))
		if err != nil {
			break
		}
		hit, _ := val.([]interface{})
		if len(hit) != 3 {
			break
		}
		child := windowContext(hit[0])
		ox, okx := hit[1].(float64)
		oy, oky := hit[2].(float64)
		if child == "" || !okx || !oky {
			break
		}
		contextID, x, y = child, x-ox, y-oy
	}
	return contextID, x, y
}

// routeFocus finds the context that holds the focus, for a key: the page
// itself, unless the focus is inside a cross-origin frame.
func (p *BiDiPage) routeFocus(ctx context.Context) string {
	contextID := p.contextID
	for depth := 0; depth < maxFrameDepth; depth++ {
		val, err := bidi.Evaluate(ctx, p.conn, contextID, pagejs.CrossFrameFocused)
		if err != nil {
			break
		}
		hit, _ := val.([]interface{})
		if len(hit) != 1 {
			break
		}
		child := windowContext(hit[0])
		if child == "" {
			break
		}
		contextID = child
	}
	return contextID
}

// PlaceFrames implements FramePlacer: one getTree, then one evaluation in
// each document that holds frames, which returns every frame's window and
// box together.
func (p *BiDiPage) PlaceFrames(ctx context.Context) (map[int]Placement, error) {
	nodes, err := p.frameNodes(ctx)
	if err != nil {
		return nil, err
	}
	local := map[string]Placement{} // child context id -> box in its parent
	asked := map[int]bool{}
	for _, n := range nodes {
		if n.parent < 0 || asked[n.parent] {
			continue
		}
		asked[n.parent] = true
		val, err := bidi.Evaluate(ctx, p.conn, nodes[n.parent].id, pagejs.FrameBoxes)
		if err != nil {
			continue
		}
		rows, _ := val.([]interface{})
		for _, row := range rows {
			r, _ := row.([]interface{})
			if len(r) != 5 {
				continue
			}
			var f [4]float64
			ok := true
			for i := range f {
				if f[i], ok = r[i+1].(float64); !ok {
					break
				}
			}
			if id := windowContext(r[0]); id != "" && ok {
				local[id] = Placement{X: f[0], Y: f[1], W: f[2], H: f[3]}
			}
		}
	}
	out := map[int]Placement{}
	for i := 1; i < len(nodes); i++ {
		box, ok := local[nodes[i].id]
		if !ok {
			continue
		}
		place := box
		for k := nodes[i].parent; k > 0; k = nodes[k].parent {
			up, ok := local[nodes[k].id]
			if !ok {
				place.W = -1
				break
			}
			place.X, place.Y = place.X+up.X, place.Y+up.Y
		}
		if place.W >= 0 {
			out[i] = place
		}
	}
	return out, nil
}
