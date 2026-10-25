// Package logtest captures JSON log records in tests.
package logtest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/logging"
)

// Buffer is a bytes.Buffer safe for a server goroutine to write while the
// test reads.
type Buffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Record is one decoded JSON log record.
type Record map[string]any

// Level returns the record's level, e.g. "DEBUG".
func (r Record) Level() string { s, _ := r["level"].(string); return s }

// Msg returns the record's message.
func (r Record) Msg() string { s, _ := r["msg"].(string); return s }

// NewLogger returns a JSON logger at level into a new Buffer, with the
// context-attribute handler the service uses.
func NewLogger(level slog.Level) (*slog.Logger, *Buffer) {
	buf := &Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})
	return slog.New(logging.NewHandler(h)), buf
}

// Records decodes every line of buf as a JSON record.
func Records(t testing.TB, buf *Buffer) []Record {
	t.Helper()
	var out []Record
	for line := range strings.Lines(buf.String()) {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

// Filter returns the records with the given level and, if msg isn't empty,
// that message.
func Filter(records []Record, level, msg string) []Record {
	var out []Record
	for _, r := range records {
		if r.Level() == level && (msg == "" || r.Msg() == msg) {
			out = append(out, r)
		}
	}
	return out
}

// AssertLacks fails if buf contains any of values, compared
// case-insensitively.
func AssertLacks(t testing.TB, buf *Buffer, values ...string) {
	t.Helper()
	out := strings.ToLower(buf.String())
	for _, v := range values {
		if v != "" && strings.Contains(out, strings.ToLower(v)) {
			t.Errorf("log output contains %q:\n%s", v, buf.String())
		}
	}
}
