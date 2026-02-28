package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmRuntime struct {
	cfg WasmConfig
}

type WasmConfig struct {
	MountDir string

	Tags               []string
	DeviceCapabilities []string
}

func NewWasmRuntime(cfg WasmConfig) *WasmRuntime {
	return &WasmRuntime{cfg: cfg}
}

func (rt *WasmRuntime) RunTask(ctx context.Context, wasmBinary []byte, stdin []byte, stdout io.Writer) ([]byte, error) {
	if len(wasmBinary) == 0 {
		return nil, fmt.Errorf("RunTask: wasmBinary is empty")
	}

	if rt.cfg.MountDir == "" {
		return nil, fmt.Errorf("RunTask: mountdir is not configured, use --mountdir=<path>")
	}

	info, err := os.Stat(rt.cfg.MountDir)
	if err != nil {
		return nil, fmt.Errorf("RunTask: mountdir invalid: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("RunTask: mountdir is not a directory")
	}

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, fmt.Errorf("RunTask: failed to instantiate WASI: %w", err)
	}

	var stderr bytes.Buffer
	var capturedStdout bytes.Buffer

	var moduleStdout io.Writer = &capturedStdout
	if stdout != nil {
		moduleStdout = io.MultiWriter(&capturedStdout, stdout)
	}

	fsConfig := wazero.NewFSConfig().
		WithDirMount(rt.cfg.MountDir, "/workspace")

	config := wazero.NewModuleConfig().
		WithStartFunctions("_initialize").
		WithStdin(bytes.NewReader(stdin)).
		WithStdout(moduleStdout).
		WithStderr(&stderr).
		WithFSConfig(fsConfig)

	mod, err := r.InstantiateWithConfig(ctx, wasmBinary, config)
	if err != nil {
		return nil, fmt.Errorf("RunTask: instantiate failed: %w", err)
	}
	defer mod.Close(ctx)

	if _, err := mod.ExportedFunction("run").Call(ctx); err != nil {
		return nil, fmt.Errorf("RunTask: run() failed: %w | stderr: %s", err, stderr.String())
	}

	return capturedStdout.Bytes(), nil
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

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed encoding jobID: %v", err)
	}

	taskTypeLength := uint64(len(task.Type))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, taskTypeLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding taskTypeLength: %v", err)
	}
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, []byte(string(task.Type)))
	if err != nil {
		return nil, fmt.Errorf("failed encoding taskType: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, task.RequestMode)
	if err != nil {
		return nil, fmt.Errorf("failed encoding requestMode: %v", err)
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

	capabilitiesLength := uint64(len(task.Capabilities))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, capabilitiesLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding capabilitiesLength: %v", err)
	}

	for _, capability := range task.Capabilities {
		capabilityLength := uint64(len(capability))
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, capabilityLength)
		if err != nil {
			return nil, fmt.Errorf("failed encoding capabilityLength: %v", err)
		}
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, []byte(capability))
		if err != nil {
			return nil, fmt.Errorf("failed encoding capability: %v", err)
		}
	}

	tagsLength := uint64(len(task.Tags))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, tagsLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding tagsLength: %v", err)
	}

	for _, tag := range task.Tags {
		tagLength := uint64(len(tag))
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, tagLength)
		if err != nil {
			return nil, fmt.Errorf("failed encoding tagLength: %v", err)
		}
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, []byte(tag))
		if err != nil {
			return nil, fmt.Errorf("failed encoding tag: %v", err)
		}
	}

	topicLength := uint64(len(task.Topic))
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, topicLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding topicLength: %v", err)
	}
	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, []byte(task.Topic))
	if err != nil {
		return nil, fmt.Errorf("failed encoding topic: %v", err)
	}

	return encodedMessage, nil
}

func DecodeTask(data []byte) (Task, error) {
	opCode := data[0]
	data = data[1:]

	taskID := kademliadfs.NodeId(data[:32])
	data = data[32:]

	jobID := kademliadfs.NodeId(data[:32])
	data = data[32:]

	taskTypeLength := binary.BigEndian.Uint64(data[:8])
	data = data[8:]
	taskType := string(data[:taskTypeLength])
	data = data[taskTypeLength:]

	requestMode := RequestMode(data[0])
	data = data[1:]

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
	data = data[stdinLength:]

	capabilities := make([]string, 0)
	tags := make([]string, 0)
	topic := ""

	// Backward compatibility: older payloads end after stdin.
	if len(data) > 0 {
		if len(data) < 8 {
			return Task{}, fmt.Errorf("invalid task payload: missing capabilities length")
		}
		capabilitiesCount := binary.BigEndian.Uint64(data[:8])
		data = data[8:]

		capabilities = make([]string, 0, capabilitiesCount)
		for range capabilitiesCount {
			if len(data) < 8 {
				return Task{}, fmt.Errorf("invalid task payload: missing capability length")
			}
			itemLength := binary.BigEndian.Uint64(data[:8])
			data = data[8:]
			if uint64(len(data)) < itemLength {
				return Task{}, fmt.Errorf("invalid task payload: malformed capability")
			}
			capabilities = append(capabilities, string(data[:itemLength]))
			data = data[itemLength:]
		}
	}

	if len(data) > 0 {
		if len(data) < 8 {
			return Task{}, fmt.Errorf("invalid task payload: missing tags length")
		}
		tagsCount := binary.BigEndian.Uint64(data[:8])
		data = data[8:]

		tags = make([]string, 0, tagsCount)
		for range tagsCount {
			if len(data) < 8 {
				return Task{}, fmt.Errorf("invalid task payload: missing tag length")
			}
			itemLength := binary.BigEndian.Uint64(data[:8])
			data = data[8:]
			if uint64(len(data)) < itemLength {
				return Task{}, fmt.Errorf("invalid task payload: malformed tag")
			}
			tags = append(tags, string(data[:itemLength]))
			data = data[itemLength:]
		}
	}

	if len(data) > 0 {
		if len(data) < 8 {
			return Task{}, fmt.Errorf("invalid task payload: missing topic length")
		}
		topicLength := binary.BigEndian.Uint64(data[:8])
		data = data[8:]
		if uint64(len(data)) < topicLength {
			return Task{}, fmt.Errorf("invalid task payload: malformed topic")
		}
		topic = string(data[:topicLength])
	}

	return Task{
		OpCode:       kademliadfs.OpCode(opCode),
		TaskID:       taskID,
		JobID:        jobID,
		Type:         TaskType(taskType),
		Topic:        topic,
		RequestMode:  requestMode,
		Binary:       wasmBinary,
		TTL:          ttl,
		Result:       result,
		Stdin:        stdin,
		Capabilities: capabilities,
		Tags:         tags,
	}, nil
}
