// updatewatch-check verifies public source access without model calls or QQ messages.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	events, failures := updatewatch.NewFetcher().Collect(ctx, updatewatch.DefaultConfig())
	counts := map[string]int{}
	for k, v := range events {
		counts[k] = len(v)
	}
	b, _ := json.MarshalIndent(struct {
		Counts   map[string]int
		Failures map[string]string
	}{counts, failures}, "", "  ")
	fmt.Println(string(b))
	if len(os.Args) == 2 {
		if err := updatewatch.Save(os.Args[1], events); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if len(events) == 0 {
		os.Exit(1)
	}
}
