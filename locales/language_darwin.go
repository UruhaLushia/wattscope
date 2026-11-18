//go:build !ios

package locales

import (
	"bytes"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

var systemLanguages = sync.OnceValue(func() string {
	library, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return ""
	}
	defer purego.Dlclose(library)

	var copyPreferredLanguages func() uintptr
	var arrayCount func(uintptr) int
	var arrayValue func(uintptr, int) uintptr
	var stringGetCString func(uintptr, *byte, int, uint32) bool
	var release func(uintptr)
	purego.RegisterLibFunc(&copyPreferredLanguages, library, "CFLocaleCopyPreferredLanguages")
	purego.RegisterLibFunc(&arrayCount, library, "CFArrayGetCount")
	purego.RegisterLibFunc(&arrayValue, library, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&stringGetCString, library, "CFStringGetCString")
	purego.RegisterLibFunc(&release, library, "CFRelease")

	preferred := copyPreferredLanguages()
	if preferred == 0 {
		return ""
	}
	defer release(preferred)

	const cfStringEncodingUTF8 = 0x08000100
	count := arrayCount(preferred)
	languages := make([]string, 0, count)
	var buffer [256]byte
	for index := range count {
		value := arrayValue(preferred, index)
		if stringGetCString(value, &buffer[0], len(buffer), cfStringEncodingUTF8) {
			languages = append(languages, string(buffer[:bytes.IndexByte(buffer[:], 0)]))
		}
	}
	return strings.Join(languages, ":")
})
