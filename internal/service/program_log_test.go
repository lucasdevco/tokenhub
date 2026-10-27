package service

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProgramLogPersistsAcrossStarts(t *testing.T) {
	original := log.Writer()
	defer log.SetOutput(original)
	configFile := filepath.Join(t.TempDir(), "config", "config.yaml")
	for _, message := range []string{"first run", "second run"} {
		file, err := OpenProgramLog(configFile)
		if err != nil {
			t.Fatal(err)
		}
		log.Print(message)
		log.SetOutput(original)
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(filepath.Dir(configFile), "app.log")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"TokenHub starting", "first run", "second run"} {
		if !strings.Contains(string(data), message) {
			t.Errorf("program log missing %q", message)
		}
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("program log permissions = %o, want 600", info.Mode().Perm())
	}
}
