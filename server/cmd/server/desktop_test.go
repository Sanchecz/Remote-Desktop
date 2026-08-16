package main

import (
	"sync"
	"testing"
	"time"
)

func TestValidDesktopInput(t *testing.T) {
	valid := []desktopInputEvent{
		{Type: "pointer", Action: "move", X: 120, Y: 80},
		{Type: "pointer", Action: "down", Button: "left", X: 1, Y: 2},
		{Type: "pointer", Action: "up", Button: "right", X: 1, Y: 2},
		{Type: "wheel", Delta: -120},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "text", Text: "RemoteIt: Привет, Shift: ? : \" + _ ( ) ! 👋"},
	}
	for _, event := range valid {
		if !validDesktopInput(event) {
			t.Fatalf("expected valid event: %#v", event)
		}
	}
	invalid := []desktopInputEvent{
		{Type: "pointer", Action: "down", Button: "unknown"},
		{Type: "pointer", Action: "move", X: -1},
		{Type: "wheel", Delta: 9999},
		{Type: "key", Action: "down", KeyCode: 0},
		{Type: "text"},
		{Type: "text", Text: string(make([]rune, 129))},
		{Type: "shell"},
	}
	for _, event := range invalid {
		if validDesktopInput(event) {
			t.Fatalf("expected invalid event: %#v", event)
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

func TestDesktopInputQueuePreservesActionsAndCoalescesPointerMoves(t *testing.T) {
	queue := newDesktopInputQueue()
	queue.enqueue([]desktopInputEvent{
		{Type: "pointer", Action: "move", X: 10, Y: 20},
		{Type: "key", Action: "down", KeyCode: 65},
		{Type: "text", Text: "?:Я+"},
		{Type: "pointer", Action: "move", X: 30, Y: 40},
		{Type: "pointer", Action: "down", Button: "left", X: 30, Y: 40},
		{Type: "pointer", Action: "up", Button: "left", X: 30, Y: 40},
	})

	items := queue.drain(64)
	if len(items) != 5 {
		t.Fatalf("expected the stale pointer move to be coalesced, got %d events", len(items))
	}
	if items[0].Event.Type != "key" || items[1].Event.Type != "text" || items[1].Event.Text != "?:Я+" || items[2].Event.Action != "move" || items[2].Event.X != 30 || items[3].Event.Action != "down" || items[4].Event.Action != "up" {
		t.Fatalf("unexpected input order after coalescing: %#v", items)
	}
	for index := 1; index < len(items); index++ {
		if items[index].ID <= items[index-1].ID {
			t.Fatalf("input identifiers must remain monotonic: %#v", items)
		}
	}
}

func TestDesktopInputQueueIsBoundedAndDrainsInBatches(t *testing.T) {
	queue := newDesktopInputQueue()
	for index := 0; index < 140; index++ {
		queue.enqueue([]desktopInputEvent{{Type: "key", Action: "down", KeyCode: 65}})
	}
	first := queue.drain(64)
	second := queue.drain(64)
	third := queue.drain(64)
	if len(first) != 64 || len(second) != 56 || len(third) != 0 {
		t.Fatalf("expected a bounded 120 event queue, got batches %d/%d/%d", len(first), len(second), len(third))
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
	s.desktopViewerTouches.Store(sessionID, time.Now().UTC())
	s.desktopSessionAccess.Store(sessionID+"\x00browser-session", cachedDesktopSessionAccess{DeviceID: deviceID, CheckedAt: time.Now().UTC()})
	s.desktopInputQueues.Store(sessionID, newDesktopInputQueue())

	s.deleteDesktopFrame(sessionID)

	for name, state := range map[string]*sync.Map{
		"frames": &s.desktopFrames, "frame lanes": &s.desktopFrameLanes, "agent seen": &s.desktopAgentSeen, "viewer touches": &s.desktopViewerTouches,
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
