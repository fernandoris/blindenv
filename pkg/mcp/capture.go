package mcp

import (
	"io"
	"strings"
)

// boundedWriter retains up to limit bytes and discards the rest while still
// reporting full success to the child. Draining (rather than not reading) is
// required: a writer that stops reading would let the OS pipe fill and block
// the child, or, via os/exec, allow the copy goroutine to stall Wait.
type boundedWriter struct {
	limit     int
	buf       []byte
	total     int
	truncated bool
}

func newBoundedWriter(limit int) *boundedWriter {
	if limit < 0 {
		limit = 0
	}
	return &boundedWriter{limit: limit, buf: make([]byte, 0, limit)}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.total += len(p)
	if room := w.limit - len(w.buf); room > 0 {
		if len(p) <= room {
			w.buf = append(w.buf, p...)
		} else {
			w.buf = append(w.buf, p[:room]...)
		}
	}
	if w.total > w.limit {
		w.truncated = true
	}
	return len(p), nil
}

// Bytes returns the retained prefix.
func (w *boundedWriter) Bytes() []byte { return w.buf }

// Total returns how many bytes the writer received in total.
func (w *boundedWriter) Total() int { return w.total }

// Truncated reports whether bytes beyond the limit were discarded.
func (w *boundedWriter) Truncated() bool { return w.truncated }

// StreamRedactor redacts text as it is written, holding back a tail window so a
// secret value split across two writes is never emitted as a fragment.
type StreamRedactor struct {
	dst   io.Writer
	r     *Redactor
	tail  []byte
	count int
	err   error
}

// NewStreamRedactor wraps w with value redaction.
func NewStreamRedactor(w io.Writer, r *Redactor) *StreamRedactor {
	return &StreamRedactor{dst: w, r: r}
}

func (s *StreamRedactor) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.tail = append(s.tail, p...)
	if err := s.flush(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close flushes any retained tail.
func (s *StreamRedactor) Close() error {
	if s.err != nil {
		return s.err
	}
	return s.flush(true)
}

// Count returns the number of substitutions performed so far.
func (s *StreamRedactor) Count() int { return s.count }

func (s *StreamRedactor) flush(final bool) error {
	data := s.tail
	if len(data) == 0 {
		return nil
	}
	limit := len(data)
	if !final {
		keep := s.r.MaxValueLen() - 1
		if keep < 0 {
			keep = 0
		}
		if len(data) <= keep {
			return nil
		}
		limit = len(data) - keep
	}

	// Do not emit a prefix that would split a secret occurrence across the
	// boundary: back the emit point up to the start of the earliest crossing
	// occurrence.
	emit := limit
	for _, e := range s.r.entries {
		offset := 0
		for {
			j := strings.Index(string(data[offset:]), e.value)
			if j < 0 {
				break
			}
			start := offset + j
			end := start + len(e.value)
			if start < limit && end > limit && start < emit {
				emit = start
			}
			offset = start + 1
		}
	}
	if final {
		emit = len(data)
	}
	if emit <= 0 && !final {
		return nil
	}

	out, n := s.r.Redact(data[:emit])
	if _, err := io.WriteString(s.dst, out); err != nil {
		s.err = err
		return err
	}
	s.count += n
	s.tail = append(s.tail[:0], data[emit:]...)
	return nil
}
