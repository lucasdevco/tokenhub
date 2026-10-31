package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestEnglishPanel(t *testing.T) {
	language.Store("en")
	a := &app{status: service.Status{State: "running", Address: "127.0.0.1:5601"}}
	panel := ui.NewTester(a.view, 360, a.panelHeight())
	for _, label := range []string{"Request logs", "Restart service", "Settings", "Quit TokenHub"} {
		if _, ok := panel.Find(label); !ok {
			t.Fatalf("missing English label %q", label)
		}
	}
}
