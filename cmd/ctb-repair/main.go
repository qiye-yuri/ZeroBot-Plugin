package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "ctb-repair archive.zip (keeps original)")
		os.Exit(2)
	}
	output, changes, err := ctb.PrepareArchive(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"output": output, "normalizedEntries": changes, "originalRetained": true})
}
