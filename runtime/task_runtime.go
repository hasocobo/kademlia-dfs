package runtime

import (
	"context"
	"net"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

type TaskRuntime interface {
	RunTask(ctx context.Context, binary []byte) ([]byte, error)
	EncodeTask(task Task) ([]byte, error)
	DecodeTask(data []byte) Task
}

type TaskNetwork interface {
	SendTask(ctx context.Context, data []byte, addr net.Addr) ([]byte, error)
	Listen(ctx context.Context) error
}

type Task struct {
	OpCode    kademliadfs.OpCode
	TaskID    [32]byte
	Binary    []byte
	TTL       time.Duration
	Result    []byte
	InputFile []byte
}
