package locales

import (
	"os"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

//go:generate go tool mygo generate ..

func LanguageName(tag string) string {
	switch tag {
	case "zh-Hans":
		return "简体中文"
	case "zh-Hant":
		return "繁體中文"
	}
	if name := display.Self.Name(language.Make(tag)); name != "" {
		return name
	}
	return tag
}

// Resolve chooses one of the shipped languages. Automatic detection checks
// WATTSCOPE_LANG, system preferred languages, then POSIX locale variables.
// Unsupported tags fall back to English.
func Resolve(requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.EqualFold(requested, "auto") {
		requested = automaticLanguages()
	}
	langs := Catalog.Languages()
	tags := make([]language.Tag, len(langs))
	for i, lang := range langs {
		tags[i] = language.Make(lang)
	}
	matcher := language.NewMatcher(tags)
	for value := range strings.SplitSeq(requested, ":") {
		// Accept POSIX forms such as zh_CN.UTF-8, and language preference lists.
		value = strings.TrimSpace(value)
		value, _, _ = strings.Cut(value, ".")
		value, _, _ = strings.Cut(value, "@")
		value = strings.ReplaceAll(value, "_", "-")
		tag, err := language.Parse(value)
		if err != nil {
			continue
		}
		_, index, confidence := matcher.Match(tag)
		if confidence != language.No {
			return langs[index]
		}
	}
	return "en"
}

func automaticLanguages() string {
	if value := strings.TrimSpace(os.Getenv("WATTSCOPE_LANG")); value != "" && !strings.EqualFold(value, "auto") {
		return value
	}
	if languages := systemLanguages(); languages != "" {
		return languages
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" && !strings.EqualFold(value, "auto") {
			return value
		}
	}
	return "en"
}
