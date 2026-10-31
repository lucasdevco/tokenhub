// tokenhub is a native menu-bar shell; settings and logs open in the browser.
package main

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lucasdevco/tokenhub/internal/management"
	"github.com/lucasdevco/tokenhub/internal/service"
)

type app struct {
	manager        *service.Manager
	server         *management.Server
	tray           *mygo.Tray
	panel          *mygo.Window
	status         service.Status
	catalog        service.ProviderCatalog
	selectionError string
	busy           atomic.Bool
	actionName     string
	restartNotice  string
	noticeUntil    time.Time
	quitting       bool
	copiedAddress  string
	copiedUntil    time.Time
	update         appUpdate
}

func (a *app) action(name string, fn func() error) {
	if !a.busy.CompareAndSwap(false, true) {
		return
	}
	a.actionName = name
	a.restartNotice = ""
	a.refresh()
	go func() {
		started := time.Now()
		err := fn()
		if err != nil {
			log.Print(err)
		}
		if name == "restart" {
			// Keep the feedback visible without delaying the service restart itself.
			time.Sleep(max(0, 600*time.Millisecond-time.Since(started)))
		}
		mygo.RunOnMain(func() {
			a.actionName = ""
			a.busy.Store(false)
			if name != "" {
				a.restartNotice = map[string]string{"restart": tr("Service restarted"), "provider": tr("Provider switched. New requests use it immediately."), "account": tr("Account switched. New requests take effect immediately.")}[name]
				if err != nil {
					a.restartNotice = tr("Operation failed: ") + err.Error()
				}
				a.noticeUntil = time.Now().Add(3 * time.Second)
			}
			a.refresh()
		})
	}()
}

func (a *app) refresh() {
	if a.server != nil {
		if prefs, err := a.server.Preferences(); err == nil {
			language.Store(prefs.Locale)
		}
	}
	a.status = a.manager.Status()
	if a.panel != nil {
		a.loadSelections()
	}
	if a.tray != nil {
		a.tray.SetToolTip("TokenHub · " + stateLabel(a.status.State))
	}
	if a.panel != nil {
		a.panel.SetSize(360, a.panelHeight())
		a.panel.Invalidate()
	}
}

func stateLabel(state string) string {
	switch state {
	case "running":
		return tr("Running")
	case "starting":
		return tr("Starting…")
	case "stopping":
		return tr("Stopping…")
	case "error":
		return tr("Failed to start")
	default:
		return tr("Stopped")
	}
}

func (a *app) open(path string) {
	if a.panel != nil {
		a.panel.Close()
	}
	go func() {
		if err := mygo.Shell.OpenExternal(a.server.URL + path); err != nil {
			log.Print(err)
		}
	}()
}

func (a *app) toggle() {
	if a.panel != nil {
		a.panel.Close()
		return
	}
	a.refresh()
	a.loadSelections()
	a.panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     360,
		Height:    a.panelHeight(),
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(a.view),
	})
	a.panel.OnClosed(func() { a.panel = nil })
}

func (a *app) ready() {
	server, err := management.New(a.manager, management.Options{
		Address:        listenAddress,
		SetOpenAtLogin: mygo.App.SetOpenAtLogin,
		GetOpenAtLogin: mygo.App.OpenAtLogin,
		OpenDirectory:  openDirectory,
	})
	if err != nil {
		log.Print(err)
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Title: tr("TokenHub could not start"), Message: err.Error()})
		mygo.App.Quit()
		return
	}
	a.server = server
	log.Printf("tokenhub settings: %s/settings", server.URL)
	go func() {
		if err := server.Serve(); err != nil {
			log.Print(err)
			mygo.App.Quit()
		}
	}()
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "TokenHub"})
	if err != nil {
		log.Print(err)
		mygo.App.Quit()
		return
	}
	a.tray.OnClick(a.toggle)
	mygo.App.OnSecondInstance(func([]string, string) { a.toggle() })
	mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
		if a.quitting {
			return
		}
		event.PreventDefault()
		go func() {
			if a.manager.Status().State == "running" {
				answer, err := mygo.Dialog.Message(mygo.MessageOptions{
					Type:          mygo.MessageQuestion,
					Title:         tr("Quit TokenHub?"),
					Message:       tr("Quitting will stop the proxy and the local management interface."),
					Detail:        tr("Recorded request logs will remain on this computer."),
					Buttons:       []string{tr("Cancel"), tr("Stop service and quit")},
					DefaultButton: 0,
					CancelButton:  0,
				})
				if err != nil || answer.Button != 1 {
					return
				}
			}
			if err := a.manager.Stop(); err != nil {
				log.Printf(tr("Failed to stop proxy or finalize logs: %v"), err)
			}
			_ = server.Close()
			mygo.RunOnMain(func() { a.quitting = true; mygo.App.Quit() })
		}()
	})
	ctx, cancel := context.WithCancel(context.Background())
	a.startUpdateChecks(ctx)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mygo.RunOnMain(a.refresh)
			}
		}
	}()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		cancel()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	prefs, err := server.Preferences()
	if err != nil {
		log.Print(err)
	} else if prefs.AutoStart {
		a.action("start", a.manager.Start)
	}
	a.refresh()
}

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	ink := color.NRGBA{A: 255}
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			if (y >= 8 && y <= 10) || (y >= 21 && y <= 23) || (x >= 10 && x <= 12 && y <= 15) || (x >= 20 && x <= 22 && y >= 16) {
				img.Set(x, y, ink)
			}
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

var listenAddress string

func main() {
	base, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	filename := flag.String("config", filepath.Join(base, ".tokenhub", "config.yaml"), "configuration file")
	flag.StringVar(&listenAddress, "listen", "127.0.0.1:5602", "loopback management address")
	flag.Parse()
	mygo.App.SetName("TokenHub")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	programLog, err := service.OpenProgramLog(*filename)
	if err != nil {
		log.Printf(tr("Could not save application log: %v"), err)
	} else {
		defer programLog.Close()
	}
	manager, err := service.NewManager(*filename)
	if err != nil {
		log.Fatal(err)
	}
	a := &app{manager: manager}
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
	if err := manager.Stop(); err != nil {
		log.Printf(tr("Failed to stop proxy or finalize logs: %v"), err)
	}
	if a.server != nil {
		_ = a.server.Close()
	}
}
