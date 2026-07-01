package claudep

import (
	"context"
	"testing"
	"time"
)

func TestAwaitTurnAtCompletes(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn"}}`,
		`{"type":"system","subtype":"turn_duration"}`,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	text, done, err := awaitTurnAt(ctx, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !done || text != "hello" {
		t.Fatalf("want done=true text=hello, got done=%v text=%q", done, text)
	}
}

// A prior turn's turn_duration can still be flushing just past the offset. The
// wait must NOT fire on it - it must wait for the terminal event that follows
// THIS turn's user echo. If the sawUserTurn guard regressed, this would return
// on the leading marker with empty text.
func TestAwaitTurnAtIgnoresStaleMarker(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"system","subtype":"turn_duration"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"real answer"}],"stop_reason":"end_turn"}}`,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	text, done, err := awaitTurnAt(ctx, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !done || text != "real answer" {
		t.Fatalf("want done=true text=%q (must skip the stale marker), got done=%v text=%q", "real answer", done, text)
	}
}

func TestAwaitTurnAtTimesOut(t *testing.T) {
	// A turn still in progress: user echo present, but no terminal event yet.
	path := writeTranscript(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, done, err := awaitTurnAt(ctx, path, 0)
	if err != nil {
		t.Fatalf("timeout should not be an error, got %v", err)
	}
	if done {
		t.Fatal("want done=false while the turn is still running")
	}
}
