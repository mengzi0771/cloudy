package recognition

import (
	"testing"
	"time"

	ncm "github.com/XiaoMengXinX/NeteaseCloudMusic-Audio-Recognize-Reverse"
)

// Recognition results carry catalog song IDs and use a different metadata shape from song details.
func TestRecognitionResults(t *testing.T) {
	results, err := matches(&ncm.Response{Code: 200, Data: []byte(`{"result":[
		{"startTime":45020,"song":{"id":19,"name":"Test Song","artists":[{"id":1,"name":"Artist"}],"album":{"id":2,"picUrl":"https://example.test/cover.jpg"},"duration":200000}},
		{"song":{"id":19,"name":"Duplicate"}},
		{"song":{"name":"No catalog ID"}},
		{"song":{"id":25,"name":"Another candidate"}},
		{"startTime":-180,"song":{"id":26,"name":"At the beginning"}}
	]}`)})
	if err != nil || len(results) != 3 {
		t.Fatalf("results=%+v, err=%v", results, err)
	}
	if song := results[0].Track; song.ID != 19 || song.Duration != 200000 || song.ArtistNames() != "Artist" || song.Album.PicURL == "" || results[1].Track.ID != 25 {
		t.Fatalf("lost playable metadata or changed result order: %+v", results)
	}
	if at := results[0].Start; at == nil || *at != 45020*time.Millisecond {
		t.Fatalf("expected a 45.020s match, got %v", at)
	}
	if results[1].Start != nil {
		t.Fatal("a missing match position became an explicit seek")
	}
	if at := results[2].Start; at == nil || *at != 0 {
		t.Fatalf("a negative position should seek to the beginning, got %v", at)
	}
}
