package cdp

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// FrameInfo describes one frame (main document or iframe) and the JavaScript
// execution context Chrome created for it. Index 0 is always the main frame;
// positive indices are embedded frames in attach order.
type FrameInfo struct {
	FrameID  string
	ParentID string
	URL      string
	Name     string
	// SessionID is the attached session that owns the frame: empty for a
	// frame in the page's own process, the session of an out-of-process
	// iframe (a cross-site one) otherwise. ContextID is only meaningful
	// together with it — every session numbers its contexts from 1.
	SessionID string
	ContextID int
	Index     int
}

// FrameTracker maintains the live frameId → execution-context mapping for a
// page by listening to Runtime/Page CDP events, on the page's own session and
// on the session of every out-of-process iframe. It is what lets Manul
// Browser evaluate JavaScript inside iframes instead of only the main
// document.
//
// A cross-site iframe runs in a process of its own (site isolation), and the
// page's session knows nothing of it: no frame, no context. Target.setAutoAttach
// with flatten attaches each one as a session on the same connection; its
// frames and contexts are tracked with that session id.
type FrameTracker struct {
	conn *Conn

	mu          sync.Mutex
	mainFrameID string
	frames      map[string]*FrameInfo // frameId -> info
	order       []string              // frame insertion order (main first)

	sub *Subscription

	// busy counts events being handled and sessions being set up, so
	// NewFrameTracker can wait for the iframes that exist when it starts.
	busy atomic.Int32
}

// NewFrameTracker enables the Page and Runtime domains, seeds the frame tree,
// attaches the page's out-of-process iframes, and starts listening for
// execution-context lifecycle events. The returned tracker keeps a background
// goroutine alive until the connection closes.
func NewFrameTracker(ctx context.Context, conn *Conn) (*FrameTracker, error) {
	ft := &FrameTracker{
		conn:   conn,
		frames: make(map[string]*FrameInfo),
	}

	// Enabling Runtime makes Chrome emit executionContextCreated for every
	// frame; enabling Page makes it emit frameNavigated/frameAttached.
	//
	// Subscribed first: Chrome reports the contexts that already exist while
	// answering Runtime.enable, so a subscription made after it heard none of
	// them, and every iframe already on the page stayed without a context —
	// unreachable until it navigated.
	ft.sub = conn.Subscribe()
	go ft.loop()
	if _, err := conn.Call(ctx, "Page.enable", nil); err != nil {
		ft.sub.Close()
		return nil, err
	}
	if _, err := conn.Call(ctx, "Runtime.enable", nil); err != nil {
		ft.sub.Close()
		return nil, err
	}
	ft.seedFrameTree(ctx, "")
	ft.autoAttach(ctx, "")
	ft.settle(ctx)
	return ft, nil
}

// autoAttach asks a session to attach its out-of-process iframes. A browser
// too old for the filter is asked without it; anything that is not an iframe
// is then ignored when it attaches.
func (ft *FrameTracker) autoAttach(ctx context.Context, session string) {
	params := map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true,
		"filter": []map[string]any{{"type": "iframe"}},
	}
	if _, err := ft.conn.CallSession(ctx, session, "Target.setAutoAttach", params); err != nil {
		delete(params, "filter")
		_, _ = ft.conn.CallSession(ctx, session, "Target.setAutoAttach", params)
	}
}

// settle waits, briefly, until every event already received has been handled
// and every iframe attached so far has its session set up. Chrome announces
// the iframes that exist while answering setAutoAttach, as it does contexts
// for Runtime.enable; without this the first snapshot ran before they were
// enabled and saw none of them.
func (ft *FrameTracker) settle(ctx context.Context) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if len(ft.sub.C()) == 0 && ft.busy.Load() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Frames returns the current frames, main frame first, then child frames in
// attach order. Frames whose execution context is not yet known are included
// with ContextID == 0 so callers can still see the frame.
func (ft *FrameTracker) Frames() []FrameInfo {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	out := make([]FrameInfo, 0, len(ft.order))
	idx := 0
	appendFrame := func(id string) {
		f, ok := ft.frames[id]
		if !ok {
			return
		}
		copyF := *f
		copyF.Index = idx
		idx++
		out = append(out, copyF)
	}
	if ft.mainFrameID != "" {
		appendFrame(ft.mainFrameID)
	}
	for _, id := range ft.order {
		if id == ft.mainFrameID {
			continue
		}
		appendFrame(id)
	}
	return out
}

// ContextForIndex resolves a frame index (as returned by Frames) to its
// execution context id. Index 0 / unknown returns 0 (the default context).
// The id is only meaningful in the frame's session; see TargetForIndex.
func (ft *FrameTracker) ContextForIndex(index int) int {
	_, id := ft.TargetForIndex(index)
	return id
}

// TargetForIndex resolves a frame index to the session and execution context
// to evaluate in. Index 0 / unknown returns ("", 0): the page's default.
func (ft *FrameTracker) TargetForIndex(index int) (session string, contextID int) {
	if index <= 0 {
		return "", 0
	}
	for _, f := range ft.Frames() {
		if f.Index == index {
			return f.SessionID, f.ContextID
		}
	}
	return "", 0
}

func (ft *FrameTracker) seedFrameTree(ctx context.Context, session string) {
	raw, err := ft.conn.CallSession(ctx, session, "Page.getFrameTree", nil)
	if err != nil {
		return
	}
	var resp struct {
		FrameTree frameTreeNode `json:"frameTree"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.ingest(&resp.FrameTree, session, session == "")
}

type frameTreeNode struct {
	Frame struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		URL      string `json:"url"`
		Name     string `json:"name"`
	} `json:"frame"`
	ChildFrames []frameTreeNode `json:"childFrames"`
}

// ingest walks the frame tree; caller holds ft.mu.
func (ft *FrameTracker) ingest(node *frameTreeNode, session string, isMain bool) {
	id := node.Frame.ID
	if id == "" {
		return
	}
	if isMain {
		ft.mainFrameID = id
	}
	f := ft.upsertLocked(id, node.Frame.URL, node.Frame.Name)
	f.SessionID = session
	if node.Frame.ParentID != "" {
		f.ParentID = node.Frame.ParentID
	}
	for i := range node.ChildFrames {
		ft.ingest(&node.ChildFrames[i], session, false)
	}
}

// upsertLocked creates/updates a frame record; caller holds ft.mu.
func (ft *FrameTracker) upsertLocked(id, url, name string) *FrameInfo {
	f, ok := ft.frames[id]
	if !ok {
		f = &FrameInfo{FrameID: id}
		ft.frames[id] = f
		ft.order = append(ft.order, id)
	}
	if url != "" {
		f.URL = url
	}
	if name != "" {
		f.Name = name
	}
	return f
}

func (ft *FrameTracker) loop() {
	ch := ft.sub.C()
	for msg := range ch {
		ft.busy.Add(1)
		s := msg.SessionID
		switch msg.Method {
		case "Runtime.executionContextCreated":
			ft.onContextCreated(s, msg.Params)
		case "Runtime.executionContextDestroyed":
			ft.onContextDestroyed(s, msg.Params)
		case "Runtime.executionContextsCleared":
			ft.onContextsCleared(s)
		case "Page.frameNavigated":
			ft.onFrameNavigated(s, msg.Params)
		case "Page.frameAttached":
			ft.onFrameAttached(s, msg.Params)
		case "Page.frameDetached":
			ft.onFrameDetached(s, msg.Params)
		case "Target.attachedToTarget":
			ft.onAttached(msg.Params)
		case "Target.detachedFromTarget":
			ft.onDetached(msg.Params)
		}
		ft.busy.Add(-1)
	}
}

// onAttached sets up the session of an out-of-process iframe. It is done on
// a goroutine of its own: the calls wait for replies this loop does not
// handle, but the events they cause are this loop's to handle.
func (ft *FrameTracker) onAttached(params json.RawMessage) {
	var p struct {
		SessionID  string `json:"sessionId"`
		TargetInfo struct {
			TargetID      string `json:"targetId"`
			Type          string `json:"type"`
			URL           string `json:"url"`
			ParentFrameID string `json:"parentFrameId"`
		} `json:"targetInfo"`
	}
	if json.Unmarshal(params, &p) != nil || p.SessionID == "" || p.TargetInfo.Type != "iframe" {
		return
	}
	ft.mu.Lock()
	f := ft.upsertLocked(p.TargetInfo.TargetID, p.TargetInfo.URL, "")
	f.SessionID = p.SessionID
	if p.TargetInfo.ParentFrameID != "" {
		f.ParentID = p.TargetInfo.ParentFrameID
	}
	ft.mu.Unlock()

	if ft.conn == nil {
		return
	}
	ft.busy.Add(1)
	go func() {
		defer ft.busy.Add(-1)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s := p.SessionID
		_, _ = ft.conn.CallSession(ctx, s, "Page.enable", nil)
		_, _ = ft.conn.CallSession(ctx, s, "Runtime.enable", nil)
		ft.seedFrameTree(ctx, s)
		ft.autoAttach(ctx, s)
		_, _ = ft.conn.CallSession(ctx, s, "Runtime.runIfWaitingForDebugger", nil)
	}()
}

func (ft *FrameTracker) onDetached(params json.RawMessage) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(params, &p) != nil || p.SessionID == "" {
		return
	}
	ft.mu.Lock()
	for id, f := range ft.frames {
		if f.SessionID == p.SessionID {
			delete(ft.frames, id)
			ft.order = removeString(ft.order, id)
		}
	}
	ft.mu.Unlock()
}

func (ft *FrameTracker) onContextCreated(session string, params json.RawMessage) {
	var p struct {
		Context struct {
			ID      int `json:"id"`
			AuxData struct {
				FrameID   string `json:"frameId"`
				IsDefault bool   `json:"isDefault"`
			} `json:"auxData"`
		} `json:"context"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	fid := p.Context.AuxData.FrameID
	if fid == "" || !p.Context.AuxData.IsDefault {
		return
	}
	ft.mu.Lock()
	f := ft.upsertLocked(fid, "", "")
	f.SessionID = session
	f.ContextID = p.Context.ID
	ft.mu.Unlock()
}

func (ft *FrameTracker) onContextDestroyed(session string, params json.RawMessage) {
	var p struct {
		ExecutionContextID int `json:"executionContextId"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	ft.mu.Lock()
	for _, f := range ft.frames {
		if f.SessionID == session && f.ContextID == p.ExecutionContextID {
			f.ContextID = 0
		}
	}
	ft.mu.Unlock()
}

func (ft *FrameTracker) onContextsCleared(session string) {
	ft.mu.Lock()
	for _, f := range ft.frames {
		if f.SessionID == session {
			f.ContextID = 0
		}
	}
	ft.mu.Unlock()
}

func (ft *FrameTracker) onFrameNavigated(session string, params json.RawMessage) {
	var p struct {
		Frame struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
			URL      string `json:"url"`
			Name     string `json:"name"`
		} `json:"frame"`
	}
	if json.Unmarshal(params, &p) != nil || p.Frame.ID == "" {
		return
	}
	ft.mu.Lock()
	if p.Frame.ParentID == "" && session == "" {
		ft.mainFrameID = p.Frame.ID
	}
	f := ft.upsertLocked(p.Frame.ID, p.Frame.URL, p.Frame.Name)
	if p.Frame.ParentID != "" {
		f.ParentID = p.Frame.ParentID
	}
	ft.mu.Unlock()
}

func (ft *FrameTracker) onFrameAttached(session string, params json.RawMessage) {
	var p struct {
		FrameID       string `json:"frameId"`
		ParentFrameID string `json:"parentFrameId"`
	}
	if json.Unmarshal(params, &p) != nil || p.FrameID == "" {
		return
	}
	ft.mu.Lock()
	f := ft.upsertLocked(p.FrameID, "", "")
	if p.ParentFrameID != "" {
		f.ParentID = p.ParentFrameID
	}
	ft.mu.Unlock()
}

// onFrameDetached removes a frame — unless it was detached to be swapped
// into a process of its own. The page's session reports that as a detach,
// and the frame lives on in the iframe's session, which may already have
// claimed it.
func (ft *FrameTracker) onFrameDetached(session string, params json.RawMessage) {
	var p struct {
		FrameID string `json:"frameId"`
		Reason  string `json:"reason"`
	}
	if json.Unmarshal(params, &p) != nil || p.FrameID == "" || p.Reason == "swap" {
		return
	}
	ft.mu.Lock()
	if f, ok := ft.frames[p.FrameID]; ok && f.SessionID == session {
		delete(ft.frames, p.FrameID)
		ft.order = removeString(ft.order, p.FrameID)
	}
	ft.mu.Unlock()
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
