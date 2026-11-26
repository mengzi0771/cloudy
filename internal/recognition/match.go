package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	ncm "github.com/XiaoMengXinX/NeteaseCloudMusic-Audio-Recognize-Reverse"

	"netease-music-mygo/internal/netease"
)

const Seconds = ncm.FingerprintSeconds

var ErrNoAudio = errors.New("no audio on the default playback device")

// Result is a catalog track and where the captured clip starts in it, if known.
type Result struct {
	Track netease.Track
	Start *time.Duration
}

// Match computes the fingerprint locally and sends it to NetEase.
func Match(ctx context.Context, pcm []float32, rate int) ([]Result, error) {
	client := ncm.Client{Duration: Seconds}
	response, err := client.RecognizePCM(ctx, pcm, rate, 0)
	if err != nil {
		return nil, err
	}
	return matches(response)
}

func matches(response *ncm.Response) ([]Result, error) {
	if response.Code != 200 {
		return nil, fmt.Errorf("NetEase audio match: code %d", response.Code)
	}
	var data struct {
		Result []struct {
			StartTime *int64 `json:"startTime"`
			Song      struct {
				ID        int64              `json:"id"`
				Name      string             `json:"name"`
				Artists   []netease.Artist   `json:"artists"`
				Album     netease.Album      `json:"album"`
				Duration  int64              `json:"duration"`
				Fee       int                `json:"fee"`
				Privilege *netease.Privilege `json:"privilege"`
			} `json:"song"`
		} `json:"result"`
	}
	if len(response.Data) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, err
	}
	var results []Result
	seen := make(map[int64]bool)
	for _, result := range data.Result {
		song := result.Song
		if song.ID <= 0 || song.Name == "" || seen[song.ID] {
			continue
		}
		seen[song.ID] = true
		track := netease.Track{
			ID: song.ID, Name: song.Name, Artists: song.Artists, Album: song.Album,
			Duration: song.Duration, Fee: song.Fee, Privilege: song.Privilege,
		}
		match := Result{Track: track}
		if result.StartTime != nil {
			ms := max(0, *result.StartTime)
			if (track.Duration <= 0 || ms < track.Duration) && ms <= int64(math.MaxInt64/time.Millisecond) {
				start := time.Duration(ms) * time.Millisecond
				match.Start = &start
			}
		}
		results = append(results, match)
		if len(results) == 3 {
			break
		}
	}
	return results, nil
}
