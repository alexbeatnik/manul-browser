package browser

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Firefox starts no drag session from WebDriver actions, so an element with
// draggable="true" is dragged by the page's own drag events, and the pointer
// is not used on top of them; anything else is still a pointer drag.
func TestDragHTML5First(t *testing.T) {
	for _, c := range []struct {
		answer      interface{}
		wantPointer bool
	}{
		{"1", false},
		{"0", true},
		{nil, true},
	} {
		var asked string
		eval := func(_ context.Context, expr string) (interface{}, error) {
			asked = expr
			return c.answer, nil
		}
		pointer := false
		err := dragHTML5First(context.Background(), eval, 10, 20, 300, 40, func() error { pointer = true; return nil })
		if err != nil {
			t.Fatalf("answer %v: %v", c.answer, err)
		}
		if pointer != c.wantPointer {
			t.Errorf("page answered %v: pointer drag = %v, want %v", c.answer, pointer, c.wantPointer)
		}
		if !strings.Contains(asked, `closest('[draggable="true"]')`) {
			t.Errorf("the page was not asked about a draggable source: %s", asked)
		}
	}

	failed := dragHTML5First(context.Background(), func(context.Context, string) (interface{}, error) {
		return nil, errors.New("gone")
	}, 0, 0, 1, 1, func() error { t.Error("pointer drag after a failed evaluation"); return nil })
	if failed == nil {
		t.Error("a failed evaluation passed")
	}
}
