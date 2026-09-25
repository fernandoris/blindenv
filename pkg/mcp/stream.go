package mcp

import (
	"io"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// NewRedactingWriter returns a writer that normalizes child output to UTF-8 and
// redacts secret values as it streams. Close flushes any buffered tail.
func NewRedactingWriter(dst io.Writer, r *Redactor) io.WriteCloser {
	return newNormalizingWriter(NewStreamRedactor(dst, r))
}

// normalizingWriter decodes a byte stream to UTF-8 incrementally, honoring a
// UTF-8/UTF-16 byte order mark and the BOM-less UTF-16LE heuristic, so
// redaction sees UTF-8 across arbitrary write boundaries.
type normalizingWriter struct {
	dst     io.Writer
	pending []byte
	dec     transform.Transformer
	decided bool
}

func newNormalizingWriter(dst io.Writer) *normalizingWriter {
	return &normalizingWriter{dst: dst}
}

func (n *normalizingWriter) Write(p []byte) (int, error) {
	n.pending = append(n.pending, p...)
	if !n.decided && !n.decide(false) {
		return len(p), nil
	}
	if err := n.decode(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (n *normalizingWriter) Close() error {
	if !n.decided {
		n.decide(true)
	}
	if err := n.decode(true); err != nil {
		return err
	}
	if c, ok := n.dst.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (n *normalizingWriter) decide(final bool) bool {
	const sample = 64
	b := n.pending
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		n.pending = append([]byte(nil), b[3:]...)
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		n.dec = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
		n.pending = append([]byte(nil), b[2:]...)
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		n.dec = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
		n.pending = append([]byte(nil), b[2:]...)
	case len(b) >= sample || (final && len(b) >= 4):
		trimmed := b[:len(b)-len(b)%2]
		if len(trimmed) >= 4 && looksUTF16LE(trimmed) {
			n.dec = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
		}
	default:
		return false
	}
	n.decided = true
	return true
}

func (n *normalizingWriter) decode(atEOF bool) error {
	if n.dec == nil {
		if len(n.pending) == 0 {
			return nil
		}
		_, err := n.dst.Write(n.pending)
		n.pending = n.pending[:0]
		return err
	}
	for len(n.pending) > 0 {
		size := len(n.pending)*2 + 16
		if size < 4096 {
			size = 4096
		}
		out := make([]byte, size)
		nDst, nSrc, err := n.dec.Transform(out, n.pending, atEOF)
		if nDst > 0 {
			if _, werr := n.dst.Write(out[:nDst]); werr != nil {
				return werr
			}
		}
		n.pending = n.pending[nSrc:]
		switch err {
		case nil:
			return nil
		case transform.ErrShortDst:
			continue
		case transform.ErrShortSrc:
			if atEOF && len(n.pending) > 0 {
				if _, werr := n.dst.Write(n.pending); werr != nil {
					return werr
				}
				n.pending = n.pending[:0]
			}
			return nil
		default:
			return err
		}
	}
	return nil
}
