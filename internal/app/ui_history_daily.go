package app

import (
	"fmt"
	"sort"
	"time"

	"netease-music-mygo/internal/netease"

	"github.com/egoist/mygo/ui"
)

// The listener's past daily recommendations: the days NetEase still keeps,
// and the songs of the day picked among them.

// historyDaily is what the page has of them: the days, the one picked, and
// the songs of the days fetched while the page was open.
type historyDaily struct {
	forUser int64 // the account the days are of
	asked   bool  // the days were asked for

	days    []string        // newest first, YYYY-MM-DD
	local   map[string]bool // the days shown from Cloudy's own record
	note    string          // NetEase's words: how many it keeps, or why there are none
	daysErr string

	date    string // the day picked
	songs   []netease.Track
	cache   map[string][]netease.Track // the days answered this time
	err     string
	loading bool
	gen     int // bumped as another day is picked, to drop that day's answer
	scroll  ui.ScrollState
}

// The API, through variables so tests answer without the network.
var (
	historyDates = netease.HistoryRecommendDates
	historySongs = netease.HistoryRecommendSongs
)

// loadHistoryDays asks for the days NetEase keeps of the listener's daily
// recommendations, and then for the newest of them, once per account.
func (a *app) loadHistoryDays() {
	if a.profile == nil {
		a.hist = historyDaily{}
		return
	}
	if a.hist.asked && a.hist.forUser == a.profile.UserID {
		return
	}
	h := &a.hist
	*h = historyDaily{forUser: a.profile.UserID, asked: true}
	gen := a.accountGen
	go func() {
		days, err := historyDates()
		a.post(func() {
			if gen != a.accountGen || h.forUser == 0 {
				return
			}
			if err != nil {
				h.daysErr = "历史日推加载失败"
				return
			}
			h.days, h.local = a.keptDays(days.Dates)
			h.note = days.Note
			if h.date == "" && len(h.days) > 0 {
				a.pickHistoryDay(h.days[0])
			}
		})
	}()
}

// keptDays puts among the days NetEase listed the ones Cloudy kept of the
// daily recommendations that it no longer lists, newest first, and says
// which of them are from that record.
func (a *app) keptDays(listed []string) ([]string, map[string]bool) {
	days := append([]string(nil), listed...)
	if a.profile == nil {
		return days, nil
	}
	known := make(map[string]bool, len(listed))
	for _, day := range listed {
		known[day] = true
	}
	var local map[string]bool
	for _, day := range a.snapshotDates(a.profile.UserID) {
		if known[day] {
			continue
		}
		if local == nil {
			local = map[string]bool{}
		}
		local[day] = true
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days, local
}

// pickHistoryDay shows the songs NetEase recommended on date, from the cache
// if the page has them, or asks for them. The songs of the day before stay
// until these come.
func (a *app) pickHistoryDay(date string) {
	h := &a.hist
	h.date, h.err = date, ""
	h.gen++
	gen := h.gen
	if songs, ok := h.cache[date]; ok {
		h.songs, h.loading = songs, false
		return
	}
	if h.local[date] {
		// NetEase no longer answers for it: Cloudy's own record has it.
		if songs := a.snapshotSongs(date, a.profile.UserID); len(songs) > 0 {
			h.cacheOf(date, songs)
			return
		}
	}
	h.loading = true
	accountGen := a.accountGen
	go func() {
		songs, err := historySongs(date)
		a.post(func() {
			if gen != h.gen || accountGen != a.accountGen {
				return
			}
			h.loading = false
			if err != nil {
				h.err = "历史日推加载失败"
				return
			}
			h.cacheOf(date, songs)
			// The day is kept, so the page shows it once NetEase stops.
			a.keepDay(date, songs, time.Now())
		})
	}()
}

// cacheOf shows the songs of day, keeping them for the page.
func (h *historyDaily) cacheOf(day string, songs []netease.Track) {
	if h.cache == nil {
		h.cache = make(map[string][]netease.Track, len(h.days))
	}
	h.cache[day] = songs
	h.songs, h.loading = songs, false
}

// playHistory plays the songs of the day that can play, saying how many it
// left out.
func (a *app) playHistory(songs []netease.Track, shuffled bool) {
	playable := make([]netease.Track, 0, len(songs))
	for _, t := range songs {
		if t.Playable() {
			playable = append(playable, t)
		}
	}
	if len(playable) == 0 {
		a.say("history", "这些歌暂时都不能播放")
		return
	}
	if left := len(songs) - len(playable); left > 0 {
		a.say("history", fmt.Sprintf("有 %d 首暂时不能播放，已跳过", left))
	}
	a.playList(playable, shuffled)
}

// historyPage shows the days NetEase keeps of the past daily
// recommendations, and the songs of the one picked.
func (a *app) historyPage(c *ui.Context) {
	if a.profile == nil {
		a.centered(c, "登录后才能查看历史日推", nil)
		return
	}
	h := &a.hist
	s := a.pageShimmer(c)
	if h.daysErr != "" && len(h.days) == 0 {
		a.centered(c, h.daysErr, func() {
			h.asked = false
			a.loadHistoryDays()
		})
		return
	}
	if len(h.days) == 0 {
		if !h.asked {
			a.centered(c, "", nil)
			return
		}
		note := h.note
		if note == "" {
			note = "还没有可以回看的每日推荐"
		}
		a.centered(c, note, func() {
			h.asked = false
			a.loadHistoryDays()
		})
		return
	}
	header := func(c *ui.Context) {
		p := a.pal
		ui.Column(c).Gap(10).Padding(0, 0, 24, 0).Children(func() {
			ui.Text(c, "每日推荐").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, "历史日推").FontSize(30).FontWeight(800).LetterSpacing(-0.6)
			ui.Text(c, fmt.Sprintf("%s · %s · %d 首", a.hist.date, dayWord(a.hist.date), len(h.songs))).FontSize(13).TextColor(p.muted)
			ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放全部", p.accent, p.onAccent).Clicked() {
					a.playHistory(h.songs, false)
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					a.playHistory(h.songs, true)
				}
				a.batchEntry(c)
			})
			ui.ScrollHorizontal(c.Key("history-days")).TrackScroll(&h.scroll).Height(44).Gap(8).Children(func() {
				for _, day := range h.days {
					bg, fg := p.hover.Over(p.bg), p.text
					if day == h.date {
						bg, fg = p.accent, p.onAccent
					}
					if pillButton(c.Key("day:"+day), nil, dayWord(day), bg, fg).Clicked() {
						a.pickHistoryDay(day)
					}
				}
			})
			if h.note != "" {
				ui.Text(c, h.note).FontSize(12).TextColor(p.faint)
			}
			if h.local[h.date] {
				ui.Text(c, "这天来自 Cloudy 自己记下的每日推荐").FontSize(12).TextColor(p.faint)
			}
		})
	}
	if h.loading && len(h.songs) == 0 {
		a.trackSkeleton(c, s, true, header)
		return
	}
	if h.err != "" && len(h.songs) == 0 {
		a.trackRows(c, nil, true, header, func(c *ui.Context) {
			a.historyNothing(c, h.err, func() { a.pickHistoryDay(h.date) })
		})
		return
	}
	if len(h.songs) == 0 {
		a.trackRows(c, nil, true, header, func(c *ui.Context) {
			a.historyNothing(c, "这一天没有记录", func() { a.pickHistoryDay(h.date) })
		})
		return
	}
	a.trackRows(c, h.songs, true, header)
}

// historyNothing says why the day picked shows no songs, with a retry if it
// can be asked for again.
func (a *app) historyNothing(c *ui.Context, msg string, retry func()) {
	w, _ := c.Size()
	ui.Column(c).Padding(0, 0, 40, 0).Gap(10).Children(func() {
		ui.Text(c, msg).FontSize(13).TextColor(a.pal.muted).TextAlign(ui.Center).Width(w - 2*pagePad)
		if retry != nil && ui.Button(c, "重试").Clicked() {
			retry()
		}
	})
}

// dayWord names a day as the page shows it: 11/18, and its weekday.
func dayWord(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return fmt.Sprintf("%d/%d %s", int(t.Month()), t.Day(), weekdayShort(t))
}
