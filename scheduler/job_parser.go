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
	ID   JobID
	Name string

	Binary       []byte
	MergerBinary []byte

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

type TaskSpec struct { // TODO: make this an interface
	Input string `yaml:"input, omitempty"`
	Run   string `yaml:"run"`
}

type ExecutionPlan struct {
	Name  string              `json:"name"`
	Total int                 `json:"total"`
	Tasks []ExecutionTaskPlan `json:"tasks"`
}

type ExecutionTaskPlan struct {
	ID        int    `json:"id"`
	Stdin     string `json:"stdin"`
	Metadata  any    `json:"metadata"`
	InputRefs []any  `json:"input_refs"`
}

func (js JobSpec) String() string {
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

type YAMLParser struct{}

func (p YAMLParser) Parse(from io.Reader, to *JobSpec) error {
	err := yaml.NewDecoder(from).Decode(to)
	if err != nil {
		log.Printf("error decoding yaml: %v", err)
		return err
	}
	return nil
}
