package ctb

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
)

func TestCancelRunningProcessAndNextTask(t *testing.T) {
	for _, action := range []string{"download", "inspect"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			s := &Service{Path: filepath.Join(root, "config.json")}
			cfg := Defaults()
			cfg.Enabled = true
			cfg.GroupAccess = "all"
			cfg.GroupDailyLimit = 1
			cfg.Executable = filepath.Join(root, "fixture.exe")
			cfg.OutputDir = filepath.Join(root, "downloads")
			if err := os.WriteFile(cfg.Executable, []byte("configuration only"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := core.Save(s.Path, cfg); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(root, "started")
			old := commandContext
			defer func() { commandContext = old }()
			commandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, exe, append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
				cmd.Env = append(os.Environ(), "CTB_TEST_HELPER=1", "CTB_CANCEL_MARKER="+marker)
				return cmd
			}
			ctx, stop := context.WithCancel(context.Background())
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				_, err := s.RunFor(ctx, Request{Action: action, Pack: "135", Mode: "light"}, 101)
				done <- err
			}()
			defer func() { stop(); <-finished }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(marker); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("helper did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			var quota []byte
			quotaPath := filepath.Join(root, "group-usage.json")
			if action == "download" {
				quota, err = os.ReadFile(quotaPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			if ok, err := s.Cancel(102); ok || err == nil {
				t.Fatal("cross-group cancellation accepted", ok, err)
			}
			snapshot, err := s.Settings("", nil)
			if err != nil || !snapshot.Busy {
				t.Fatal("denial released task", snapshot, err)
			}
			cancelFrom := int64(101)
			if action == "inspect" {
				cancelFrom = 0
			}
			if ok, err := s.Cancel(cancelFrom); !ok || err != nil {
				t.Fatal("cancel failed", ok, err)
			}
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "任务已取消") {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not stop child")
			}
			snapshot, err = s.Settings("", nil)
			if err != nil || snapshot.Busy {
				t.Fatal("busy not released", snapshot, err)
			}
			if action == "download" {
				after, err := os.ReadFile(quotaPath)
				if err != nil || !bytes.Equal(quota, after) {
					t.Fatal("cancel refunded quota", err)
				}
				parts, err := filepath.Glob(filepath.Join(cfg.OutputDir, "ctb-135-*", "partial.part"))
				if err != nil || len(parts) != 1 {
					t.Fatal("partial download lost", parts, err)
				}
			}
			if ok, err := s.Cancel(0); ok || err != nil {
				t.Fatal("finished task still cancellable", ok, err)
			}
			marker = ""
			if _, err = s.Run(context.Background(), Request{Action: "inspect", Pack: "135"}); err != nil {
				t.Fatal("next task inherited cancellation", err)
			}
		})
	}
}

func TestCancelKeepsBusyUntilTaskCompletes(t *testing.T) {
	s := &Service{}
	if ok, err := s.Cancel(0); ok || err != nil {
		t.Fatal(ok, err)
	}
	s.busy = true
	if ok, err := s.Cancel(0); ok || err == nil || !s.busy {
		t.Fatal("resend incorrectly cancelled", ok, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.cancel = cancel
	s.activeGroup = 101
	for i := 0; i < 2; i++ {
		if ok, err := s.Cancel(101); !ok || err != nil || !s.busy {
			t.Fatal("cancel prematurely released state", ok, err)
		}
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("context not cancelled")
	}
}
