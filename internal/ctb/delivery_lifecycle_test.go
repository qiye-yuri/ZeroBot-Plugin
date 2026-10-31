package ctb

import (
	"bytes"
	"context"
	"errors"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func deliveryLifecycleFixture(t *testing.T) (*Service, Config, string) {
	t.Helper()
	s, c, task, _ := resendFixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c.Executable = filepath.Join(t.TempDir(), "fixture.exe")
	if err = os.WriteFile(c.Executable, []byte("configuration only"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = core.Save(s.Path, c); err != nil {
		t.Fatal(err)
	}
	old := commandContext
	t.Cleanup(func() { commandContext = old })
	commandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, exe, append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "CTB_TEST_HELPER=1", "CTB_CANCEL_MARKER=")
		return cmd
	}
	return s, c, task
}

func TestInitialDeliveryRetainsBusy(t *testing.T) {
	s, c, oldTask := deliveryLifecycleFixture(t)
	_, err := s.RunWithDelivery(context.Background(), Request{Action: "download", Pack: "135", Mode: "light"}, 101, func(result Result, mode string) error {
		if mode != c.GroupDelivery {
			t.Fatal("delivery policy changed", mode)
		}
		return Deliver(result.Files, 123, 101, mode, func(string, map[string]any) error {
			snapshot, err := s.Settings("", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !snapshot.Busy {
				t.Error("first QQ upload reports idle before its receipt")
			}
			quotaPath := filepath.Join(filepath.Dir(s.Path), "group-usage.json")
			before, err := os.ReadFile(quotaPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.RunFor(context.Background(), Request{Action: "download", Pack: "135", Mode: "light"}, 101); err == nil || !strings.Contains(err.Error(), "已有任务") {
				t.Fatal("overlapping download allowed", err)
			}
			if err = s.Resend(oldTask, 123, 101, func(string, map[string]any) error { t.Fatal("overlapping upload invoked"); return nil }); err == nil {
				t.Fatal("overlapping resend allowed")
			}
			next := c
			next.Threads++
			if _, err = s.Settings(snapshot.Revision, &next); err == nil {
				t.Fatal("settings changed during upload")
			}
			if ok, err := s.Cancel(0); ok || err == nil || !strings.Contains(err.Error(), "文件发送阶段") {
				t.Fatal("upload cancellation misreported", ok, err)
			}
			after, err := os.ReadFile(quotaPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected request changed quota", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Settings("", nil)
	if err != nil || snapshot.Busy {
		t.Fatal("upload did not release state", snapshot, err)
	}
}

func TestInitialDeliveryCompletionAndFailure(t *testing.T) {
	for _, mode := range []string{"success-after-download-context-cancel", "send-failure", "upload-disabled"} {
		t.Run(mode, func(t *testing.T) {
			s, c, _ := deliveryLifecycleFixture(t)
			if mode == "upload-disabled" {
				c.Upload = false
				if err := core.Save(s.Path, c); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := errors.New("synthetic upload failure")
			calls := 0
			result, err := s.RunWithDelivery(ctx, Request{Action: "download", Pack: "135", Mode: "light"}, 101, func(Result, string) error {
				calls++
				cancel()
				if mode == "send-failure" {
					return want
				}
				return nil
			})
			if mode == "send-failure" {
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			expected := 1
			if mode == "upload-disabled" {
				expected = 0
			}
			if calls != expected || len(result.Files) != 1 {
				t.Fatal(calls, result)
			}
			snapshot, e := s.Settings("", nil)
			if e != nil || snapshot.Busy {
				t.Fatal("busy not released", snapshot, e)
			}
			if _, e = s.Run(context.Background(), Request{Action: "inspect", Pack: "135"}); e != nil {
				t.Fatal("next task blocked", e)
			}
		})
	}
}
