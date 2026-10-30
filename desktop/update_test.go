package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestUpdatePanelAndWindowStates(t *testing.T) {
	for _, test := range []struct {
		phase updatePhase
		label string
	}{
		{updateIdle, "检查更新…"},
		{updateChecking, "正在检查更新…"},
		{updateAvailable, "发现新版本 v0.2.0"},
		{updateDownloading, "正在下载更新…"},
		{updateReady, "更新已就绪，重启生效"},
		{updateFailed, "检查更新…"},
		{updateRestarting, "正在重启更新…"},
	} {
		t.Run(test.label, func(t *testing.T) {
			a := &app{status: service.Status{State: "stopped"}, update: appUpdate{
				phase: test.phase, release: &mygo.Update{Version: "0.2.0"}, downloaded: 2e6, total: 4e6,
			}}
			panel := ui.NewTester(a.view, 360, a.panelHeight())
			box, found := panel.Find(test.label)
			if !found || box.Y < 0 || box.Y+box.H > float32(a.panelHeight()) {
				t.Fatalf("update entry missing or clipped: %+v", box)
			}
			window := ui.NewTester(a.updateView, 480, 260)
			if !window.HasText(test.label) {
				t.Fatal("update window does not show its state")
			}
			if test.phase == updateDownloading && !window.HasText("已下载 2.0 / 4.0 MB") {
				t.Fatal("download progress is missing")
			}
			_, restart := window.Find("重启更新")
			if restart != (test.phase == updateReady) {
				t.Fatal("restart must be offered only after installation succeeds")
			}
		})
	}
}
