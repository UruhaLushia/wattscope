package diagnostics

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

type loggerKey struct{}

var discard = slog.New(slog.DiscardHandler)

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func Logger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return discard
}

type Entry struct {
	Sequence uint64
	Time     time.Time
	Level    slog.Level
	Message  string
	Fields   []Field
	Error    string
}

type Field struct {
	Key   string
	Value string
}

type Buffer struct {
	mu       sync.Mutex
	standard entryRing
	debug    entryRing
	seq      uint64
}

const entryLimit = 200

type entryRing struct {
	entries [entryLimit]Entry
	next    int
	size    int
}

func (ring *entryRing) push(entry Entry) {
	ring.entries[ring.next] = entry
	ring.next = (ring.next + 1) % entryLimit
	ring.size = min(ring.size+1, entryLimit)
}

func (ring *entryRing) newest(offset int) *Entry {
	if offset >= ring.size {
		return nil
	}
	index := ring.next - 1 - offset
	if index < 0 {
		index += entryLimit
	}
	return &ring.entries[index]
}

func (buf *Buffer) Revision() uint64 {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	return buf.seq
}

func (buf *Buffer) LatestFirst(minimum slog.Level) []Entry {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	capacity := buf.standard.size
	standard := buf.standard.newest(0)
	var debug *Entry
	if minimum < slog.LevelInfo {
		capacity += buf.debug.size
		debug = buf.debug.newest(0)
	}
	latest := make([]Entry, 0, min(capacity, entryLimit))
	fieldCount := 0
	for standardOffset, debugOffset := 0, 0; len(latest) < entryLimit && (standard != nil || debug != nil); {
		entry := standard
		if standard == nil || (debug != nil && debug.Sequence > standard.Sequence) {
			entry = debug
			debugOffset++
			debug = buf.debug.newest(debugOffset)
		} else {
			standardOffset++
			standard = buf.standard.newest(standardOffset)
		}
		if entry.Level >= minimum {
			latest = append(latest, *entry)
			fieldCount += len(entry.Fields)
		}
	}
	if fieldCount == 0 {
		return latest
	}
	fields := make([]Field, fieldCount)
	for index := range latest {
		entry := &latest[index]
		if len(entry.Fields) > 0 {
			copied := copy(fields, entry.Fields)
			entry.Fields = fields[:copied:copied]
			fields = fields[copied:]
		}
	}
	return latest
}

func (buf *Buffer) Handler() slog.Handler {
	return bufferHandler{buffer: buf}
}

type bufferHandler struct {
	buffer *Buffer
	group  string
	bound  Entry
}

func (bufferHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

func (handler bufferHandler) Handle(_ context.Context, record slog.Record) error {
	entry := handler.bound
	entry.Fields = slices.Clip(entry.Fields)
	entry.Time, entry.Level, entry.Message = record.Time, record.Level, record.Message
	record.Attrs(func(attr slog.Attr) bool {
		entry.addAttr(handler.group, attr)
		return true
	})
	buf := handler.buffer
	buf.mu.Lock()
	buf.seq++
	entry.Sequence = buf.seq
	if entry.Level < slog.LevelInfo {
		buf.debug.push(entry)
	} else {
		buf.standard.push(entry)
	}
	buf.mu.Unlock()
	return nil
}

func (handler bufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handler.bound.Fields = slices.Clip(handler.bound.Fields)
	for _, attr := range attrs {
		handler.bound.addAttr(handler.group, attr)
	}
	return handler
}

func (handler bufferHandler) WithGroup(name string) slog.Handler {
	if name != "" {
		handler.group += name + "."
	}
	return handler
}

func (entry *Entry) addAttr(prefix string, attr slog.Attr) {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			prefix += attr.Key + "."
		}
		for _, child := range value.Group() {
			entry.addAttr(prefix, child)
		}
		return
	}
	if value.Kind() == slog.KindAny && value.Any() == nil {
		return
	}
	text := value.String()
	if text == "" {
		return
	}
	key := prefix + attr.Key
	if key == "error" {
		entry.Error = text
		return
	}
	entry.Fields = append(entry.Fields, Field{Key: key, Value: text})
}
