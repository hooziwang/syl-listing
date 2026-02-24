package logging

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	mu sync.Mutex
	w  io.Writer
}

type Event struct {
	TS         string `json:"ts"`
	Level      string `json:"level"`
	Event      string `json:"event"`
	Input      string `json:"input,omitempty"`
	Candidate  int    `json:"candidate,omitempty"`
	Lang       string `json:"lang,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	WaitMS     int64  `json:"wait_ms,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
	Error      string `json:"error,omitempty"`
}

func New(stdout io.Writer, logFile string) (*Logger, io.Closer, error) {
	if logFile == "" {
		return &Logger{w: stdout}, nil, nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	w := io.MultiWriter(stdout, f)
	return &Logger{w: w}, f, nil
}

func (l *Logger) Emit(ev Event) {
	if l == nil || l.w == nil {
		return
	}
	if ev.TS == "" {
		ev.TS = time.Now().Format(time.RFC3339Nano)
	}
	if ev.Level == "" {
		ev.Level = "info"
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(append(b, '\n'))
}
