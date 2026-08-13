//go:build windows

package main

import "testing"

func TestDecodeDesktopInputStreamMessage(t *testing.T) {
	events, err := decodeDesktopInputStreamMessage([]byte(`{"events":[{"id":7,"event":{"type":"pointer","action":"move","x":123,"y":456}},{"id":8,"event":{"type":"key","action":"down","keyCode":13}}]}`))
	if err != nil {
		t.Fatalf("decode stream message: %v", err)
	}
	if len(events) != 2 || events[0].Type != "pointer" || events[0].X != 123 || events[0].Y != 456 || events[1].Type != "key" || events[1].KeyCode != 13 {
		t.Fatalf("unexpected decoded events: %#v", events)
	}
}

func TestDecodeDesktopInputStreamMessageRejectsInvalidBatch(t *testing.T) {
	for _, payload := range [][]byte{nil, []byte(`{}`), []byte(`{"events":[]}`), []byte(`not-json`)} {
		if _, err := decodeDesktopInputStreamMessage(payload); err == nil {
			t.Fatalf("payload %q should be rejected", payload)
		}
	}
}

func TestCoalesceDesktopInputRetainsActionsAndNewestMove(t *testing.T) {
	events := coalesceDesktopInput([]desktopInput{
		{Type: "pointer", Action: "move", X: 10, Y: 20},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "pointer", Action: "down", Button: "left", X: 10, Y: 20},
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "up", Button: "left", X: 30, Y: 40},
	})
	if len(events) != 4 {
		t.Fatalf("expected one stale move to be removed, got %#v", events)
	}
	if events[0].Type != "key" || events[1].Action != "down" || events[2].Action != "move" || events[2].X != 30 || events[3].Action != "up" {
		t.Fatalf("input actions changed while coalescing: %#v", events)
	}
}
