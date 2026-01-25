package runtime

import (
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

func (runtime WasmRuntime) RunTask(ctx context.Context, wasmBinary []byte) ([]byte, error) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize")

	mod, err := r.InstantiateWithConfig(ctx, wasmBinary, config)
	if err != nil {
		log.Printf("error instantiating module: %v", err)
		return nil, err
	}

	res, err := mod.ExportedFunction("run").Call(ctx, 5, 8)
	if err != nil {
		log.Printf("error calling func %v", err)
		return nil, err
	}

	resultValue := int32(res[0])
	resultBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(resultBytes, uint32(resultValue))
	log.Printf("result of the operation: %v", resultValue)
	return resultBytes, nil
}

func (runtime WasmRuntime) EncodeTask(task Task) ([]byte, error) {
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

	inputFileLength := uint64(len(task.InputFile))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, inputFileLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding inputFileLength: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.InputFile)
	if err != nil {
		return nil, fmt.Errorf("failed encoding inputFile: %v", err)
	}

	return encodedMessage, nil
}

func (runtime WasmRuntime) DecodeTask(data []byte) Task {
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

	inputFileLength := binary.BigEndian.Uint64(data[:8])
	data = data[8:]

	inputFile := data[:inputFileLength]

	return Task{
		OpCode:    kademliadfs.OpCode(opCode),
		TaskID:    taskID,
		Binary:    wasmBinary,
		TTL:       ttl,
		Result:    result,
		InputFile: inputFile,
	}
}
