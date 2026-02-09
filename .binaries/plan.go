//go:build tinygo

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
)

type Plan struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
	Tasks []Task `json:"tasks"`
}

type Task struct {
	ID    int    `json:"id"`
	Stdin string `json:"stdin"`
	// Optional, if you want:
	// Meta any `json:"meta,omitempty"`
}

//go:wasmexport run
func run() int32 {
	const targetSize = 64
	const name = "count-words"

	data, err := readAll(os.Stdin)
	if err != nil {
		_, _ = os.Stderr.WriteString("read stdin failed\n")
		return 1
	}

	tasks := splitToTasksBase64(data, targetSize)

	plan := Plan{Name: name, Total: len(tasks), Tasks: tasks}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		_, _ = os.Stderr.WriteString("json encode failed\n")
		return 2
	}

	return 0
}

func main() {}

func readAll(r io.Reader) ([]byte, error) {
	br := bufio.NewReader(r)
	return io.ReadAll(br)
}

func splitToTasksBase64(data []byte, targetSize int) []Task {
	if targetSize <= 0 {
		targetSize = 64
	}

	tasks := make([]Task, 0, (len(data)/targetSize)+1)

	n := len(data)
	i := 0
	id := 0

	for i < n {
		for i < n && isSpace(data[i]) {
			i++
		}
		if i >= n {
			break
		}

		start := i
		end := start + targetSize
		if end > n {
			end = n
		}

		if end < n {
			for end < n && !isSpace(data[end]) {
				end++
			}
		}

		chunk := data[start:end]
		tasks = append(tasks, Task{
			ID:    id,
			Stdin: base64.StdEncoding.EncodeToString(chunk),
		})

		id++
		i = end
	}

	return tasks
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t', '\v', '\f':
		return true
	default:
		return false
	}
}
