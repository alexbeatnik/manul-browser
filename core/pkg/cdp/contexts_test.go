package cdp

import (
	"encoding/json"
	"testing"
	"time"
)

// newTestTracker builds a FrameTracker without a live connection so the
// event-handling logic can be unit-tested with synthetic CDP event params.
func newTestTracker() *FrameTracker {
	return &FrameTracker{frames: make(map[string]*FrameInfo)}
}

func raw(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestFrameTracker_MapsContextsPerFrame(t *testing.T) {
	ft := newTestTracker()

	// Main frame navigates (no parentId) and gets its default context.
	ft.onFrameNavigated(raw(map[string]any{
		"frame": map[string]any{"id": "MAIN", "url": "https://example.com", "name": ""},
	}))
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 1, "auxData": map[string]any{"frameId": "MAIN", "isDefault": true}},
	}))

	// A child iframe attaches and gets its own execution context.
	ft.onFrameAttached(raw(map[string]any{"frameId": "CHILD"}))
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 2, "auxData": map[string]any{"frameId": "CHILD", "isDefault": true}},
	}))

	frames := ft.Frames()
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if frames[0].Index != 0 || frames[0].FrameID != "MAIN" || frames[0].ContextID != 1 {
		t.Fatalf("main frame mismatch: %+v", frames[0])
	}
	if frames[1].Index != 1 || frames[1].FrameID != "CHILD" || frames[1].ContextID != 2 {
		t.Fatalf("child frame mismatch: %+v", frames[1])
	}
	if got := ft.ContextForIndex(1); got != 2 {
		t.Fatalf("ContextForIndex(1) = %d, want 2", got)
	}
	if got := ft.ContextForIndex(0); got != 0 {
		t.Fatalf("ContextForIndex(0) = %d, want 0 (default context)", got)
	}
}

func TestFrameTracker_NonDefaultContextIgnored(t *testing.T) {
	ft := newTestTracker()
	ft.onFrameNavigated(raw(map[string]any{"frame": map[string]any{"id": "MAIN"}}))
	// An isolated-world / non-default context must NOT override the frame's
	// default execution context.
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 1, "auxData": map[string]any{"frameId": "MAIN", "isDefault": true}},
	}))
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 99, "auxData": map[string]any{"frameId": "MAIN", "isDefault": false}},
	}))
	if got := ft.ContextForIndex(0); got != 0 {
		t.Fatalf("main is index 0 (default), ContextForIndex(0) should be 0, got %d", got)
	}
	frames := ft.Frames()
	if frames[0].ContextID != 1 {
		t.Fatalf("default context should remain 1, got %d", frames[0].ContextID)
	}
}

func TestFrameTracker_DestroyAndDetach(t *testing.T) {
	ft := newTestTracker()
	ft.onFrameNavigated(raw(map[string]any{"frame": map[string]any{"id": "MAIN"}}))
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 1, "auxData": map[string]any{"frameId": "MAIN", "isDefault": true}},
	}))
	ft.onFrameAttached(raw(map[string]any{"frameId": "CHILD"}))
	ft.onContextCreated(raw(map[string]any{
		"context": map[string]any{"id": 2, "auxData": map[string]any{"frameId": "CHILD", "isDefault": true}},
	}))

	// Context destroyed → frame keeps existing but loses its context id.
	ft.onContextDestroyed(raw(map[string]any{"executionContextId": 2}))
	if got := ft.ContextForIndex(1); got != 0 {
		t.Fatalf("after destroy, child context should be 0, got %d", got)
	}

	// Frame detached → frame removed entirely.
	ft.onFrameDetached(raw(map[string]any{"frameId": "CHILD"}))
	if len(ft.Frames()) != 1 {
		t.Fatalf("after detach, only the main frame should remain")
	}
}

// Chrome reports the execution contexts that already exist while answering
// Runtime.enable. The tracker subscribed after the call, heard none of them,
// and every iframe already on the page was left without a context.
func TestFrameTracker_SeesContextsAnnouncedDuringEnable(t *testing.T) {
	s, c, ctx := startScriptedCDP(t, map[string]string{
		"Page.getFrameTree": `{"frameTree":{"frame":{"id":"MAIN","url":"http://a.test/"},"childFrames":[{"frame":{"id":"F1","parentId":"MAIN","url":"http://a.test/f.html"}}]}}`,
	})
	s.beforeReply = func(method string) {
		if method == "Runtime.enable" {
			s.push(`{"method":"Runtime.executionContextCreated","params":{"context":{"id":42,"auxData":{"frameId":"F1","isDefault":true}}}}`)
		}
	}
	ft, err := NewFrameTracker(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for ft.ContextForIndex(1) != 42 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := ft.ContextForIndex(1); got != 42 {
		t.Errorf("frame 1 context = %d, want 42; frames %+v", got, ft.Frames())
	}
}
