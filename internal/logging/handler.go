package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type Handler struct {
	writer io.Writer
	level  slog.Leveler
	mu     *sync.Mutex
	attrs  []attribute
	groups []string
}

func NewHandler(writer io.Writer, level slog.Leveler) *Handler {
	return &Handler{
		writer: writer,
		level:  level,
		mu:     &sync.Mutex{},
	}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	attributes := append([]attribute(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attributes = appendAttributes(attributes, h.groups, attr)
		return true
	})

	var errors []string
	var details []string
	for _, attr := range attributes {
		if attr.key == "error" || strings.HasSuffix(attr.key, ".error") {
			errors = append(errors, attr.value)
			continue
		}
		details = append(details, fmt.Sprintf("%s=%s", attr.key, attr.value))
	}

	var line strings.Builder
	line.WriteString(timestamp.Format("2006-01-02 15:04:05"))
	line.WriteString(" [")
	line.WriteString(levelName(record.Level))
	line.WriteString("] ")
	line.WriteString(singleLine(record.Message))
	if len(errors) > 0 {
		line.WriteString(": ")
		line.WriteString(strings.Join(errors, "; "))
	}
	if len(details) > 0 {
		line.WriteString(" (")
		line.WriteString(strings.Join(details, ", "))
		line.WriteString(")")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintln(h.writer, line.String())
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	for _, attr := range attrs {
		clone.attrs = appendAttributes(clone.attrs, clone.groups, attr)
	}
	return clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *Handler) clone() *Handler {
	return &Handler{
		writer: h.writer,
		level:  h.level,
		mu:     h.mu,
		attrs:  append([]attribute(nil), h.attrs...),
		groups: append([]string(nil), h.groups...),
	}
}

type attribute struct {
	key   string
	value string
}

func appendAttributes(attributes []attribute, groups []string, attr slog.Attr) []attribute {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return attributes
	}

	if attr.Value.Kind() == slog.KindGroup {
		nestedGroups := groups
		if attr.Key != "" {
			nestedGroups = append(append([]string(nil), groups...), attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			attributes = appendAttributes(attributes, nestedGroups, nested)
		}
		return attributes
	}

	keyParts := append([]string(nil), groups...)
	if attr.Key != "" {
		keyParts = append(keyParts, attr.Key)
	}
	return append(attributes, attribute{
		key:   strings.Join(keyParts, "."),
		value: singleLine(valueString(attr.Value)),
	})
}

func valueString(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	default:
		return fmt.Sprint(value.Any())
	}
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\r", `\r`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func levelName(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
