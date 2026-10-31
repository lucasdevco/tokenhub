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
		return tr("Checking for updates…")
	case updateAvailable:
		return tr("New version available: v") + u.release.Version
	case updateDownloading:
		return tr("Downloading update…")
	case updateReady:
		return tr("Update ready. Restart to apply.")
	case updateRestarting:
		return tr("Restarting to update…")
	default:
		return tr("Check for updates…")
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
					u.err = tr("Install the release app in Applications first. Development builds and read-only disk images do not support in-app updates.")
				} else {
					u.err = tr("Could not check for updates. Check your network and try again.")
				}
				log.Printf(tr("Update check failed: %v"), err)
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
			Title: tr("TokenHub software update"), Width: 480, Height: 260,
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
				u.phase, u.err = updateFailed, tr("Download or installation failed. The current app remains usable. Please try again.")
				log.Printf(tr("Update installation failed: %v"), err)
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
				a.update.err = tr("Could not stop the proxy. Try restarting the update again.")
				a.redrawUpdate()
			})
			return
		}
		if err := a.server.Close(); err != nil {
			log.Printf(tr("Close management service: %v"), err)
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
		message := tr("Current version: v") + mygo.App.Version()
		switch u.phase {
		case updateIdle:
			message += tr(". You are up to date.")
		case updateChecking:
			message = tr("Connecting to update server…")
		case updateAvailable:
			message = tr("A new version is available. Download it, then restart when ready.")
		case updateDownloading:
			message = tr("Downloading and verifying the update. Your proxy keeps running.")
			if u.total > 0 {
				message = fmt.Sprintf(tr("Downloaded %.1f / %.1f MB"), float64(u.downloaded)/1e6, float64(u.total)/1e6)
				if u.downloaded >= u.total {
					message = tr("Verifying and installing update…")
				}
			}
		case updateReady:
			message = tr("Update downloaded and installed. Restart to apply. Configuration and logs will be preserved.")
		case updateRestarting:
			message = tr("Stopping the proxy and saving logs before launching the new version…")
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
			ui.Button(c.Key("later"), tr("Later")).OnClick(func() { u.window.Close() })
			ui.Spacer(c)
			switch u.phase {
			case updateReady:
				ui.Button(c.Key("relaunch-update"), tr("Restart to update")).Disabled(a.locked()).OnClick(a.restartUpdatedApp)
			case updateAvailable:
				ui.Button(c.Key("download-update"), tr("Download update")).OnClick(a.downloadUpdate)
			case updateFailed:
				ui.Button(c.Key("retry-update"), tr("Retry")).OnClick(func() {
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
