package main

import (
	"sync/atomic"

	"github.com/lucasdevco/tokenhub/internal/i18n"
)

var language atomic.Value

func tr(message string) string {
	locale, _ := language.Load().(string)
	return i18n.T(locale, message)
}
