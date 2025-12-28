package main

//go:wasmexport run
func run(x, y uint32) uint32 {
	return x + y
}

func main() {}
