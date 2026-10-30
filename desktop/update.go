package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type updatePhase int

const (
	updateIdle updatePhase = iota
	updateChecking
	updateAvailable
	updateDownloading
	updateReady
	updateFailed
	updateRestarting
)

// All update state belongs to the UI thread; workers post their results there.
type appUpdate struct {
	phase      updatePhase
	release    *mygo.Update
	err        string
	downloaded int64
	total      int64
	window     *mygo.Window
	ctx        context.Context
}

func (u *appUpdate) label() string {
	switch u.phase {
	case updateChecking:
		return "正在检查更新…"
	case updateAvailable:
		return "发现新版本 v" + u.release.Version
	case updateDownloading:
		return "正在下载更新…"
	case updateReady:
		return "更新已就绪，重启生效"
	case updateRestarting:
		return "正在重启更新…"
	default:
		return "检查更新…"
	}
}

func (a *app) redrawUpdate() {
	if a.update.window != nil {
		a.update.window.Invalidate()
	}
	a.refresh()
}

func (a *app) startUpdateChecks(ctx context.Context) {
	a.update.ctx = ctx
	if !mygo.Updater.Enabled() {
		return
	}
	a.checkUpdate()
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mygo.RunOnMain(a.checkUpdate)
			}
		}
	}()
}

func (a *app) checkUpdate() {
	u := &a.update
	if u.phase == updateChecking || u.phase == updateDownloading || u.phase == updateReady || u.phase == updateRestarting {
		return
	}
	u.phase, u.err, u.release = updateChecking, "", nil
	a.redrawUpdate()
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 30*time.Second)
		defer cancel()
		release, err := mygo.Updater.Check(ctx)
		mygo.RunOnMain(func() {
			u.phase, u.release = updateIdle, release
			if err != nil {
				u.phase = updateFailed
				if !mygo.Updater.Enabled() {
					u.err = "请先将正式版应用安装到「应用程序」。开发版本和只读磁盘镜像不支持应用内更新。"
				} else {
					u.err = "检查更新失败，请检查网络后重试。"
				}
				log.Printf("检查更新失败: %v", err)
			} else if release != nil {
				u.phase = updateAvailable
			}
			a.redrawUpdate()
		})
	}()
}

func (a *app) openUpdate() {
	if a.panel != nil {
		a.panel.Close()
	}
	u := &a.update
	if u.window == nil {
		u.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "TokenHub 软件更新", Width: 480, Height: 260,
			DisableResize: true, Content: ui.View(a.updateView),
		})
		u.window.OnClosed(func() { u.window = nil })
	}
	u.window.Show()
	u.window.Focus()
	switch u.phase {
	case updateAvailable:
		a.downloadUpdate()
	case updateIdle, updateFailed:
		a.checkUpdate()
	}
}

func (a *app) downloadUpdate() {
	u := &a.update
	if u.release == nil || u.phase == updateDownloading || u.phase == updateReady {
		return
	}
	u.phase, u.err, u.downloaded, u.total = updateDownloading, "", 0, 0
	a.redrawUpdate()
	release := u.release
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 15*time.Minute)
		defer cancel()
		err := release.Install(ctx, func(downloaded, total int64) {
			mygo.RunOnMain(func() {
				u.downloaded, u.total = downloaded, total
				a.redrawUpdate()
			})
		})
		mygo.RunOnMain(func() {
			u.phase = updateReady
			if err != nil {
				u.phase, u.err = updateFailed, "下载或安装失败，当前应用保持可用，请重试。"
				log.Printf("安装更新失败: %v", err)
			}
			a.redrawUpdate()
		})
	}()
}

func (a *app) restartUpdatedApp() {
	if a.update.phase != updateReady || a.quitting || !a.busy.CompareAndSwap(false, true) {
		return
	}
	a.update.phase = updateRestarting
	a.redrawUpdate()
	// Finish proxy requests and close the management listener before relaunch.
	// Setting quitting only after cleanup lets Relaunch pass OnBeforeQuit.
	go func() {
		if err := a.manager.Stop(); err != nil {
			mygo.RunOnMain(func() {
				a.busy.Store(false)
				a.update.phase = updateReady
				a.update.err = "停止代理失败，请重试重启更新。"
				a.redrawUpdate()
			})
			return
		}
		if err := a.server.Close(); err != nil {
			log.Printf("关闭管理服务: %v", err)
		}
		mygo.RunOnMain(func() {
			a.quitting = true
			mygo.App.Relaunch()
		})
	}()
}

func (a *app) updateView(c *ui.Context) {
	u := &a.update
	theme := neutralMenuTheme(c.Theme())
	c.SetTheme(&theme)
	ui.Column(c).Fill().Padding(22).Gap(12).Background(theme.Background).Children(func() {
		ui.Text(c, u.label()).FontSize(18).FontWeight(600)
		message := "当前版本 v" + mygo.App.Version()
		switch u.phase {
		case updateIdle:
			message += "，已是最新版本。"
		case updateChecking:
			message = "正在连接更新服务器…"
		case updateAvailable:
			message = "新版已可下载，下载完成后可选择重启更新。"
		case updateDownloading:
			message = "正在下载并校验更新，当前代理继续运行。"
			if u.total > 0 {
				message = fmt.Sprintf("已下载 %.1f / %.1f MB", float64(u.downloaded)/1e6, float64(u.total)/1e6)
				if u.downloaded >= u.total {
					message = "正在校验并安装更新…"
				}
			}
		case updateReady:
			message = "更新已下载并安装。重启应用后生效，配置和日志会保留。"
		case updateRestarting:
			message = "正在停止代理并保存日志，随后将启动新版本…"
		}
		if u.err != "" {
			message = u.err
		}
		ui.Text(c, message).FontSize(12).TextColor(theme.TextMuted).MaxLines(3)
		if u.phase == updateDownloading || u.phase == updateChecking || u.phase == updateRestarting {
			progress := -1.0
			if u.phase == updateDownloading && u.total > 0 {
				progress = float64(u.downloaded) / float64(u.total)
			}
			ui.Progress(c, progress).Height(6)
		}
		ui.Spacer(c)
		ui.Row(c).Gap(10).Children(func() {
			ui.Button(c.Key("later"), "稍后").OnClick(func() { u.window.Close() })
			ui.Spacer(c)
			switch u.phase {
			case updateReady:
				ui.Button(c.Key("relaunch-update"), "重启更新").Disabled(a.locked()).OnClick(a.restartUpdatedApp)
			case updateAvailable:
				ui.Button(c.Key("download-update"), "下载更新").OnClick(a.downloadUpdate)
			case updateFailed:
				ui.Button(c.Key("retry-update"), "重试").OnClick(func() {
					if u.release != nil {
						a.downloadUpdate()
					} else {
						a.checkUpdate()
					}
				})
			}
		})
	})
}
