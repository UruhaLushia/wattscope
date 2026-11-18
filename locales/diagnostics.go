package locales

import (
	"log/slog"
	"strings"
)

func DiagnosticTextOptions() *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.MessageKey {
				attr.Value = slog.StringValue(In("en").DiagnosticTitle(attr.Value.String()))
			}
			return attr
		},
	}
}

func (msgs Messages) DiagnosticTitle(event string) string {
	if !strings.HasPrefix(event, "log_") {
		return event
	}
	return Catalog.Format(msgs.locale, event, nil)
}

func (msgs Messages) DiagnosticLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return msgs.LogLevelError()
	case level >= slog.LevelWarn:
		return msgs.LogLevelWarning()
	case level >= slog.LevelInfo:
		return msgs.LogLevelInfo()
	default:
		return msgs.LogLevelDebug()
	}
}

func (msgs Messages) DiagnosticField(key, value string) string {
	labelKey := "log_field_" + key
	label := Catalog.Format(msgs.locale, labelKey, nil)
	if label == labelKey {
		label = strings.ReplaceAll(key, "_", " ")
	}
	switch key {
	case "os":
		switch value {
		case "darwin":
			value = "macOS"
		case "linux":
			value = "Linux"
		case "windows":
			value = "Windows"
		case "android":
			value = "Android"
		}
	case "stage":
		stageKey := "log_stage_" + value
		if stage := Catalog.Format(msgs.locale, stageKey, nil); stage != stageKey {
			value = stage
		}
	case "transport":
		switch value {
		case "simulated":
			value = msgs.LogTransportSimulated()
		case "usb":
			value = "USB"
		case "hid":
			value = "HID"
		}
	case "demo", "reconnect", "retry":
		switch value {
		case "true":
			value = msgs.LogValueYes()
		case "false":
			value = msgs.LogValueNo()
		}
	}
	if label == "" {
		return value
	}
	return msgs.LogFieldValue(label, value)
}

func (msgs Messages) DiagnosticError(kind string) string {
	key := "log_error_" + kind
	text := Catalog.Format(msgs.locale, key, nil)
	if text == key {
		return ""
	}
	return text
}
