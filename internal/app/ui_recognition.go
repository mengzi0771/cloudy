package app

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/recognition"
)

var iconRecognition = strokeIcon(`<path d="M3 10v4m4-8v12m5-15v18m5-15v12m4-8v4"/>`)

type recognitionView struct {
	open    bool
	request context.Context
	cancel  context.CancelFunc

	status  string
	results []recognition.Result
}

func (a *app) songRecognitionButton(c *ui.Context) {
	if recognition.Available {
		iconButton(c.Key("song-recognition"), iconRecognition, 15, a.pal.muted, "听歌识曲").Size(24, 28).Tooltip("听歌识曲").OnClick(func() {
			a.songRecognition.open = true
		})
	}
}

func (a *app) closeSongRecognition() {
	a.songRecognition.open = false
	a.cancelSongRecognition()
}

func (a *app) cancelSongRecognition() {
	s := &a.songRecognition
	if s.cancel != nil {
		s.cancel()
		s.request, s.cancel = nil, nil
		s.status = ""
	}
}

func (a *app) startSongRecognition() {
	s := &a.songRecognition
	if !s.open || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	s.request, s.cancel = ctx, cancel
	s.status, s.results = "正在采集系统音频…", nil
	go func() {
		pcm, rate, err := recognition.Capture(ctx)
		var results []recognition.Result
		if err == nil && ctx.Err() == nil {
			a.post(func() {
				if a.songRecognition.request == ctx {
					a.songRecognition.status = "正在匹配歌曲…"
				}
			})
			results, err = recognition.Match(ctx, pcm, rate)
		}
		status := ""
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			status = "识别超时，请重试。"
		case errors.Is(err, recognition.ErrNoAudio):
			status = "没听到声音。请在 Windows 默认输出设备上播放音乐，再试一次。"
		case err != nil && pcm == nil:
			status = "无法采集系统音频。请检查 Windows 的默认输出设备。"
		case err != nil:
			status = "暂时无法匹配歌曲，请检查网络后重试。"
		case len(results) == 0:
			status = "没找到匹配的歌，可以换一段再试。"
		}
		a.post(func() {
			s := &a.songRecognition
			if s.request != ctx {
				return
			}
			a.cancelSongRecognition()
			s.results, s.status = results, status
		})
	}()
}

func (a *app) songRecognitionDialog(c *ui.Context) {
	if !recognition.Available {
		return
	}
	s := &a.songRecognition
	a.dialog(c, "recognition", 0.25, &s.open, func(panel ui.Element) {
		ink := a.inkOn(a.nowPlaying)
		width, _ := c.Size()
		a.dialogPane(panel.Width(min(460, width-32)).Clip().TextColor(ink.text).Label("听歌识曲"), 24, a.nowPlaying)
		panel.Children(func() {
			dialogHeader(c, ink, "听歌识曲", "采集默认输出设备上约 6 秒的声音，只发送音频指纹给网易云。", a.closeSongRecognition)
			ui.Column(c.Key("results")).Padding(4, 28, 4, 28).Gap(8).Children(func() {
				status := s.status
				if status == "" && len(s.results) == 0 {
					status = "先在电脑上播放要找的歌，再点击开始识曲。"
				}
				if status != "" {
					ui.Text(c.Key("status"), status).FontSize(13).TextColor(ink.muted).LineHeight(1.5)
				}
				for _, result := range s.results {
					track := result.Track
					ui.Row(c.Key(track.ID)).AlignItems(ui.Center).Gap(10).Children(func() {
						a.coverBox(c, "cover", track.Album.PicURL, 96, 40, 6)
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, track.Name).SingleLine().FontSize(14).FontWeight(600)
							ui.Text(c, track.ArtistNames()).SingleLine().FontSize(12).TextColor(ink.muted)
						})
						label := "播放 " + track.Name
						if result.Start != nil {
							at := clock(*result.Start)
							ui.Text(c.Key("match-time"), at).FontSize(12).FontFeatures("tnum").TextColor(ink.muted)
							label = "从 " + at + " 播放 " + track.Name
						}
						iconButton(c.Key("play"), iconPlay, 16, ink.text, label).Tooltip(label).Disabled(!track.Playable()).OnClick(func() {
							a.closeSongRecognition()
							if result.Start != nil {
								a.pl.PlayNowAt(track, *result.Start)
							} else {
								a.pl.PlayNow(track)
							}
						})
					})
				}
			})
			dialogFooter(c, func() {
				if s.cancel != nil {
					fillButton(c.Key("cancel"), "取消识别", nil, ink.hover, ink.text, !s.open).OnClick(a.cancelSongRecognition)
				} else {
					label := "开始识曲"
					if s.status != "" || len(s.results) != 0 {
						label = "再识别一次"
					}
					primaryButton(c.Key("start"), label, iconRecognition, a.pal, !s.open).OnClick(a.startSongRecognition)
				}
			})
		})
	})
	if !s.open && s.cancel != nil {
		a.cancelSongRecognition()
	}
}
