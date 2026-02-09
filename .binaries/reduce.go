//go:build tinygo

package main

import (
	"bufio"
	"encoding/json"
	"os"
)

//go:wasmexport run
func run() int32 {
	in := bufio.NewScanner(os.Stdin)

	// Allow bigger lines than default 64K in case your maps get chunky.
	buf := make([]byte, 0, 64*1024)
	in.Buffer(buf, 4*1024*1024)

	total := make(map[string]uint64)

	for in.Scan() {
		line := in.Bytes()
		if len(trimSpaces(line)) == 0 {
			continue
		}

		var partial map[string]uint64
		if err := json.Unmarshal(line, &partial); err != nil {
			_, _ = os.Stderr.WriteString("bad json line\n")
			return 2
		}

		for k, v := range partial {
			total[k] += v
		}
	}

	if err := in.Err(); err != nil {
		_, _ = os.Stderr.WriteString("stdin read error\n")
		return 1
	}

	enc := json.NewEncoder(os.Stdout)
	// enc.SetIndent("", "  ") // optional
	if err := enc.Encode(total); err != nil {
		_, _ = os.Stderr.WriteString("json encode failed\n")
		return 3
	}

	return 0
}

func main() {}

func trimSpaces(b []byte) []byte {
	// tiny helper to treat whitespace-only lines as empty
	i, j := 0, len(b)
	for i < j {
		c := b[i]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' && c != '\v' && c != '\f' {
			break
		}
		i++
	}
	for j > i {
		c := b[j-1]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' && c != '\v' && c != '\f' {
			break
		}
		j--
	}
	return b[i:j]
}
