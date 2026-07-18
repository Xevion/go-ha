package hatest

import (
	"runtime"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A websocket never goes idle, so httptest's Close does not reap the goroutine
// reading it. Close has to shut the connection itself, and without waiting for
// a graceful reply the peer is not going to send.
func TestCloseReleasesTheConnectionGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()

	s := New(t)
	ws, _, err := websocket.Dial(t.Context(), s.URL()+"/api/websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	// Complete the handshake so the server enters its read loop.
	if _, _, err := ws.Read(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := ws.Write(t.Context(), websocket.MessageText,
		[]byte(`{"type":"auth","access_token":"`+Token+`"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.Read(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	s.Close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Close took %s, which means it waited out a close handshake", elapsed)
	}

	for range 100 {
		if runtime.NumGoroutine() <= before+1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines before=%d after=%d: the read loop outlived Close",
		before, runtime.NumGoroutine())
}

func TestCloseIsIdempotent(t *testing.T) {
	s := New(t)
	s.Close()
	s.Close()
}
