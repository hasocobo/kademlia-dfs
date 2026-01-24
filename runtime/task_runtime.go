package runtime

import (
	"context"
	"net"
)

type TaskRuntime interface {
	RunTask(ctx context.Context, binary []byte)
	EncodeTask(task Task) ([]byte, error)
	DecodeTask(data []byte) Task
}

type TaskNetwork interface {
	SendTask(ctx context.Context, data []byte, addr net.Addr) error
	Listen(ctx context.Context) error
}

type Task struct {
	BinaryLength uint64
	Binary       []byte
	InputFile    []byte
	// TTL              time.Duration
}
