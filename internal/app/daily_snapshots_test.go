package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"

	"github.com/egoist/mygo/ui"
)

// snapshotFileOf puts the days kept in a file of the test's own, and hands
// back where it is.
func snapshotFileOf(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "daily-snapshots.json")
	old := snapshotPath
	snapshotPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { snapshotPath = old })
	return path
}

// waitSnapshotWrite waits for the days kept to be written.
func waitSnapshotWrite(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the days kept were not written to %s", path)
}

// awaitSnapshotWrites waits for the writes begun to have all run, so the
// test's directory is not taken away under one.
func awaitSnapshotWrites(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshotWrites.Lock()
		done := snapshotWrites.last == snapshotGen.Load()
		snapshotWrites.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the writes of the days kept did not finish")
}

// The day's songs are of the day NetEase picked them, which changes at 6:00
// Beijing time.
func TestDailyDay(t *testing.T) {
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 11, 19, hour, minute, 0, 0, beijing)
	}
	if day := dailyDay(at(5, 59)); day != "2026-11-18" {
		t.Fatalf("before six: %q", day)
	}
	if day := dailyDay(at(6, 0)); day != "2026-11-19" {
		t.Fatalf("at six: %q", day)
	}
}

// The days kept are read again the next time the app runs, newest first, and
// only some sixty of them stay.
func TestSnapshotsKeptAndTrimmed(t *testing.T) {
	a := newTestApp()
	path := snapshotFileOf(t)
	a.snaps.loaded = false
	a.profile = &netease.Profile{UserID: 1, Nickname: "me"}

	at := time.Date(2026, 11, 19, 12, 0, 0, 0, beijing)
	for i := range snapshotDays + 1 {
		day := at.AddDate(0, 0, -i).Format("2006-01-02")
		a.keepDay(day, []netease.Track{{ID: int64(i + 1), Name: "歌"}}, at)
	}
	waitSnapshotWrite(t, path)
	defer awaitSnapshotWrites(t)

	b := newTestApp()
	b.snaps.loaded = false
	b.loadSnapshots()
	days := b.snapshotDates(1)
	if len(days) != snapshotDays {
		t.Fatalf("kept %d days", len(days))
	}
	oldest := at.AddDate(0, 0, -(snapshotDays - 1)).Format("2006-01-02")
	if days[0] != "2026-11-19" || days[len(days)-1] != oldest {
		t.Fatalf("days %v … %v, want %v … %v", days[0], days[len(days)-1], "2026-11-19", oldest)
	}
	if got := b.snapshotSongs("2026-11-19", 1); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("songs %+v", got)
	}
	if got := b.snapshotSongs("2026-11-19", 2); got != nil {
		t.Fatalf("another account's day: %+v", got)
	}
}

// The history page shows the days Cloudy kept itself that NetEase no longer
// lists, and opens them without asking NetEase.
func TestKeptDayOpensFromRecord(t *testing.T) {
	oldSongs := historySongs
	t.Cleanup(func() { historySongs = oldSongs })
	historySongs = func(day string) ([]netease.Track, error) {
		t.Fatalf("asked NetEase for %s, a day Cloudy kept itself", day)
		return nil, nil
	}

	a := newTestApp()
	a.snaps.loaded = true
	a.profile = &netease.Profile{UserID: 1, Nickname: "me"}
	a.snaps.days = map[string]snapshot{
		"2026-11-10": {User: 1, At: 1, Songs: []netease.Track{{ID: 9, Name: "旧歌", Privilege: &netease.Privilege{ID: 9, St: 0, Pl: 320000}}}},
	}
	days, local := a.keptDays([]string{"2026-11-18"})
	if len(days) != 2 || days[0] != "2026-11-18" || !local["2026-11-10"] {
		t.Fatalf("days %v, local %v", days, local)
	}
	a.hist = historyDaily{forUser: 1, asked: true, days: days, local: local, note: "近 5 次", date: "2026-11-10"}
	a.pickHistoryDay("2026-11-10")
	a.route = route{page: pageHistoryDaily}

	tt := ui.NewTester(a.view, 1200, 900)
	tt.Advance(2 * time.Second)
	if !tt.HasText("11/10 二") {
		t.Fatal("want the day Cloudy kept, among the days NetEase keeps")
	}
	if !tt.HasText("旧歌") {
		t.Fatal("want the songs Cloudy kept of that day")
	}
	if !tt.HasText("这天来自 Cloudy 自己记下的每日推荐") {
		t.Fatal("want the page to say where the day is from")
	}
}
