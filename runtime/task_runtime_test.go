package runtime

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func TestWasm(t *testing.T) {
	ctx := context.Background()
	stdin, err := os.ReadFile("../.binaries/example.txt")
	if err != nil {
		t.Fatalf("failed reading test stdin: %v", err)
	}
	wasmBin, err := os.ReadFile("../.binaries/map.wasm")
	if err != nil {
		t.Fatalf("failed reading test wasm: %v", err)
	}

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
