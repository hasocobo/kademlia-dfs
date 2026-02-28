package runtime

import (
	"context"
	"io"
	"net"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

// type DeviceCapabilities int

type TaskRuntime interface {
	RunTask(ctx context.Context, binary []byte, stdin []byte, stdout io.Writer) ([]byte, error)
}

type TaskNetwork interface {
	RequestTask(context.Context, []byte, net.Addr) ([]byte, error)
	SendTask(ctx context.Context, data []byte, addr net.Addr) ([]byte, error)
	Listen(ctx context.Context) error
}

type TaskType string

const (
	TaskTypeBatch  TaskType = "batch"
	TaskTypeStream TaskType = "stream"
)

type RequestMode uint8

const (
	RequestModeNeedBatch RequestMode = iota
	RequestModeNeedBatchOrStream
)

type Task struct {
	OpCode       kademliadfs.OpCode
	TaskID       kademliadfs.NodeId
	JobID        kademliadfs.NodeId
	Type         TaskType
	Topic        string
	RequestMode  RequestMode
	Binary       []byte
	TTL          time.Duration
	Result       []byte
	Stdin        []byte
	Capabilities []string
	Tags         []string
}
