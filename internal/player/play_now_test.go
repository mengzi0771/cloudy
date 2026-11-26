package player

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"netease-music-mygo/internal/netease"
)

// A song picked from a search plays at once, right after the one that was
// playing, and what was to come stays as it was rather than the search's
// results replacing the queue; one queued already moves up, and picking the
// song playing leaves it playing.
func TestPlayNowKeepsTheQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		quietNavigation(t)
		songURL = func(context.Context, int64, string) (*netease.Stream, error) { return nil, context.DeadlineExceeded }
		p, ch := navigationPlayer(t, []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
		p.at, p.track = 1, &p.queue[1]

		p.PlayNow(netease.Track{ID: 9})
		settleRetries(ch)
		if got := playOrder(p); !slices.Equal(got, []int64{1, 2, 9, 3, 4}) {
			t.Fatalf("order %v, want 1 2 9 3 4", got)
		}
		if p.Current().ID != 9 {
			t.Fatalf("playing %d, want 9", p.Current().ID)
		}

		p.PlayNow(netease.Track{ID: 4})
		settleRetries(ch)
		if got := playOrder(p); !slices.Equal(got, []int64{1, 2, 9, 4, 3}) {
			t.Fatalf("order %v, want 1 2 9 4 3", got)
		}
		if p.Current().ID != 4 {
			t.Fatalf("playing %d, want 4", p.Current().ID)
		}

		p.PlayNow(netease.Track{ID: 4})
		settleRetries(ch)
		if got := playOrder(p); !slices.Equal(got, []int64{1, 2, 9, 4, 3}) || p.Current().ID != 4 {
			t.Fatalf("picking the song playing changed things: order %v, playing %d", got, p.Current().ID)
		}
	})
}

// With nothing queued, the song picked is the queue.
func TestPlayNowIntoAnEmptyQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		quietNavigation(t)
		songURL = func(context.Context, int64, string) (*netease.Stream, error) { return nil, context.DeadlineExceeded }
		ch := make(chan func(), 1024)
		p := &Player{post: func(fn func()) { ch <- fn }, at: -1, volume: 0, gain: 1}
		t.Cleanup(p.stop)
		p.PlayNow(netease.Track{ID: 7})
		settleRetries(ch)
		if got := playOrder(p); !slices.Equal(got, []int64{7}) || p.Current() == nil || p.Current().ID != 7 {
			t.Fatalf("order %v, current %v", got, p.Current())
		}
	})
}

// Playing an identified song keeps its start position during loading and preserves the queue.
func TestPlayNowAtWhileLoading(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		quietNavigation(t)
		songURL = func(context.Context, int64, string) (*netease.Stream, error) { return nil, context.DeadlineExceeded }
		p, ch := navigationPlayer(t, []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}})
		p.PlayNowAt(netease.Track{ID: 3}, 45*time.Second)
		settleRetries(ch)
		if got := playOrder(p); !slices.Equal(got, []int64{1, 3, 2}) || p.Current().ID != 3 || p.Position() != 45*time.Second {
			t.Fatalf("order=%v, current=%v, position=%v", got, p.Current(), p.Position())
		}
		p.PlayNowAt(netease.Track{ID: 3}, 0)
		if p.Position() != 0 {
			t.Fatalf("explicit start did not replace the pending seek: %v", p.Position())
		}
	})
}
