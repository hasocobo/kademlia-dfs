//go:build tinygo

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:wasmexport run
func run() int32 {
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to read stdin\n")
		return 1
	}

	counts := countWords(data)

	enc := json.NewEncoder(os.Stdout)
	// enc.SetIndent("", "  ")
	if err := enc.Encode(counts); err != nil {
		_, _ = os.Stderr.WriteString("failed to write json\n")
		return 2
	}
	return 0
}

func main() {}

func countWords(b []byte) map[string]uint32 {
	// Treat input as UTF-8 text. If it's arbitrary bytes, you should define a different tokenization.
	s := string(b)

	// Split on whitespace (unicode-aware). This is robust for “normal text”.
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	})

	m := make(map[string]uint32, len(fields))
	for _, w := range fields {
		w = normalizeWord(w)
		if w == "" {
			continue
		}
		m[w]++
	}
	return m
}

func normalizeWord(w string) string {
	// Minimal normalization: lowercase.
	// If you want punctuation stripping, do it here.
	w = strings.ToLower(w)

	// Optional: strip leading/trailing non-letters/digits (keeps "alice," as "alice").
	// This is deliberately simple and not “NLP”.
	w = stripEdgeJunk(w)
	return w
}

func stripEdgeJunk(s string) string {
	// Trim left
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			return ""
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		s = s[size:]
	}
	// Trim right
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			return ""
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		s = s[:len(s)-size]
	}
	return s
}
