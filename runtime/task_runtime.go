package runtime

import (
	"context"
	"net"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

// type DeviceCapabilities int

type TaskRuntime interface {
	RunTask(ctx context.Context, binary []byte, stdin []byte) ([]byte, error)
}

type TaskNetwork interface {
	RequestTask(context.Context, []byte, net.Addr) ([]byte, error)
	SendTask(ctx context.Context, data []byte, addr net.Addr) ([]byte, error)
	Listen(ctx context.Context) error
}

type Task struct {
	OpCode kademliadfs.OpCode
	TaskID [32]byte
	Binary []byte
	TTL    time.Duration
	Result []byte
	Stdin  []byte
}
