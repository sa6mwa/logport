package logport

import "strings"

const httpTLSHandshakePrefix = "http: TLS handshake error"

func classifyLogLine(raw string) (Level, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return NoLevel, ""
	}

	if strings.HasPrefix(trimmed, httpTLSHandshakePrefix) {
		msg := strings.TrimSpace(trimmed[len(httpTLSHandshakePrefix):])
		msg = strings.TrimLeft(msg, ": ")
		return ErrorLevel, msg
	}

	if strings.HasPrefix(trimmed, "[") {
		if idx := strings.Index(trimmed, "]"); idx > 1 {
			token := trimmed[1:idx]
			if level, ok := levelFromToken(token); ok {
				msg := strings.TrimSpace(trimmed[idx+1:])
				return level, msg
			}
		}
	}

	token, rest := splitLeadingToken(trimmed)
	if level, ok := levelFromToken(token); ok {
		msg := strings.TrimLeft(rest, " \t")
		msg = strings.TrimPrefix(msg, ":")
		msg = strings.TrimLeft(msg, " \t-|:")
		return level, strings.TrimSpace(msg)
	}

	if idx := strings.IndexAny(trimmed, ":|-"); idx > 0 {
		token := trimmed[:idx]
		if level, ok := levelFromToken(token); ok {
			msg := strings.TrimSpace(trimmed[idx+1:])
			return level, msg
		}
	}

	if idx := indexErrorSubstring(trimmed); idx >= 0 {
		msg := ""
		if idx+len(errorNeedle) < len(trimmed) {
			msg = strings.TrimSpace(trimmed[idx+len(errorNeedle):])
			msg = strings.TrimLeft(msg, ": ")
		}
		if msg == "" {
			msg = trimmed
		}
		return ErrorLevel, msg
	}

	return NoLevel, trimmed
}

func splitLeadingToken(s string) (string, string) {
	for i := range len(s) {
		switch s[i] {
		case ' ', '\t', ':', '|', '-':
			return s[:i], s[i:]
		}
	}
	return s, ""
}

func levelFromToken(token string) (Level, bool) {
	start := 0
	end := len(token)
	for start < end && isDelimiter(token[start]) {
		start++
	}
	for end > start && isDelimiter(token[end-1]) {
		end--
	}
	if start == end {
		return NoLevel, false
	}
	normalized := token[start:end]
	switch {
	case asciiEqualFold(normalized, "trace"), asciiEqualFold(normalized, "trc"):
		return TraceLevel, true
	case asciiEqualFold(normalized, "debug"), asciiEqualFold(normalized, "dbg"):
		return DebugLevel, true
	case asciiEqualFold(normalized, "info"), asciiEqualFold(normalized, "inf"), asciiEqualFold(normalized, "information"):
		return InfoLevel, true
	case asciiEqualFold(normalized, "warn"), asciiEqualFold(normalized, "warning"), asciiEqualFold(normalized, "wrn"):
		return WarnLevel, true
	case asciiEqualFold(normalized, "error"), asciiEqualFold(normalized, "err"):
		return ErrorLevel, true
	case asciiEqualFold(normalized, "fatal"), asciiEqualFold(normalized, "crit"), asciiEqualFold(normalized, "critical"):
		return FatalLevel, true
	case asciiEqualFold(normalized, "panic"), asciiEqualFold(normalized, "alert"):
		return PanicLevel, true
	default:
		return NoLevel, false
	}
}

func isDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', ':', '|', '-', '[', ']', '(', ')', '{', '}', '<', '>', ';', '.', ',', '"', '\'':
		return true
	default:
		return false
	}
}

const errorNeedle = "error"

func indexErrorSubstring(s string) int {
	n := len(errorNeedle)
	limit := len(s) - n
	for i := 0; i <= limit; i++ {
		if toLowerASCII(s[i]) != 'e' {
			continue
		}
		match := true
		for j := 1; j < n; j++ {
			if toLowerASCII(s[i+j]) != errorNeedle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func toLowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func asciiEqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := range len(s) {
		if toLowerASCII(s[i]) != t[i] {
			return false
		}
	}
	return true
}
