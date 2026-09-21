package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	fmt.Printf("Пользователь: %s\n", username)
	fmt.Printf("Аргументы CLI: %q\n", os.Args[1:])
	fmt.Printf("Версия Go: %s\n", runtime.Version())
}
