package app

// What Cloudy itself saw of the listener's daily recommendations is kept in
// daily-snapshots.json in the user data directory: the days the daily songs
// were opened on. NetEase keeps the last five only, and not forever, so the
// 历史日推 page shows those days from it, and the days NetEase no longer
// answers for from here. A day is of the account that saw it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// snapshotDays is how many days are kept: some thirty songs a day, a few
// kilobytes each, under a megabyte together.
const snapshotDays = 60

// snapshotVersion is the shape of the file; one newer than this is not read.
const snapshotVersion = 1

// snapshot is a day's songs as they were picked then.
type snapshot struct {
	User  int64           `json:"user"`
	At    int64           `json:"at"` // Unix milliseconds, when it was kept
	Songs []netease.Track `json:"songs"`
}

// snapshotFile is the shape of the file.
type snapshotFile struct {
	Version int                 `json:"version"`
	Days    map[string]snapshot `json:"days"`
}

// snapshots is what the app has of them: the days kept, once read.
type snapshots struct {
	loaded bool
	days   map[string]snapshot
}

// snapshotPath is where the days kept are; tests replace it.
var snapshotPath = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daily-snapshots.json"), nil
}

// loadSnapshots reads the days kept, once.
func (a *app) loadSnapshots() {
	s := &a.snaps
	if s.loaded {
		return
	}
	s.loaded = true
	path, err := snapshotPath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var f snapshotFile
	if json.Unmarshal(b, &f) != nil || f.Version > snapshotVersion || len(f.Days) == 0 {
		return
	}
	s.days = f.Days
}

// snapshotDates are the days kept of user, newest first.
func (a *app) snapshotDates(user int64) []string {
	a.loadSnapshots()
	out := make([]string, 0, len(a.snaps.days))
	for day, s := range a.snaps.days {
		if s.User == user && len(s.Songs) > 0 {
			out = append(out, day)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// snapshotSongs are the songs kept of day for user, if they were.
func (a *app) snapshotSongs(day string, user int64) []netease.Track {
	a.loadSnapshots()
	s, ok := a.snaps.days[day]
	if !ok || s.User != user {
		return nil
	}
	return s.Songs
}

// rememberDaily keeps the songs of the day they were picked for, off the main
// thread, if they are ones the page could show again.
func (a *app) rememberDaily(songs []netease.Track, at time.Time) {
	if a.profile == nil || len(songs) == 0 {
		return
	}
	a.keepDay(dailyDay(at), songs, time.Now())
}

// keepDay keeps the songs of the past daily recommendations of day, if they
// are not the ones kept already.
func (a *app) keepDay(day string, songs []netease.Track, now time.Time) {
	if a.profile == nil || len(songs) == 0 || day == "" {
		return
	}
	a.loadSnapshots()
	s := &a.snaps
	if s.days == nil {
		s.days = map[string]snapshot{}
	}
	if old, ok := s.days[day]; ok && sameSongs(old.Songs, songs) {
		return
	}
	s.days[day] = snapshot{User: a.profile.UserID, At: now.UnixMilli(), Songs: songs}
	a.trimSnapshots()
	a.saveSnapshots()
}

// sameSongs reports whether a and b are the songs in the same order.
func sameSongs(a, b []netease.Track) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].ProgramID != b[i].ProgramID {
			return false
		}
	}
	return true
}

// trimSnapshots keeps the newest snapshotDays days.
func (a *app) trimSnapshots() {
	days := a.snaps.days
	if len(days) <= snapshotDays {
		return
	}
	dates := make([]string, 0, len(days))
	for day := range days {
		dates = append(dates, day)
	}
	sort.Strings(dates)
	for _, day := range dates[:len(dates)-snapshotDays] {
		delete(days, day)
	}
}

// snapshotWrites keeps the file to the latest days written: writes run off
// the main thread, and one begun later never loses to one begun before.
var (
	snapshotWrites struct {
		sync.Mutex
		last int64
	}
	snapshotGen atomic.Int64
)

// saveSnapshots writes the days kept, off the main thread.
func (a *app) saveSnapshots() {
	f := snapshotFile{Version: snapshotVersion, Days: a.snaps.days}
	b, err := json.Marshal(f)
	if err != nil {
		return
	}
	path, err := snapshotPath()
	if err != nil {
		return
	}
	gen := snapshotGen.Add(1)
	go func() {
		w := &snapshotWrites
		w.Lock()
		defer w.Unlock()
		if gen <= w.last {
			return
		}
		if writeFileAtomic(path, b, 0o600) == nil {
			w.last = gen
		}
	}()
}

// dailyDay names the day of the recommendations given at t: NetEase picks new
// ones at 6:00, Beijing time, so before then they are of the day before.
func dailyDay(t time.Time) string {
	b := t.In(beijing)
	if b.Hour() < 6 {
		b = b.AddDate(0, 0, -1)
	}
	return b.Format("2006-01-02")
}
