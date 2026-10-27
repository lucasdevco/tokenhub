package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lucasdevco/tokenhub/internal/service"
)

// Small geometric icons share a stroke and take their color from the theme.
func panelIcon(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
}

var (
	settingsIcon = panelIcon(`<path d="M4 5h12M4 10h12M4 15h12M7 3v4M13 8v4M8 13v4"/>`)
	programIcon  = panelIcon(`<rect x="3" y="4" width="14" height="12" rx="2"/><path d="m6 8 2 2-2 2M10 12h4"/>`)
	restartIcon  = panelIcon(`<path d="M16 9a6 6 0 1 0-1 5M16 3v6h-6"/>`)
	powerIcon    = panelIcon(`<path d="M10 2v7M6 5a7 7 0 1 0 8 0"/>`)
	chevronIcon  = panelIcon(`<path d="m6 8 4 4 4-4"/>`)
	logsIcon     = panelIcon(`<path d="M3 6h13m-4-3 4 3-4 3M17 14H4m4-3-4 3 4 3"/>`)
	folderIcon   = panelIcon(`<path d="M3 6V4h5l2 2h7v10H3z"/>`)
	arrowIcon    = panelIcon(`<path d="M6 14 14 6M6 6h8v8"/>`)
	copyIcon     = panelIcon(`<rect x="7" y="7" width="9" height="10" rx="2"/><path d="M12 7V3H3v10h4"/>`)
)

// Surface, text and accent roles keep the service and primary entry distinct.
func neutralMenuTheme(base *ui.Theme) ui.Theme {
	theme := *base
	if theme.Dark {
		theme.Background = ui.Hex("#1c1f25")
		theme.Surface = ui.Hex("#272c34")
		theme.SurfaceHover = ui.Hex("#333b47")
		theme.SurfacePressed = ui.Hex("#404b5b")
		theme.Border = ui.Hex("#333b46")
		theme.Text = ui.Hex("#edf1f7")
		theme.TextMuted = ui.Hex("#a9b4c4")
		theme.Selection = ui.Hex("#354c70")
	} else {
		theme.Background = ui.Hex("#f3f5f8")
		theme.Surface = ui.Hex("#ffffff")
		theme.SurfaceHover = ui.Hex("#e6ebf2")
		theme.SurfacePressed = ui.Hex("#dbe3ee")
		theme.Border = ui.Hex("#e0e5ec")
		theme.Text = ui.Hex("#202b3b")
		theme.TextMuted = ui.Hex("#606d80")
		theme.Selection = ui.Hex("#dbeafe")
	}
	theme.Accent = ui.Hex("#2563eb")
	theme.AccentText = ui.Hex("#ffffff")
	theme.AccentHover = ui.Hex("#1d4ed8")
	theme.AccentPressed = ui.Hex("#1e40af")
	theme.Focus = ui.Hex("#3b82f6")
	return theme
}

func (a *app) panelNotice() string {
	if a.restartNotice != "" && time.Now().Before(a.noticeUntil) {
		return a.restartNotice
	}
	if a.selectionError != "" {
		return a.selectionError
	}
	return a.status.Error
}

func (a *app) panelHeight() int {
	height := 380
	if a.panelNotice() != "" {
		height += 32
	}
	if a.status.Pending {
		height += 30
	}
	return height
}

func (a *app) view(c *ui.Context) {
	theme := neutralMenuTheme(c.Theme())
	c.SetTheme(&theme)
	c.Root().Background(ui.Transparent)
	panel := ui.Column(c).Fill().Padding(14).Gap(10).Background(theme.Background).Radius(12)
	panel.Children(func() {
		a.selectionRow(c)
		ui.Column(c).Padding(12).Gap(4).Radius(10).Background(theme.Surface).Shrink(0).Children(func() {
			a.serviceControls(c)
			a.addressRow(c)
			notice := a.panelNotice()
			if notice != "" {
				ui.Text(c, notice).FontSize(11).TextColor(theme.Text).MaxLines(2).Ellipsis("…").Tooltip(notice)
			}
			if a.status.Pending {
				ui.Row(c).Height(26).Gap(6).Children(func() {
					ui.Text(c, "配置待应用").FontSize(11).TextColor(theme.TextMuted).Grow(1)
					ui.Button(c.Key("apply"), "重启并应用").FontSize(11).Disabled(a.locked()).OnClick(func() { a.action("restart", a.manager.Restart) })
				})
			}
		})

		menuRow(c, "logs", logsIcon, "请求日志", "浏览器", func() { a.open("/logs/") })
		ui.Column(c).Gap(2).Shrink(0).Children(func() {
			ui.Text(c, "诊断工具").FontSize(10).TextColor(theme.TextMuted).Padding(0, 8, 3, 8)
			menuRow(c, "program-log", programIcon, "程序运行日志", "文件", a.openProgramLog)
			menuRow(c, "app-directory", folderIcon, "打开程序主目录", "Finder", a.openAppDirectory)
		})
		ui.Spacer(c)
		ui.Divider(c)
		ui.Row(c).Height(24).Shrink(0).Gap(8).Children(func() {
			settings := ui.ButtonBase(c.Key("settings")).Label("设置").Tooltip("在浏览器中打开设置").Padding(6).Gap(6).Radius(6)
			if settings.Hovered() {
				settings.Background(theme.SurfaceHover)
			}
			settings.Children(func() {
				ui.Icon(c, settingsIcon).Size(15, 15).TextColor(theme.TextMuted)
				ui.Text(c, "设置").FontSize(11).TextColor(theme.TextMuted)
			}).OnClick(func() { a.open("/settings") })
			ui.Spacer(c)
			ui.Text(c, "TokenHub").FontSize(10).TextColor(theme.TextMuted)
			button := ui.ButtonBase(c.Key("quit")).Label("退出 TokenHub").Padding(6).Radius(6)
			if button.Hovered() {
				button.Background(theme.SurfaceHover)
			}
			button.Children(func() { ui.Text(c, "退出").FontSize(11).TextColor(theme.TextMuted) })
			button.OnClick(mygo.App.Quit)
		})
	})
}

func (a *app) addressRow(c *ui.Context) {
	theme := c.Theme()
	row := ui.Row(c).Height(44).Gap(10).Shrink(0)
	row.Children(func() {
		ui.Column(c).Grow(1).Gap(4).Children(func() {
			ui.Text(c, "代理地址").FontSize(10).TextColor(theme.TextMuted)
			if a.status.Address != "" && a.status.State == "running" {
				ui.Text(c, a.status.Address).Font("Menlo").FontSize(12).FontWeight(500).Selectable().MaxLines(1).Ellipsis("…")
			} else {
				ui.Text(c, "启动后显示地址").FontSize(12).TextColor(theme.TextMuted)
			}
		})
		copied := a.copiedAddress == a.status.Address && time.Now().Before(a.copiedUntil)
		label := "复制地址"
		if copied {
			label = "已复制"
		}
		button := ui.ButtonBase(c.Key("copy")).Label(label).Padding(8).Radius(6).Disabled(a.status.State != "running" || a.status.Address == "")
		if button.Hovered() {
			button.Background(theme.SurfaceHover)
		}
		button.Children(func() {
			if copied {
				ui.Text(c, "已复制").FontSize(10).TextColor(theme.TextMuted)
			} else {
				ui.Icon(c, copyIcon).Size(16, 16).TextColor(theme.TextMuted)
			}
		})
		button.OnClick(func() {
			mygo.Clipboard.WriteText(a.status.Address)
			a.copiedAddress = a.status.Address
			a.copiedUntil = time.Now().Add(2 * time.Second)
		})
	})
}

func (a *app) locked() bool {
	return a.busy.Load() || a.status.State == "starting" || a.status.State == "stopping"
}

func (a *app) serviceControls(c *ui.Context) {
	theme := c.Theme()
	ui.Row(c).Height(26).Gap(8).Shrink(0).Children(func() {
		color := theme.TextMuted
		if a.status.State == "running" {
			color = theme.Success
		} else if a.status.State == "error" {
			color = theme.Danger
		}
		ui.Box(c).Size(6, 6).Radius(3).Background(color)
		label := stateLabel(a.status.State)
		if a.actionName == "restart" {
			label = "重启中…"
		}
		ui.Text(c, label).FontSize(12).FontWeight(500).Grow(1)
		if a.status.State == "running" || a.actionName == "restart" {
			panelAction(c, "restart", "重启服务", restartIcon, a.locked(), func() { a.action("restart", a.manager.Restart) })
			panelAction(c, "stop", "停止服务", powerIcon, a.locked(), func() { a.action("stop", a.manager.Stop) })
		} else {
			ui.Button(c.Key("start"), "启动服务").FontSize(12).Disabled(a.locked()).OnClick(func() { a.action("start", a.manager.Start) })
		}
	})
}

func panelAction(c *ui.Context, key, label string, icon *ui.SVG, disabled bool, action func()) {
	button := ui.ButtonBase(c.Key(key)).Label(label).Tooltip(label).Size(28, 28).Center().Radius(6).Disabled(disabled)
	if button.Hovered() {
		button.Background(c.Theme().SurfaceHover)
	}
	button.Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(c.Theme().TextMuted) })
	button.OnClick(action)
}

func (a *app) loadSelections() {
	catalog, err := a.manager.ReadProviders()
	a.selectionError = ""
	if err != nil {
		a.catalog = service.ProviderCatalog{}
		a.selectionError = "无法读取服务商和账号，请前往设置检查"
		return
	}
	a.catalog = catalog
}

func (a *app) changeProvider(change service.ProviderChange) {
	change.Version = a.catalog.Version
	action := "provider"
	if change.Action == "account" {
		action = "account"
	}
	a.action(action, func() error {
		_, err := a.manager.ChangeProvider(change)
		return err
	})
}

func (a *app) selectionRow(c *ui.Context) {
	var current service.ProviderView
	for _, provider := range a.catalog.Services {
		if provider.ID == a.catalog.Active {
			current = provider
			break
		}
	}
	locked := a.locked() || a.status.Pending || a.selectionError != ""
	ui.Row(c).Height(32).Gap(8).Shrink(0).Children(func() {
		providerName := current.Name
		if providerName == "" {
			providerName = "未选择"
		}
		selectionButton(c, "provider", "服务商", providerName, locked, func(m *ui.Menu) {
			if len(a.catalog.Services) == 0 {
				m.Item("尚未添加服务商").Disabled(true)
			}
			for _, provider := range a.catalog.Services {
				if m.Item(provider.Name).Checked(provider.ID == a.catalog.Active).Chosen() && provider.ID != a.catalog.Active {
					a.changeProvider(service.ProviderChange{Action: "select", Provider: provider.ID})
				}
			}
			m.Separator()
			if m.Item("管理服务商…").Chosen() {
				a.open("/settings/providers")
			}
		})
		accountName := "未选择"
		if current.ID == "" {
			accountName = "先选服务商"
		} else if len(current.Accounts) == 0 {
			accountName = "无需账号"
		}
		for _, account := range current.Accounts {
			if account.ID == current.Active {
				accountName = account.Name
			}
		}
		selectionButton(c, "account", "账号", accountName, locked || current.ID == "", func(m *ui.Menu) {
			if len(current.Accounts) == 0 {
				m.Item("此服务商未配置账号").Disabled(true)
			}
			for _, account := range current.Accounts {
				if m.Item(account.Name).Checked(account.ID == current.Active).Chosen() && account.ID != current.Active {
					a.changeProvider(service.ProviderChange{Action: "account", Provider: current.ID, Account: account.ID})
				}
			}
			m.Separator()
			if m.Item("管理账号…").Chosen() {
				a.open("/settings/providers")
			}
		})
	})
}

func selectionButton(c *ui.Context, key, label, value string, disabled bool, menu func(*ui.Menu)) {
	theme := c.Theme()
	button := ui.ButtonBase(c.Key(key)).Label(label + "：" + value).Tooltip(label + "：" + value).Grow(1).Basis(0).Height(30).Padding(6).Gap(4).Radius(6).Disabled(disabled)
	if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
	button.Children(func() {
		ui.Text(c, label+"：").FontSize(11).TextColor(theme.TextMuted)
		ui.Text(c, value).FontSize(12).FontWeight(500).Grow(1).MaxLines(1).Ellipsis("…")
		ui.Icon(c, chevronIcon).Size(12, 12).TextColor(theme.TextMuted)
	})
	button.Menu(menu)
}

func menuRow(c *ui.Context, key string, icon *ui.SVG, title, hint string, action func()) {
	theme := c.Theme()
	primary := key == "logs"
	button := ui.ButtonBase(c.Key(key)).Label(title).AlignSelf(ui.Stretch).Height(32).Padding(8).Gap(10).Radius(8).Shrink(0)
	if primary {
		button.Height(44).Background(theme.Surface)
	}
	if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
	button.Children(func() {
		if primary {
			iconFill, iconColor := ui.Hex("#e9f0ff"), ui.Hex("#2563eb")
			if theme.Dark {
				iconFill, iconColor = ui.Hex("#293b59"), ui.Hex("#91b8ff")
			}
			ui.Box(c).Size(28, 28).Radius(7).Background(iconFill).Center().Children(func() {
				ui.Icon(c, icon).Size(16, 16).TextColor(iconColor)
			})
			ui.Text(c, title).FontSize(13).FontWeight(500).Grow(1)
		} else {
			ui.Icon(c, icon).Size(16, 16).TextColor(theme.TextMuted)
			ui.Text(c, title).FontSize(12).TextColor(theme.TextMuted).Grow(1)
		}
		ui.Text(c, hint).FontSize(10).TextColor(theme.TextMuted)
		ui.Icon(c, arrowIcon).Size(12, 12).TextColor(theme.TextMuted)
	})
	button.OnClick(action)
}

func (a *app) openAppDirectory() {
	if a.panel != nil {
		a.panel.Close()
	}
	go func() {
		if err := openDirectory(filepath.Dir(a.manager.Filename)); err != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type:    mygo.MessageError,
				Title:   "无法打开程序主目录",
				Message: err.Error(),
			})
		}
	}()
}

func (a *app) openProgramLog() {
	if a.panel != nil {
		a.panel.Close()
	}
	go func() {
		filename := filepath.Join(filepath.Dir(a.manager.Filename), "app.log")
		if err := mygo.Shell.OpenPath(filename); err != nil {
			log.Printf("无法打开程序日志: %v", err)
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type:    mygo.MessageError,
				Title:   "无法打开程序日志",
				Message: err.Error(),
			})
		}
	}()
}

func openDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	return mygo.Shell.OpenPath(directory)
}
