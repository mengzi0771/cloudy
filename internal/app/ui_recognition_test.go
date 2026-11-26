package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/recognition"
)

// Opening the dialog does not capture audio, and dismissing it cancels an active request.
func TestSongRecognitionDismissesCapture(t *testing.T) {
	if !recognition.Available {
		t.Skip("system audio capture is available on Windows")
	}
	a := newTestApp()
	tt := ui.NewTester(a.view, 900, 600)
	tt.SetScale(0.25)
	if err := tt.Click("听歌识曲"); err != nil {
		t.Fatal(err)
	}
	if !a.songRecognition.open || a.songRecognition.cancel != nil {
		t.Fatal("opening the dialog should wait for the start action")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.songRecognition.request, a.songRecognition.cancel = ctx, cancel
	settle(tt)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.songRecognition.open || ctx.Err() != context.Canceled {
		t.Fatal("Escape should close the dialog and cancel recognition")
	}
}

// Playing a match starts at the recognized position even when it is the current song.
func TestSongRecognitionPlaysMatchedPosition(t *testing.T) {
	if !recognition.Available {
		t.Skip("system audio capture is available on Windows")
	}
	a := newTestApp()
	track := netease.Track{ID: 19, Name: "Test song", Duration: 120000}
	a.pl.Cue([]netease.Track{track})
	a.pl.Hold(5 * time.Second)
	at := 45020 * time.Millisecond
	a.songRecognition.open = true
	a.songRecognition.results = []recognition.Result{{Track: track, Start: &at}}
	tt := ui.NewTester(a.view, 900, 600)
	tt.SetScale(0.25)
	settle(tt)
	if err := tt.Click("从 0:45 播放 Test song"); err != nil {
		t.Fatal(err)
	}
	if a.songRecognition.open || a.pl.Position() != at {
		t.Fatalf("click left dialog open=%v, position=%v, want %v", a.songRecognition.open, a.pl.Position(), at)
	}
}
