package main

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestPanelMenusAndStates(t *testing.T) {
	a := &app{
		status: service.Status{State: "running", Address: "http://127.0.0.1:5601", ActiveProvider: "OpenRouter"},
		catalog: service.ProviderCatalog{Active: "router", Services: []service.ProviderView{
			{ID: "anthropic", Name: "Anthropic", Active: "company", Accounts: []service.ProviderAccount{{ID: "company", Name: "公司"}}},
			{ID: "router", Name: "OpenRouter", Active: "personal", Accounts: []service.ProviderAccount{{ID: "personal", Name: "个人"}, {ID: "work", Name: "工作"}}},
		}},
	}
	tt := ui.NewTester(a.view, 360, a.panelHeight())
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		for _, label := range []string{"服务商：OpenRouter", "账号：个人", "重启服务", "停止服务", "请求日志", "程序运行日志", "打开程序主目录", "设置", "退出 TokenHub"} {
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > 360 || box.Y+box.H > float32(a.panelHeight()) {
				t.Fatalf("%q outside panel: %+v (found %v)", label, box, ok)
			}
		}
		if err := tt.Click("服务商：OpenRouter"); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(tt.Menu(), []string{"Anthropic", "OpenRouter", "-", "管理服务商…"}) {
			t.Fatalf("provider menu: %v", tt.Menu())
		}
		tt.CloseMenu()
		if err := tt.Click("账号：个人"); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(tt.Menu(), []string{"个人", "工作", "-", "管理账号…"}) {
			t.Fatalf("account menu: %v", tt.Menu())
		}
		tt.CloseMenu()
		tt.Move(0, 0)
		if dir := os.Getenv("TOKENHUB_PANEL_PREVIEW"); dir != "" {
			name := "panel-light.png"
			if dark {
				name = "panel-dark.png"
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	a.catalog.Active = "anthropic"
	tt.Frame()
	if !tt.HasText("账号：公司") {
		t.Fatal("account selector did not follow provider")
	}
	if err := tt.Click("账号：公司"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tt.Menu(), []string{"公司", "-", "管理账号…"}) {
		t.Fatalf("accounts leaked from another provider: %v", tt.Menu())
	}
	tt.CloseMenu()
	a.catalog.Active = "router"
	a.status.Pending = true
	tt.SetSize(360, a.panelHeight())
	tt.Frame()
	if !tt.HasText("重启并应用") {
		t.Fatal("pending config has no apply action")
	}
	if err := tt.Click("服务商：OpenRouter"); err != nil {
		t.Fatal(err)
	}
	if tt.Menu() != nil {
		t.Fatal("provider switch must be locked while config is pending")
	}
	if err := tt.Click("账号：个人"); err != nil {
		t.Fatal(err)
	}
	if tt.Menu() != nil {
		t.Fatal("account switch must be locked while config is pending")
	}
	a.status.Error = "服务启动失败，请检查端口和服务商配置。"
	a.restartNotice = "操作失败：请检查配置后重试。"
	a.noticeUntil = time.Now().Add(time.Minute)
	a.selectionError = "无法读取服务商和账号，请前往设置检查"
	tt.SetSize(360, a.panelHeight())
	tt.Frame()
	box, ok := tt.Find("退出 TokenHub")
	if !ok || box.Y+box.H > float32(a.panelHeight()) {
		t.Fatalf("error messages push footer out: %+v", box)
	}
	for _, label := range []string{"重启并应用", "请求日志", "程序运行日志", "打开程序主目录"} {
		box, ok := tt.Find(label)
		if !ok || box.Y+box.H > float32(a.panelHeight()) || box.H <= 0 {
			t.Fatalf("feedback squeezes control %q: %+v", label, box)
		}
	}
	if dir := os.Getenv("TOKENHUB_PANEL_PREVIEW"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "panel-feedback.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	a.catalog.Services[1].Name = "OpenRouter with a very long provider name"
	a.catalog.Services[1].Accounts[0].Name = "个人账号名称很长时也应该正确截断"
	tt.SetSize(360, a.panelHeight())
	tt.Frame()
	for _, label := range []string{"服务商：" + a.catalog.Services[1].Name, "账号：" + a.catalog.Services[1].Accounts[0].Name} {
		box, ok := tt.Find(label)
		if !ok || box.X < 0 || box.X+box.W > 360 {
			t.Fatalf("long selection overflows: %+v", box)
		}
	}
	a.status = service.Status{State: "stopped"}
	tt.SetSize(360, a.panelHeight())
	tt.Frame()
	if !tt.HasText("启动服务") || tt.HasText("停止服务") {
		t.Fatal("stopped panel has incorrect service actions")
	}
}

func TestLoadPanelSelections(t *testing.T) {
	manager, err := service.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Save([]byte("server: {host: 127.0.0.1, port: 5601}\nproviders:\n  Zebra:\n    base_url: https://example.com\n  Alpha:\n    base_url: https://example.org\nactive_provider: Alpha\n")); err != nil {
		t.Fatal(err)
	}
	a := &app{manager: manager}
	a.loadSelections()
	if a.selectionError != "" || len(a.catalog.Services) != 2 || a.catalog.Services[0].Name != "Alpha" || a.catalog.Services[1].Name != "Zebra" || a.catalog.Version == "" || a.catalog.Active != "Alpha" {
		t.Fatalf("invalid panel selection snapshot: %+v", a)
	}
	if err := os.WriteFile(manager.Filename, []byte("providers: ["), 0600); err != nil {
		t.Fatal(err)
	}
	a.loadSelections()
	if a.selectionError == "" || len(a.catalog.Services) != 0 {
		t.Fatal("invalid config left stale provider choices")
	}
}
