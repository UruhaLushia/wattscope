//go:build !windows && (!darwin || ios)

package locales

func systemLanguages() string { return "" }
