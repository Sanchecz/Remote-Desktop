//go:build windows

package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestPublicAgentJournalIsSanitizedDeduplicatedAndBounded(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	requestedUserInstall = false

	appendPublicAgentEvent("unknown", "network\nchange", "Сеть\r\nобновлена", "Новый маршрут\nприменён")
	appendPublicAgentEvent("info", "network change", "Сеть  обновлена", "Новый маршрут применён")
	events, err := loadPublicAgentEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("deduplicated events=%d, want 1", len(events))
	}
	if events[0].Level != "info" {
		t.Fatalf("normalized level=%q, want info", events[0].Level)
	}
	for _, value := range []string{events[0].Kind, events[0].Title, events[0].Detail} {
		if strings.ContainsAny(value, "\r\n") {
			t.Fatalf("public journal contains a line break: %q", value)
		}
	}

	for index := 0; index < maxPublicAgentEvents+7; index++ {
		appendPublicAgentEvent("success", "service", fmt.Sprintf("Событие %03d", index), "Проверенное событие")
	}
	events, err = loadPublicAgentEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != maxPublicAgentEvents {
		t.Fatalf("bounded events=%d, want %d", len(events), maxPublicAgentEvents)
	}
	if events[len(events)-1].Title != fmt.Sprintf("Событие %03d", maxPublicAgentEvents+6) {
		t.Fatalf("last event=%q", events[len(events)-1].Title)
	}
}
