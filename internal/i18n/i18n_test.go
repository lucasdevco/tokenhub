package i18n

import "testing"

func TestTranslations(t *testing.T) {
	for _, locale := range []string{"", "en", "unsupported"} {
		if got := T(locale, "Request logs"); got != "Request logs" {
			t.Fatalf("%q: %q", locale, got)
		}
	}
	if got := T("zh-CN", "Request logs"); got != "请求日志" {
		t.Fatal(got)
	}
	if got := T("en", "external diagnostic"); got != "external diagnostic" {
		t.Fatal(got)
	}
}

func TestMissingTranslationsFallBackToEnglish(t *testing.T) {
	original := dictionaries["zh-CN"]
	dictionaries["zh-CN"] = map[string]string{"Request logs": "   ", "Settings": "设置"}
	t.Cleanup(func() { dictionaries["zh-CN"] = original })
	for _, message := range []string{"Request logs", "App preferences"} {
		if got := T("zh-CN", message); got != T("en", message) {
			t.Fatalf("missing translation did not fall back to English: %q", got)
		}
	}
	if got := T("zh-CN", "Settings"); got != "设置" {
		t.Fatal(got)
	}
}

func TestAdditionalLocaleUsesRegistry(t *testing.T) {
	originalName, hadLocale := catalog.Locales["ja"]
	originalDictionary := dictionaries["ja"]
	catalog.Locales["ja"] = "日本語"
	dictionaries["ja"] = map[string]string{"Request logs": "リクエストログ"}
	t.Cleanup(func() {
		if hadLocale {
			catalog.Locales["ja"] = originalName
			dictionaries["ja"] = originalDictionary
		} else {
			delete(catalog.Locales, "ja")
			delete(dictionaries, "ja")
		}
	})
	if !Supported("ja") || T("ja", "Request logs") != "リクエストログ" {
		t.Fatal("registered locale is not available")
	}
	if got := T("ja", "Settings"); got != "Settings" {
		t.Fatalf("English fallback: %q", got)
	}
}
