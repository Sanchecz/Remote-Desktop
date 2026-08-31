package main

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestValidDesktopInput(t *testing.T) {
	valid := []desktopInputEvent{
		{Type: "pointer", Action: "move", X: 120, Y: 80},
		{Type: "pointer", Action: "move", X: 120, Y: 80, CoordinateWidth: 2256, CoordinateHeight: 1504},
		{Type: "pointer", Action: "down", Button: "left", X: 1, Y: 2},
		{Type: "pointer", Action: "up", Button: "right", X: 1, Y: 2},
		{Type: "wheel", Delta: -120},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "text", Text: "RemoteIt: Привет, Shift: ? : \" + _ ( ) ! 👋"},
		{Type: "sas"},
		{Type: "clipboard_write", Text: "двусторонний буфер"},
		{Type: "clipboard_read"},
	}
	for _, event := range valid {
		if !validDesktopInput(event) {
			t.Fatalf("expected valid event: %#v", event)
		}
	}
	invalid := []desktopInputEvent{
		{Type: "pointer", Action: "down", Button: "unknown"},
		{Type: "pointer", Action: "move", X: -1},
		{Type: "pointer", Action: "move", X: 1, Y: 1, CoordinateWidth: 1920},
		{Type: "pointer", Action: "move", X: 1, Y: 1, CoordinateWidth: 12001, CoordinateHeight: 1080},
		{Type: "wheel", Delta: 9999},
		{Type: "key", Action: "down", KeyCode: 0},
		{Type: "text"},
		{Type: "text", Text: string(make([]rune, 129))},
		{Type: "sas", Action: "down"},
		{Type: "clipboard_write", Text: "bad\x00value"},
		{Type: "clipboard_write", Text: string(make([]byte, (32<<10)+1))},
		{Type: "clipboard_read", Action: "down"},
		{Type: "shell"},
	}
	for _, event := range invalid {
		if validDesktopInput(event) {
			t.Fatalf("expected invalid event: %#v", event)
		}
	}
}

func TestDesktopOSSupportsWindowsAndAndroidAgents(t *testing.T) {
	for _, osName := range []string{"Windows 11", "windows server", "Android", "Android 16"} {
		if !desktopOSSupportsRemoteScreen(osName) {
			t.Fatalf("remote-capable OS %q was rejected", osName)
		}
	}
	for _, osName := range []string{"Linux", "macOS", "iOS", ""} {
		if desktopOSSupportsRemoteScreen(osName) {
			t.Fatalf("unsupported OS %q was accepted", osName)
		}
	}
}

func TestValidDesktopTargetFPS(t *testing.T) {
	for _, value := range []int{0, 15, 30, 60} {
		if !validDesktopTargetFPS(value) {
			t.Fatalf("expected supported target FPS: %d", value)
		}
	}
	for _, value := range []int{-1, 1, 14, 24, 59, 61, 120} {
		if validDesktopTargetFPS(value) {
			t.Fatalf("expected unsupported target FPS: %d", value)
		}
	}
}

func TestDesktopViewerInputLaneNeverCarriesFrames(t *testing.T) {
	if desktopViewerLaneCarriesFrames(-2) {
		t.Fatal("the dedicated input lane must not inspect video frames")
	}
	for _, lane := range []int{-1, 0, 1, desktopVideoLaneCount - 1} {
		if !desktopViewerLaneCarriesFrames(lane) {
			t.Fatalf("viewer lane %d must continue carrying frames", lane)
		}
	}
}

func TestDesktopViewerLaneOwnsOnlyOneDatabaseKeepalive(t *testing.T) {
	if !desktopViewerLaneOwnsKeepalive(-1) || !desktopViewerLaneOwnsKeepalive(0) {
		t.Fatal("legacy and primary viewer lanes must renew the session lease")
	}
	for _, lane := range []int{-2, 1, 2, 3, 4, 5} {
		if desktopViewerLaneOwnsKeepalive(lane) {
			t.Fatalf("lane %d must not duplicate the database keepalive", lane)
		}
	}
}

func TestImmutableDesktopFrameReusesViewerEnvelope(t *testing.T) {
	source := []byte{0xff, 0xd8, 1, 2, 3, 4, 0xff, 0xd9}
	raw, envelope := immutableDesktopFrame(source, 91)
	if len(envelope) != len(source)+12 || string(envelope[:4]) != "RTV1" {
		t.Fatalf("unexpected viewer envelope: %x", envelope)
	}
	if got := binary.BigEndian.Uint64(envelope[4:12]); got != 91 {
		t.Fatalf("unexpected producer sequence: %d", got)
	}
	if !bytes.Equal(raw, source) || !bytes.Equal(envelope[12:], source) {
		t.Fatalf("raw JPEG does not share the expected payload: raw=%x envelope=%x", raw, envelope)
	}
	if &raw[0] != &envelope[12] {
		t.Fatal("raw JPEG and viewer envelope must share one immutable allocation")
	}

	stored := desktopFrameState{Frame: raw, ViewerPayload: envelope, ProducerSequence: 91}
	payload := desktopViewerPayload(stored, int(91%desktopVideoLaneCount))
	if &payload[0] != &envelope[0] {
		t.Fatal("multi-lane viewer payload must be reused without a full-frame copy")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = desktopViewerPayload(stored, int(91%desktopVideoLaneCount))
	}); allocations != 0 {
		t.Fatalf("stored viewer envelope must be allocation-free, got %.2f allocations per frame", allocations)
	}

	source[2] = 99
	if raw[2] == 99 {
		t.Fatal("stored frame must not alias the Agent upload buffer")
	}
}

func TestParseDesktopFrameAcceptsBoundedNative4KPayload(t *testing.T) {
	frameSize := (9 << 20) + 137
	payload := make([]byte, 12+frameSize)
	copy(payload[:4], "RIT1")
	binary.BigEndian.PutUint32(payload[4:8], 3840)
	binary.BigEndian.PutUint32(payload[8:12], 2160)
	payload[12], payload[len(payload)-1] = 0xff, 0xd9

	parsed, err := parseDesktopFrameMessage(payload)
	if err != nil {
		t.Fatalf("a bounded 4K payload above the legacy 8 MiB limit was rejected: %v", err)
	}
	if parsed.Width != 3840 || parsed.Height != 2160 || len(parsed.Frame) != frameSize {
		t.Fatalf("unexpected parsed 4K frame: %#v frame=%d", parsed, len(parsed.Frame))
	}
}

func TestParseDesktopFrameRejectsPayloadBeyondEnvelopeBound(t *testing.T) {
	payload := make([]byte, desktopMaxFrameEnvelope+1)
	copy(payload[:4], "RIT1")
	binary.BigEndian.PutUint32(payload[4:8], 3840)
	binary.BigEndian.PutUint32(payload[8:12], 2160)
	if _, err := parseDesktopFrameMessage(payload); err == nil {
		t.Fatal("a payload beyond the bounded transport limit was accepted")
	}
}

func TestSignalDesktopFrameWakesOnlyMatchingVideoLane(t *testing.T) {
	s := &server{}
	const sessionID = "session-lane-signal"
	legacySignal := s.desktopFrameSignal(sessionID)
	matchingSignal := s.desktopFrameSignal(sessionID + "\x00viewer3")
	otherSignal := s.desktopFrameSignal(sessionID + "\x00viewer4")

	s.signalDesktopFrame(sessionID, 3)
	for name, signal := range map[string]chan struct{}{
		"legacy":        legacySignal,
		"matching lane": matchingSignal,
	} {
		select {
		case <-signal:
		default:
			t.Fatalf("%s was not notified", name)
		}
	}
	select {
	case <-otherSignal:
		t.Fatal("an unrelated video lane must not be woken for this frame")
	default:
	}
}

func TestResetDesktopFrameDatabaseTouchKeepsNewerClaim(t *testing.T) {
	s := &server{}
	oldClaim := time.Now().UTC()
	newClaim := oldClaim.Add(time.Second)
	s.desktopFrames.Store("session", desktopFrameState{Frame: []byte{1}, DatabaseTouchAt: newClaim})

	s.resetDesktopFrameDatabaseTouch("session", oldClaim)
	stored, ok := s.loadDesktopFrame("session")
	if !ok || !stored.DatabaseTouchAt.Equal(newClaim) {
		t.Fatalf("an older failed heartbeat erased a newer claim: %#v", stored)
	}

	s.resetDesktopFrameDatabaseTouch("session", newClaim)
	stored, ok = s.loadDesktopFrame("session")
	if !ok || !stored.DatabaseTouchAt.IsZero() {
		t.Fatalf("the matching failed heartbeat was not released: %#v", stored)
	}
}

func TestDesktopInputQueuePreservesActionsAndCoalescesPointerMoves(t *testing.T) {
	queue := newDesktopInputQueue()
	queue.enqueue([]desktopInputEvent{
		{Type: "pointer", Action: "move", X: 10, Y: 20},
		{Type: "pointer", Action: "move", X: 20, Y: 30},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "text", Text: "?:Я+"},
		{Type: "sas"},
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "move", X: 40, Y: 50},
		{Type: "pointer", Action: "down", Button: "left", X: 40, Y: 50},
		{Type: "pointer", Action: "move", X: 50, Y: 60},
		{Type: "pointer", Action: "move", X: 60, Y: 70},
		{Type: "pointer", Action: "up", Button: "left", X: 60, Y: 70},
	})

	items := queue.drain(64)
	if len(items) != 8 {
		t.Fatalf("expected consecutive pointer runs to be coalesced, got %d events", len(items))
	}
	if items[0].Event.Action != "move" || items[0].Event.X != 20 || items[1].Event.Type != "key" || items[2].Event.Type != "text" || items[2].Event.Text != "?:Я+" || items[3].Event.Type != "sas" || items[4].Event.Action != "move" || items[4].Event.X != 40 || items[5].Event.Action != "down" || items[6].Event.Action != "move" || items[6].Event.X != 60 || items[7].Event.Action != "up" {
		t.Fatalf("unexpected input order after coalescing: %#v", items)
	}
	for index := 1; index < len(items); index++ {
		if items[index].ID <= items[index-1].ID {
			t.Fatalf("input identifiers must remain monotonic: %#v", items)
		}
	}
}

func TestDesktopInputQueueNeverDropsStatefulEventsAtTheSoftLimit(t *testing.T) {
	queue := newDesktopInputQueue()
	for index := 0; index < 140; index++ {
		inputID := queue.enqueue([]desktopInputEvent{{Type: "key", Action: "down", KeyCode: 65}})
		if inputID != int64(index+1) {
			t.Fatalf("expected monotonically increasing input id %d, got %d", index+1, inputID)
		}
	}
	first := queue.drain(64)
	second := queue.drain(64)
	third := queue.drain(64)
	fourth := queue.drain(64)
	if len(first) != 64 || len(second) != 64 || len(third) != 12 || len(fourth) != 0 {
		t.Fatalf("expected all stateful events to survive, got batches %d/%d/%d/%d", len(first), len(second), len(third), len(fourth))
	}
	if first[0].ID != 1 || third[len(third)-1].ID != 140 {
		t.Fatalf("expected retained input ids 1..140, got %d..%d", first[0].ID, third[len(third)-1].ID)
	}
}

func TestDesktopInputQueueTrimsOnlyFreeMovesUnderPointerStorm(t *testing.T) {
	queue := newDesktopInputQueue()
	events := make([]desktopInputEvent, 0, desktopInputQueueSoftLimit+220)
	for index := 0; index < desktopInputQueueSoftLimit+200; index++ {
		events = append(events, desktopInputEvent{
			Type:          "pointer",
			Action:        "move",
			X:             index,
			Y:             index,
			ClientInputID: "storm:" + strconv.Itoa(index),
		})
		if index == 40 {
			events = append(events,
				desktopInputEvent{Type: "pointer", Action: "down", Button: "left", ClientInputID: "button:down"},
				desktopInputEvent{Type: "key", Action: "down", KeyCode: 16, ClientInputID: "shift:down"},
			)
		}
		if index == desktopInputQueueSoftLimit+150 {
			events = append(events,
				desktopInputEvent{Type: "key", Action: "up", KeyCode: 16, ClientInputID: "shift:up"},
				desktopInputEvent{Type: "pointer", Action: "up", Button: "left", ClientInputID: "button:up"},
			)
		}
	}

	queue.enqueue(events)
	drained := queue.drain(len(events))
	stateful := make([]string, 0, 4)
	for _, item := range drained {
		if item.Event.Action != "move" {
			stateful = append(stateful, item.Event.ClientInputID)
		}
	}
	want := []string{"button:down", "shift:down", "shift:up", "button:up"}
	if len(stateful) != len(want) {
		t.Fatalf("stateful input was dropped: got %#v want %#v", stateful, want)
	}
	for index := range want {
		if stateful[index] != want[index] {
			t.Fatalf("stateful input order changed: got %#v want %#v", stateful, want)
		}
	}
	if len(drained) > desktopInputQueueSoftLimit {
		t.Fatalf("pointer-only excess was not trimmed: got %d queued events", len(drained))
	}
}

func TestTrimQueuedDesktopInputsDropsOldestMovesButKeepsStateBoundaries(t *testing.T) {
	items := make([]queuedDesktopInput, 0, desktopInputQueueSoftLimit+96)
	items = append(items, queuedDesktopInput{ID: 1, Event: desktopInputEvent{Type: "pointer", Action: "down", Button: "left", ClientInputID: "down"}})
	for index := 0; index < desktopInputQueueSoftLimit+92; index++ {
		items = append(items, queuedDesktopInput{
			ID: int64(index + 2),
			Event: desktopInputEvent{
				Type:          "pointer",
				Action:        "move",
				X:             index,
				Y:             index,
				ClientInputID: "move:" + strconv.Itoa(index),
			},
		})
	}
	items = append(items, queuedDesktopInput{ID: int64(len(items) + 1), Event: desktopInputEvent{Type: "pointer", Action: "up", Button: "left", ClientInputID: "up"}})

	trimmed := trimQueuedDesktopInputs(items, desktopInputQueueSoftLimit)
	if len(trimmed) != desktopInputQueueSoftLimit {
		t.Fatalf("expected exact soft limit after dropping free moves, got %d", len(trimmed))
	}
	if trimmed[0].Event.ClientInputID != "down" || trimmed[len(trimmed)-1].Event.ClientInputID != "up" {
		t.Fatalf("state boundaries changed: first=%q last=%q", trimmed[0].Event.ClientInputID, trimmed[len(trimmed)-1].Event.ClientInputID)
	}
	if trimmed[1].Event.ClientInputID != "move:94" {
		t.Fatalf("expected oldest 94 free moves to be dropped, first retained move=%q", trimmed[1].Event.ClientInputID)
	}
}

func TestDesktopInputQueueDeduplicatesRetriedClientEventsAfterDelivery(t *testing.T) {
	queue := newDesktopInputQueue()
	events := []desktopInputEvent{
		{Type: "text", Text: "RemoteIt", ClientInputID: "viewer-a:41"},
		{Type: "key", Action: "up", KeyCode: 16, ClientInputID: "viewer-a:42"},
	}
	firstID := queue.enqueue(events)
	delivered := queue.drain(64)
	if len(delivered) != 2 || firstID != delivered[1].ID {
		t.Fatalf("unexpected initial delivery: id=%d events=%#v", firstID, delivered)
	}

	retriedID := queue.enqueue(events)
	if retriedID != firstID {
		t.Fatalf("a retry must return its original id: first=%d retry=%d", firstID, retriedID)
	}
	if duplicate := queue.drain(64); len(duplicate) != 0 {
		t.Fatalf("a response-loss retry duplicated remote input: %#v", duplicate)
	}
}

func TestDesktopClientInputIDValidation(t *testing.T) {
	for _, valid := range []string{"", "viewer-a:42", "01J_remote.input-9"} {
		if !validDesktopClientInputID(valid) {
			t.Fatalf("expected a valid client input id: %q", valid)
		}
	}
	for _, invalid := range []string{"contains space", "slash/value", string(make([]byte, 97))} {
		if validDesktopClientInputID(invalid) {
			t.Fatalf("expected an invalid client input id: %q", invalid)
		}
	}
}

func TestDesktopInputBatchParsesAcknowledgementIDAndStableEvents(t *testing.T) {
	batch, err := desktopInputBatchFromJSON([]byte(`{"batchId":"viewer-a:b17","events":[{"type":"pointer","action":"down","button":"left","x":20,"y":30,"clientInputId":"viewer-a:41"},{"type":"pointer","action":"up","button":"left","x":20,"y":30,"clientInputId":"viewer-a:42"}]}`))
	if err != nil {
		t.Fatalf("expected a valid acknowledged batch: %v", err)
	}
	if batch.BatchID != "viewer-a:b17" || len(batch.Events) != 2 || batch.Events[0].ClientInputID != "viewer-a:41" || batch.Events[1].Action != "up" {
		t.Fatalf("unexpected parsed input batch: %#v", batch)
	}
}

func TestDesktopInputBatchRejectsUnsafeAcknowledgementID(t *testing.T) {
	if _, err := desktopInputBatchFromJSON([]byte(`{"batchId":"viewer a/b","events":[{"type":"sas","clientInputId":"viewer-a:1"}]}`)); err == nil {
		t.Fatal("unsafe batch acknowledgement id was accepted")
	}
}

func TestDesktopInputQueueRestorePreservesActionsAndNewestMove(t *testing.T) {
	queue := newDesktopInputQueue()
	queue.enqueue([]desktopInputEvent{
		{Type: "pointer", Action: "move", X: 10, Y: 20},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "pointer", Action: "down", Button: "left", X: 10, Y: 20},
	})
	drained := queue.drain(64)
	if len(drained) != 3 {
		t.Fatalf("unexpected drained batch: %#v", drained)
	}
	queue.enqueue([]desktopInputEvent{
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "up", Button: "left", X: 30, Y: 40},
	})
	queue.restore(drained)
	restored := queue.drain(64)
	if len(restored) != 5 {
		t.Fatalf("expected moves on opposite sides of a pointer barrier to be retained, got %#v", restored)
	}
	if restored[0].Event.Action != "move" || restored[0].Event.X != 10 || restored[1].Event.Type != "key" || restored[2].Event.Action != "down" || restored[3].Event.Action != "move" || restored[3].Event.X != 30 || restored[4].Event.Action != "up" {
		t.Fatalf("restored input order changed: %#v", restored)
	}
	if restored[0].ID != drained[0].ID || restored[1].ID != drained[1].ID || restored[2].ID != drained[2].ID {
		t.Fatalf("restore must retain original input IDs: %#v", restored)
	}
}

func TestDesktopInputQueueNewestReconnectOwnsSerializedDelivery(t *testing.T) {
	queue := newDesktopInputQueue()
	first := queue.claimInputOwner()
	if !queue.lockInputDelivery(first) {
		t.Fatal("the initial input connection must own delivery")
	}

	second := queue.claimInputOwner()
	acquired := make(chan bool, 1)
	go func() {
		acquired <- queue.lockInputDelivery(second)
	}()
	select {
	case <-acquired:
		t.Fatal("replacement input delivery overlapped the in-flight batch")
	case <-time.After(20 * time.Millisecond):
	}

	queue.unlockInputDelivery()
	select {
	case current := <-acquired:
		if !current {
			t.Fatal("the replacement connection did not become the current owner")
		}
		queue.unlockInputDelivery()
	case <-time.After(time.Second):
		t.Fatal("replacement input delivery did not resume after the old batch")
	}

	if queue.lockInputDelivery(first) {
		queue.unlockInputDelivery()
		t.Fatal("the superseded connection regained ownership")
	}
}

func TestDesktopRuntimeCleanupRemovesEverySessionObject(t *testing.T) {
	s := &server{}
	sessionID := "session-1"
	deviceID := "device-1"
	s.storeDesktopRuntime(sessionID, desktopSessionRuntimeState{DeviceID: deviceID, Control: true, TargetFPS: 30, CursorVisible: true, ValidatedAt: time.Now().UTC()})
	s.desktopFrames.Store(sessionID, desktopFrameState{Frame: []byte{1, 2, 3}, DeviceID: deviceID, At: time.Now().UTC()})
	s.desktopFrameLanes.Store(sessionID+"\x000", desktopFrameState{Frame: []byte{1, 2, 3}, DeviceID: deviceID, At: time.Now().UTC()})
	s.desktopAgentSeen.Store(sessionID, time.Now().UTC())
	s.desktopInputAcks.Store(sessionID, desktopInputAck{ID: 7, Type: "sas", At: time.Now().UTC()})
	s.desktopClipboardAcks.Store(sessionID, desktopInputAck{ID: 8, Type: "clipboard_read", Value: "test", At: time.Now().UTC()})
	s.desktopViewerTouches.Store(sessionID, time.Now().UTC())
	s.desktopSessionAccess.Store(sessionID+"\x00browser-session", cachedDesktopSessionAccess{DeviceID: deviceID, CheckedAt: time.Now().UTC()})
	s.desktopInputQueues.Store(sessionID, newDesktopInputQueue())

	s.deleteDesktopFrame(sessionID)

	for name, state := range map[string]*sync.Map{
		"frames": &s.desktopFrames, "frame lanes": &s.desktopFrameLanes, "agent seen": &s.desktopAgentSeen, "input acks": &s.desktopInputAcks, "clipboard acks": &s.desktopClipboardAcks, "viewer touches": &s.desktopViewerTouches,
		"runtime": &s.desktopSessionRuntime, "access": &s.desktopSessionAccess, "queues": &s.desktopInputQueues,
	} {
		found := false
		state.Range(func(_, _ any) bool { found = true; return false })
		if found {
			t.Fatalf("%s state was not removed", name)
		}
	}
	if _, ok := s.desktopDeviceSessions.Load(deviceID); ok {
		t.Fatal("device to session mapping was not removed")
	}
}

func TestDesktopRuntimeCarriesCursorPolicy(t *testing.T) {
	s := &server{}
	runtime := desktopSessionRuntimeState{DeviceID: "device", Control: true, TargetFPS: 60, CursorVisible: true, ValidatedAt: time.Now().UTC()}
	s.storeDesktopRuntime("session", runtime)
	value, ok := s.desktopSessionRuntime.Load("session")
	if !ok {
		t.Fatal("desktop runtime was not stored")
	}
	stored := value.(desktopSessionRuntimeState)
	if !stored.CursorVisible || stored.TargetFPS != 60 || stored.DeviceID != "device" {
		t.Fatalf("desktop runtime lost cursor policy: %#v", stored)
	}
}

func TestDesktopAgentStreamLaneSupportsRollingUpgrade(t *testing.T) {
	tests := []struct {
		name       string
		rawLane    string
		videoOnly  bool
		wantLane   int
		wantInput  bool
		wantReject bool
	}{
		{name: "legacy single lane", rawLane: "", wantLane: 0, wantInput: true},
		{name: "old primary lane", rawLane: "0", wantLane: 0, wantInput: true},
		{name: "new primary video lane", rawLane: "0", videoOnly: true, wantLane: 0},
		{name: "auxiliary video lane", rawLane: "5", videoOnly: true, wantLane: 5},
		{name: "dedicated input", rawLane: "input", wantLane: -1, wantInput: true},
		{name: "trimmed dedicated input", rawLane: " input ", wantLane: -1, wantInput: true},
		{name: "negative lane", rawLane: "-1", wantReject: true},
		{name: "too high lane", rawLane: strconv.Itoa(desktopVideoLaneCount), wantReject: true},
		{name: "not a lane", rawLane: "video", wantReject: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lane, ownsInput, err := desktopAgentStreamLane(test.rawLane, test.videoOnly)
			if test.wantReject {
				if err == nil {
					t.Fatalf("lane %q must be rejected", test.rawLane)
				}
				return
			}
			if err != nil || lane != test.wantLane || ownsInput != test.wantInput {
				t.Fatalf("desktopAgentStreamLane(%q, %v) = (%d, %v, %v), want (%d, %v, nil)", test.rawLane, test.videoOnly, lane, ownsInput, err, test.wantLane, test.wantInput)
			}
		})
	}
}

func TestDesktopViewerTouchIsThrottled(t *testing.T) {
	s := &server{}
	now := time.Now().UTC()
	if !s.shouldTouchDesktopViewer("session", now) {
		t.Fatal("first viewer touch must reach the database")
	}
	if s.shouldTouchDesktopViewer("session", now.Add(500*time.Millisecond)) {
		t.Fatal("viewer touch inside one second must be throttled")
	}
	if !s.shouldTouchDesktopViewer("session", now.Add(1100*time.Millisecond)) {
		t.Fatal("viewer touch after one second must be accepted")
	}
}
