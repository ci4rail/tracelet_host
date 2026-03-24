package tracelet

import (
	"path/filepath"
	"testing"
)

func TestLoadReplayMessages(t *testing.T) {
	trackFile := filepath.Join("..", "..", "tracks", "HHAG-2025-12-17_track1.json")

	messages, err := loadReplayMessages(trackFile, "test-device", "192.168.7.248")
	if err != nil {
		t.Fatalf("loadReplayMessages returned error: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("loadReplayMessages returned no messages")
	}

	first := messages[0]
	if first.GetLocation() == nil {
		t.Fatal("first replay message has no location payload")
	}
	if first.GetTraceletId() != "test-device" {
		t.Fatalf("unexpected tracelet id %q", first.GetTraceletId())
	}
}
