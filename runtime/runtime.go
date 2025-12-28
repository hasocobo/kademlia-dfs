package runtime

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmRuntime struct {
	network Network
}
type WasmTask struct {
	WasmBinaryLength uint64
	WasmBinary       []byte
	InputFile        []byte
}

func NewWasmRuntime(n Network) (*WasmRuntime, error) {
	return &WasmRuntime{network: n}, nil
}

func (wr *WasmRuntime) Serve(addr string) error {
	ln, err := wr.network.Listen(addr)
	if err != nil {
		return fmt.Errorf("error creating listener: %v", err)
	}
	log.Printf("listening tcp on %v", addr)
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("error accepting packet: %v", err)
			continue
		}

		go func(conn net.Conn) {
			packet, err := io.ReadAll(conn)
			if err != nil {
				log.Printf("error accepting packet: %v", err)
				return
			}

			data := wr.Decode(packet)

			if len(data.WasmBinary) == 0 {
				return
			}

			wr.Run(data.WasmBinary)
		}(conn)
	}
}

func (wr *WasmRuntime) SendTask(data []byte, addr string) error {
	conn, err := wr.network.Dial(addr)
	if err != nil {
		return fmt.Errorf("error dialing %v: %v", addr, err)
	}
	log.Printf("sending task to :%v", addr)
	defer conn.Close()

	_, writeErr := conn.Write(data)
	if writeErr != nil {
		return fmt.Errorf("error writing to %v: %v", addr, err)
	}

	return nil
}

func (wr *WasmRuntime) Encode(task WasmTask) ([]byte, error) {
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

func (wr *WasmRuntime) Decode(data []byte) WasmTask {
	length := binary.BigEndian.Uint64(data[:8])

	data = data[8:]
	wasmBinary := data[:length]
	return WasmTask{WasmBinary: wasmBinary, WasmBinaryLength: length}
}

func (wr *WasmRuntime) Run(wasmBinary []byte) {
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
