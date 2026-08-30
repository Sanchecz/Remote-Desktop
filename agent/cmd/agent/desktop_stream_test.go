//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDesktopVideoHTTPClientKeepsCapacityForEveryStreamLane(t *testing.T) {
	client := newDesktopHTTPClientWithLimit(8*time.Second, desktopAutoVideoLaneCount+2)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected desktop transport type: %T", client.Transport)
	}
	minimum := desktopAutoVideoLaneCount + 2
	if transport.MaxConnsPerHost < minimum || transport.MaxIdleConnsPerHost < minimum || transport.MaxIdleConns < minimum {
		t.Fatalf("video transport can serialize %d lanes: max=%d idle-host=%d idle=%d", desktopAutoVideoLaneCount, transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost, transport.MaxIdleConns)
	}
}

func TestDesktopGenericHTTPClientRemainsBounded(t *testing.T) {
	client := newDesktopHTTPClient(3 * time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected desktop transport type: %T", client.Transport)
	}
	if transport.MaxConnsPerHost != 2 || transport.MaxIdleConnsPerHost != 2 {
		t.Fatalf("control transport lost its bounded connection pool: max=%d idle-host=%d", transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
}

func TestDesktopStreamRetryIsFastAfterDisconnectAndBoundedDuringOutage(t *testing.T) {
	if got := nextDesktopStreamRetry(3*time.Second, true); got != desktopStreamRetryInitial {
		t.Fatalf("established stream must retry from initial delay, got %v", got)
	}
	delay := time.Duration(0)
	for range 12 {
		delay = nextDesktopStreamRetry(delay, false)
	}
	if delay != desktopStreamRetryMaximum {
		t.Fatalf("retry must stop growing at %v, got %v", desktopStreamRetryMaximum, delay)
	}
}

func TestDesktopAuxiliaryLaneWaitsThroughReconnectBackoff(t *testing.T) {
	now := time.Unix(100, 0)
	if got := desktopStreamRetryWait(time.Time{}, now); got != 0 {
		t.Fatalf("empty retry deadline must be immediately ready, got %v", got)
	}
	if got := desktopStreamRetryWait(now.Add(-time.Millisecond), now); got != 0 {
		t.Fatalf("expired retry deadline must be immediately ready, got %v", got)
	}
	if got := desktopStreamRetryWait(now.Add(375*time.Millisecond), now); got != 375*time.Millisecond {
		t.Fatalf("active retry deadline = %v, want 375ms", got)
	}
}

func TestDesktopStreamRetryIdentityChangeClearsOldBackoff(t *testing.T) {
	stream := &desktopFrameStreamClient{
		retryAfter:     time.Now().Add(time.Minute),
		retryDelay:     desktopStreamRetryMaximum,
		retrySessionID: "old-session",
		retryAccessKey: "old-key",
	}
	stream.resetRetry()
	if !stream.retryAfter.IsZero() || stream.retryDelay != 0 || stream.retrySessionID != "" || stream.retryAccessKey != "" {
		t.Fatalf("reset must clear the complete retry identity: %+v", stream)
	}
}

func TestDesktopInputStreamRetryResetsAfterSuccessfulHandshake(t *testing.T) {
	if got := nextDesktopInputStreamRetry(desktopInputStreamRetryMaximum, true); got != desktopInputStreamRetryInitial {
		t.Fatalf("connected input stream must restore fast retry, got %v", got)
	}
	delay := time.Duration(0)
	for range 12 {
		delay = nextDesktopInputStreamRetry(delay, false)
	}
	if delay != desktopInputStreamRetryMaximum {
		t.Fatalf("input retry must remain bounded at %v, got %v", desktopInputStreamRetryMaximum, delay)
	}
}

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
		{Type: "pointer", Action: "move", X: 20, Y: 30},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "text", Text: "?:Я+"},
		{Type: "pointer", Action: "down", Button: "left", X: 20, Y: 30},
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "move", X: 40, Y: 50},
		{Type: "pointer", Action: "up", Button: "left", X: 40, Y: 50},
		{Type: "pointer", Action: "move", X: 50, Y: 60},
		{Type: "pointer", Action: "move", X: 60, Y: 70},
	})
	if len(events) != 7 {
		t.Fatalf("expected each consecutive move run to retain only its newest position, got %#v", events)
	}
	if events[0].Action != "move" || events[0].X != 20 || events[1].Type != "key" || events[2].Type != "text" || events[2].Text != "?:Я+" || events[3].Action != "down" || events[4].Action != "move" || events[4].X != 40 || events[5].Action != "up" || events[6].Action != "move" || events[6].X != 60 {
		t.Fatalf("input actions changed while coalescing: %#v", events)
	}
}

func TestDesktopInputDispatcherSuppressesRestoredDuplicateIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batches := make(chan desktopInputBatch, 8)
	tasks := make(chan desktopInputTask, 8)
	var latestCapture atomic.Value
	latestCapture.Store(desktopCapture{FrameWidth: 1920, FrameHeight: 1080, ScreenWidth: 1920, ScreenHeight: 1080})
	var latestInput atomic.Int64
	var activeSession atomic.Value
	activeSession.Store("session-a")
	go runDesktopStreamInputDispatcher(ctx, batches, tasks, &latestCapture, &latestInput, &activeSession)

	batches <- desktopInputBatch{SessionID: "session-a", Events: []desktopInput{{ID: 10, Type: "pointer", Action: "down", Button: "left", X: 100, Y: 100}, {ID: 11, Type: "pointer", Action: "up", Button: "left", X: 100, Y: 100}}}
	select {
	case task := <-tasks:
		if task.sessionID != "session-a" || len(task.events) != 2 || task.events[0].ID != 10 || task.events[1].ID != 11 {
			t.Fatalf("unexpected initial dispatch: %#v", task.events)
		}
	case <-time.After(time.Second):
		t.Fatal("initial input batch was not dispatched")
	}

	batches <- desktopInputBatch{SessionID: "session-a", Events: []desktopInput{{ID: 10, Type: "pointer", Action: "down", Button: "left", X: 100, Y: 100}, {ID: 11, Type: "pointer", Action: "up", Button: "left", X: 100, Y: 100}, {ID: 12, Type: "key", Action: "down", KeyCode: 65}}}
	select {
	case task := <-tasks:
		if len(task.events) != 1 || task.events[0].ID != 12 {
			t.Fatalf("restored duplicates were dispatched twice: %#v", task.events)
		}
	case <-time.After(time.Second):
		t.Fatal("new input after restored duplicates was not dispatched")
	}
}

func TestDesktopInputDispatcherResetsIDsForNewSessionAndDropsStaleSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batches := make(chan desktopInputBatch, 8)
	tasks := make(chan desktopInputTask, 8)
	var latestCapture atomic.Value
	latestCapture.Store(desktopCapture{FrameWidth: 3840, FrameHeight: 2160, ScreenWidth: 3840, ScreenHeight: 2160})
	var latestInput atomic.Int64
	var activeSession atomic.Value
	activeSession.Store("session-a")
	go runDesktopStreamInputDispatcher(ctx, batches, tasks, &latestCapture, &latestInput, &activeSession)

	batches <- desktopInputBatch{SessionID: "session-a", Events: []desktopInput{{ID: 90, Type: "key", Action: "down", KeyCode: 65}}}
	select {
	case task := <-tasks:
		if task.sessionID != "session-a" || len(task.events) != 1 || task.events[0].ID != 90 {
			t.Fatalf("unexpected first session dispatch: %#v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("first session input was not dispatched")
	}

	activeSession.Store("session-b")
	batches <- desktopInputBatch{SessionID: "session-a", Events: []desktopInput{{ID: 91, Type: "key", Action: "down", KeyCode: 66}}}
	batches <- desktopInputBatch{SessionID: "session-b", Events: []desktopInput{{ID: 1, Type: "pointer", Action: "down", Button: "left", X: 200, Y: 300}, {ID: 2, Type: "pointer", Action: "up", Button: "left", X: 200, Y: 300}}}
	select {
	case task := <-tasks:
		if task.sessionID != "session-b" || len(task.events) != 2 || task.events[0].ID != 1 || task.events[1].ID != 2 {
			t.Fatalf("new session IDs were not reset: %#v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("new session input was incorrectly treated as a duplicate")
	}

	batches <- desktopInputBatch{SessionID: "session-b", Events: []desktopInput{{ID: 1, Type: "pointer", Action: "down", Button: "left", X: 200, Y: 300}, {ID: 2, Type: "pointer", Action: "up", Button: "left", X: 200, Y: 300}}}
	select {
	case task := <-tasks:
		t.Fatalf("restored duplicate from the new session was dispatched: %#v", task)
	case <-time.After(100 * time.Millisecond):
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

func TestDispatchDesktopFrameUsesFirstAvailableLaneWithoutQueueing(t *testing.T) {
	lanes := []chan desktopFrameUpload{
		make(chan desktopFrameUpload),
		make(chan desktopFrameUpload),
		make(chan desktopFrameUpload),
	}
	received := make(chan desktopFrameUpload, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		received <- <-lanes[1]
	}()
	<-ready

	upload := desktopFrameUpload{sequence: 42, capture: desktopCapture{JPEG: []byte{1, 2, 3}}}
	deadline := time.Now().Add(time.Second)
	for {
		next, sent := dispatchDesktopFrameToAvailableLane(lanes, 0, upload)
		if sent {
			if next != 2 {
				t.Fatalf("next lane = %d, want 2", next)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("available lane did not receive the frame")
		}
		runtime.Gosched()
	}
	select {
	case got := <-received:
		if got.sequence != 42 {
			t.Fatalf("sequence = %d, want 42", got.sequence)
		}
	case <-time.After(time.Second):
		t.Fatal("frame was not dispatched")
	}
}

func TestDispatchDesktopFrameDropsWhenEveryLaneIsBusy(t *testing.T) {
	lanes := []chan desktopFrameUpload{make(chan desktopFrameUpload), make(chan desktopFrameUpload)}
	upload := desktopFrameUpload{sequence: 7, capture: desktopCapture{JPEG: []byte{7}}, pooled: false}
	next, sent := dispatchDesktopFrameToAvailableLane(lanes, 1, upload)
	if sent {
		t.Fatal("frame was queued despite every lane being busy")
	}
	if next != 1 {
		t.Fatalf("next lane = %d, want unchanged start lane 1", next)
	}
	for index, lane := range lanes {
		if len(lane) != 0 {
			t.Fatalf("lane %d retained a stale frame", index)
		}
	}
}

func TestDesktopFrameStreamHeader(t *testing.T) {
	upload := desktopFrameUpload{
		sequence: 0x0102030405060708,
		capture: desktopCapture{
			FrameWidth:     3840,
			FrameHeight:    2160,
			CaptureMillis:  300,
			CopyMillis:     -1,
			ScaleMillis:    17,
			EncodeMillis:   42,
			CaptureBackend: strings.Repeat("x", 64),
		},
	}
	header := desktopFrameStreamHeader(upload)
	if len(header) != 26+48 {
		t.Fatalf("header length = %d, want %d", len(header), 26+48)
	}
	if string(header[:4]) != "RIT3" {
		t.Fatalf("magic = %q", header[:4])
	}
	if got := binary.BigEndian.Uint32(header[4:8]); got != 3840 {
		t.Fatalf("width = %d", got)
	}
	if got := binary.BigEndian.Uint32(header[8:12]); got != 2160 {
		t.Fatalf("height = %d", got)
	}
	if got := header[12:18]; got[0] != 255 || got[1] != 0 || got[2] != 17 || got[3] != 42 || got[4] != 0 || got[5] != 48 {
		t.Fatalf("diagnostics/backend length = %v", got)
	}
	if got := binary.BigEndian.Uint64(header[18:26]); got != upload.sequence {
		t.Fatalf("sequence = %#x", got)
	}
	if got := string(header[26:]); got != strings.Repeat("x", 48) {
		t.Fatalf("backend = %q", got)
	}
}
