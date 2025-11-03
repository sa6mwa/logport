package benchmark

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	plog "github.com/phuslu/log"
	"github.com/rs/zerolog"
	"go.uber.org/zap"

	logport "pkt.systems/logport"
)

type productionEntry struct {
	level   logport.Level
	message string
	keyvals []any
	fields  []productionKV
}

type productionKV struct {
	key   string
	value any
}

func (e productionEntry) log(logger logport.ForLogging) {
	logger.Logp(e.level, e.message, e.keyvals...)
}

func (e productionEntry) applyZerolog(ev *zerolog.Event) *zerolog.Event {
	for _, field := range e.fields {
		switch v := field.value.(type) {
		case string:
			ev.Str(field.key, v)
		case bool:
			ev.Bool(field.key, v)
		case int:
			ev.Int(field.key, v)
		case int64:
			ev.Int64(field.key, v)
		case uint64:
			ev.Uint64(field.key, v)
		case float64:
			ev.Float64(field.key, v)
		case []byte:
			ev.Bytes(field.key, v)
		default:
			ev.Interface(field.key, v)
		}
	}
	return ev
}

func (e productionEntry) applyPhuslu(entry *plog.Entry) {
	for _, field := range e.fields {
		switch v := field.value.(type) {
		case string:
			entry.Str(field.key, v)
		case bool:
			entry.Bool(field.key, v)
		case int:
			entry.Int(field.key, v)
		case int64:
			entry.Int64(field.key, v)
		case uint64:
			entry.Uint64(field.key, v)
		case float64:
			entry.Float64(field.key, v)
		default:
			entry.Any(field.key, v)
		}
	}
}

func (e productionEntry) zapFieldsSlice() []zap.Field {
	if len(e.fields) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, len(e.fields))
	for _, field := range e.fields {
		fields = append(fields, zapFieldFromValue(field.key, field.value))
	}
	return fields
}

func (e productionEntry) toMap() map[string]any {
	out := make(map[string]any, len(e.fields))
	for _, kv := range e.fields {
		out[kv.key] = kv.value
	}
	return out
}

func (e productionEntry) forEachField(fn func(string, any)) {
	for _, kv := range e.fields {
		fn(kv.key, kv.value)
	}
}

type dataset struct {
	entries []productionEntry
}

func loadEmbeddedProductionDataset(limit int) ([]productionEntry, error) {
	max := len(embeddedProductionDataset)
	if max == 0 {
		return nil, errors.New("embedded production dataset empty")
	}
	if limit <= 0 || limit > max {
		limit = max
	}
	entries := make([]productionEntry, 0, limit)
	for i := 0; i < limit; i++ {
		line := embeddedProductionDataset[i]
		entry, err := parseProductionLine(line)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errors.New("no production log entries parsed")
	}
	return entries, nil
}

func parseProductionLine(line string) (productionEntry, error) {
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	raw := make(map[string]any)
	if err := decoder.Decode(&raw); err != nil {
		return productionEntry{}, err
	}

	level := logport.InfoLevel
	if lvl, ok := raw["lvl"].(string); ok {
		if parsed, ok := logport.ParseLevel(lvl); ok {
			level = parsed
			// Promote TRACE → DEBUG and DEBUG → INFO so that benchmarks remain comparable
			// for loggers without fine-grained levels.
			switch level {
			case logport.TraceLevel:
				level = logport.DebugLevel
			case logport.DebugLevel:
				level = logport.InfoLevel
			}
		}
	}
	delete(raw, "lvl")

	message := ""
	if msg, ok := raw["msg"].(string); ok {
		message = msg
	}
	delete(raw, "msg")
	delete(raw, "ts")
	delete(raw, "ts_iso")
	delete(raw, "time")

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	keyvals := make([]any, 0, len(keys)*2)
	fields := make([]productionKV, 0, len(keys))
	for _, k := range keys {
		value := sanitizeJSONValue(raw[k])
		keyvals = append(keyvals, k, value)
		fields = append(fields, productionKV{key: k, value: value})
	}

	return productionEntry{
		level:   level,
		message: message,
		keyvals: keyvals,
		fields:  fields,
	}, nil
}

func sanitizeJSONValue(v any) any {
	switch val := v.(type) {
	case json.Number:
		s := val.String()
		if !strings.ContainsAny(s, ".eE") {
			if i, err := val.Int64(); err == nil {
				return i
			}
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return s
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = sanitizeJSONValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = sanitizeJSONValue(vv)
		}
		return out
	case string:
		return val
	default:
		return val
	}
}

func zapFieldFromValue(key string, value any) zap.Field {
	switch v := value.(type) {
	case string:
		return zap.String(key, v)
	case bool:
		return zap.Bool(key, v)
	case int:
		return zap.Int(key, v)
	case int64:
		return zap.Int64(key, v)
	case uint64:
		return zap.Uint64(key, v)
	case float64:
		return zap.Float64(key, v)
	case []byte:
		return zap.ByteString(key, v)
	default:
		return zap.Any(key, v)
	}
}

func productionStaticArgs(entries []productionEntry) ([]any, map[string]any, map[string]struct{}) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	constants := entries[0].toMap()
	for _, entry := range entries[1:] {
		entryMap := entry.toMap()
		for key, value := range constants {
			other, ok := entryMap[key]
			if !ok || !reflect.DeepEqual(other, value) {
				delete(constants, key)
			}
			if len(constants) == 0 {
				break
			}
		}
		if len(constants) == 0 {
			break
		}
	}
	if len(constants) == 0 {
		return nil, nil, nil
	}
	keys := make([]string, 0, len(constants))
	for key := range constants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	withArgs := make([]any, 0, len(keys)*2)
	staticSet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		withArgs = append(withArgs, key, constants[key])
		staticSet[key] = struct{}{}
	}
	return withArgs, constants, staticSet
}

func productionEntriesWithoutStatic(entries []productionEntry, staticKeys map[string]struct{}) []productionEntry {
	if len(staticKeys) == 0 {
		return entries
	}
	filtered := make([]productionEntry, len(entries))
	for i, entry := range entries {
		filtered[i] = entry.withoutStatic(staticKeys)
	}
	return filtered
}

func (e productionEntry) withoutStatic(staticKeys map[string]struct{}) productionEntry {
	if len(staticKeys) == 0 {
		return e
	}
	filtered := productionEntry{
		level:   e.level,
		message: e.message,
	}
	if len(e.keyvals) > 0 {
		filtered.keyvals = filterStaticKeyvals(e.keyvals, staticKeys)
	}
	if len(e.fields) > 0 {
		fields := make([]productionKV, 0, len(e.fields))
		for _, field := range e.fields {
			if _, ok := staticKeys[field.key]; ok {
				continue
			}
			fields = append(fields, field)
		}
		filtered.fields = fields
	}
	return filtered
}

func filterStaticKeyvals(keyvals []any, staticKeys map[string]struct{}) []any {
	if len(keyvals) == 0 {
		return nil
	}
	filtered := make([]any, 0, len(keyvals))
	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		if _, exists := staticKeys[key]; exists {
			if i+1 < len(keyvals) {
				continue
			}
			break
		}
		if i+1 < len(keyvals) {
			filtered = append(filtered, keyvals[i], keyvals[i+1])
		} else {
			filtered = append(filtered, keyvals[i])
		}
	}
	return filtered
}
