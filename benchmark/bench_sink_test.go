package benchmark

import (
	"io"
	"sync"
	"testing"
)

// lockedDiscard is a sink that drops everything while keeping the writer hot
// path closer to a real logger by serialising access.
type lockedDiscard struct {
	mu  sync.Mutex
	sum int64
	tee io.Writer
}

func (l *lockedDiscard) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.sum += int64(len(p))
	tee := l.tee
	l.mu.Unlock()

	if tee != nil {
		if _, err := tee.Write(p); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (l *lockedDiscard) Sync() error {
	return nil
}

func newBenchmarkSink() *lockedDiscard {
	return &lockedDiscard{}
}

func (l *lockedDiscard) resetCount() {
	l.mu.Lock()
	l.sum = 0
	l.mu.Unlock()
}

func (l *lockedDiscard) bytesWritten() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sum
}

func (l *lockedDiscard) setTee(w io.Writer) {
	l.mu.Lock()
	l.tee = w
	l.mu.Unlock()
}

func reportBytesPerOp(b *testing.B, sink *lockedDiscard) {
	total := sink.bytesWritten()
	if b.N > 0 {
		b.ReportMetric(float64(total)/float64(b.N), "bytes/op")
	} else {
		b.ReportMetric(0, "bytes/op")
	}
}
