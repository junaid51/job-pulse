package api

import (
	"testing"
	"time"
)

// The cursor has to survive a round trip exactly, including the id half: without
// it, a page of matches that all share one created_at loses every tied row.
func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 19, 6, 20, 36, 746449000, time.UTC)
	const id int64 = 1447

	gotAt, gotID, err := parseCursor(formatCursor(at, id))
	if err != nil {
		t.Fatalf("parseCursor(formatCursor(...)) failed: %v", err)
	}
	if !gotAt.Equal(at) {
		t.Errorf("timestamp = %v, want %v", gotAt, at)
	}
	if gotID != id {
		t.Errorf("id = %d, want %d", gotID, id)
	}
}

func TestParseCursorRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "yesterday", "2026-08-19T06:20:36Z", "2026-08-19T06:20:36Z,abc", ",12"} {
		if _, _, err := parseCursor(raw); err == nil {
			t.Errorf("parseCursor(%q) succeeded, want an error", raw)
		}
	}
}
