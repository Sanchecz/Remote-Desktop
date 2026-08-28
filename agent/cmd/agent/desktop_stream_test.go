//go:build windows

package main

import "testing"

func TestDecodeDesktopInputStreamMessage(t *testing.T) {
	events, err := decodeDesktopInputStreamMessage([]byte(`{"events":[{"id":7,"event":{"type":"pointer","action":"move","x":123,"y":456}},{"id":8,"event":{"type":"key","action":"down","keyCode":13}},{"id":9,"event":{"type":"text","text":"?:Я+👋"}}]}`))
	if err != nil {
		t.Fatalf("decode stream message: %v", err)
	}
	if len(events) != 3 || events[0].Type != "pointer" || events[0].X != 123 || events[0].Y != 456 || events[1].Type != "key" || events[1].KeyCode != 13 || events[2].Type != "text" || events[2].Text != "?:Я+👋" {
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
		{Type: "text", Text: "?:Я+"},
		{Type: "pointer", Action: "down", Button: "left", X: 10, Y: 20},
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "up", Button: "left", X: 30, Y: 40},
	})
	if len(events) != 5 {
		t.Fatalf("expected one stale move to be removed, got %#v", events)
	}
	if events[0].Type != "key" || events[1].Type != "text" || events[1].Text != "?:Я+" || events[2].Action != "down" || events[3].Action != "move" || events[3].X != 30 || events[4].Action != "up" {
		t.Fatalf("input actions changed while coalescing: %#v", events)
	}
}

func TestDesktopUploadBufferIsImmutableAndReleased(t *testing.T) {
	source := []byte{1, 2, 3, 4}
	cloned, pooled := cloneDesktopJPEGForUpload(source)
	if !pooled || len(cloned) != len(source) {
		t.Fatalf("clone = %v, pooled = %v", cloned, pooled)
	}
	source[0] = 9
	if cloned[0] != 1 {
		t.Fatalf("uploader buffer aliases the reusable capture buffer: %v", cloned)
	}
	upload := desktopFrameUpload{capture: desktopCapture{JPEG: cloned}, pooled: pooled}
	releaseDesktopFrameUpload(&upload)
	if upload.capture.JPEG != nil || upload.pooled {
		t.Fatalf("released upload still owns a JPEG buffer: %#v", upload)
	}
}

func TestEnqueueLatestDesktopFrameReleasesReplacedUpload(t *testing.T) {
	uploads := make(chan desktopFrameUpload, 1)
	firstJPEG, firstPooled := cloneDesktopJPEGForUpload([]byte{1})
	first := desktopFrameUpload{sequence: 1, capture: desktopCapture{JPEG: firstJPEG}, pooled: firstPooled}
	uploads <- first

	secondJPEG, secondPooled := cloneDesktopJPEGForUpload([]byte{2})
	second := desktopFrameUpload{sequence: 2, capture: desktopCapture{JPEG: secondJPEG}, pooled: secondPooled}
	enqueueLatestDesktopFrame(uploads, second)
	queued := <-uploads
	if queued.sequence != 2 || len(queued.capture.JPEG) != 1 || queued.capture.JPEG[0] != 2 {
		t.Fatalf("latest upload was not retained: %#v", queued)
	}
	releaseDesktopFrameUpload(&queued)
}
