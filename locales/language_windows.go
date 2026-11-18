package locales

import (
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

var systemLanguages = sync.OnceValue(func() string {
	languages, _ := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	return strings.Join(languages, ":")
})
