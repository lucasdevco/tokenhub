package management

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasdevco/tokenhub/internal/service"
)

func TestSavePreferencesOnlyChangesLoginWhenNeeded(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stored     bool
		actual     bool
		requested  bool
		getter     bool
		denied     bool
		wantCalls  int
		wantStatus int
	}{
		{name: "auto start with login disabled", getter: true, denied: true, wantStatus: http.StatusOK},
		{name: "auto start with login enabled", stored: true, actual: true, requested: true, getter: true, denied: true, wantStatus: http.StatusOK},
		{name: "system enabled outside app", actual: true, requested: true, getter: true, denied: true, wantStatus: http.StatusOK},
		{name: "system disabled outside app", stored: true, getter: true, denied: true, wantStatus: http.StatusOK},
		{name: "enable login", requested: true, getter: true, wantCalls: 1, wantStatus: http.StatusOK},
		{name: "disable login", stored: true, actual: true, getter: true, wantCalls: 1, wantStatus: http.StatusOK},
		{name: "login permission denied", requested: true, getter: true, denied: true, wantCalls: 1, wantStatus: http.StatusBadRequest},
		{name: "stored state without getter", stored: true, requested: true, denied: true, wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{Manager: &service.Manager{Filename: filepath.Join(t.TempDir(), "config.yaml")}}
			original := Preferences{OpenAtLogin: tc.stored}
			data, _ := json.Marshal(original)
			if err := os.WriteFile(server.preferencesFile(), data, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			server.options.SetOpenAtLogin = func(enabled bool) error {
				calls++
				if enabled != tc.requested {
					t.Fatalf("unexpected login state: %v", enabled)
				}
				if tc.denied {
					return errors.New("mygo: Operation not permitted")
				}
				return nil
			}
			if tc.getter {
				server.options.GetOpenAtLogin = func() bool { return tc.actual }
			}
			requested := Preferences{AutoStart: true, OpenAtLogin: tc.requested}
			data, _ = json.Marshal(requested)
			request := httptest.NewRequest(http.MethodPost, "/api/preferences", strings.NewReader(string(data)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.savePreferences(response, request)
			if response.Code != tc.wantStatus || calls != tc.wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body)
			}
			want := requested
			if tc.wantStatus != http.StatusOK {
				want = original
			}
			got, err := server.Preferences()
			if err != nil || got != want {
				t.Fatalf("persisted=%+v want=%+v err=%v", got, want, err)
			}
		})
	}
}

func TestPreferenceLocalePersistence(t *testing.T) {
	server := &Server{Manager: &service.Manager{Filename: filepath.Join(t.TempDir(), "config.yaml")}}
	for _, locale := range []string{"zh-CN", "en", "__unsupported__", ""} {
		data, _ := json.Marshal(Preferences{AutoStart: true, Locale: locale})
		request := httptest.NewRequest(http.MethodPost, "/api/preferences", strings.NewReader(string(data)))
		response := httptest.NewRecorder()
		server.savePreferences(response, request)
		want := http.StatusOK
		if locale == "__unsupported__" {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Fatalf("locale=%q: status=%d body=%s", locale, response.Code, response.Body)
		}
		prefs, err := server.Preferences()
		if err != nil {
			t.Fatal(err)
		}
		expected := locale
		if locale == "__unsupported__" {
			expected = "en"
		}
		if prefs.Locale != expected || !prefs.AutoStart {
			t.Fatalf("locale=%q preferences=%+v", locale, prefs)
		}
	}
}
