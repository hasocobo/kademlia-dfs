package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmRuntime struct{}

func (runtime WasmRuntime) RunTask(ctx context.Context, wasmBinary []byte, stdin []byte) ([]byte, error) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var stdout, stderr bytes.Buffer
	var config wazero.ModuleConfig

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	if len(stdin) == 0 || stdin == nil {
		config = wazero.NewModuleConfig().WithStartFunctions("_initialize").
			WithStdout(&stdout).WithStderr(&stderr)
	} else {
		config = wazero.NewModuleConfig().WithStartFunctions("_initialize").
			WithStdin(bytes.NewReader(stdin)).WithStdout(&stdout).WithStderr(&stderr)
	}

	mod, instErr := r.InstantiateWithConfig(ctx, wasmBinary, config)
	if instErr != nil {
		log.Printf("error in InstantiateWithConfig: %v", instErr)
		return nil, fmt.Errorf("error in InstantiateWithConfig: %v", instErr)
	}
	defer mod.Close(ctx)

	res, err := mod.ExportedFunction("run").Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("error in method call: %v", err)
	}

	log.Printf("result of the operation(stdout): %v", stdout.String())
	log.Printf("result of the operation: %v", res)
	return stdout.Bytes(), nil
}

func EncodeTask(task Task) ([]byte, error) {
	encodedMessage := make([]byte, 0)
	var err error

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.OpCode)
	if err != nil {
		return nil, fmt.Errorf("failed encoding opCode: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed encoding taskID: %v", err)
	}

	binaryLength := uint64(len(task.Binary))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, binaryLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding binaryLength: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.Binary)
	if err != nil {
		return nil, fmt.Errorf("failed encoding binary: %v", err)
	}

	ttlNanos := uint64(task.TTL.Abs().Nanoseconds())
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, ttlNanos)
	if err != nil {
		return nil, fmt.Errorf("failed encoding TTL: %v", err)
	}

	resultLength := uint64(len(task.Result))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, resultLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding resultLength: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.Result)
	if err != nil {
		return nil, fmt.Errorf("failed encoding Result: %v", err)
	}

	stdinLength := uint64(len(task.Stdin))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, stdinLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding inputFileLength: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.Stdin)
	if err != nil {
		return nil, fmt.Errorf("failed encoding inputFile: %v", err)
	}

	return encodedMessage, nil
}

func DecodeTask(data []byte) Task {
	opCode := data[0]
	data = data[1:]

	taskID := [32]byte(data[:32])
	data = data[32:]

	binaryLength := binary.BigEndian.Uint64(data[:8])
	data = data[8:]

	wasmBinary := data[:binaryLength]
	data = data[binaryLength:]

	ttlNanos := binary.BigEndian.Uint64(data[:8])
	data = data[8:]
	ttl := time.Duration(ttlNanos)

	resultLength := binary.BigEndian.Uint64(data[:8])
	data = data[8:]

	result := data[:resultLength]
	data = data[resultLength:]

	stdinLength := binary.BigEndian.Uint64(data[:8])
	data = data[8:]

	stdin := data[:stdinLength]

	return Task{
		OpCode: kademliadfs.OpCode(opCode),
		TaskID: taskID,
		Binary: wasmBinary,
		TTL:    ttl,
		Result: result,
		Stdin:  stdin,
	}
}
