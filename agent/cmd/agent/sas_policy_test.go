package main

import "testing"

func TestDesiredSoftwareSASGeneration(t *testing.T) {
	tests := []struct {
		name    string
		current uint64
		exists  bool
		want    uint32
		changed bool
	}{
		{name: "missing", exists: false, want: 1, changed: true},
		{name: "disabled", current: 0, exists: true, want: 1, changed: true},
		{name: "services", current: 1, exists: true, want: 1, changed: false},
		{name: "accessibility", current: 2, exists: true, want: 3, changed: true},
		{name: "both", current: 3, exists: true, want: 3, changed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, changed, err := desiredSoftwareSASGeneration(test.current, test.exists)
			if err != nil || got != test.want || changed != test.changed {
				t.Fatalf("desiredSoftwareSASGeneration(%d, %v) = %d, %v, %v; want %d, %v, nil", test.current, test.exists, got, changed, err, test.want, test.changed)
			}
		})
	}
}

func TestDesiredSoftwareSASGenerationRejectsUnknownPolicy(t *testing.T) {
	if _, _, err := desiredSoftwareSASGeneration(4, true); err == nil {
		t.Fatal("unknown policy value must not be overwritten")
	}
}

func TestWindowsSASEventNameIncludesSessionAndKind(t *testing.T) {
	if got := windowsSASEventName("Request", 17); got != `Global\RemoteIt-SAS-Request-17` {
		t.Fatalf("unexpected event name %q", got)
	}
}
