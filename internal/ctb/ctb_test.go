package ctb

import (
	"archive/zip"
	"context"
	"encoding/json"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestReadableOutput(t *testing.T) {
	raw := "\x1b[32m（正在使用系统代理）\x1b[0m\r\n┌─────┬─────────────────┐\r\n│ Id  │ Name            │\r\n│ 135 │ FTB Evolution 2 │\r\n└─────┴─────────────────┘\r\n按任意键退出..."
	want := "（正在使用系统代理）\nId | Name\n135 | FTB Evolution 2"
	gb, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{[]byte(raw), gb} {
		got := readableOutput(input)
		if got != want || !utf8.ValidString(got) {
			t.Fatalf("decoded output: %q", got)
		}
	}
	if got := readableOutput([]byte("失败：无法找到整合包135")); got != "失败：无法找到整合包135" {
		t.Fatal(got)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("CTB_TEST_HELPER") != "1" {
		return
	}
	if strings.Contains(strings.Join(os.Args, " "), "999999") {
		os.Stderr.WriteString("unknown pack")
		os.Exit(2)
	}
	for i, a := range os.Args {
		if a == "--output" {
			f, e := os.Create(filepath.Join(os.Args[i+1], "pack.zip"))
			if e != nil {
				os.Exit(3)
			}
			z := zip.NewWriter(f)
			w, _ := z.Create("manifest.json")
			w.Write([]byte(`{"name":"fixture"}`))
			w, _ = z.Create("overrides//default-server.properties")
			w.Write([]byte("server-port=25565\n"))
			z.Close()
			f.Close()
		}
	}
	encoded, _ := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("查询成功：fixture result"))
	os.Stdout.Write(encoded)
	os.Exit(0)
}
func TestRunAndSettings(t *testing.T) {
	exe, _ := os.Executable()
	s := &Service{Path: filepath.Join(t.TempDir(), "config.json")}
	c := Defaults()
	// Config validation targets a Windows exe, while the injected runner below
	// executes this platform's test binary (which has no .exe suffix on Unix).
	c.Executable = filepath.Join(t.TempDir(), "fixture.exe")
	if e := os.WriteFile(c.Executable, []byte("configuration fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	c.OutputDir = filepath.Join(t.TempDir(), "downloads")
	if e := core.Save(s.Path, c); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Run(context.Background(), Request{Action: "inspect", Pack: "135"}); e == nil {
		t.Fatal("disabled allowed")
	}
	snap, _ := s.Settings("", nil)
	c.Enabled = true
	if _, e := s.Settings(snap.Revision, &c); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Settings(snap.Revision, &c); e == nil {
		t.Fatal("stale allowed")
	}
	old := commandContext
	defer func() { commandContext = old }()
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, exe, append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "CTB_TEST_HELPER=1")
		return cmd
	}
	for _, r := range []Request{{Action: "inspect", Pack: "135"}, {Action: "search", Pack: "test"}, {Action: "download", Pack: "135", Version: "100577", Mode: "light"}} {
		v, e := s.Run(context.Background(), r)
		if e != nil {
			t.Fatal(e)
		}
		if r.Action == "download" && len(v.Files) != 1 {
			t.Fatal("artifact missing")
		}
		if r.Action == "download" && !strings.HasSuffix(v.Files[0], "-windows.zip") {
			t.Fatal("download did not select the repaired artifact")
		}
		if r.Action == "download" {
			files, err := deliveryFiles(c.OutputDir, v.TaskID)
			if err != nil || len(files) != 1 || files[0] != v.Files[0] {
				t.Fatal("download receipt does not identify final artifact", files, err)
			}
		}
		if r.Action != "download" && !strings.Contains(v.Text, "查询成功") {
			t.Fatalf("process output encoding lost: %q", v.Text)
		}
	}
	if _, e := s.Run(context.Background(), Request{Action: "inspect", Pack: "999999"}); e == nil {
		t.Fatal("failure hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Run(ctx, Request{Action: "inspect", Pack: "135"}); e == nil {
		t.Fatal("cancel ignored")
	}
	s.busy = true
	if _, e := s.Run(context.Background(), Request{Action: "inspect", Pack: "135"}); e == nil {
		t.Fatal("busy ignored")
	}
	s.busy = false
	b, _ := json.Marshal(c)
	if strings.Contains(string(b), "token") {
		t.Fatal("unexpected credentials")
	}
}
func TestArgumentBoundaries(t *testing.T) {
	for _, r := range []Request{{Action: "inspect", Pack: "135;calc"}, {Action: "download", Pack: "135", Version: "--help", Mode: "light"}, {Action: "download", Pack: "135", Mode: "server"}, {Action: "search", Pack: "--help"}, {Action: "other", Pack: "135"}} {
		if _, e := Args(r); e == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	a, e := Args(Request{Action: "search", Pack: "hello world"})
	if e != nil || len(a) != 2 || a[1] != "hello world" {
		t.Fatal("keywords not one argument")
	}
}
