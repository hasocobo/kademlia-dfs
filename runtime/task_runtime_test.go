package runtime

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed ../.binaries/example.txt
var stdin []byte

//go:embed ../.binaries/map.wasm
var wasmBin []byte

func TestWasm(t *testing.T) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var stdout, stderr bytes.Buffer

	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	config := wazero.NewModuleConfig().WithStartFunctions("_initialize").
		WithStdin(bytes.NewReader(stdin)).WithStdout(&stdout).WithStderr(&stderr)

	mod, instErr := r.InstantiateWithConfig(ctx, wasmBin, config)
	if instErr != nil {
		t.Fatalf("error in InstantiateWithConfig: %v", instErr)
	}
	defer mod.Close(ctx)

	res, err := mod.ExportedFunction("run").Call(ctx)
	if err != nil {
		t.Fatalf("error calling func %v", err)
	}

	t.Log("result: ", res)
	t.Log("stdout: ", stdout.String())
}
