package main

import (
	"os"
)

func main() {
	if err := Execute(); err != nil {
		os.Stderr.WriteString("错误: " + err.Error() + "\n")
		os.Exit(1)
	}
}
