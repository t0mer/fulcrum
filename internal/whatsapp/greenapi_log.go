package whatsapp

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// The green-api bot library logs through Go's package-level logger with no way
// to inject one: it prints every incoming notification body verbatim (leaking
// other group members' messages — a privacy problem for this app) and, when the
// instance is misconfigured, spins in a near-tight loop dumping the raw 403
// response. botLogWriter wraps that output to (a) drop the lines that carry
// message content and (b) throttle identical repeats so a bad instance can't
// flood the log. Fulcrum's own logging uses slog, not the std log package, so
// taking over the std logger only affects this third-party library.

// botLogDrop lists substrings whose lines dump message content and must never
// reach the log.
var botLogDrop = []string{
	"Webhook received - ",           // full decoded notification (message text, senders)
	"Unknown or empty message type", // raw messageData dump
}

// botLogRepeatWindow is how long an identical line is suppressed after being
// emitted once, so an auth-failure loop collapses to roughly one line per window.
const botLogRepeatWindow = 30 * time.Second

type botLogWriter struct {
	out io.Writer
	now func() time.Time // overridable in tests; defaults to time.Now

	mu       sync.Mutex
	lastLine string
	lastAt   time.Time
	seen     bool
}

func (w *botLogWriter) Write(p []byte) (int, error) {
	// Always report the full length written so the std logger never errors,
	// even when we drop or rewrite the line.
	n := len(p)
	line := string(p)

	for _, s := range botLogDrop {
		if strings.Contains(line, s) {
			return n, nil
		}
	}

	// The library appends the raw response to unmarshal errors ("... Raw: <html>
	// 403 ...>"). Trim it so a 403 page (or any payload) can't spill into the log.
	if i := strings.Index(line, ". Raw:"); i != -1 {
		line = strings.TrimRight(line[:i], "\n") + "\n"
	}

	nowFn := w.now
	if nowFn == nil {
		nowFn = time.Now
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	t := nowFn()
	if w.seen && line == w.lastLine && t.Sub(w.lastAt) < botLogRepeatWindow {
		return n, nil
	}
	w.lastLine = line
	w.lastAt = t
	w.seen = true
	if _, err := io.WriteString(w.out, line); err != nil {
		return n, err
	}
	return n, nil
}

// installBotLogFilter routes the standard logger (used only by the green-api bot
// library in this process) through botLogWriter for the caller's lifetime and
// returns a function that restores the previous output and flags.
func installBotLogFilter() func() {
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&botLogWriter{out: os.Stderr})
	log.SetFlags(0) // slog already timestamps real logs; drop the std prefix so repeats coalesce
	return func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}
}
