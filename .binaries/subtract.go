package main

//go:wasmexport run
func run(x, y int32) int32 {
	return x - y
}

func main() {}
