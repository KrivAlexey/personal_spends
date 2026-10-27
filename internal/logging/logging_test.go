package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/logging"
	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		format    string
		wantErr   []string // substrings of the error; nil = no error
		wantDebug bool
		wantInfo  bool
		wantJSON  bool
	}{
		{name: "defaults", wantInfo: true},
		{name: "debug json", level: "debug", format: "json", wantDebug: true, wantInfo: true, wantJSON: true},
		{name: "case-insensitive level", level: "DEBUG", format: "text", wantDebug: true, wantInfo: true},
		{name: "warn hides info", level: "warn", format: "text"},
		{name: "error hides info", level: "Error", format: "json", wantJSON: true},
		{name: "invalid level", level: "verbose", wantErr: []string{"LOG_LEVEL", "verbose"}},
		{name: "level offset", level: "info+4", wantErr: []string{"LOG_LEVEL", "info+4"}},
		{name: "invalid format", format: "xml", wantErr: []string{"LOG_FORMAT", "xml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := logging.New(&buf, tt.level, tt.format)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				for _, s := range tt.wantErr {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("error %q does not name %q", err, s)
					}
				}
				if logger != nil {
					t.Error("expected a nil logger with the error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			logger.Debug("debug record")
			logger.Info("info record")
			out := buf.String()
			if got := strings.Contains(out, "debug record"); got != tt.wantDebug {
				t.Errorf("debug written = %v, want %v:\n%s", got, tt.wantDebug, out)
			}
			if got := strings.Contains(out, "info record"); got != tt.wantInfo {
				t.Errorf("info written = %v, want %v:\n%s", got, tt.wantInfo, out)
			}
			for line := range strings.Lines(out) {
				var rec map[string]any
				isJSON := json.Unmarshal([]byte(line), &rec) == nil
				if isJSON != tt.wantJSON {
					t.Errorf("line is JSON = %v, want %v: %q", isJSON, tt.wantJSON, line)
				}
				if isJSON {
					for _, k := range []string{"time", "level", "msg"} {
						if _, ok := rec[k]; !ok {
							t.Errorf("JSON record lacks %q: %q", k, line)
						}
					}
				} else if !strings.Contains(line, "time=") || !strings.Contains(line, "level=") || !strings.Contains(line, "msg=") {
					t.Errorf("text record lacks time/level/msg: %q", line)
				}
			}
		})
	}
}

func TestNew_CarriesContextAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.New(&buf, "info", "json")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := logging.WithAttrs(context.Background(), slog.String("request_id", "abc"))
	logger.InfoContext(ctx, "hello")

	if !strings.Contains(buf.String(), `"request_id":"abc"`) {
		t.Errorf("record lacks request_id from the context: %s", buf.String())
	}
}

func TestHandler_ContextAttrs(t *testing.T) {
	ctx := logging.WithAttrs(context.Background(), slog.String("request_id", "r1"), slog.Int("batch", 1), slog.Int("batches", 2))

	tests := []struct {
		name string
		log  func(l *slog.Logger, ctx context.Context)
		want map[string]any // top-level fields, besides the context ones
		path []string       // where the context attrs land; nil = top level
	}{
		{
			name: "plain record",
			log:  func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "m") },
		},
		{
			name: "logger.With keeps context attrs",
			log:  func(l *slog.Logger, ctx context.Context) { l.With("k", "v").InfoContext(ctx, "m") },
			want: map[string]any{"k": "v"},
		},
		{
			name: "WithGroup keeps context attrs",
			log:  func(l *slog.Logger, ctx context.Context) { l.WithGroup("g").InfoContext(ctx, "m", "k", "v") },
			path: []string{"g"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := logtest.NewLogger(slog.LevelInfo)
			tt.log(logger, ctx)

			recs := logtest.Records(t, buf)
			if len(recs) != 1 {
				t.Fatalf("got %d records, want 1:\n%s", len(recs), buf)
			}
			fields := map[string]any(recs[0])
			for _, g := range tt.path {
				fields, _ = fields[g].(map[string]any)
			}
			for k, want := range map[string]any{"request_id": "r1", "batch": 1.0, "batches": 2.0} {
				if fields[k] != want {
					t.Errorf("%s = %v, want %v:\n%s", k, fields[k], want, buf)
				}
			}
			for k, want := range tt.want {
				if recs[0][k] != want {
					t.Errorf("%s = %v, want %v", k, recs[0][k], want)
				}
			}
		})
	}
}

func TestWithAttrs_ReplacesKey(t *testing.T) {
	ctx := logging.WithAttrs(context.Background(), slog.String("request_id", "r1"), slog.Int("batch", 1))
	ctx = logging.WithAttrs(ctx, slog.Int("batch", 2))

	logger, buf := logtest.NewLogger(slog.LevelInfo)
	logger.InfoContext(ctx, "m")

	out := buf.String()
	if n := strings.Count(out, `"batch":`); n != 1 {
		t.Errorf("batch appears %d times, want 1: %s", n, out)
	}
	if !strings.Contains(out, `"batch":2`) || !strings.Contains(out, `"request_id":"r1"`) {
		t.Errorf("want batch=2 and request_id=r1: %s", out)
	}
}

func TestWithAttrs_DoesNotChangeParent(t *testing.T) {
	parent := logging.WithAttrs(context.Background(), slog.Int("batch", 1))
	_ = logging.WithAttrs(parent, slog.Int("batch", 2))

	logger, buf := logtest.NewLogger(slog.LevelInfo)
	logger.InfoContext(parent, "m")

	if !strings.Contains(buf.String(), `"batch":1`) {
		t.Errorf("parent context changed: %s", buf)
	}
}
