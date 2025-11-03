package benchmark

import (
	"io"
	"sync"
	"testing"

	plog "github.com/phuslu/log"
	logport "pkt.systems/logport"
	charm "pkt.systems/logport/adapters/charmlogger"
	onelog "pkt.systems/logport/adapters/onelogger"
	phuslu "pkt.systems/logport/adapters/phuslu"
	psl "pkt.systems/logport/adapters/psl"
	slogadapter "pkt.systems/logport/adapters/slogger"
	zapadapter "pkt.systems/logport/adapters/zaplogger"
	zeroadapter "pkt.systems/logport/adapters/zerologger"
)

type adapterFactory struct {
	name string
	make func(io.Writer) logport.ForLogging
}

var benchmarkStaticFields = []any{
	"component", "benchmark",
	"env", "test",
}

func withBenchmarkFields(logger logport.ForLogging, datasetStatic []any) logport.ForLogging {
	if logger == nil {
		return nil
	}
	if len(datasetStatic) > 0 {
		logger = logger.With(datasetStatic...)
	}
	logger = logger.With(benchmarkStaticFields...)
	return logger.LogLevel(logport.TraceLevel)
}

func adapterFactories(datasetStatic []any) []adapterFactory {
	return []adapterFactory{
		{name: "zerolog/console", make: func(w io.Writer) logport.ForLogging { return withBenchmarkFields(zeroadapter.New(w), datasetStatic) }},
		{name: "zerolog/json", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(zeroadapter.NewStructured(w), datasetStatic)
		}},
		{name: "charm/console", make: func(w io.Writer) logport.ForLogging { return withBenchmarkFields(charm.New(w), datasetStatic) }},
		{name: "charm/json", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(charm.NewStructured(w), datasetStatic)
		}},
		{name: "slog/text", make: func(w io.Writer) logport.ForLogging { return withBenchmarkFields(slogadapter.New(w), datasetStatic) }},
		{name: "slog/json", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(slogadapter.NewJSON(w), datasetStatic)
		}},
		{name: "psl/console", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(psl.NewWithOptions(w, psl.Options{Mode: psl.ModeConsole}), datasetStatic)
		}},
		{name: "psl/json", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(psl.NewWithOptions(w, psl.Options{Mode: psl.ModeStructured}), datasetStatic)
		}},
		{name: "phuslu", make: func(w io.Writer) logport.ForLogging {
			return withBenchmarkFields(phuslu.NewWithOptions(w, phuslu.Options{
				Configure: func(l *plog.Logger) { l.Level = plog.TraceLevel },
			}), datasetStatic)
		}},
		{name: "zap", make: func(w io.Writer) logport.ForLogging { return withBenchmarkFields(zapadapter.New(w), datasetStatic) }},
		{name: "onelog", make: func(w io.Writer) logport.ForLogging { return withBenchmarkFields(onelog.New(w), datasetStatic) }},
	}
}

func performanceAdapterFactories(datasetStatic []any) []adapterFactory {
	all := adapterFactories(datasetStatic)
	selected := make([]adapterFactory, 0, len(all))
	for _, adapter := range all {
		switch adapter.name {
		case "zerolog/console",
			"zerolog/json",
			"phuslu",
			"onelog",
			"zap",
			"charm/console",
			"charm/json",
			"slog/text",
			"slog/json",
			"psl/console",
			"psl/json":
			selected = append(selected, adapter)
		}
	}
	return selected
}

var (
	productionEntriesOnce sync.Once
	productionEntries     []productionEntry
	productionEntriesErr  error
)

func loadProductionEntries(b testing.TB) []productionEntry {
	b.Helper()
	productionEntriesOnce.Do(func() {
		productionEntries, productionEntriesErr = loadEmbeddedProductionDataset(0)
	})
	if productionEntriesErr != nil {
		b.Fatalf("load production dataset: %v", productionEntriesErr)
	}
	return productionEntries
}

func BenchmarkAdaptersProductionDataset(b *testing.B) {
	entries := loadProductionEntries(b)
	if len(entries) == 0 {
		b.Fatal("production dataset empty")
	}
	staticWith, _, staticKeys := productionStaticArgs(entries)
	dynamicEntries := productionEntriesWithoutStatic(entries, staticKeys)
	entryCount := len(dynamicEntries)

	factories := adapterFactories(staticWith)

	for _, factory := range factories {
		factory := factory
		b.Run(factory.name, func(b *testing.B) {
			sink := newBenchmarkSink()
			logger := factory.make(sink)
			if logger == nil {
				b.Fatal("logger factory returned nil")
			}

			b.ReportAllocs()
			sink.resetCount()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				entry := dynamicEntries[i%entryCount]
				logger.Logp(entry.level, entry.message, entry.keyvals...)
			}

			reportBytesPerOp(b, sink)
			if written := sink.bytesWritten(); written == 0 {
				b.Fatalf("expected logger to emit output, wrote %d bytes", written)
			}
		})
	}
}
