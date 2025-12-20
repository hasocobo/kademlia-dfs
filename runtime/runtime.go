package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
)

type WasmTask struct{}

func EncodeTask() {
}

func DecodeTask() {
}

func main() {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasmAdd, _ := os.ReadFile("main.wasm")
	mod, _ := r.Instantiate(ctx, wasmAdd)
	res, _ := mod.ExportedFunction("add").Call(ctx, 1, 2)

	fmt.Println(res)
}
