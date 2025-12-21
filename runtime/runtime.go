package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"

	quic "github.com/quic-go/quic-go"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmRuntime struct {
	quicConn *net.UDPConn
}

func NewWasmRuntime(laddr *net.UDPAddr) (*WasmRuntime, error) {
	conn, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		return nil, fmt.Errorf("error creating new wasm runtime: %v", err)
	}
	return &WasmRuntime{quicConn: conn}, nil
}

type WasmTask struct {
	WasmBinaryLength uint64
	WasmBinary       []byte
	InputFile        os.File
}

func (wt *WasmRuntime) ListenQuic() error {
	ctx := context.Background()
	tr := quic.Transport{
		Conn: wt.quicConn,
	}

	ln, err := tr.Listen(&tls.Config{}, &quic.Config{})
	if err != nil {
		return fmt.Errorf("error listening quic: %v", err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			return err
		}
		go func() {
			packet, err := conn.AcceptStream(ctx)
			if err != nil {
				return
			}
			var buf []byte
			n, _ := packet.Read(buf)

			data := buf[:n]

			wasmTask := wt.Decode(data)

			if wasmTask.WasmBinaryLength != 0 {
				wt.Run(wasmTask.WasmBinary)
			}
		}()
	}
}

func (wt *WasmRuntime) Encode(task WasmTask) []byte {
	return nil
}

func (wt *WasmRuntime) Decode(b []byte) WasmTask {
	return WasmTask{}
}

func (wt *WasmRuntime) Run(wasmBinary []byte) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize")

	mod, _ := r.InstantiateWithConfig(ctx, wasmBinary, config)
	res, err := mod.ExportedFunction("add").Call(ctx, 1, 2)
	if err != nil {
		log.Printf("error calling func %v", err)
	}
	log.Println(res)
}
