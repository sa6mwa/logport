package logport_test

import (
	"io"
	"testing"

	logport "pkt.systems/logport"
)

func BenchmarkAdapterInfo(b *testing.B) {
	for _, adapter := range adapterFactories() {
		adapter := adapter
		b.Run(adapter.name, func(b *testing.B) {
			logger := adapter.make(io.Discard)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				logger.Info("bench-info")
			}
		})
	}
}

func BenchmarkAdapterError(b *testing.B) {
	for _, adapter := range adapterFactories() {
		adapter := adapter
		b.Run(adapter.name, func(b *testing.B) {
			logger := adapter.make(io.Discard)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				logger.Error("bench-error")
			}
		})
	}
}

func BenchmarkAdapterLogLogger(b *testing.B) {
	cases := []struct {
		name    string
		message string
	}{
		{name: "info_prefix", message: "[INFO] started"},
		{name: "no_match", message: "plain line"},
		{name: "error_substring", message: "worker reported error condition"},
	}

	for _, adapter := range adapterFactories() {
		adapter := adapter
		for _, tc := range cases {
			tc := tc
			b.Run(adapter.name+"/"+tc.name, func(b *testing.B) {
				logger := adapter.make(io.Discard)
				std := logport.LogLogger(logger)
				std.SetFlags(0)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					std.Println(tc.message)
				}
			})
		}
	}
}
