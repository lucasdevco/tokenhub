// Package i18n contains the interface translations shared by the web and desktop clients.
package i18n

import (
	"embed"
	"encoding/json"
	"strings"
)

//go:embed catalog.json locales/*.json
var assets embed.FS

var catalog struct {
	DefaultLocale string            `json:"defaultLocale"`
	Locales       map[string]string `json:"locales"`
}

var dictionaries = func() map[string]map[string]string {
	data, err := assets.ReadFile("catalog.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		panic(err)
	}
	if !Supported(catalog.DefaultLocale) {
		panic("invalid default locale")
	}
	result := make(map[string]map[string]string, len(catalog.Locales))
	for locale := range catalog.Locales {
		data, err := assets.ReadFile("locales/" + locale + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(err)
		}
		result[locale] = messages
	}
	return result
}()

func Supported(locale string) bool {
	_, ok := catalog.Locales[locale]
	return ok
}

// T falls back to English for unsupported locales and missing translations.
// Unknown external diagnostics are preserved verbatim.
func T(locale, message string) string {
	if Supported(locale) {
		if translated := dictionaries[locale][message]; strings.TrimSpace(translated) != "" {
			return translated
		}
	}
	if translated := dictionaries[catalog.DefaultLocale][message]; strings.TrimSpace(translated) != "" {
		return translated
	}
	return message
}
