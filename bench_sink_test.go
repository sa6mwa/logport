package logport_test

import "sync"

type lockedDiscard struct {
	mu sync.Mutex
}

func (l *lockedDiscard) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.mu.Unlock()
	return len(p), nil
}

func (l *lockedDiscard) Sync() error {
	return nil
}

func newBenchmarkSink() *lockedDiscard {
	return &lockedDiscard{}
}
