package main

import (
	"encoding/binary"
	"testing"
)

func TestDesktopInputEventsFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		count   int
	}{
		{name: "single", payload: `{"type":"pointer","action":"move","x":10,"y":20}`, count: 1},
		{name: "batch", payload: `{"events":[{"type":"pointer","action":"down","button":"left","x":10,"y":20},{"type":"pointer","action":"up","button":"left","x":10,"y":20}]}`, count: 2},
		{name: "shift symbols and unicode", payload: `{"events":[{"type":"key","action":"down","keyCode":16},{"type":"text","text":"?:Я+👋"},{"type":"key","action":"up","keyCode":16}]}`, count: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events, err := desktopInputEventsFromJSON([]byte(test.payload))
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}
			if len(events) != test.count {
				t.Fatalf("got %d events, want %d", len(events), test.count)
			}
		})
	}
}

func TestDesktopInputEventsFromJSONRejectsInvalid(t *testing.T) {
	for _, payload := range []string{"", `{}`, `{"events":[]}`, `{"type":"pointer","action":"down","button":"invalid"}`, `{"type":"key","action":"down","keyCode":999}`} {
		if _, err := desktopInputEventsFromJSON([]byte(payload)); err == nil {
			t.Fatalf("payload %q should be rejected", payload)
		}
	}
}

func TestParseDesktopFrameMessageSupportsLegacyAndDiagnostics(t *testing.T) {
	jpeg := make([]byte, 120)
	for i := range jpeg {
		jpeg[i] = byte(i)
	}
	legacy := make([]byte, 12+len(jpeg))
	copy(legacy[:4], "RIT1")
	binary.BigEndian.PutUint32(legacy[4:8], 1920)
	binary.BigEndian.PutUint32(legacy[8:12], 1080)
	copy(legacy[12:], jpeg)
	parsed, err := parseDesktopFrameMessage(legacy)
	if err != nil {
		t.Fatalf("parse legacy: %v", err)
	}
	if parsed.Width != 1920 || parsed.Height != 1080 || parsed.HasDiagnostics || len(parsed.Frame) != len(jpeg) {
		t.Fatalf("unexpected legacy frame: %#v", parsed)
	}

	backend := []byte("dxgi")
	modern := make([]byte, 18+len(backend)+len(jpeg))
	copy(modern[:4], "RIT2")
	binary.BigEndian.PutUint32(modern[4:8], 2560)
	binary.BigEndian.PutUint32(modern[8:12], 1920)
	copy(modern[12:16], []byte{7, 2, 1, 11})
	modern[17] = byte(len(backend))
	copy(modern[18:], backend)
	copy(modern[18+len(backend):], jpeg)
	parsed, err = parseDesktopFrameMessage(modern)
	if err != nil {
		t.Fatalf("parse diagnostics: %v", err)
	}
	if parsed.Width != 2560 || parsed.Height != 1920 || !parsed.HasDiagnostics || parsed.Diagnostics.Backend != "dxgi" || parsed.Diagnostics.CaptureMillis != 7 || parsed.Diagnostics.EncodeMillis != 11 || len(parsed.Frame) != len(jpeg) {
		t.Fatalf("unexpected diagnostics frame: %#v", parsed)
	}

	sequenced := make([]byte, 26+len(backend)+len(jpeg))
	copy(sequenced[:4], "RIT3")
	binary.BigEndian.PutUint32(sequenced[4:8], 3840)
	binary.BigEndian.PutUint32(sequenced[8:12], 2160)
	copy(sequenced[12:16], []byte{4, 1, 2, 7})
	sequenced[17] = byte(len(backend))
	binary.BigEndian.PutUint64(sequenced[18:26], 42)
	copy(sequenced[26:], backend)
	copy(sequenced[26+len(backend):], jpeg)
	parsed, err = parseDesktopFrameMessage(sequenced)
	if err != nil {
		t.Fatalf("parse sequenced diagnostics: %v", err)
	}
	if parsed.Width != 3840 || parsed.Height != 2160 || parsed.ProducerSequence != 42 || parsed.Diagnostics.Backend != "dxgi" || len(parsed.Frame) != len(jpeg) {
		t.Fatalf("unexpected sequenced frame: %#v", parsed)
	}
}

func TestParseDesktopFrameMessageRejectsMalformedEnvelope(t *testing.T) {
	valid := make([]byte, 18+120)
	copy(valid[:4], "RIT2")
	binary.BigEndian.PutUint32(valid[4:8], 1920)
	binary.BigEndian.PutUint32(valid[8:12], 1080)
	for _, payload := range [][]byte{
		nil,
		append([]byte("NOPE"), make([]byte, 120)...),
		append(append([]byte(nil), valid[:17]...), make([]byte, 100)...),
		func() []byte { broken := append([]byte(nil), valid...); broken[17] = 49; return broken }(),
	} {
		if _, err := parseDesktopFrameMessage(payload); err == nil {
			t.Fatalf("malformed envelope of %d bytes was accepted", len(payload))
		}
	}
}

func TestDesktopViewerPayloadAndLaneSelection(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 1, 2, 3, 0xff, 0xd9}
	frame := desktopFrameState{Frame: jpeg, ProducerSequence: 42}
	if !desktopViewerLaneMatches(frame, 0) || desktopViewerLaneMatches(frame, 1) || desktopViewerLaneMatches(frame, 2) || desktopViewerLaneMatches(frame, 3) || desktopViewerLaneMatches(frame, 4) || desktopViewerLaneMatches(frame, 5) {
		t.Fatal("producer sequence 42 was not routed exclusively to lane 0")
	}
	frame.ProducerSequence = 43
	if desktopViewerLaneMatches(frame, 0) || !desktopViewerLaneMatches(frame, 1) || desktopViewerLaneMatches(frame, 2) || desktopViewerLaneMatches(frame, 3) || desktopViewerLaneMatches(frame, 4) || desktopViewerLaneMatches(frame, 5) {
		t.Fatal("producer sequence 43 was not routed exclusively to lane 1")
	}
	payload := desktopViewerPayload(frame, 1)
	if string(payload[:4]) != "RTV1" || binary.BigEndian.Uint64(payload[4:12]) != 43 || string(payload[12:]) != string(jpeg) {
		t.Fatalf("unexpected viewer envelope: %x", payload)
	}
	if legacy := desktopViewerPayload(frame, -1); string(legacy) != string(jpeg) {
		t.Fatalf("legacy payload changed: %x", legacy)
	}
	frame.ProducerSequence = 0
	if !desktopViewerLaneMatches(frame, 0) || desktopViewerLaneMatches(frame, 1) || desktopViewerLaneMatches(frame, 2) || desktopViewerLaneMatches(frame, 3) || desktopViewerLaneMatches(frame, 4) || desktopViewerLaneMatches(frame, 5) {
		t.Fatal("legacy frame must be routed to lane 0 only")
	}
	if desktopViewerLaneMatches(frame, -2) {
		t.Fatal("input-only channel must never carry a video frame")
	}
}

func TestDesktopViewerUsesAllLowLatencyLanes(t *testing.T) {
	for sequence := uint64(1); sequence <= desktopVideoLaneCount*3; sequence++ {
		frame := desktopFrameState{ProducerSequence: sequence}
		expected := int(sequence % desktopVideoLaneCount)
		for lane := 0; lane < desktopVideoLaneCount; lane++ {
			if got := desktopViewerLaneMatches(frame, lane); got != (lane == expected) {
				t.Fatalf("sequence %d lane %d match=%v, expected lane %d", sequence, lane, got, expected)
			}
		}
	}
}
