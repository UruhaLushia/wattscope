package locales

import "testing"

func TestResolveCachingAndDynamicAuto(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"en", "en"},
		{"ja", "ja"},
		{"zh-Hans", "zh-Hans"},
		{"zh-Hant", "zh-Hant"},
		{"zh_CN.UTF-8", "zh-Hans"},
		{"zh_TW.UTF-8", "zh-Hant"},
		{"ja_JP.UTF-8", "ja"},
		{"fr_FR:zh_CN.UTF-8", "zh-Hans"},
		{"xx-Invalid-Locale", "en"},
	} {
		if got := Resolve(tc.in); got != tc.want {
			t.Fatalf("Resolve(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Second call should hit fast path or cache with identical result.
		if got := Resolve(tc.in); got != tc.want {
			t.Fatalf("Resolve(%q) cached = %q, want %q", tc.in, got, tc.want)
		}
	}

	t.Setenv("WATTSCOPE_LANG", "zh_CN.UTF-8")
	if got := Resolve("auto"); got != "zh-Hans" {
		t.Fatalf("Resolve(auto) with WATTSCOPE_LANG=zh_CN.UTF-8 = %q, want zh-Hans", got)
	}

	t.Setenv("WATTSCOPE_LANG", "ja")
	if got := Resolve(""); got != "ja" {
		t.Fatalf("Resolve(\"\") with WATTSCOPE_LANG=ja = %q, want ja", got)
	}

	t.Setenv("WATTSCOPE_LANG", "zh_Hant_HK")
	if got := Resolve("auto"); got != "zh-Hant" {
		t.Fatalf("Resolve(auto) with WATTSCOPE_LANG=zh_Hant_HK = %q, want zh-Hant", got)
	}
}
