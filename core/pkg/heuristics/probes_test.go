package heuristics

import "testing"

func TestParseProbeResult_PreservesFrameIndexMetadata(t *testing.T) {
	raw := []byte(`{
		"url": "https://example.com",
		"title": "iFrame Routing Lab",
		"visible_text": "login save",
		"elements": [
			{
				"id": 1,
				"xpath": "/html/body/button[1]",
				"tag": "button",
				"html_id": "main_submit",
				"visible_text": "Submit Order",
				"is_visible": true,
				"rect": {"top": 10, "left": 20, "bottom": 40, "right": 120, "width": 100, "height": 30},
				"frame_index": 0
			},
			{
				"id": 2,
				"xpath": "/html/body/input[1]",
				"tag": "input",
				"input_type": "text",
				"html_id": "iframe_user",
				"placeholder": "Username",
				"is_visible": true,
				"is_editable": true,
				"rect": {"top": 50, "left": 20, "bottom": 80, "right": 220, "width": 200, "height": 30},
				"frame_index": 1
			}
		]
	}`)

	elements, err := ParseProbeResult(raw)
	if err != nil {
		t.Fatalf("ParseProbeResult failed: %v", err)
	}
	if len(elements) != 2 {
		t.Fatalf("len(elements) = %d, want 2", len(elements))
	}
	if elements[0].FrameIndex != 0 {
		t.Fatalf("elements[0].FrameIndex = %d, want 0", elements[0].FrameIndex)
	}
	if elements[1].FrameIndex != 1 {
		t.Fatalf("elements[1].FrameIndex = %d, want 1", elements[1].FrameIndex)
	}
	if elements[1].NormPlaceholder != "username" {
		t.Fatalf("NormPlaceholder = %q, want username", elements[1].NormPlaceholder)
	}
}

func TestParseProbeResult_RawArrayFallbackPreservesFrameIndex(t *testing.T) {
	raw := []byte(`[
		{
			"id": 7,
			"xpath": "/html/body/button[1]",
			"tag": "button",
			"html_id": "widget_save",
			"visible_text": "Save",
			"is_visible": true,
			"rect": {"top": 10, "left": 20, "bottom": 40, "right": 120, "width": 100, "height": 30},
			"frame_index": 2
		}
	]`)

	elements, err := ParseProbeResult(raw)
	if err != nil {
		t.Fatalf("ParseProbeResult failed: %v", err)
	}
	if len(elements) != 1 {
		t.Fatalf("len(elements) = %d, want 1", len(elements))
	}
	if elements[0].FrameIndex != 2 {
		t.Fatalf("elements[0].FrameIndex = %d, want 2", elements[0].FrameIndex)
	}
	if elements[0].NormHTMLId != "widget_save" {
		t.Fatalf("NormHTMLId = %q, want widget_save", elements[0].NormHTMLId)
	}
}

// YouTube's tabs keep the index of the selected one in `selected`, and the
// probe sent it as is_selected: 1. One element like that failed the whole
// snapshot, and every step on the page with it.
func TestParseProbeResult_AnElementThatDoesNotFitCostsOnlyItself(t *testing.T) {
	raw := []byte(`{"elements": [
		{"id": 1, "tag": "x-tabs", "is_selected": 1},
		{"id": 2, "tag": "x-tab", "visible_text": "Videos", "is_visible": true},
		{"id": 3, "tag": "x-slider", "value": 50},
		{"id": 4, "tag": "x-tab", "visible_text": "Shorts", "is_visible": true}
	]}`)
	elements, err := ParseProbeResult(raw)
	if err != nil {
		t.Fatalf("one element that does not fit failed the snapshot: %v", err)
	}
	var got []string
	for _, e := range elements {
		got = append(got, e.NormText)
	}
	if len(elements) != 2 || got[0] != "videos" || got[1] != "shorts" {
		t.Errorf("elements = %v, want the two that fit", got)
	}
}
