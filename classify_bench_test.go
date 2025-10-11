package logport

import "testing"

func BenchmarkClassifyPrefix(b *testing.B) {
	line := "ERROR: database unavailable"
	for i := 0; i < b.N; i++ {
		classifyLogLine(line)
	}
}

func BenchmarkClassifyBracketPrefix(b *testing.B) {
	line := "[WARN] backlog growing"
	for i := 0; i < b.N; i++ {
		classifyLogLine(line)
	}
}

func BenchmarkClassifyTLSHandshake(b *testing.B) {
	line := "http: TLS handshake error from 1.2.3.4: remote error: tls: bad certificate"
	for i := 0; i < b.N; i++ {
		classifyLogLine(line)
	}
}

func BenchmarkClassifySubstringFallback(b *testing.B) {
	line := "request failed with unexpected ERROR code 42"
	for i := 0; i < b.N; i++ {
		classifyLogLine(line)
	}
}

func BenchmarkClassifyNoMatch(b *testing.B) {
	line := "plain message without level metadata"
	for i := 0; i < b.N; i++ {
		classifyLogLine(line)
	}
}
