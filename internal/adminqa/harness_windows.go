// Package adminqa supplies isolated Windows processes for local admin acceptance checks.
package adminqa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/RomiChan/websocket"
	"golang.org/x/sys/windows"
)

const Owner = int64(12345678)
const Password = "Isolated-admin-acceptance-only"

type peer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}
type Harness struct {
	Root, Binary, ConfigPath, AdminDir, Address string
	server                                      *http.Server
	listener                                    net.Listener
	connections                                 chan *peer
	replies                                     chan string
	deletions                                   chan int64
	ImageReceived                               bool // read after synchronous Send returns, in isolated tests only
	sentMu                                      sync.Mutex
	lastSent                                    json.RawMessage
	apiActions                                  []string
	requestCalls                                []RequestCall
	failAction                                  string
	voiceModels                                 any
	current                                     *peer
	messageID                                   int
	mockQQ                                      int64
	groupHistory                                string
}

func WriteJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func FreeAddress() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func PrepareFixture(root, destination string) error {
	for _, relative := range []string{"data/Control/kanban.png", "data/Control/zbpuwu.png", "data/Font/GlowSansSC-Normal-ExtraBold.ttf", "data/Font/Impact.ttf", "data/Font/Torus-Regular.otf", "data/Font/FOT-Yuruka Std.ttf", "data/Chat/dict.txt", "data/McFish/articlesInfo.json"} {
		source, err := os.Open(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return err
		}
		path := filepath.Join(destination, filepath.FromSlash(relative))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			source.Close()
			return err
		}
		target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			source.Close()
			return err
		}
		_, err = io.Copy(target, source)
		closeErr := target.Close()
		source.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func New(root, binary, destination string) (*Harness, error) {
	return NewWithIdentity(root, binary, destination, 87654321)
}

// NewWithIdentity prepares a separate synthetic OneBot account for multi-connection checks.
func NewWithIdentity(root, binary, destination string, mockQQ int64) (*Harness, error) {
	if mockQQ <= 0 {
		return nil, fmt.Errorf("synthetic account must be positive")
	}
	if err := PrepareFixture(root, destination); err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	address, err := FreeAddress()
	if err != nil {
		l.Close()
		return nil, err
	}
	h := &Harness{Root: destination, Binary: binary, ConfigPath: filepath.Join(destination, "config.json"), AdminDir: filepath.Join(destination, "admin"), Address: address, listener: l, connections: make(chan *peer, 8), replies: make(chan string, 64), deletions: make(chan int64, 64), mockQQ: mockQQ}
	if err = os.MkdirAll(h.AdminDir, 0700); err != nil {
		l.Close()
		return nil, err
	}
	config := map[string]any{"zero": map[string]any{"nickname": []string{"本机测试"}, "command_prefix": "/", "super_users": []int64{Owner}, "ring_len": 4096, "latency": 233000000, "max_process_time": 240000000000, "mark_message": true}, "ws": []map[string]any{{"Url": "ws://" + l.Addr().String(), "AccessToken": "isolated-lifecycle-token"}}, "wss": nil, "force_base64_file": false}
	if err = WriteJSON(h.ConfigPath, config); err != nil {
		l.Close()
		return nil, err
	}
	if err = h.Options(true, address); err != nil {
		l.Close()
		return nil, err
	}
	// Only synthetic configuration tables are initialized. No live database is read.
	m := ctrl.NewManager[any](filepath.ToSlash(filepath.Join(destination, "data/control/plugins.db")))
	if err = m.Response(0); err == nil {
		for _, name := range []string{"aichat", "chat", "atri", "thesaurus"} {
			c := m.NewControl(name, &ctrl.Options[any]{})
			if err = c.Disable(0); err != nil {
				break
			}
		}
	}
	closeErr := m.D.Close()
	if err != nil {
		l.Close()
		return nil, err
	}
	if closeErr != nil {
		l.Close()
		return nil, closeErr
	}
	upgrader := websocket.Upgrader{}
	h.server = &http.Server{ReadHeaderTimeout: 5 * time.Second}
	h.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer isolated-lifecycle-token" {
			http.Error(w, "unauthorized", 401)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		p := &peer{conn: conn}
		p.mu.Lock()
		err = conn.WriteJSON(map[string]any{"self_id": h.mockQQ, "post_type": "meta_event", "meta_event_type": "lifecycle", "sub_type": "connect", "time": time.Now().Unix()})
		p.mu.Unlock()
		if err != nil {
			return
		}
		select {
		case h.connections <- p:
		default:
			return
		}
		for {
			var request struct {
				Action string          `json:"action"`
				Echo   json.RawMessage `json:"echo"`
				Params struct {
					Message   json.RawMessage `json:"message"`
					Messages  json.RawMessage `json:"messages"`
					GroupID   int64           `json:"group_id"`
					MessageID int64           `json:"message_id"`
					Flag      string          `json:"flag"`
					Approve   bool            `json:"approve"`
					SubType   string          `json:"sub_type"`
				} `json:"params"`
			}
			if conn.ReadJSON(&request) != nil {
				return
			}
			h.sentMu.Lock()
			h.apiActions = append(h.apiActions, request.Action)
			fail := h.failAction == request.Action
			voiceModels := h.voiceModels
			if request.Action == "set_friend_add_request" || request.Action == "set_group_add_request" {
				h.requestCalls = append(h.requestCalls, RequestCall{request.Action, request.Params.Flag, request.Params.SubType, request.Params.Approve})
			}
			h.sentMu.Unlock()
			p.mu.Lock()
			userID := Owner
			if request.Action == "get_login_info" {
				userID = h.mockQQ
			}
			var data any = map[string]any{"user_id": userID, "nickname": "Synthetic acceptance user"}
			if request.Action == "send_private_msg" || request.Action == "send_group_msg" || request.Action == "send_private_forward_msg" || request.Action == "send_group_forward_msg" {
				data = map[string]any{"message_id": 999}
			}
			if request.Action == "get_group_info" {
				data = map[string]any{"group_id": request.Params.GroupID, "group_name": "合成验收群", "member_count": 2}
			}
			if request.Action == "get_group_msg_history" {
				data = map[string]any{"messages": []any{map[string]any{"raw_message": h.groupHistory, "sender": map[string]any{"nickname": "合成发言者"}}}}
			}
			if request.Action == "get_ai_characters" {
				data = voiceModels
			}
			status, code := "ok", 0
			if fail {
				status, code = "failed", 1200
			}
			err = conn.WriteJSON(map[string]any{"status": status, "retcode": code, "data": data, "echo": request.Echo})
			p.mu.Unlock()
			if err != nil {
				return
			}
			if request.Action == "delete_msg" {
				select {
				case h.deletions <- request.Params.MessageID:
				default:
				}
			}
			if request.Action == "send_private_msg" || request.Action == "send_group_msg" || request.Action == "send_private_forward_msg" || request.Action == "send_group_forward_msg" {
				raw := request.Params.Message
				if len(request.Params.Messages) > 0 {
					raw = request.Params.Messages
				}
				text, parseErr := fixtureMessageText(raw)
				h.sentMu.Lock()
				h.lastSent = append(h.lastSent[:0], raw...)
				h.sentMu.Unlock()
				h.ImageReceived = bytes.Contains(raw, []byte(`"type":"image"`)) || bytes.Contains(raw, []byte(`"type": "image"`))
				if parseErr == nil {
					select {
					case h.replies <- text:
					default:
					}
				}
			}
		}
	})
	go h.server.Serve(l)
	return h, nil
}

// RequestCall records synthetic request decisions only.
type RequestCall struct {
	Action, Flag, SubType string
	Approve               bool
}

func (h *Harness) RequestCalls() []RequestCall {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	return append([]RequestCall(nil), h.requestCalls...)
}
func (h *Harness) FailAction(action string) {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	h.failAction = action
}
func (h *Harness) SendRequest(ctx context.Context, uid, gid int64, flag string) (string, error) {
	p := h.current
	p.mu.Lock()
	kind, sub := "friend", ""
	if gid != 0 {
		kind, sub = "group", "invite"
	}
	err := p.conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": h.mockQQ, "post_type": "request", "request_type": kind, "sub_type": sub, "user_id": uid, "group_id": gid, "flag": flag, "comment": "合成验收申请"})
	p.mu.Unlock()
	if err != nil {
		return "", err
	}
	return h.AwaitReply(ctx)
}

// SetVoiceModels configures only the synthetic OneBot voice catalogue.
func (h *Harness) SetVoiceModels(models any) {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	h.voiceModels = models
}

func (h *Harness) Close() {
	if h.current != nil {
		h.current.conn.Close()
	}
	h.server.Close()
}

// MockURL identifies this fixture's synthetic OneBot endpoint.
func (h *Harness) MockURL() string { return "ws://" + h.listener.Addr().String() }

// RecallLast emits a synthetic recall notice for the last incoming message and
// returns the bot reply ID deleted by the plugin, or the caller's timeout.
func (h *Harness) RecallLast(ctx context.Context, uid, gid int64) (int64, error) {
	for {
		select {
		case <-h.deletions:
			continue
		default:
		}
		break
	}
	kind := "friend_recall"
	if gid != 0 {
		kind = "group_recall"
	}
	p := h.current
	p.mu.Lock()
	err := p.conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": h.mockQQ, "post_type": "notice", "notice_type": kind, "group_id": gid, "user_id": uid, "operator_id": uid, "message_id": h.messageID})
	p.mu.Unlock()
	if err != nil {
		return 0, err
	}
	select {
	case id := <-h.deletions:
		return id, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// WaitConnection attaches an extra fixture endpoint to the already started test process.
func (h *Harness) WaitConnection(ctx context.Context) error {
	select {
	case h.current = <-h.connections:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *Harness) Options(enabled bool, address string) error {
	return WriteJSON(filepath.Join(h.AdminDir, "options.json"), map[string]any{"enabled": enabled, "address": address})
}

type Bot struct {
	cmd    *exec.Cmd
	done   chan error
	handle windows.Handle
	logs   []*os.File
	stop   sync.Once
}

// Running checks only this fixture's owned process handle.
func (b *Bot) Running() bool {
	if b == nil || b.handle == 0 {
		return false
	}
	var code uint32
	return windows.GetExitCodeProcess(b.handle, &code) == nil && code == 259
}

func (b *Bot) Stop() {
	if b == nil {
		return
	}
	b.stop.Do(func() {
		_ = b.cmd.Process.Kill()
		<-b.done
		windows.CloseHandle(b.handle)
		for _, f := range b.logs {
			f.Close()
		}
	})
}

func (h *Harness) Start(ctx context.Context, label string) (*Bot, error) {
	var options struct {
		Enabled bool   `json:"enabled"`
		Address string `json:"address"`
	}
	raw, err := os.ReadFile(filepath.Join(h.AdminDir, "options.json"))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	args := []string{"-c", h.ConfigPath}
	if options.Enabled {
		args = append(args, "-admin", "-admin-addr", options.Address, "-admin-data", h.AdminDir)
	}
	cmd := exec.CommandContext(ctx, h.Binary, args...)
	cmd.Dir = h.Root
	cmd.Env = append(os.Environ(), "DEBUG_MODE=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	b := &Bot{cmd: cmd, done: make(chan error, 1)}
	for _, stream := range []string{"stdout", "stderr"} {
		f, err := os.Create(filepath.Join(h.Root, label+"-"+stream+".log"))
		if err != nil {
			for _, old := range b.logs {
				old.Close()
			}
			return nil, err
		}
		b.logs = append(b.logs, f)
	}
	cmd.Stdout = b.logs[0]
	cmd.Stderr = b.logs[1]
	if err = cmd.Start(); err != nil {
		for _, f := range b.logs {
			f.Close()
		}
		return nil, err
	}
	go func() { b.done <- cmd.Wait() }()
	b.handle, err = windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		<-b.done
		for _, f := range b.logs {
			f.Close()
		}
		return nil, err
	}
	select {
	case h.current = <-h.connections:
	case <-ctx.Done():
		b.Stop()
		return nil, ctx.Err()
	case <-time.After(45 * time.Second):
		b.Stop()
		return nil, fmt.Errorf("isolated bot handshake timed out")
	}
	if options.Enabled {
		h.Address = options.Address
		client := &http.Client{Timeout: time.Second}
		ready := false
		for i := 0; i < 50; i++ {
			rsp, e := client.Get("http://" + h.Address + "/api/session")
			if e == nil {
				rsp.Body.Close()
				if rsp.StatusCode == 200 {
					ready = true
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			b.Stop()
			return nil, fmt.Errorf("isolated admin did not become ready")
		}
	}
	time.Sleep(300 * time.Millisecond)
	return b, nil
}

func (h *Harness) Send(uid int64, text string) (string, time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reply, elapsed, err := h.SendWithContext(ctx, uid, text)
	if err == context.DeadlineExceeded {
		err = nil // Existing smoke callers also send commands that have no reply.
	}
	return reply, elapsed, err
}

// SendPoke emits a synthetic notification to the isolated bot only.
func (h *Harness) SendPoke(ctx context.Context, uid, gid int64) (string, error) {
	return h.SendPokeTarget(ctx, uid, gid, h.mockQQ)
}

// SendPokeTarget permits target-routing checks on the synthetic connection.
func (h *Harness) SendPokeTarget(ctx context.Context, uid, gid, target int64) (string, error) {
	p := h.current
	p.mu.Lock()
	err := p.conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": h.mockQQ,
		"post_type": "notice", "notice_type": "notify", "sub_type": "poke",
		"user_id": uid, "group_id": gid, "target_id": target})
	p.mu.Unlock()
	if err != nil {
		return "", err
	}
	select {
	case reply := <-h.replies:
		return reply, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// SendWithContext sends only to this harness's synthetic OneBot connection.
// AwaitReply observes delayed synthetic sends without injecting another event.
func (h *Harness) AwaitReply(ctx context.Context) (string, error) {
	select {
	case reply := <-h.replies:
		return reply, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ActionCount inspects only API calls sent to this synthetic OneBot instance.
func (h *Harness) ActionCount(action string) int {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	n := 0
	for _, got := range h.apiActions {
		if got == action {
			n++
		}
	}
	return n
}

// LastSentMessage returns a copy of the last synthetic send payload for identity checks.
func (h *Harness) LastSentMessage() json.RawMessage {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	return append(json.RawMessage(nil), h.lastSent...)
}

// Real model checks can wait longer than the deterministic smoke timeout.
func (h *Harness) SendWithContext(ctx context.Context, uid int64, text string, segments ...map[string]any) (string, time.Duration, error) {
	h.messageID++
	start := time.Now()
	if len(segments) == 0 {
		segments = []map[string]any{{"type": "text", "data": map[string]string{"text": text}}}
	}
	p := h.current
	p.mu.Lock()
	err := p.conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": h.mockQQ, "post_type": "message", "message_type": "private", "sub_type": "friend", "message_id": h.messageID, "user_id": uid, "raw_message": text, "message": segments, "sender": map[string]any{"user_id": uid, "nickname": "Synthetic acceptance user", "role": "member"}})
	p.mu.Unlock()
	if err != nil {
		return "", time.Since(start), err
	}
	select {
	case reply := <-h.replies:
		return reply, time.Since(start), nil
	case <-ctx.Done():
		return "", time.Since(start), ctx.Err()
	}
}

// SendGroupWithContext exercises group summaries using only supplied synthetic
// history. Ignore the command's preliminary acknowledgement and wait for output.
func (h *Harness) SendGroupWithContext(ctx context.Context, uid, gid int64, text, history string) (string, error) {
	h.groupHistory = history
	return h.SendGroupAs(ctx, uid, gid, text, "owner")
}

// SendGroupAs permits explicit roles and message segments in isolated tests.
func (h *Harness) SendGroupAs(ctx context.Context, uid, gid int64, text, role string, segments ...map[string]any) (string, error) {
	h.messageID++
	p := h.current
	p.mu.Lock()
	if len(segments) == 0 {
		segments = []map[string]any{{"type": "text", "data": map[string]string{"text": text}}}
	}
	err := p.conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": h.mockQQ, "post_type": "message", "message_type": "group", "sub_type": "normal", "message_id": h.messageID, "user_id": uid, "group_id": gid, "raw_message": text, "message": segments, "sender": map[string]any{"user_id": uid, "nickname": "Synthetic acceptance user", "role": role}})
	p.mu.Unlock()
	if err != nil {
		return "", err
	}
	for {
		select {
		case reply := <-h.replies:
			if reply != "少女思考中..." {
				return reply, nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

type Client struct {
	Base string
	HTTP *http.Client
	CSRF string
}

func NewClient(address string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{Base: "http://" + address, HTTP: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}
func (c *Client) Request(method, path string, body any) (int, map[string]any, time.Duration, error) {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return 0, nil, 0, err
		}
	}
	req, err := http.NewRequest(method, c.Base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, 0, err
	}
	if method == "POST" {
		req.Header.Set("Origin", c.Base)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Local-Request", "1")
		req.Header.Set("X-CSRF-Token", c.CSRF)
	}
	start := time.Now()
	rsp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, time.Since(start), err
	}
	defer rsp.Body.Close()
	var value map[string]any
	err = json.NewDecoder(io.LimitReader(rsp.Body, 2<<20)).Decode(&value)
	return rsp.StatusCode, value, time.Since(start), err
}
func (c *Client) Auth(adminDir string) (time.Duration, error) {
	return c.AuthWithPassword(adminDir, Password)
}

// AuthWithPassword keeps interactive fixture passwords separate from test defaults.
func (c *Client) AuthWithPassword(adminDir, password string) (time.Duration, error) {
	code, err := os.ReadFile(filepath.Join(adminDir, "setup-code.txt"))
	path := "/api/login"
	body := map[string]string{"password": password}
	if err == nil {
		path = "/api/setup"
		body["setupCode"] = strings.TrimSpace(string(code))
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	status, value, duration, err := c.Request("POST", path, body)
	if err != nil || status != 200 {
		return duration, fmt.Errorf("isolated admin authentication failed: status %d", status)
	}
	c.CSRF, _ = value["csrfToken"].(string)
	if c.CSRF == "" {
		return duration, fmt.Errorf("missing CSRF")
	}
	return duration, nil
}
func (c *Client) Checked(method, path string, body any) (map[string]any, error) {
	status, value, _, err := c.Request(method, path, body)
	if err != nil || status != 200 {
		return nil, fmt.Errorf("isolated API %s failed: status %d", path, status)
	}
	return value, nil
}

type Memory struct {
	Size                                                                                                                                                                                 uint32
	PageFaultCount                                                                                                                                                                       uint32
	PeakWorkingSetSize, WorkingSetSize, QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage, QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage, PagefileUsage, PeakPagefileUsage, PrivateUsage uintptr
}

var memoryProc = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func (b *Bot) Metrics() (float64, uint64, uint64, error) {
	var create, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(b.handle, &create, &exit, &kernel, &user); err != nil {
		return 0, 0, 0, err
	}
	m := Memory{Size: uint32(unsafe.Sizeof(Memory{}))}
	ok, _, err := memoryProc.Call(uintptr(b.handle), uintptr(unsafe.Pointer(&m)), uintptr(m.Size))
	if ok == 0 {
		return 0, 0, 0, err
	}
	seconds := float64((uint64(kernel.HighDateTime)<<32|uint64(kernel.LowDateTime))+(uint64(user.HighDateTime)<<32|uint64(user.LowDateTime))) / 1e7
	return seconds, uint64(m.PrivateUsage), uint64(m.WorkingSetSize), nil
}

func fixtureMessageText(raw json.RawMessage) (string, error) {
	// OneBot accepts string/CQ and segment-array forms, including node content.
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) {
		var text string
		err := json.Unmarshal(raw, &text)
		return text, err
	}
	var segments []struct {
		Type string
		Data map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var text string
			if err := json.Unmarshal(segment.Data["text"], &text); err != nil {
				return "", err
			}
			out.WriteString(text)
		case "node":
			text, err := fixtureMessageText(segment.Data["content"])
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		}
	}
	return out.String(), nil
}
