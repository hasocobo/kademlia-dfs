package runtime

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmRuntime struct{}

func (runtime WasmRuntime) RunTask(ctx context.Context, binary []byte) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize")

	mod, _ := r.InstantiateWithConfig(ctx, binary, config)
	res, err := mod.ExportedFunction("run").Call(ctx, 5, 8)
	if err != nil {
		log.Printf("error calling func %v", err)
	}
	log.Printf("result of the operation: %v", int32(res[0]))
}

func (runtime WasmRuntime) EncodeTask(task Task) ([]byte, error) {
	encodedMessage := make([]byte, 0)
	var err error

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.BinaryLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding messageId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.Binary)
	if err != nil {
		return nil, fmt.Errorf("failed encoding messageId: %v", err)
	}

	return encodedMessage, nil
}

func (runtime WasmRuntime) DecodeTask(data []byte) Task {
	length := binary.BigEndian.Uint64(data[:8])

	data = data[8:]
	binary := data[:length]
	return Task{Binary: binary, BinaryLength: length}
}
