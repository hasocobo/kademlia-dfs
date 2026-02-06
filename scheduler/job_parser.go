package scheduler

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

type JobDescription struct {
	ID         JobID
	Name       string
	Binary     []byte
	InputFile  []byte
	TasksDone  int
	TasksTotal int // TODO: replace this part with a job description language like GDL
}

type JobParser interface {
	Parse(from io.Reader, to *JobSpec) error
}

type JobSpec struct {
	Name  string              `yaml:"name"`
	Tasks map[string]TaskSpec `yaml:"tasks"`
}

func (js JobSpec) ToString() string {
	name := js.Name
	if name == "" {
		name = "<unnamed>"
	}

	taskNames := make([]string, 0, len(js.Tasks))
	for n := range js.Tasks {
		taskNames = append(taskNames, n)
	}
	sort.Strings(taskNames)

	var b strings.Builder
	fmt.Fprintf(&b, "job: %s\n", name)
	fmt.Fprintf(&b, "tasks: %d\n", len(taskNames))
	for _, n := range taskNames {
		t := js.Tasks[n]
		if t.Input != "" {
			fmt.Fprintf(&b, "  - %s: run=%s input=%s\n", n, t.Run, t.Input)
		} else {
			fmt.Fprintf(&b, "  - %s: run=%s\n", n, t.Run)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

type TaskSpec struct {
	Input string `yaml:"input, omitempty"`
	Run   string `yaml:"run"`
}

type YAMLParser struct{}

func (p YAMLParser) Parse(from io.Reader, to *JobSpec) error {
	err := yaml.NewDecoder(from).Decode(to)
	if err != nil {
		log.Printf("error decoding yaml: %v", err)
		return err
	}
	return nil
}
