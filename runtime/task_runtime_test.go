package runtime

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed binaries/add.wasm
var wasmAdd []byte

func TestWasm(t *testing.T) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize")

	mod, _ := r.InstantiateWithConfig(ctx, wasmAdd, config)
	res, err := mod.ExportedFunction("add").Call(ctx, 1, 2)
	if err != nil {
		t.Fatalf("error calling func %v", err)
	}

	t.Log("result: ", res)
}
