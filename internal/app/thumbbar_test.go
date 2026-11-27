package app

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"netease-music-mygo/internal/netease"
)

// Each glyph is a PNG of its own, ink on nothing, of the size Windows
// scales from.
func TestThumbGlyphs(t *testing.T) {
	ink := color.NRGBA{255, 255, 255, 255}
	seen := map[string]glyph{}
	for g := glyphPrev; g <= glyphHeartFull; g++ {
		data := thumbGlyph(g, ink)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("glyph %d: %v", g, err)
		}
		if b := img.Bounds(); b.Dx() != thumbSize || b.Dy() != thumbSize {
			t.Errorf("glyph %d is %v", g, b)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("glyph %d: corner not clear", g)
		}
		if _, _, _, a := img.At(thumbSize/2, thumbSize/2).RGBA(); g != glyphPause && g != glyphHeart && a == 0 {
			t.Errorf("glyph %d: middle not drawn", g)
		}
		if other, ok := seen[string(data)]; ok {
			t.Errorf("glyphs %d and %d are the same", other, g)
		}
		seen[string(data)] = g
	}
}

// Before a song is chosen there are no buttons to show, and no song to
// read them from.
func TestThumbsWithoutSong(t *testing.T) {
	a := newTestApp()
	if a.hasSong() {
		t.Fatal("a song before any was cued")
	}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "七"}})
	if !a.hasSong() {
		t.Fatal("no song once cued")
	}
}

// The heart not liked is an outline, clear inside; the liked one full.
func TestThumbHearts(t *testing.T) {
	ink := color.NRGBA{255, 255, 255, 255}
	at := func(g glyph) uint32 {
		img, _ := png.Decode(bytes.NewReader(thumbGlyph(g, ink)))
		_, _, _, a := img.At(thumbSize/2, thumbSize/2).RGBA()
		return a
	}
	if a := at(glyphHeart); a != 0 {
		t.Errorf("outline heart drawn inside: alpha %d", a)
	}
	if a := at(glyphHeartFull); a == 0 {
		t.Error("full heart clear inside")
	}
}

// The window is titled with the song and its artists, as NetEase's is, and
// ends with the app's name, which apps listing windows take for its name.
func TestSongTitle(t *testing.T) {
	if got := songTitle("痴人说梦", "HOYO-MiX", "Cloudy"); got != "痴人说梦 - HOYO-MiX - Cloudy" {
		t.Errorf("title %q", got)
	}
	if got := songTitle("一", "", "Cloudy"); got != "一 - Cloudy" {
		t.Errorf("title without artists %q", got)
	}
	if got := songTitle("一", "某人", ""); got != "一 - 某人" {
		t.Errorf("title without the app's name %q", got)
	}
}

// Replacing another song's cover resets the thumbnail first so Windows 10
// remeasures it, even when the cover arrived after the song switched (#89).
func TestThumbNeedsReset(t *testing.T) {
	cases := []struct {
		name    string
		shownOf int64
		track   int64
		want    bool
	}{
		{name: "cover arrives after track switch", shownOf: 1, track: 2, want: true},
		{name: "first cover shown", shownOf: 0, track: 1, want: false},
		{name: "same track cover refreshed", shownOf: 1, track: 1, want: false},
		{name: "window currently shown", shownOf: 0, track: 2, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := thumbNeedsReset(tc.shownOf, tc.track); got != tc.want {
				t.Errorf("thumbNeedsReset(%d, %d) = %v, want %v", tc.shownOf, tc.track, got, tc.want)
			}
		})
	}
}
