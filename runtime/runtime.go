package runtime

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmJob struct {
	// Name             string
	WasmBinaryLength uint64
	WasmBinary       []byte
	InputFile        []byte
}

type WasmTask struct {
	WasmBinaryLength uint64
	WasmBinary       []byte
	InputFile        []byte
	// TTL              time.Duration
}

type WasmNetwork interface {
	SendTask(ctx context.Context, data []byte, addr net.Addr) error
	Listen(ctx context.Context) error
}

func EncodeWasmTask(task WasmTask) ([]byte, error) {
	encodedMessage := make([]byte, 0)
	var err error

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.WasmBinaryLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding messageId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.WasmBinary)
	if err != nil {
		return nil, fmt.Errorf("failed encoding messageId: %v", err)
	}

	return encodedMessage, nil
}

func DecodeWasmTask(data []byte) WasmTask {
	length := binary.BigEndian.Uint64(data[:8])

	data = data[8:]
	wasmBinary := data[:length]
	return WasmTask{WasmBinary: wasmBinary, WasmBinaryLength: length}
}

func RunWasmTask(wasmBinary []byte) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize")

	mod, _ := r.InstantiateWithConfig(ctx, wasmBinary, config)
	res, err := mod.ExportedFunction("run").Call(ctx, 5, 8)
	if err != nil {
		log.Printf("error calling func %v", err)
	}
	log.Printf("result of the operation: %v", int32(res[0]))
}
