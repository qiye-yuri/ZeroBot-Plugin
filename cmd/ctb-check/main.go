package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 3 {
		panic("ctb-check action pack [version] [mode]")
	}
	dir, e := os.MkdirTemp(".local", "ctb-check-")
	if e != nil {
		panic(e)
	}
	s := &ctb.Service{Path: filepath.Join(dir, "config.json")}
	saved, e := ctb.Default.Settings("", nil)
	if e != nil {
		panic(e)
	}
	c := saved.Config
	c.Enabled = true
	c.Upload = false
	c.OutputDir, _ = filepath.Abs(filepath.Join(dir, "downloads"))
	if e = core.Save(s.Path, c); e != nil {
		panic(e)
	}
	r := ctb.Request{Action: os.Args[1], Pack: os.Args[2], Mode: "light"}
	if len(os.Args) > 3 {
		r.Version = os.Args[3]
	}
	if len(os.Args) > 4 {
		r.Mode = os.Args[4]
	}
	v, e := s.Run(context.Background(), r)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
