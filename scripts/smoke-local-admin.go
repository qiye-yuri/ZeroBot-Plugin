//go:build ignore

// Run from the repository root: go run scripts/smoke-local-admin.go
// Uses synthetic accounts and a fresh fixture directory, never the live config.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/RomiChan/websocket"
)

type apiRequest struct {
	Action string `json:"action"`
	Params struct {
		UserID    int64           `json:"user_id"`
		GroupID   int64           `json:"group_id"`
		Message   json.RawMessage `json:"message"`
		Messages  json.RawMessage `json:"messages"`
		Character string          `json:"character"`
		Text      string          `json:"text"`
	} `json:"params"`
	Echo json.RawMessage `json:"echo"`
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var uiWait = flag.Bool("ui-wait", false, "Wait for local browser QA after integration checks.")
var binaryName = flag.String("binary-name", "zerobot.exe", "Windows binary filename under .local/bin.")

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if filepath.Base(*binaryName) != *binaryName || filepath.Ext(*binaryName) != ".exe" {
		return fmt.Errorf("binary-name must be an exe filename")
	}
	artifactRoot := filepath.Join(root, ".local", "smoke-admin")
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		return err
	}
	runtimeRoot, err := os.MkdirTemp(artifactRoot, "run-")
	if err != nil {
		return err
	}
	if err := prepareFixture(root, runtimeRoot); err != nil {
		return err
	}
	var modelMu sync.Mutex
	modelRequests := []map[string]any{}
	modelRounds := map[string]int{}
	var knowledgeFailures []string
	var visionImageGets, visionLargeGets atomic.Int32
	var descriptionImageGets atomic.Int32
	var preparationImageWait, preparationVisionWait atomic.Bool
	var preparationImageGets, preparationNextImageGets atomic.Int32
	var visionFixture bytes.Buffer
	visionImage := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			visionImage.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 120, A: 255})
		}
	}
	if err := png.Encode(&visionFixture, visionImage); err != nil {
		return err
	}
	var taskModelInFlight, taskActionInFlight, taskFallbackInFlight atomic.Bool
	shortModelReply := "本地模型模拟回复🙂" + strings.Repeat("中", 200-utf8.RuneCountInString("本地模型模拟回复🙂"))
	longModelReply := shortModelReply + "😀"
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/description-image-") {
			descriptionImageGets.Add(1)
			w.Header().Set("Content-Type", "image/png")
			w.Write(visionFixture.Bytes())
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/prepare-image-") {
			if r.URL.Path == "/prepare-image-next" {
				preparationNextImageGets.Add(1)
			} else {
				preparationImageGets.Add(1)
			}
			if r.URL.Path == "/prepare-image-download" {
				preparationImageWait.Store(true)
				defer preparationImageWait.Store(false)
				time.Sleep(1500 * time.Millisecond)
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(visionFixture.Bytes())
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/agent-image-ok" {
			visionImageGets.Add(1)
			w.Header().Set("Content-Type", "image/png")
			w.Write(visionFixture.Bytes())
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/agent-image-large" {
			visionLargeGets.Add(1)
			w.Header().Set("Content-Length", strconv.Itoa((8<<20)+1))
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-model-key" {
			http.Error(w, "fake key mismatch", 401)
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad model request", 400)
			return
		}
		modelMu.Lock()
		modelRequests = append(modelRequests, body)
		modelRounds[fmt.Sprint(body["model"])]++
		round := modelRounds[fmt.Sprint(body["model"])]
		modelMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		bodyJSON, _ := json.Marshal(body)
		reply := shortModelReply
		if body["model"] == "smoke-voice-chat" {
			reply = "VOICE_TEXT_OK"
		}
		if body["model"] == "smoke-task-model-wait" {
			taskModelInFlight.Store(true)
			defer taskModelInFlight.Store(false)
			time.Sleep(1500 * time.Millisecond)
			reply = `{"action":"send_group_msg","params":{"group_id":303,"message":"TASK_MUST_NOT_SEND"}}` + "\n" + `{"action":"end_action","params":{}}`
		}
		if body["model"] == "smoke-task-action-wait" {
			reply = `{"action":"get_group_member_info","params":{"group_id":303,"user_id":987654321}}` + "\n" + `{"action":"send_group_msg","params":{"group_id":303,"message":"TASK_MUST_NOT_SEND"}}` + "\n" + `{"action":"end_action","params":{}}`
		}
		if body["model"] == "smoke-task-fallback-wait" {
			reply = `{"action":"end_action","params":{}}`
		}
		if body["model"] == "smoke-task-chat-wait" {
			taskFallbackInFlight.Store(true)
			defer taskFallbackInFlight.Store(false)
			time.Sleep(1500 * time.Millisecond)
			reply = "TASK_MUST_NOT_SEND"
		}
		if bytes.Contains(bodyJSON, []byte("quoted smoke")) {
			reply = longModelReply
		}
		if body["model"] == "smoke-role-agent" {
			kind := "user"
			messages, _ := body["messages"].([]any)
			if len(messages) > 0 {
				system, _ := messages[0].(map[string]any)
				prompt, _ := system["content"].(string)
				if !strings.Contains(prompt, "SMOKE_CONVERSATION_PERSONA") {
					http.Error(w, "conversation persona missing from model prompt", 500)
					return
				}
				if strings.Contains(prompt, "|群组踢人|set_group_kick|") {
					kind = "admin"
				}
				if strings.Contains(prompt, "|群组设置管理员|set_group_admin|") {
					kind = "owner"
				}
			}
			text := "ROLE_AGENT_" + kind
			pre := `{"action":"search_history","params":{"from":"2000-01-01T00:00:00Z","limit":1}}` + "\n" + `{"action":"save_memory","params":{"text":"synthetic managed memory"}}` + "\n"
			if strings.LastIndex(string(bodyJSON), "ROLE_AGENT_CROSS_INFO") > strings.LastIndex(string(bodyJSON), "ROLE_AGENT_CHECK") {
				pre = `{"action":"get_group_info","params":{"group_id":202}}` + "\n"
				text = "ROLE_BLOCKED_OK"
			}
			if strings.LastIndex(string(bodyJSON), "ROLE_AGENT_CQ_ALL") > max(strings.LastIndex(string(bodyJSON), "ROLE_AGENT_CHECK"), strings.LastIndex(string(bodyJSON), "ROLE_AGENT_CROSS_INFO")) {
				pre = `{"action":"send_group_msg","params":{"group_id":303,"message":"[CQ:at,qq=all]UNAUTHORIZED"}}` + "\n"
				text = "ROLE_BLOCKED_OK"
			}
			reply = pre + `{"action":"send_group_msg","params":{"group_id":303,"message":"` + text + `"}}` + "\n" + `{"action":"end_action","params":{}}`
		}
		if strings.HasPrefix(fmt.Sprint(body["model"]), "smoke-knowledge-") {
			var err error
			reply, err = smokeKnowledgeReply(body, round)
			if err != nil {
				modelMu.Lock()
				knowledgeFailures = append(knowledgeFailures, err.Error())
				modelMu.Unlock()
				http.Error(w, "synthetic knowledge assertion failed", 500)
				return
			}
		}
		if strings.HasPrefix(fmt.Sprint(body["model"]), "smoke-vision-") || body["model"] == "smoke-prepare-vision" {
			if body["model"] == "smoke-prepare-vision" {
				preparationVisionWait.Store(true)
				defer preparationVisionWait.Store(false)
				time.Sleep(1500 * time.Millisecond)
			}
			var err error
			reply, err = smokeVisionReply(body)
			if err != nil {
				modelMu.Lock()
				knowledgeFailures = append(knowledgeFailures, err.Error())
				modelMu.Unlock()
				http.Error(w, "synthetic vision assertion failed", 500)
				return
			}
		}
		if strings.HasPrefix(fmt.Sprint(body["model"]), "smoke-description-") {
			var err error
			reply, err = smokeDescriptionReply(body, round)
			if err != nil {
				modelMu.Lock()
				knowledgeFailures = append(knowledgeFailures, err.Error())
				modelMu.Unlock()
				http.Error(w, "synthetic description assertion failed", 500)
				return
			}
		}
		if strings.HasPrefix(fmt.Sprint(body["model"]), "smoke-output-limit-") {
			kind := strings.TrimPrefix(fmt.Sprint(body["model"]), "smoke-output-limit-")
			if status, text, ok := smokeModelRequestFailure(kind); ok {
				w.Header().Set("Content-Length", strconv.Itoa(len(text)))
				w.WriteHeader(status)
				io.WriteString(w, text)
				return
			}
			var err error
			reply, err = smokeActionOutputReply(kind)
			if err != nil {
				http.Error(w, "invalid output fixture", 500)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(smokeModelReplyEnvelope(fmt.Sprint(body["model"]), reply))
	}))
	defer modelServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	connected := make(chan *websocket.Conn, 1)
	sent := make(chan apiRequest, 8)
	var failServiceForward atomic.Bool
	var roleCrossGroupQuery atomic.Bool
	var localActionForwarded atomic.Bool
	var voiceGets, voiceDirects, voiceRecordAttempts atomic.Int32
	var voiceEmptyRecord, voiceInvalidTarget atomic.Bool
	var writeMu sync.Mutex
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	upgrader := websocket.Upgrader{}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-smoke-only" {
			http.Error(w, "unauthorized", 401)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		writeMu.Lock()
		err = conn.WriteJSON(map[string]any{"self_id": 87654321, "post_type": "meta_event", "meta_event_type": "lifecycle", "sub_type": "connect", "time": time.Now().Unix()})
		writeMu.Unlock()
		if err != nil {
			return
		}
		connected <- conn
		for {
			var request apiRequest
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			if request.Action == "get_group_member_info" && request.Params.UserID == 987654321 {
				taskActionInFlight.Store(true)
				time.Sleep(1500 * time.Millisecond)
			}
			data := map[string]any{"message_id": 999, "user_id": 12345678, "nickname": "Local smoke user"}
			var apiData any = data
			if request.Action == "get_ai_characters" {
				apiData = []any{map[string]any{"characters": []any{map[string]string{"character_name": "Synthetic voice", "character_id": "synthetic-voice-1"}, map[string]string{"character_name": "Synthetic voice", "character_id": "synthetic-voice-2"}}}, map[string]any{"characters": []any{map[string]string{"character_name": "Repeated voice", "character_id": "synthetic-voice-1"}}}}
			}
			voiceAccepted := false
			apiStatus, apiRetcode := "ok", 0
			if request.Action == "get_ai_record" {
				// Installed NapCat posts to the group before returning the URL.
				voiceGets.Add(1)
				apiData = "https://synthetic.voice/record"
				voiceAccepted = true
			}
			if request.Action == "send_group_ai_record" {
				voiceDirects.Add(1)
				if (request.Params.GroupID != 303 && request.Params.GroupID != 404) || request.Params.Character != "synthetic-voice-1" {
					voiceInvalidTarget.Store(true)
				}
				// Native successful delivery has no message ID.
				data["message_id"] = 0
				voiceAccepted = true
				if request.Params.Text == "group-failure" {
					apiStatus, apiRetcode, voiceAccepted = "failed", 1400, false
				}
			}
			failedVoiceSend := false
			if request.Action == "send_private_msg" || request.Action == "send_group_msg" {
				var segments []struct {
					Type string            `json:"type"`
					Data map[string]string `json:"data"`
				}
				_ = json.Unmarshal(request.Params.Message, &segments)
				for _, segment := range segments {
					if segment.Type == "record" {
						voiceRecordAttempts.Add(1)
						if segment.Data["file"] == "" {
							voiceEmptyRecord.Store(true)
						}
						if segment.Data["file"] == "https://synthetic.voice/fail" {
							failedVoiceSend = true
							data["message_id"] = 0
						}
					}
				}
			}
			if request.Action == "get_group_info" && request.Params.GroupID == 202 {
				roleCrossGroupQuery.Store(true)
			}
			if request.Action == "search_history" || request.Action == "read_history" || request.Action == "search_memory" || request.Action == "read_memory" || request.Action == "save_memory" {
				localActionForwarded.Store(true)
			}
			failedForward := strings.HasSuffix(request.Action, "forward_msg") && failServiceForward.CompareAndSwap(true, false)
			if failedForward {
				data["message_id"] = 0
			}
			if request.Action == "get_msg" {
				data["message"] = []map[string]any{{"type": "text", "data": map[string]string{"text": "quoted context"}}}
				data["message_type"] = "private"
				data["sender"] = map[string]any{"user_id": 12345678, "nickname": "Quoted smoke user"}
			}
			writeMu.Lock()
			err := conn.WriteJSON(map[string]any{"status": apiStatus, "retcode": apiRetcode, "data": apiData, "echo": request.Echo})
			writeMu.Unlock()
			if request.Action == "get_group_member_info" && request.Params.UserID == 987654321 {
				taskActionInFlight.Store(false)
			}
			if err != nil {
				return
			}
			if voiceAccepted {
				native := request
				native.Action = "send_group_ai_record"
				native.Params.Message = json.RawMessage(`[{"type":"record","data":{"file":"synthetic-native-record"}}]`)
				sent <- native
			}
			if !failedForward && !failedVoiceSend && (request.Action == "send_private_msg" || request.Action == "send_group_msg" || request.Action == "send_private_forward_msg" || request.Action == "send_group_forward_msg") {
				sent <- request
			}
		}
	})
	go server.Serve(listener)
	defer server.Close()
	config := map[string]any{
		"zero": map[string]any{"nickname": []string{"七叶叶"}, "command_prefix": "/", "super_users": []int64{12345678}, "ring_len": 4096, "latency": 233000000, "max_process_time": 240000000000, "mark_message": true},
		"ws":   []map[string]any{{"Url": "ws://" + listener.Addr().String(), "AccessToken": "local-smoke-only"}}, "wss": nil, "force_base64_file": false,
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	configPath := filepath.Join(runtimeRoot, "config.json")
	if err := os.WriteFile(configPath, encoded, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	adminListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	adminAddress := adminListener.Addr().String()
	adminListener.Close()
	adminData := filepath.Join(runtimeRoot, "admin-"+time.Now().Format("20060102-150405"))
	legacyDir := filepath.Join(runtimeRoot, "data", "aichat", "agent")
	if err = os.MkdirAll(legacyDir, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(legacyDir, "404.txt"), []byte("synthetic legacy fact\n"), 0600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, ".local", "bin", *binaryName), "-c", configPath, "-admin", "-admin-addr", adminAddress, "-admin-data", adminData)
	cmd.Dir = runtimeRoot
	cmd.Env = append(os.Environ(), "DEBUG_MODE=1")
	stdout, err := os.Create(filepath.Join(runtimeRoot, "stdout.log"))
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(runtimeRoot, "stderr.log"))
	if err != nil {
		return err
	}
	defer stderr.Close()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
		}
	}()
	var conn *websocket.Conn
	select {
	case conn = <-connected:
	case err := <-exited:
		return fmt.Errorf("isolated bot exited before connecting: %v", err)
	case <-ctx.Done():
		return fmt.Errorf("isolated bot startup timed out")
	}
	defer conn.Close()
	// Let RunAndBlock finish installing the event listener after the handshake.
	time.Sleep(500 * time.Millisecond)
	controlMessageID := 100
	sendMessage := func(uid, gid int64, role, command string, atBot bool) (string, error) {
		controlMessageID++
		segments := []map[string]any{{"type": "text", "data": map[string]string{"text": command}}}
		if command == "VISION_IMAGE_CHECK" {
			for _, source := range []string{modelServer.URL + "/agent-image-ok", modelServer.URL + "/agent-image-large", "base64://!", "file:///synthetic-not-read.png"} {
				segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": source}})
			}
		}
		if command == "VISION_DESCRIPTION_CHECK" {
			for _, name := range []string{"a", "b", "c"} {
				segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": modelServer.URL + "/description-image-" + name}})
			}
		}
		if strings.HasPrefix(command, "PREPARE_CANCEL_") {
			for _, source := range []string{modelServer.URL + "/prepare-image-" + strings.TrimPrefix(command, "PREPARE_CANCEL_"), modelServer.URL + "/prepare-image-next"} {
				segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"file": source}})
			}
		}
		// This one fixture command exercises QQ reply-element preprocessing.
		if command == "/gpt quoted smoke" {
			segments = append([]map[string]any{{"type": "reply", "data": map[string]string{"id": "42"}}}, segments...)
		}
		if atBot {
			segments = append([]map[string]any{{"type": "at", "data": map[string]string{"qq": "87654321"}}}, segments...)
		}
		event := map[string]any{"time": time.Now().Unix(), "self_id": 87654321, "post_type": "message", "message_type": "private", "sub_type": "friend", "message_id": controlMessageID, "user_id": uid, "raw_message": command, "message": segments, "sender": map[string]any{"user_id": uid, "nickname": "Synthetic permission user", "sex": "unknown", "age": 0, "role": role}}
		// Repeated provider IDs must not change task or memory provenance.
		if strings.HasPrefix(command, "ROLE_AGENT_") {
			event["message_id"] = 777
		}
		if gid != 0 {
			event["message_type"], event["sub_type"], event["group_id"] = "group", "normal", gid
		}
		writeMu.Lock()
		err := conn.WriteJSON(event)
		writeMu.Unlock()
		if err != nil {
			return "", err
		}
		select {
		case reply := <-sent:
			if (gid == 0 && (reply.Params.UserID != uid || !strings.HasPrefix(reply.Action, "send_private_"))) || (gid != 0 && (reply.Params.GroupID != gid || !strings.HasPrefix(reply.Action, "send_group_"))) {
				return "", fmt.Errorf("unexpected command recipient")
			}
			raw := reply.Params.Message
			if (gid > 0 && strings.HasPrefix(command, "发送AI语音") && command != "发送AI语音group-failure") || command == "VOICE_CHAT_GROUP" {
				var segments []struct {
					Type string            `json:"type"`
					Data map[string]string `json:"data"`
				}
				if json.Unmarshal(raw, &segments) != nil || len(segments) != 1 || segments[0].Type != "record" || segments[0].Data["file"] == "" {
					return "", fmt.Errorf("voice command did not send one nonempty record")
				}
				select {
				case <-sent:
					return "", fmt.Errorf("native voice command produced an extra reply")
				case <-time.After(200 * time.Millisecond):
				}
				return "VOICE_OK", nil
			}
			if strings.HasSuffix(reply.Action, "forward_msg") {
				raw = reply.Params.Messages
			}
			if command == "/gpt group smoke" || command == "/gpt quoted smoke" {
				expectedAction, expectedText := "send_group_msg", shortModelReply
				if command == "/gpt quoted smoke" {
					expectedAction, expectedText = "send_private_forward_msg", longModelReply
				}
				text, err := smokeMessageText(raw)
				if err != nil || reply.Action != expectedAction || text != expectedText {
					return "", fmt.Errorf("GPT Unicode length boundary changed delivery or lost content: %s", reply.Action)
				}
			}
			if (strings.HasPrefix(command, "/服务列表") || strings.HasPrefix(command, "/service_list")) && strings.HasSuffix(reply.Action, "forward_msg") {
				if err := smokeServiceListNodes(raw); err != nil {
					return "", err
				}
			}
			return smokeMessageText(raw)
		case err := <-exited:
			return "", fmt.Errorf("isolated command process exited: %v", err)
		case <-time.After(3 * time.Second):
			return "", nil
		}
	}
	sendControl := func(uid int64, command string) (string, error) {
		return sendMessage(uid, 0, "member", command, false)
	}
	inFlight := func(phase string) bool {
		switch phase {
		case "model":
			return taskModelInFlight.Load()
		case "action":
			return taskActionInFlight.Load()
		case "fallback":
			return taskFallbackInFlight.Load()
		case "prepare-download":
			return preparationImageWait.Load()
		case "prepare-vision":
			return preparationVisionWait.Load()
		default:
			return false
		}
	}
	if err := verifyAdmin("http://"+adminAddress, adminData, modelServer.URL, sendControl, sendMessage, inFlight); err != nil {
		return err
	}
	if err := verifyGroupResponses(sendMessage); err != nil {
		return err
	}
	if voiceGets.Load() != 0 || voiceDirects.Load() != 5 || voiceRecordAttempts.Load() != 0 || voiceEmptyRecord.Load() || voiceInvalidTarget.Load() {
		return fmt.Errorf("voice side-effect regression: get=%d direct=%d record=%d empty=%t invalid=%t", voiceGets.Load(), voiceDirects.Load(), voiceRecordAttempts.Load(), voiceEmptyRecord.Load(), voiceInvalidTarget.Load())
	}
	modelMu.Lock()
	chatCalls, agentCalls, taskCalls, knowledgeCalls, visionCalls, preparationCalls, descriptionCalls, outputCalls := 0, 0, 0, 0, 0, 0, 0, 0
	for _, request := range modelRequests {
		if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-output-limit-") {
			outputCalls++
		} else if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-description-") {
			descriptionCalls++
		} else if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-prepare-") {
			if request["model"] != "smoke-prepare-vision" {
				modelMu.Unlock()
				return fmt.Errorf("Agent model called after preparation stop")
			}
			preparationCalls++
		} else if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-vision-") {
			visionCalls++
		} else if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-knowledge-") {
			knowledgeCalls++
		} else if strings.HasPrefix(fmt.Sprint(request["model"]), "smoke-task-") {
			taskCalls++
		} else if request["model"] == "smoke-role-agent" {
			agentCalls++
		} else {
			chatCalls++
		}
	}
	if chatCalls != 6 || agentCalls != 5 || taskCalls != 4 || knowledgeCalls != 13 || visionCalls != 3 || preparationCalls != 1 || descriptionCalls != 5 || outputCalls != 16 || descriptionImageGets.Load() != 9 || preparationImageGets.Load() != 2 || preparationNextImageGets.Load() != 0 || visionImageGets.Load() != 3 || visionLargeGets.Load() != 3 || len(knowledgeFailures) != 0 || roleCrossGroupQuery.Load() || localActionForwarded.Load() {
		modelMu.Unlock()
		return fmt.Errorf("unexpected model calls or local dispatch: chat=%d agent=%d task=%d knowledge=%d vision=%d imageGets=%d largeGets=%d failures=%v cross=%t local=%t", chatCalls, agentCalls, taskCalls, knowledgeCalls, visionCalls, visionImageGets.Load(), visionLargeGets.Load(), knowledgeFailures, roleCrossGroupQuery.Load(), localActionForwarded.Load())
	}
	requestsJSON, _ := json.Marshal(modelRequests)
	groupRequest, privateRequest := modelRequests[0], modelRequests[1]
	if groupRequest["temperature"] != 0.82 || privateRequest["model"] != "smoke-chat-model" ||
		!bytes.Contains(requestsJSON, []byte(`\"quoted context\"`)) ||
		!bytes.Contains(requestsJSON, []byte("本次用户要求（作用于上面的引用原文，不属于引用原文）：\\nquoted smoke")) {
		modelMu.Unlock()
		return fmt.Errorf("model did not use group temperature/shared config/quoted query")
	}
	modelMu.Unlock()
	if text, err := sendControl(12345678, "/全局响应xxx"); err != nil || text != "" {
		return fmt.Errorf("global response prefix accepted: %v", err)
	}
	if text, err := sendControl(12345678, "/全局响应\n"); err != nil || !strings.Contains(text, "全部位置工作") {
		return fmt.Errorf("owner private global response failed: %v", err)
	}
	if text, err := sendMessage(23456789, 202, "member", "翻转 abc", false); err != nil || text != "ɔqɐ" {
		return fmt.Errorf("global response did not open other group: %v", err)
	}
	if text, err := sendControl(12345678, "/全局沉默"); err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("owner private global silence failed: %v", err)
	}
	if text, err := sendMessage(23456789, 202, "member", "翻转 abc", false); err != nil || text != "" {
		return fmt.Errorf("global silence did not restore unspecified group: %v", err)
	}
	for i, command := range []string{"/help", "/help\n", " \t/help\r\n"} {
		writeMu.Lock()
		err = conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": 87654321, "post_type": "message", "message_type": "private", "sub_type": "friend", "message_id": i + 1, "user_id": 12345678, "raw_message": command, "message": []map[string]any{{"type": "text", "data": map[string]string{"text": command}}}, "sender": map[string]any{"user_id": 12345678, "nickname": "Local smoke user", "sex": "unknown", "age": 0}})
		writeMu.Unlock()
		if err != nil {
			return err
		}
		var request apiRequest
		select {
		case request = <-sent:
		case err := <-exited:
			return fmt.Errorf("isolated bot exited during %s: %v", command, err)
		case <-time.After(15 * time.Second):
			return fmt.Errorf("no private response to %s", command)
		}
		if request.Params.UserID != 12345678 {
			return fmt.Errorf("wrong response recipient")
		}
		var segments []struct {
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal(request.Params.Message, &segments); err != nil {
			return err
		}
		valid := false
		for _, segment := range segments {
			if strings.TrimSpace(command) == "/help" && segment.Type == "text" && strings.Contains(segment.Data["text"], "OneBot + ZeroBot + Golang") {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("unexpected response payload to %s", command)
		}
	}
	failServiceForward.Store(true)
	if text, err := sendControl(12345678, "/服务列表"); err != nil || !strings.Contains(text, "服务列表聊天记录发送失败") || failServiceForward.Load() {
		return fmt.Errorf("service list failed forward did not give truthful feedback: %v", err)
	}
	if err = verifyReverseDriver(root, runtimeRoot, *binaryName); err != nil {
		return err
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "helpTextResponse": true, "helpWhitespaceResponse": true, "serviceListForwardUsagePassed": true, "adminAuthPassed": true, "adminActualRegistryPassed": true, "adminEnabledCommandRepliesPassed": true, "adminConnectionsSaveBackupPassed": true, "adminPluginSwitchPassed": true, "chatWebSharedStatePassed": true, "pluginMessageGatePassed": true, "nonOwnerGlobalCommandRejected": true, "completedAt": time.Now().Format(time.RFC3339)}
	result["groupResponseMentionRequiredPassed"] = true
	result["groupSilenceMentionRequiredPassed"] = true
	result["groupResponsePermissionsPassed"] = true
	result["responseCommandBoundaryPassed"] = true
	result["responseLocationIsolationPassed"] = true
	result["ownerPrivateGlobalResponsePassed"] = true
	result["aiSharedConfigPassed"] = true
	result["aiConfigChecksPassed"] = true
	result["aiSecretRedactionPassed"] = true
	result["aiGroupParametersPassed"] = true
	result["gptMockReplyAndQuotePassed"] = true
	result["gptQuoteBoundaryPassed"] = true
	result["aiRoleCommandsPassed"] = true
	result["aiAgentRolePromptsPassed"] = true
	result["aiConversationPersonaModelAndHTTPPassed"] = true
	result["agentArchiveReadSearchIsolationPassed"] = true
	result["agentArchiveTimeRangeSearchPassed"] = true
	result["agentArchiveSourceContextHTTPPassed"] = true
	result["agentArchiveContextRadiusHTTPPassed"] = true
	result["agentArchiveExactLookupHTTPPassed"] = true
	result["agentTaskReasonFilterHTTPPassed"] = true
	result["agentTaskPhaseFilterHTTPPassed"] = true
	result["agentTaskOutcomeFilterHTTPPassed"] = true
	result["agentTaskStartRangeHTTPPassed"] = true
	result["agentHistorySenderModelRoundTripPassed"] = true
	result["agentHistorySenderOnlyModelRoundTripPassed"] = true
	result["agentHistoryEventTypeModelRoundTripPassed"] = true
	result["agentArchiveEventTypeSearchHTTPPassed"] = true
	result["agentArchiveSenderSearchHTTPPassed"] = true
	result["agentArchiveSenderOnlySearchHTTPPassed"] = true
	result["agentHistoryTimeRangeModelRoundTripPassed"] = true
	result["agentHistoryTimeOnlyQueryPassed"] = true
	result["agentArchiveTimeOnlySearchHTTPPassed"] = true
	result["agentHistoryContinuationModelRoundTripPassed"] = true
	result["agentArchiveSearchContinuationAssetsPassed"] = true
	result["agentMemoryContinuationModelRoundTripPassed"] = true
	result["agentMemorySourceSearchModelRoundTripPassed"] = true
	result["agentMemorySourceSearchHTTPPassed"] = true
	result["agentMemorySourceOnlyModelAndHTTPPassed"] = true
	result["agentMemoryRevisionTimeModelAndHTTPPassed"] = true
	result["agentMemoryCurrentReadAndLifecycleHTTPPassed"] = true
	result["agentMemoryNewestVersionsHTTPPassed"] = true
	result["agentMemoryExactRevisionHTTPPassed"] = true
	result["agentMemoryHistoryComparisonAssetsPassed"] = true
	result["agentMemoryDisabledSearchAndRestoreHTTPPassed"] = true
	result["agentHistorySourceContextModelRoundTripPassed"] = true
	result["agentLocalSearchMemoryManagementMigrationPassed"] = true
	result["agentTaskModelAndActionCancellationPassed"] = true
	result["agentManualMemoryDedupAndVersionLifecyclePassed"] = true
	result["agentMemorySearchReadModelRoundTripPassed"] = true
	result["agentMemoryBeforeModelRoundTripPassed"] = true
	result["agentMemoryBeforeAdminAPIPassed"] = true
	result["agentHistoryBeforeReadModelRoundTripPassed"] = true
	result["agentStaleMemoryReadRejectedPassed"] = true
	result["agentTaskRetrievalAndActivitySummaryPassed"] = true
	result["agentTaskExactReadScopeAndTerminalPassed"] = true
	result["agentTaskLatestEntryAssetsPassed"] = true
	result["agentTaskStatisticsDisclosureAssetsPassed"] = true
	result["agentTaskTriggerSourceArchivePassed"] = true
	result["agentMemoryExactTriggerSourcePassed"] = true
	result["agentTaskSettledResultSummaryPassed"] = true
	result["agentTaskQueryUsageAndCoveragePassed"] = true
	result["agentTaskPhaseAndStopTimesPassed"] = true
	result["agentTaskObservationSnapshotsPassed"] = true
	result["agentVisionBoundedInputAndCachePassed"] = true
	result["agentPreparationStopBeforeNextRequestPassed"] = true
	result["agentVisionDescriptionAndResponseLimitsPassed"] = true
	result["agentActionOutputBatchAndResponseLimitsPassed"] = true
	result["agentActionStructureLimitsAndRecoveryPassed"] = true
	result["agentMalformedActionBatchRejectedPassed"] = true
	result["agentDuplicateJSONFieldsRejectedPassed"] = true
	result["agentModelRequestFailureAndRecoveryPassed"] = true
	result["agentModelTextPartsAndInvalidOutputPassed"] = true
	result["agentVisionTextPartsPassed"] = true
	result["agentMemorySaveReferenceRoundTripPassed"] = true
	result["agentMemoryAndTaskFilteredPaginationPassed"] = true
	result["aiAgentGroupIsolationPassed"] = true
	result["aiAgentMemberAtAllDeniedPassed"] = true
	result["qqVoiceDeliveryPassed"] = true
	result["qqVoiceNativeDirectPassed"] = true
	result["qqVoicePrivateNoGroupSendPassed"] = true
	result["qqVoiceChatOutputScopePassed"] = true
	result["qqVoiceZeroIDReceiptPassed"] = true
	result["qqVoiceNoResourceForwardingPassed"] = true
	result["qqVoiceFailureAndScopePassed"] = true
	result["qqVoiceModelSelectionPassed"] = true
	result["gptReplyDeliveryBoundaryPassed"] = true
	result["gptCommandBoundaryAndQuotaPassed"] = true
	result["authenticatedLogsPassed"] = true
	result["logsRedactionAndCursorPassed"] = true
	result["logExportPassed"] = true
	result["appearanceAssetsPassed"] = true
	result["pluginSearchAssetsPassed"] = true
	result["pluginDraftAssetsPassed"] = true
	result["connectionDraftAssetsPassed"] = true
	result["connectionDiscardAssetsPassed"] = true
	result["logIssueFilterPassed"] = true
	result["logTimeFilterPassed"] = true
	result["backupPreviewExpiryAssetsPassed"] = true
	result["backupCatalogExportAssetsPassed"] = true
	result["logSummaryPassed"] = true
	result["logQuickRangeAssetsPassed"] = true
	result["pluginSortAssetsPassed"] = true
	result["pluginDiscardOneAssetsPassed"] = true
	result["backupSortAssetsPassed"] = true
	result["backupSizeSummaryAssetsPassed"] = true
	result["logReadOnceAssetsPassed"] = true
	result["backupDateFilterAssetsPassed"] = true
	result["pluginStateCatalogAssetsPassed"] = true
	result["logCopyEntryAssetsPassed"] = true
	result["aiDraftFieldsAssetsPassed"] = true
	result["connectionDraftFieldsAssetsPassed"] = true
	result["logClearDisplayAssetsPassed"] = true
	if _, err := os.Stat(filepath.Join(runtimeRoot, "data/Chat/dict.txt")); !os.IsNotExist(err) {
		return fmt.Errorf("startup unexpectedly loaded wordcount dictionary: %v", err)
	}
	result["wordcountLazyStartupPassed"] = true
	result["adminPasswordChangePassed"] = true
	result["backupImpactPassed"] = true
	result["connectionAdditionPassed"] = true
	result["connectionRemovalPassed"] = true
	result["startupDifferencesPassed"] = true
	result["reverseConnectionPassed"] = true
	result["reverseDriverAuthAndCommandPassed"] = true
	result["backupSearchAssetsPassed"] = true
	result["backupPaginationAssetsPassed"] = true
	result["pluginPaginationAssetsPassed"] = true
	result["pluginPendingAssetsPassed"] = true
	result["pluginResponseStatusPassed"] = true
	result["pluginUsageExportAssetsPassed"] = true
	result["pluginSaveAllAssetsPassed"] = true
	result["pluginSequentialSwitchesPassed"] = true
	result["aiDraftAssetsPassed"] = true
	result["aiDiscardAssetsPassed"] = true
	result["aiPrivateConversationPassed"] = true
	result["overviewAlertsPassed"] = true
	result["backupListAndManualCreatePassed"] = true
	result["backupPreviewRestoreAndConflictPassed"] = true
	result["backupCleanupPreviewAndRetentionPassed"] = true
	result["controlPolicyBackupsPassed"] = true
	result["controlPolicyFailureFeedbackPassed"] = true
	encoded, err = json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "result.json"), encoded, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "latest.json"), encoded, 0600); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Println("ARTIFACTS=" + runtimeRoot)
	if *uiWait {
		fmt.Println("SMOKE_ADMIN_URL=http://" + adminAddress)
		select {
		case <-time.After(5 * time.Minute):
		case <-ctx.Done():
		}
	}
	return nil
}

func prepareFixture(root, destination string) error {
	for _, relative := range []string{
		"data/Control/kanban.png", "data/Control/zbpuwu.png",
		"data/Font/GlowSansSC-Normal-ExtraBold.ttf", "data/Font/Impact.ttf",
		"data/Font/Torus-Regular.otf", "data/Font/FOT-Yuruka Std.ttf",
		"data/hyaku/小倉百人一首.csv",
		"data/Dish/dishes.db", "data/Pig/pig_data.json", "data/McFish/articlesInfo.json",
	} {
		if err := copyFixtureFile(filepath.Join(root, filepath.FromSlash(relative)), filepath.Join(destination, filepath.FromSlash(relative))); err != nil {
			return fmt.Errorf("fixture %s: %w", relative, err)
		}
	}
	return nil
}

func copyFixtureFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Every event below uses synthetic IDs and the isolated OneBot connection.
// Assert replies AND subsequent plugin behavior so a success message alone
// cannot hide an unchanged response gate or a change leaking to another group.
func verifyGroupResponses(send func(int64, int64, string, string, bool) (string, error)) error {
	cases := []struct {
		name, role, command, want string
		uid, gid                  int64
		atBot                     bool
	}{
		{"owner response without mention rejected", "member", "/响应", "", 12345678, 101, false},
		{"missing mention preserves silence", "member", "翻转 abc", "", 23456789, 101, false},
		{"owner response with mention", "member", "/响应", "工作", 12345678, 101, true},
		{"current group enabled", "member", "翻转 abc", "ɔqɐ", 23456789, 101, false},
		{"other group stays silent", "member", "翻转 abc", "", 23456789, 202, false},
		{"member silence rejected", "member", "/沉默", "", 23456789, 101, false},
		{"member silence with mention rejected", "member", "/沉默", "", 23456789, 101, true},
		{"rejected silence preserves response", "member", "翻转 abc", "ɔqɐ", 23456789, 101, false},
		{"admin silence without mention rejected", "admin", "/沉默\n", "", 23456789, 101, false},
		{"missing mention preserves response", "member", "翻转 abc", "ɔqɐ", 23456789, 101, false},
		{"admin silence with mention", "admin", "/沉默\n", "休息", 23456789, 101, true},
		{"current group silent", "member", "翻转 abc", "", 23456789, 101, false},
		{"member response rejected", "member", "/响应", "", 23456789, 101, false},
		{"member response with mention rejected", "member", "/响应", "", 23456789, 101, true},
		{"response prefix rejected", "admin", "/响应xxx", "", 23456789, 101, true},
		{"response arguments rejected", "admin", "/响应 202", "", 23456789, 101, true},
		{"rejected response preserves silence", "member", "翻转 abc", "", 23456789, 101, false},
		{"admin response without mention rejected", "admin", "/响应\r\n", "", 23456789, 101, false},
		{"missing admin mention preserves silence", "member", "翻转 abc", "", 23456789, 101, false},
		{"admin response with whitespace and mention", "admin", "/响应\r\n", "工作", 23456789, 101, true},
		{"admin response enables plugin", "member", "翻转 abc", "ɔqɐ", 23456789, 101, false},
		{"group owner silence", "owner", "/沉默", "休息", 34567890, 101, true},
		{"admin response with mention", "admin", "/response", "工作", 23456789, 101, true},
		{"other group remains silent", "member", "翻转 abc", "", 23456789, 202, false},
		{"group owner silence alias", "owner", "/silence", "休息", 34567890, 101, true},
		{"global response restricted to bot owner", "admin", "/全局响应", "", 23456789, 101, true},
		{"global denial preserves other group", "member", "翻转 abc", "", 23456789, 202, false},
	}
	for _, c := range cases {
		text, err := send(c.uid, c.gid, c.role, c.command, c.atBot)
		if err != nil || (c.want == "" && text != "") || (c.want != "" && !strings.Contains(text, c.want)) {
			return fmt.Errorf("group response check %q failed: got %q, want %q, error %v", c.name, text, c.want, err)
		}
	}
	return nil
}

func smokeServiceListNodes(raw json.RawMessage) error {
	var nodes []struct {
		Type string
		Data map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return err
	}
	if len(nodes) < 2 || len(nodes) > 21 {
		return fmt.Errorf("service list forward bounds invalid")
	}
	for i, node := range nodes {
		var sender string
		_ = json.Unmarshal(node.Data["uin"], &sender)
		if node.Type != "node" || sender != "87654321" {
			return fmt.Errorf("service list node sender/type invalid")
		}
		text, err := smokeMessageText(node.Data["content"])
		if err != nil || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 1200 {
			return fmt.Errorf("service list text invalid or exceeds bounds")
		}
		if i == 0 && !strings.Contains(text, "服务列表与用法") {
			return fmt.Errorf("service list header missing")
		}
		if i > 0 && (!strings.Contains(text, "插件：") || !strings.Contains(text, "当前会话：") || !strings.Contains(text, "使用方式：")) {
			return fmt.Errorf("service list usage/state missing")
		}
	}
	return nil
}

func smokeMessageText(raw json.RawMessage) (string, error) {
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
			text, err := smokeMessageText(segment.Data["content"])
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		}
	}
	return out.String(), nil
}

func verifyBackupImpacts(preview map[string]any) error {
	backup, ok := preview["backup"].(map[string]any)
	if !ok {
		return fmt.Errorf("impact backup metadata missing")
	}
	scope, _ := backup["scope"].(string)
	labels := map[string][]string{
		"startup-config": {"正向 OneBot 连接", "反向 OneBot 连接", "机器人主人", "其他机器人参数", "其他启动设置"},
		"admin-options":  {"后台启用状态", "后台访问地址"},
		"control-db":     {"响应规则与AI扩展", "全局用户封禁", "插件开关继承标记", "插件与其他库内数据"},
	}[scope]
	items, ok := preview["impacts"].([]any)
	if !ok || len(labels) == 0 || len(items) < len(labels) || len(items) > len(labels)+1 {
		return fmt.Errorf("impact categories incomplete")
	}
	changed, rows := false, float64(0)
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid impact record")
		}
		label := "字段名称或表示差异"
		if i < len(labels) {
			label = labels[i]
		} else if scope != "startup-config" {
			return fmt.Errorf("unexpected impact category")
		}
		if item["label"] != label {
			return fmt.Errorf("unexpected impact label")
		}
		flag, ok := item["changed"].(bool)
		if !ok {
			return fmt.Errorf("impact change marker missing")
		}
		changed = changed || flag
		for key := range item {
			switch key {
			case "label", "changed", "currentCount", "backupCount", "unit":
			default:
				return fmt.Errorf("unexpected impact value")
			}
		}
		if scope != "admin-options" && i < len(labels) {
			for _, key := range []string{"currentCount", "backupCount"} {
				count, ok := item[key].(float64)
				if !ok || count < 0 || count != float64(int(count)) {
					return fmt.Errorf("invalid impact count")
				}
				if key == "backupCount" {
					rows += count
				}
			}
		} else if item["currentCount"] != nil || item["backupCount"] != nil {
			return fmt.Errorf("unexpected impact count")
		}
	}
	if preview["changed"] != changed || scope == "control-db" && preview["rows"] != rows {
		return fmt.Errorf("impact summary inconsistent")
	}
	encoded, _ := json.Marshal(preview)
	for _, secret := range []string{"synthetic-model-key", "local-smoke-only", "127.0.0.1:3010", "127.0.0.1:3111"} {
		if bytes.Contains(encoded, []byte(secret)) {
			return fmt.Errorf("impact preview leaked a value")
		}
	}
	return nil
}

func verifyPluginResponseStatus(do func(string, string, any, string) (int, map[string]any, error), sendControl func(int64, string) (string, error), sendGroup func(int64, int64, string, string, bool) (string, error)) error {
	read := func(group string, global, local bool) error {
		status, before, err := do("GET", "/api/backups", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("response status backup count unavailable")
		}
		status, value, err := do("GET", "/api/plugins?group="+group, nil, "")
		if err != nil || status != 200 || value["groupId"] != group {
			return fmt.Errorf("response status read failed")
		}
		state, ok := value["response"].(map[string]any)
		source := "none"
		if global {
			source = "global"
		} else if local {
			source = "group"
		}
		if !ok || len(state) != 4 || state["globalEnabled"] != global || state["groupEnabled"] != local || state["effectiveEnabled"] != (global || local) || state["source"] != source {
			return fmt.Errorf("response status disagreed with command rules")
		}
		status, after, err := do("GET", "/api/backups", nil, "")
		if err != nil || status != 200 || len(before["backups"].([]any)) != len(after["backups"].([]any)) {
			return fmt.Errorf("response status read created a backup")
		}
		return nil
	}
	for _, group := range []string{"0", "101", "202"} {
		if err := read(group, false, false); err != nil {
			return err
		}
	}
	text, err := sendGroup(12345678, 101, "member", "/响应", true)
	if err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("response status group setup failed")
	}
	if err := read("101", false, true); err != nil {
		return err
	}
	if err := read("202", false, false); err != nil {
		return err
	}
	text, err = sendControl(12345678, "/全局响应")
	if err != nil || !strings.Contains(text, "全部位置工作") {
		return fmt.Errorf("response status global setup failed")
	}
	if err := read("0", true, false); err != nil {
		return err
	}
	if err := read("101", true, true); err != nil {
		return err
	}
	if err := read("202", true, false); err != nil {
		return err
	}
	text, err = sendGroup(12345678, 202, "member", "/沉默", true)
	if err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("response status covered group silence failed")
	}
	if err := read("202", true, false); err != nil {
		return err
	}
	text, err = sendControl(12345678, "/全局沉默")
	if err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("response status global cleanup failed")
	}
	if err := read("101", false, true); err != nil {
		return err
	}
	if err := read("202", false, false); err != nil {
		return err
	}
	text, err = sendGroup(12345678, 101, "member", "/沉默", true)
	if err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("response status group cleanup failed")
	}
	return read("101", false, false)
}

func verifySequentialPluginSaves(do func(string, string, any, string) (int, map[string]any, error), csrf string) error {
	read := func() (map[string]map[string]any, error) {
		status, value, err := do("GET", "/api/plugins?group=0", nil, "")
		if err != nil || status != 200 || value["controlAvailable"] != true {
			return nil, fmt.Errorf("sequential plugin read failed")
		}
		states := map[string]map[string]any{}
		for _, raw := range value["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "chat" || p["name"] == "atri" {
				states[p["name"].(string)] = p["state"].(map[string]any)
			}
		}
		if len(states) != 2 {
			return nil, fmt.Errorf("sequential plugin fixtures absent")
		}
		return states, nil
	}
	backups := func() (int, error) {
		status, value, err := do("GET", "/api/backups", nil, "")
		if err != nil || status != 200 {
			return 0, fmt.Errorf("sequential backup count failed")
		}
		n := 0
		for _, raw := range value["backups"].([]any) {
			if raw.(map[string]any)["scope"] == "control-db" {
				n++
			}
		}
		return n, nil
	}
	write := func(name, mode string, old map[string]any, expected int) (map[string]any, error) {
		status, value, err := do("POST", "/api/plugin-switch", map[string]any{"name": name, "groupId": "0", "mode": mode, "revision": old["revision"]}, csrf)
		if err != nil || status != expected {
			return nil, fmt.Errorf("sequential plugin save status %d expected %d", status, expected)
		}
		if status != 200 {
			return nil, nil
		}
		state, ok := value["state"].(map[string]any)
		if !ok || value["name"] != name || value["groupId"] != "0" || state["globalMode"] != mode || len(fmt.Sprint(state["revision"])) != 64 {
			return nil, fmt.Errorf("sequential plugin result mismatched")
		}
		return state, nil
	}
	initial, err := read()
	if err != nil {
		return err
	}
	before, err := backups()
	if err != nil {
		return err
	}
	for _, name := range []string{"chat", "atri"} {
		mode := "enabled"
		if initial[name]["globalMode"] == mode {
			mode = "disabled"
		}
		if _, err := write(name, mode, initial[name], 200); err != nil {
			return err
		}
	}
	after, err := backups()
	if err != nil || after != before+2 {
		return fmt.Errorf("sequential changes did not create individual backups")
	}
	changed, err := read()
	if err != nil {
		return err
	}
	// The first restore succeeds; the second uses its old revision and conflicts.
	// The already successful item stays committed, matching UI stop-on-error rules.
	first, err := write("chat", initial["chat"]["globalMode"].(string), changed["chat"], 200)
	if err != nil {
		return err
	}
	if _, err := write("atri", initial["atri"]["globalMode"].(string), initial["atri"], 409); err != nil {
		return err
	}
	partial, err := read()
	if err != nil || partial["chat"]["revision"] != first["revision"] || partial["atri"]["revision"] != changed["atri"]["revision"] {
		return fmt.Errorf("partial failure lost successful or remaining state")
	}
	if _, err := write("atri", initial["atri"]["globalMode"].(string), partial["atri"], 200); err != nil {
		return err
	}
	final, err := read()
	if err != nil || final["chat"]["globalMode"] != initial["chat"]["globalMode"] || final["atri"]["globalMode"] != initial["atri"]["globalMode"] {
		return fmt.Errorf("sequential fixture cleanup did not restore modes")
	}
	after, err = backups()
	if err != nil || after != before+4 {
		return fmt.Errorf("failed item wrote a backup or restore failed")
	}
	if _, err := write("chat", final["chat"]["globalMode"].(string), final["chat"], 200); err != nil {
		return err
	}
	unchanged, err := backups()
	if err != nil || unchanged != after {
		return fmt.Errorf("unchanged plugin produced an unnecessary backup")
	}
	return nil
}

func verifyStartupSummary(value map[string]any, target string, running, saved int, expectedChanged bool) error {
	items, ok := value["startupDifferences"].([]any)
	labels := []string{"正向 OneBot 连接", "反向 OneBot 连接", "机器人主人", "其他机器人参数", "其他启动设置", "字段名称或表示差异"}
	if !ok || len(items) < 5 || len(items) > 6 {
		return fmt.Errorf("startup difference categories missing")
	}
	changed, found := false, false
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item["label"] != labels[i] {
			return fmt.Errorf("startup difference labels incorrect")
		}
		state, ok := item["changed"].(bool)
		if !ok {
			return fmt.Errorf("startup difference state missing")
		}
		changed = changed || state
		for key := range item {
			if key != "label" && key != "changed" && key != "unit" && key != "runningCount" && key != "savedCount" {
				return fmt.Errorf("startup difference contains unapproved field")
			}
		}
		if i < 5 {
			a, aok := item["runningCount"].(float64)
			b, bok := item["savedCount"].(float64)
			if !aok || !bok || a < 0 || b < 0 {
				return fmt.Errorf("startup difference counts invalid")
			}
			if item["label"] == target {
				found = true
				if a != float64(running) || b != float64(saved) || state != expectedChanged {
					return fmt.Errorf("startup difference target did not match snapshot")
				}
			}
		} else if item["runningCount"] != nil || item["savedCount"] != nil {
			return fmt.Errorf("representation row invented counts")
		}
	}
	if !found || value["restartRequired"] != changed {
		return fmt.Errorf("startup summary disagrees with restart decision")
	}
	return nil
}

func verifyLogSummary(value map[string]any) error {
	summary, ok := value["summary"].(map[string]any)
	if !ok || len(summary) != 2 {
		return fmt.Errorf("log summary schema unavailable")
	}
	matched, ok := summary["matched"].(float64)
	retained, rok := value["retained"].(float64)
	if !ok || !rok || matched < 0 || matched > retained || retained > 1000 || matched != float64(int(matched)) {
		return fmt.Errorf("log matching total invalid")
	}
	levels, ok := summary["levels"].(map[string]any)
	if !ok || len(levels) != 8 {
		return fmt.Errorf("log level summary schema invalid")
	}
	var total float64
	for _, name := range []string{"panic", "fatal", "error", "warning", "info", "debug", "trace", "other"} {
		n, ok := levels[name].(float64)
		if !ok || n < 0 || n > 1000 || n != float64(int(n)) {
			return fmt.Errorf("log level count invalid")
		}
		total += n
	}
	if total != matched || len(value["entries"].([]any)) > int(matched) {
		return fmt.Errorf("log summary differs from retained matching rows")
	}
	return nil
}

func verifyAIConfigChecks(value map[string]any) error {
	global, ok := value["global"].(map[string]any)
	if !ok {
		return fmt.Errorf("AI checks global snapshot unavailable")
	}
	fields, _ := global["fields"].(map[string]any)
	keys, _ := global["keys"].(map[string]any)
	checks, ok := global["checks"].(map[string]any)
	if !ok || len(checks) != 3 {
		return fmt.Errorf("AI checks missing independent interfaces")
	}
	for _, spec := range []struct{ name, prefix, key string }{{"chat", "", "Key"}, {"agent", "Agent", "AgentKey"}, {"image", "Image", "ImageKey"}} {
		check, ok := checks[spec.name].(map[string]any)
		if !ok || len(check) != 8 {
			return fmt.Errorf("AI check schema invalid")
		}
		for _, value := range check {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("AI check exposes field contents")
			}
		}
		kind, _ := fields[spec.prefix+"Type"].(float64)
		model, _ := fields[spec.prefix+"ModelName"].(string)
		address, _ := fields[spec.prefix+"API"].(string)
		u, err := url.Parse(address)
		addressValid := err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.RawQuery == "" && u.Fragment == ""
		if addressValid && u.Port() != "" {
			port, err := strconv.ParseUint(u.Port(), 10, 16)
			addressValid = err == nil && port != 0
		}
		modelConfigured, addressConfigured := strings.TrimSpace(model) != "", strings.TrimSpace(address) != ""
		keyConfigured := keys[spec.key] == true
		typeValid, modelRequired, keyRequired := kind >= 0 && kind <= 2, spec.name != "image", kind != 1
		expected := map[string]bool{"typeValid": typeValid, "modelConfigured": modelConfigured, "modelRequired": modelRequired, "addressConfigured": addressConfigured, "addressValid": addressValid, "keyConfigured": keyConfigured, "keyRequired": keyRequired,
			"complete": typeValid && addressConfigured && addressValid && (modelConfigured || !modelRequired) && (keyConfigured || !keyRequired)}
		for name, want := range expected {
			if check[name] != want {
				return fmt.Errorf("AI check differs from returned snapshot: %s/%s", spec.name, name)
			}
		}
	}
	return nil
}

func verifyAdmin(baseURL, dataDir, modelURL string, sendControl func(int64, string) (string, error), sendGroup func(int64, int64, string, string, bool) (string, error), inFlight func(string) bool) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	for _, asset := range []struct{ path, kind, marker string }{
		{"/", "text/html; charset=utf-8", `id="memory-search-continue"`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `clearMemorySearchContinue`},
		{"/", "text/html; charset=utf-8", `id="memory-current-id"`},
		{"/", "text/html; charset=utf-8", `id="memory-search-state-filter"`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `loadCurrentMemory`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `的版本记录（从新到旧）`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `定位历史修订`},
		{"/", "text/html; charset=utf-8", `id="agent-search-continue"`},
		{"/", "text/html; charset=utf-8", `id="agent-record-form"`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `readAgentRecord`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `对比两次修订`},
		{"/agent-memory.js", "text/javascript; charset=utf-8", `memoryHistoryComparison`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `clearAgentSearchContinue`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `searchScope:saved.scope`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `查看前后事件`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `context:String(radius)`},
		{"/agent-archive.js", "text/javascript; charset=utf-8", `相邻事件范围（每侧最多）`},
		{"/", "text/html; charset=utf-8", `id="tasks-reason"`},
		{"/", "text/html; charset=utf-8", `id="tasks-outcome"`},
		{"/", "text/html; charset=utf-8", `id="tasks-from"`},
		{"/", "text/html; charset=utf-8", `id="tasks-until"`},
		{"/", "text/html; charset=utf-8", `id="agent-search-sender"`},
		{"/", "text/html; charset=utf-8", `id="agent-search-event-type"`},
		{"/agent-tasks.js", "text/javascript; charset=utf-8", `tasksReason`},
		{"/agent-tasks.js", "text/javascript; charset=utf-8", `taskDurationText`},
		{"/agent-tasks.js", "text/javascript; charset=utf-8", `核对最新状态`},
		{"/agent-tasks.js", "text/javascript; charset=utf-8", `上下文、记忆与执行统计`},
		{"/style.css", "text/css; charset=utf-8", `.task-statistics>summary`},
		{"/agent-tasks.js", "text/javascript; charset=utf-8", `容量、动作数量或结构限制`},
		{"/", "text/html; charset=utf-8", `data-theme-select`},
		{"/", "text/html; charset=utf-8", `id="password-change-form"`},
		{"/security.js", "text/javascript; charset=utf-8", `changeAdminPassword`},
		{"/", "text/html; charset=utf-8", `id="ai-config-checks"`},
		{"/ai.js", "text/javascript; charset=utf-8", `renderAIConfigChecks`},
		{"/style.css", "text/css; charset=utf-8", `.ai-config-check-list`},
		{"/", "text/html; charset=utf-8", `id="log-summary-state"`},
		{"/logs.js", "text/javascript; charset=utf-8", `renderLogSummary`},
		{"/", "text/html; charset=utf-8", `id="log-recent-15m"`},
		{"/", "text/html; charset=utf-8", `id="log-recent-1h"`},
		{"/", "text/html; charset=utf-8", `id="log-recent-24h"`},
		{"/", "text/html; charset=utf-8", `id="log-any-time"`},
		{"/logs.js", "text/javascript; charset=utf-8", `applyRecentLogRange`},
		{"/", "text/html; charset=utf-8", `id="plugin-sort"`},
		{"/app.js", "text/javascript; charset=utf-8", `function sortPluginItems`},
		{"/", "text/html; charset=utf-8", `id="model-request-categories"`},
		{"/overview.js", "text/javascript; charset=utf-8", `function renderModelRequests`},
		{"/", "text/html; charset=utf-8", `id="reply-task-categories"`},
		{"/reply-metrics.js", "text/javascript; charset=utf-8", `function renderReplyTasks`},
		{"/", "text/html; charset=utf-8", `id="download-reply-statistics"`},
		{"/reply-metrics.js", "text/javascript; charset=utf-8", `function downloadReplyStatistics`},
		{"/", "text/html; charset=utf-8", `id="download-model-observation"`},
		{"/model-observation.js", "text/javascript; charset=utf-8", `function downloadModelObservation`},
		{"/style.css", "text/css; charset=utf-8", `.model-observation-actions`},
		{"/style.css", "text/css; charset=utf-8", `.model-request-grid`},
		{"/app.js", "text/javascript; charset=utf-8", `function discardOnePluginDraft`},
		{"/app.js", "text/javascript; charset=utf-8", `name !== plugin.name`},
		{"/app.js", "text/javascript; charset=utf-8", `放弃此项更改`},
		{"/style.css", "text/css; charset=utf-8", `.plugin-single-draft`},
		{"/", "text/html; charset=utf-8", `id="backup-sort"`},
		{"/backups.js", "text/javascript; charset=utf-8", `function sortBackupEntries`},
		{"/", "text/html; charset=utf-8", `id="backup-size-summary"`},
		{"/backups.js", "text/javascript; charset=utf-8", `function renderBackupSizeSummary`},
		{"/style.css", "text/css; charset=utf-8", `.backup-size-scopes`},
		{"/", "text/html; charset=utf-8", `id="read-logs-once"`},
		{"/logs.js", "text/javascript; charset=utf-8", `async function readLogsOnce`},
		{"/", "text/html; charset=utf-8", `id="backup-from-date"`},
		{"/", "text/html; charset=utf-8", `id="backup-until-date"`},
		{"/backups.js", "text/javascript; charset=utf-8", `function applyBackupDateFilter`},
		{"/", "text/html; charset=utf-8", `id="download-plugin-catalog"`},
		{"/app.js", "text/javascript; charset=utf-8", `function downloadPluginCatalog`},
		{"/logs.js", "text/javascript; charset=utf-8", `async function copyLogEntry`},
		{"/style.css", "text/css; charset=utf-8", `.log-copy-actions`},
		{"/", "text/html; charset=utf-8", `id="ai-draft-fields"`},
		{"/", "text/html; charset=utf-8", `id="connection-draft-fields"`},
		{"/app.js", "text/javascript; charset=utf-8", `function renderConnectionDraftFields`},
		{"/", "text/html; charset=utf-8", `id="clear-log-display"`},
		{"/logs.js", "text/javascript; charset=utf-8", `function clearLogDisplay`},
		{"/style.css", "text/css; charset=utf-8", `.connection-draft-field-list`},
		{"/ai.js", "text/javascript; charset=utf-8", `function renderAIDraftFields`},
		{"/style.css", "text/css; charset=utf-8", `.ai-draft-field-list`},
		{"/style.css", "text/css; charset=utf-8", `.log-summary-levels`},
		{"/theme.js", "text/javascript; charset=utf-8", `zerobot.localadmin.theme.v1`},
		{"/appearance.css", "text/css; charset=utf-8", `:root[data-theme="dark"]`},
		{"/", "text/html; charset=utf-8", `id="plugin-state-filter"`},
		{"/", "text/html; charset=utf-8", `id="plugin-only-drafts"`},
		{"/", "text/html; charset=utf-8", `id="discard-plugin-drafts"`},
		{"/app.js", "text/javascript; charset=utf-8", `discardPluginDrafts`},
		{"/", "text/html; charset=utf-8", `id="save-all-plugin-drafts"`},
		{"/", "text/html; charset=utf-8", `id="stop-plugin-batch"`},
		{"/app.js", "text/javascript; charset=utf-8", `saveAllPluginDrafts`},
		{"/", "text/html; charset=utf-8", `id="plugin-response-panel"`},
		{"/app.js", "text/javascript; charset=utf-8", `renderPluginResponse`},
		{"/", "text/html; charset=utf-8", `id="copy-plugin-usage"`},
		{"/", "text/html; charset=utf-8", `id="download-plugin-usage"`},
		{"/app.js", "text/javascript; charset=utf-8", `copyPluginUsage`},
		{"/app.js", "text/javascript; charset=utf-8", `downloadPluginUsage`},
		{"/style.css", "text/css; charset=utf-8", `.usage-actions`},
		{"/style.css", "text/css; charset=utf-8", `.plugin-response-panel`},
		{"/app.js", "text/javascript; charset=utf-8", `pluginBatchResultValid`},
		{"/style.css", "text/css; charset=utf-8", `#plugin-batch-status`},
		{"/app.js", "text/javascript; charset=utf-8", `pluginOnlyDrafts`},
		{"/", "text/html; charset=utf-8", `id="plugin-page-size"`},
		{"/", "text/html; charset=utf-8", `id="plugin-page-status"`},
		{"/app.js", "text/javascript; charset=utf-8", `changePluginPage`},
		{"/app.js", "text/javascript; charset=utf-8", `renderPluginPagination`},
		{"/", "text/html; charset=utf-8", `id="clear-plugin-filter"`},
		{"/app.js", "text/javascript; charset=utf-8", `[p.name, p.brief, p.usage]`},
		{"/appearance.css", "text/css; charset=utf-8", `.plugin-filter-label select`},
		{"/", "text/html; charset=utf-8", `id="ai-draft-status"`},
		{"/", "text/html; charset=utf-8", `id="discard-ai-global"`},
		{"/ai.js", "text/javascript; charset=utf-8", `discardAIForm`},
		{"/ai.js", "text/javascript; charset=utf-8", `data-ai-discard`},
		{"/ai.js", "text/javascript; charset=utf-8", `confirmAIDiscard`},
		{"/ai.js", "text/javascript; charset=utf-8", `beforeunload`},
		{"/app.js", "text/javascript; charset=utf-8", `csrf === requestToken`},
		{"/", "text/html; charset=utf-8", `id="plugin-draft-status"`},
		{"/app.js", "text/javascript; charset=utf-8", `confirmPluginDiscard`},
		{"/app.js", "text/javascript; charset=utf-8", `generation !== pluginGeneration`},
		{"/", "text/html; charset=utf-8", `id="connection-delete-preview"`},
		{"/connections-delete.js", "text/javascript; charset=utf-8", `deletionCurrent`},
		{"/", "text/html; charset=utf-8", `id="startup-differences"`},
		{"/app.js", "text/javascript; charset=utf-8", `renderStartupDifferences`},
		{"/style.css", "text/css; charset=utf-8", `.startup-difference-row`},
		{"/", "text/html; charset=utf-8", `id="connection-add-kind"`},
		{"/connections-add.js", "text/javascript; charset=utf-8", `本机反向监听必须填写 Token`},
		{"/", "text/html; charset=utf-8", `id="connection-add-form"`},
		{"/connections-add.js", "text/javascript; charset=utf-8", `connectionAddSnapshot`},
		{"/", "text/html; charset=utf-8", `id="connection-draft-status"`},
		{"/", "text/html; charset=utf-8", `id="discard-current-connection"`},
		{"/", "text/html; charset=utf-8", `id="log-from"`},
		{"/", "text/html; charset=utf-8", `id="log-until"`},
		{"/logs.js", "text/javascript; charset=utf-8", `function appliedLogQuery`},
		{"/", "text/html; charset=utf-8", `id="discard-connection-owners"`},
		{"/app.js", "text/javascript; charset=utf-8", `discardConnectionDraft`},
		{"/app.js", "text/javascript; charset=utf-8", `settings === connectionSettings`},
		{"/", "text/html; charset=utf-8", `id="connection-draft-confirmation"`},
		{"/app.js", "text/javascript; charset=utf-8", `finishConnectionDraftConfirmation`},
		{"/app.js", "text/javascript; charset=utf-8", `confirmConnectionDiscard`},
		{"/app.js", "text/javascript; charset=utf-8", `generation !== connectionGeneration`},
		{"/backups.js", "text/javascript; charset=utf-8", `resetConnections()`},
		{"/", "text/html; charset=utf-8", `id="view-issue-logs"`},
		{"/", "text/html; charset=utf-8", `id="clear-log-filter"`},
		{"/logs.js", "text/javascript; charset=utf-8", `applyLogFilters`},
		{"/", "text/html; charset=utf-8", `id="restore-impact-list"`},
		{"/", "text/html; charset=utf-8", `id="restore-impact-status"`},
		{"/backups.js", "text/javascript; charset=utf-8", `renderRestoreImpacts`},
		{"/", "text/html; charset=utf-8", `id="backup-scope-filter"`},
		{"/", "text/html; charset=utf-8", `id="backup-state-filter"`},
		{"/", "text/html; charset=utf-8", `id="restore-backup-id"`},
		{"/backups.js", "text/javascript; charset=utf-8", `applyBackupFilter`},
		{"/backups.js", "text/javascript; charset=utf-8", `backupReadFailed`},
		{"/backups.js", "text/javascript; charset=utf-8", `尚未取得记录`},
		{"/", "text/html; charset=utf-8", `id="restore-expiry-state"`},
		{"/", "text/html; charset=utf-8", `id="cleanup-expiry-state"`},
		{"/backups.js", "text/javascript; charset=utf-8", `function renderBackupExpiry`},
		{"/backups.js", "text/javascript; charset=utf-8", `expiredBackupPreviews`},
		{"/style.css", "text/css; charset=utf-8", `.preview-expiry-state`},
		{"/", "text/html; charset=utf-8", `id="download-backup-catalog"`},
		{"/", "text/html; charset=utf-8", `id="backup-export-status"`},
		{"/backups.js", "text/javascript; charset=utf-8", `function downloadBackupCatalog`},
		{"/backups.js", "text/javascript; charset=utf-8", `zerobot-backup-catalog`},
		{"/style.css", "text/css; charset=utf-8", `.backup-export`},
		{"/", "text/html; charset=utf-8", `id="backup-page-size"`},
		{"/", "text/html; charset=utf-8", `id="backup-page-status"`},
		{"/backups.js", "text/javascript; charset=utf-8", `changeBackupPage`},
		{"/backups.js", "text/javascript; charset=utf-8", `prefers-reduced-motion`},
		{"/", "text/html; charset=utf-8", `id="ai-scope-kind"`},
		{"/ai.js", "text/javascript; charset=utf-8", `body.privateId=value.privateId`},
	} {
		rsp, err := client.Get(baseURL + asset.path)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(rsp.Body, 128<<10))
		rsp.Body.Close()
		if readErr != nil || rsp.StatusCode != 200 || rsp.Header.Get("Content-Type") != asset.kind || rsp.Header.Get("Cache-Control") != "no-store" || !bytes.Contains(body, []byte(asset.marker)) {
			return fmt.Errorf("admin asset check failed: %s", asset.path)
		}
	}
	do := func(method, path string, body any, csrf string) (int, map[string]any, error) {
		var encoded []byte
		if body != nil {
			encoded, _ = json.Marshal(body)
		}
		req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return 0, nil, err
		}
		if method == "POST" {
			req.Header.Set("Origin", baseURL)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Local-Request", "1")
			req.Header.Set("X-CSRF-Token", csrf)
		}
		rsp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer rsp.Body.Close()
		var value map[string]any
		err = json.NewDecoder(rsp.Body).Decode(&value)
		if err == nil && path == "/api/backups/preview" && rsp.StatusCode == 200 {
			err = verifyBackupImpacts(value)
		}
		return rsp.StatusCode, value, err
	}
	status, _, err := do("GET", "/api/plugins", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("unauthed admin request was not denied: %d %v", status, err)
	}
	status, _, err = do("GET", "/api/logs", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("unauthed logs were not denied")
	}
	status, _, err = do("GET", "/api/logs/export", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("unauthed log export was not denied")
	}
	status, _, err = do("GET", "/api/backups", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("unauthed backups were not denied")
	}
	code, err := os.ReadFile(filepath.Join(dataDir, "setup-code.txt"))
	if err != nil {
		return err
	}
	status, session, err := do("POST", "/api/setup", map[string]string{"setupCode": strings.TrimSpace(string(code)), "password": "Local-ui-test-password-only"}, "")
	if err != nil || status != 200 {
		return fmt.Errorf("isolated admin setup failed: %d %v", status, err)
	}
	csrf, _ := session["csrfToken"].(string)
	// Only this run's isolated options file is edited; runtime uses explicit CLI flags.
	optionsPath := filepath.Join(dataDir, "options.json")
	optionsBefore := []byte("{\"enabled\":true,\"address\":\"127.0.0.1:3010\"}")
	if err = os.WriteFile(optionsPath, optionsBefore, 0600); err != nil {
		return err
	}
	status, optionsBackup, err := do("POST", "/api/backups", map[string]string{"scope": "admin-options"}, csrf)
	if err != nil || status != 200 || optionsBackup["backupId"] == "" {
		return fmt.Errorf("options backup fixture failed: %d %v", status, err)
	}
	status, equalOptions, err := do("POST", "/api/backups/preview", map[string]any{"id": optionsBackup["backupId"]}, csrf)
	if err != nil || status != 200 || equalOptions["changed"] != false {
		return fmt.Errorf("equal options impacts failed: %d %v", status, err)
	}
	optionsChanged := []byte("{\"enabled\":false,\"address\":\"127.0.0.1:3111\"}")
	if err = os.WriteFile(optionsPath, optionsChanged, 0600); err != nil {
		return err
	}
	status, changedOptions, err := do("POST", "/api/backups/preview", map[string]any{"id": optionsBackup["backupId"]}, csrf)
	if err != nil || status != 200 || changedOptions["changed"] != true || changedOptions["restartRequired"] != true {
		return fmt.Errorf("changed options impacts failed: %d %v", status, err)
	}
	optionsAfter, readErr := os.ReadFile(optionsPath)
	if readErr != nil || !bytes.Equal(optionsAfter, optionsChanged) {
		return fmt.Errorf("preview changed isolated options")
	}

	status, overview, err := do("GET", "/api/overview", nil, "")
	if err != nil || status != 200 || overview["connectionState"] != "connected" {
		return fmt.Errorf("isolated runtime overview failed: %d %v", status, err)
	}
	modelRequests, statsOK := overview["modelRequests"].(map[string]any)
	if !statsOK || modelRequests["started"] != float64(0) || modelRequests["active"] != float64(0) || modelRequests["completed"] != float64(0) || modelRequests["failed"] != float64(0) || modelRequests["incomplete"] != float64(0) || modelRequests["startedAt"] == nil || modelRequests["checkedAt"] == nil {
		return fmt.Errorf("isolated zero model statistics absent or incorrect")
	}
	categories, statsOK := modelRequests["categories"].([]any)
	if !statsOK || len(categories) != 4 {
		return fmt.Errorf("model statistics category bounds absent")
	}
	accounts, _ := overview["botAccounts"].([]any)
	if len(accounts) != 1 || accounts[0] != "87654321" {
		return fmt.Errorf("overview does not reflect actual isolated bot")
	}
	alerts, ok := overview["alerts"].(map[string]any)
	if !ok {
		return fmt.Errorf("overview retained alerts absent")
	}
	alertRows, ok := alerts["entries"].([]any)
	if !ok || len(alertRows) > 5 || alerts["warningCount"] == nil || alerts["errorCount"] == nil || alerts["retained"] == nil {
		return fmt.Errorf("overview alert bounds/counts absent")
	}
	lastID := uint64(^uint64(0))
	for _, item := range alertRows {
		row, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid overview alert row")
		}
		id, _ := strconv.ParseUint(fmt.Sprint(row["id"]), 10, 64)
		level := fmt.Sprint(row["level"])
		if id >= lastID || (level != "warning" && level != "error" && level != "fatal" && level != "panic") {
			return fmt.Errorf("overview alerts not ordered/filtered")
		}
		lastID = id
	}
	assetResponse, assetErr := client.Get(baseURL + "/overview.js")
	if assetErr != nil {
		return assetErr
	}
	assetBody, assetErr := io.ReadAll(assetResponse.Body)
	assetResponse.Body.Close()
	if assetErr != nil || assetResponse.StatusCode != 200 || !strings.Contains(string(assetBody), "visibilitychange") || !strings.Contains(string(assetBody), "AbortController") {
		return fmt.Errorf("overview polling asset unavailable")
	}
	status, list, err := do("GET", "/api/plugins", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("isolated plugin registry failed")
	}
	plugins, _ := list["plugins"].([]any)
	found := false
	for _, value := range plugins {
		if p, ok := value.(map[string]any); ok && p["name"] == "chat" {
			found = true
		}
	}
	if !found || float64(len(plugins)) != overview["pluginCount"] {
		return fmt.Errorf("actual plugin registry does not match overview")
	}
	pages := (len(plugins) + 19) / 20
	for page := 1; page <= pages; page++ {
		text, err := sendControl(23456789, fmt.Sprintf("/服务列表 %d", page))
		if err != nil || !strings.Contains(text, fmt.Sprintf("第 %d/%d 页", page, pages)) {
			return fmt.Errorf("service list page %d failed: %v", page, err)
		}
		for _, item := range plugins[(page-1)*20 : min(page*20, len(plugins))] {
			plugin := item.(map[string]any)
			if !strings.Contains(text, "插件："+plugin["name"].(string)+"\n") {
				return fmt.Errorf("service list lost registered plugin")
			}
		}
	}
	for _, arg := range []string{"0", "abc", strconv.Itoa(pages + 1)} {
		text, err := sendControl(23456789, "/服务列表 "+arg)
		if err != nil || !strings.Contains(text, "页") {
			return fmt.Errorf("service list invalid page not rejected")
		}
	}
	if text, err := sendControl(23456789, "/service_list"); err != nil || !strings.Contains(text, "第 1/") {
		return fmt.Errorf("service list alias failed")
	}
	if text, err := sendGroup(23456789, 202, "member", "/服务列表", false); err != nil || text != "" {
		return fmt.Errorf("service list lost group @ condition")
	}
	if text, err := sendGroup(23456789, 202, "member", "/服务列表", true); err != nil || !strings.Contains(text, "当前群") {
		return fmt.Errorf("group service list forward failed")
	}
	if err := verifySequentialPluginSaves(do, csrf); err != nil {
		return err
	}
	// Re-read versions after the fixture writes, even when effective modes match.
	status, list, err = do("GET", "/api/plugins", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("plugin versions after sequential fixture unavailable")
	}
	plugins = list["plugins"].([]any)
	// Isolate routing checks from unsolicited random chat and AI responses.
	// These overrides affect only this synthetic database. GPT remains enabled.
	for _, item := range plugins {
		p := item.(map[string]any)
		if p["name"] == "aichat" || p["name"] == "chat" || p["name"] == "atri" || p["name"] == "thesaurus" {
			state := p["state"].(map[string]any)
			status, _, err = do("POST", "/api/plugin-switch", map[string]any{"name": p["name"], "groupId": "0", "mode": "disabled", "revision": state["revision"]}, csrf)
			if err != nil || status != 200 {
				return fmt.Errorf("isolated unsolicited-reply disable failed: %v status %d: %v", p["name"], status, err)
			}
		}
	}
	if err := verifyPluginResponseStatus(do, sendControl, sendGroup); err != nil {
		return err
	}
	pluginState := func(group string) (map[string]any, error) {
		status, list, err := do("GET", "/api/plugins?group="+group, nil, "")
		if err != nil || status != 200 || list["controlAvailable"] != true {
			return nil, fmt.Errorf("plugin state read failed")
		}
		items, _ := list["plugins"].([]any)
		for _, item := range items {
			if p, ok := item.(map[string]any); ok && p["name"] == "chrev" {
				state, _ := p["state"].(map[string]any)
				if state != nil {
					return state, nil
				}
			}
		}
		return nil, fmt.Errorf("compiled chrev control missing")
	}
	setPlugin := func(group, mode string) (map[string]any, error) {
		state, err := pluginState(group)
		if err != nil {
			return nil, err
		}
		body := map[string]any{"name": "chrev", "groupId": group, "mode": mode, "revision": state["revision"]}
		status, _, err := do("POST", "/api/plugin-switch", body, "invalid")
		if err != nil || status != 403 {
			return nil, fmt.Errorf("plugin save lacked CSRF protection")
		}
		status, value, err := do("POST", "/api/plugin-switch", body, csrf)
		if err != nil || status != 200 {
			return nil, fmt.Errorf("plugin save failed: %d %v", status, err)
		}
		saved, _ := value["state"].(map[string]any)
		return saved, nil
	}
	if text, err := sendControl(12345678, "/响应"); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("synthetic response permission setup failed: %v", err)
	}
	if _, err := setPlugin("0", "disabled"); err != nil {
		return err
	}
	if text, err := sendControl(12345678, "翻转 abc"); err != nil || text != "" {
		return fmt.Errorf("disabled plugin still replied: %v", err)
	}
	if text, err := sendControl(23456789, "/全局启用 chrev"); err != nil || text != "" {
		return fmt.Errorf("non-owner global command was accepted: %v", err)
	}
	state, err := pluginState("101")
	if err != nil || state["effectiveEnabled"] != false || state["source"] != "global" {
		return fmt.Errorf("disabled global state lost")
	}
	if text, err := sendControl(12345678, "/全局启用 chrev"); err != nil || !strings.Contains(text, "已全局启用") {
		return fmt.Errorf("owner global command failed: %v", err)
	}
	state, err = pluginState("101")
	if err != nil || state["effectiveEnabled"] != true || state["source"] != "global" {
		return fmt.Errorf("chat update not visible to admin")
	}
	if text, err := sendControl(12345678, "翻转 abc"); err != nil || text != "ɔqɐ" {
		return fmt.Errorf("enabled plugin did not reply correctly: %v", err)
	}
	state, err = setPlugin("101", "disabled")
	if err != nil || state["effectiveEnabled"] != true || state["source"] != "global" {
		return fmt.Errorf("group did not respect global override")
	}
	if _, err = setPlugin("0", "inherit"); err != nil {
		return err
	}
	state, err = pluginState("101")
	if err != nil || state["effectiveEnabled"] != false || state["source"] != "group" {
		return fmt.Errorf("inherited global did not reveal group override")
	}
	state, err = pluginState("202")
	if err != nil || state["effectiveEnabled"] != true || state["source"] != "default" {
		return fmt.Errorf("group override leaked")
	}
	if _, err = setPlugin("101", "inherit"); err != nil {
		return err
	}
	status, beforeAIRead, err := do("GET", "/api/backups", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("AI checks backup baseline unavailable")
	}
	status, ai, err := do("GET", "/api/ai?group=101", nil, "")
	if err != nil || status != 200 || ai["available"] != true {
		return fmt.Errorf("AI configuration unavailable")
	}
	if err := verifyAIConfigChecks(ai); err != nil {
		return err
	}
	status, afterAIRead, err := do("GET", "/api/backups", nil, "")
	if err != nil || status != 200 || len(beforeAIRead["backups"].([]any)) != len(afterAIRead["backups"].([]any)) {
		return fmt.Errorf("AI checks read created a backup")
	}
	aiGlobal := ai["global"].(map[string]any)
	aiChange := map[string]any{"revision": aiGlobal["revision"], "fields": map[string]any{"API": modelURL, "ModelName": "smoke-web-model", "Type": 0, "SystemP": "local mock system"}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic-model-key"}}}
	status, _, err = do("POST", "/api/ai", aiChange, "bad")
	if err != nil || status != 403 {
		return fmt.Errorf("AI save lacked CSRF protection")
	}
	status, ai, err = do("POST", "/api/ai", aiChange, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("AI web save failed")
	}
	if err := verifyAIConfigChecks(ai); err != nil || ai["global"].(map[string]any)["checks"].(map[string]any)["chat"].(map[string]any)["complete"] != true {
		return fmt.Errorf("AI checks did not update after saving complete fields: %v", err)
	}
	encodedAI, _ := json.Marshal(ai)
	if bytes.Contains(encodedAI, []byte("synthetic-model-key")) {
		return fmt.Errorf("AI API leaked a key")
	}
	status, _, err = do("POST", "/api/ai", aiChange, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("AI stale web version accepted")
	}
	if text, err := sendControl(12345678, "查看AI聊天配置"); err != nil || !strings.Contains(text, "smoke-web-model") || strings.Contains(text, "synthetic-model-key") {
		return fmt.Errorf("AI web config invisible/secret exposed in chat")
	}
	if text, err := sendControl(12345678, "设置AI聊天模型名smoke-chat-model"); err != nil || !strings.Contains(text, "成功") {
		return fmt.Errorf("AI chat save failed")
	}
	status, ai, err = do("GET", "/api/ai?group=101", nil, "")
	if err != nil || status != 200 || ai["global"].(map[string]any)["fields"].(map[string]any)["ModelName"] != "smoke-chat-model" {
		return fmt.Errorf("AI chat update invisible to web")
	}
	if err := verifyAIConfigChecks(ai); err != nil {
		return err
	}
	for _, item := range ai["groups"].([]any) {
		group := item.(map[string]any)
		change := map[string]any{"service": group["service"], "groupId": "101", "revision": group["revision"], "temperature": 82}
		if group["service"] == "aichatcfg" {
			change["rate"] = 11
			change["useAgent"] = false
			change["voiceOutput"] = false
			change["respondAt"] = true
		}
		status, _, err = do("POST", "/api/ai-group", change, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("AI group save failed")
		}
	}
	if text, err := sendGroup(12345678, 101, "member", "/响应", true); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("AI group response setup failed")
	}
	if text, err := sendGroup(12345678, 101, "member", "设置AI聊天温度80", true); err != nil || !strings.Contains(text, "成功") {
		return fmt.Errorf("AI chat group setting failed")
	}
	status, ai, err = do("GET", "/api/ai?group=101", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("AI group reread failed")
	}
	group := ai["groups"].([]any)[0].(map[string]any)
	if group["temperature"] != float64(80) || group["rate"] != float64(11) {
		return fmt.Errorf("AI group chat/web state differs")
	}
	status, other, err := do("GET", "/api/ai?group=202", nil, "")
	if err != nil || status != 200 || other["groups"].([]any)[0].(map[string]any)["temperature"] != float64(0) {
		return fmt.Errorf("AI group settings leaked to other group")
	}
	status, privateAI, err := do("GET", "/api/ai?private=12345678", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("private AI configuration unavailable")
	}
	var privateChatChange map[string]any
	for _, item := range privateAI["groups"].([]any) {
		conversation := item.(map[string]any)
		if conversation["privateId"] != "12345678" || conversation["groupId"] != "" {
			return fmt.Errorf("private AI target identity is ambiguous")
		}
		change := map[string]any{"service": conversation["service"], "privateId": "12345678", "revision": conversation["revision"], "temperature": 81}
		if conversation["service"] == "aichatcfg" {
			change["rate"], change["useAgent"], change["voiceOutput"], change["respondAt"] = 0, false, false, false
			privateChatChange = change
		}
		status, _, err = do("POST", "/api/ai-group", change, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("private AI save failed")
		}
	}
	if text, err := sendControl(12345678, "设置AI聊天温度76"); err != nil || !strings.Contains(text, "成功") {
		return fmt.Errorf("private AI chat configuration failed")
	}
	status, privateAI, err = do("GET", "/api/ai?private=12345678", nil, "")
	if err != nil || status != 200 || privateAI["groups"].([]any)[0].(map[string]any)["temperature"] != float64(76) || privateAI["groups"].([]any)[1].(map[string]any)["temperature"] != float64(81) {
		return fmt.Errorf("private AI chat/web storage mismatch")
	}
	status, _, err = do("POST", "/api/ai-group", privateChatChange, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("stale private AI update accepted")
	}
	for _, path := range []string{"/api/ai?group=12345678", "/api/ai?private=12345679"} {
		status, isolated, err := do("GET", path, nil, "")
		if err != nil || status != 200 || isolated["groups"].([]any)[0].(map[string]any)["temperature"] != float64(0) {
			return fmt.Errorf("private AI change leaked: %s", path)
		}
	}
	status, _, err = do("GET", "/api/ai?group=12345678&private=12345678", nil, "")
	if err != nil || status != 400 {
		return fmt.Errorf("ambiguous private/group target accepted")
	}
	if text, err := sendGroup(12345678, 101, "member", "/gpt group smoke", false); err != nil || !strings.Contains(text, "本地模型模拟回复") {
		return fmt.Errorf("group GPT request failed: %v %q", err, text)
	}
	if text, err := sendControl(12345678, "/gpt quoted smoke"); err != nil || !strings.Contains(text, "本地模型模拟回复") {
		return fmt.Errorf("private quoted GPT request failed: %v %q", err, text)
	}
	if text, err := sendControl(12345678, "/gpt second"); err != nil || !strings.Contains(text, "30 秒") {
		return fmt.Errorf("GPT request quota not enforced")
	}
	for _, command := range []string{"/gptabc", "普通提及 /gpt"} {
		if text, err := sendControl(12345678, command); err != nil || text != "" {
			return fmt.Errorf("GPT command boundary accepted %q", command)
		}
	}
	if text, err := sendControl(12345678, "/gpt"); err != nil || !strings.Contains(text, "填写问题") {
		return fmt.Errorf("empty GPT did not show usage")
	}
	if err := verifyAIRolePermissions(do, sendControl, sendGroup, csrf, modelURL, inFlight); err != nil {
		return err
	}
	if err := verifyAgentArchive(do, csrf); err != nil {
		return err
	}
	if err := verifyVoiceCommands(do, csrf, sendControl, sendGroup); err != nil {
		return err
	}
	if text, err := sendGroup(12345678, 101, "member", "/沉默", true); err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("AI fixture response cleanup failed")
	}
	status, logs, err := do("GET", "/api/logs?limit=200", nil, "")
	if err != nil || status != 200 || len(logs["entries"].([]any)) == 0 {
		return fmt.Errorf("runtime logs unavailable")
	}
	if err := verifyLogSummary(logs); err != nil {
		return err
	}
	logJSON, _ := json.Marshal(logs)
	if bytes.Contains(logJSON, []byte("synthetic-model-key")) || bytes.Contains(logJSON, []byte("local-smoke-only")) || bytes.Contains(logJSON, []byte(smokeModelFailureText)) || !bytes.Contains(logJSON, []byte("正文已隐藏")) {
		return fmt.Errorf("runtime logs did not protect credentials/message bodies")
	}
	status, updatedOverview, err := do("GET", "/api/overview", nil, "")
	if err != nil || status != 200 || updatedOverview["alerts"] == nil {
		return fmt.Errorf("post-AI overview alerts unavailable")
	}
	stats, ok := updatedOverview["modelRequests"].(map[string]any)
	if !ok || stats["started"] != float64(53) || stats["completed"] != float64(50) || stats["active"] != float64(0) || stats["failed"] != float64(3) || stats["incomplete"] != float64(0) {
		return fmt.Errorf("post-AI model statistics disagree with 53 synthetic requests (two oversized responses and one HTTP 503 expected): started=%v completed=%v active=%v failed=%v incomplete=%v", stats["started"], stats["completed"], stats["active"], stats["failed"], stats["incomplete"])
	}
	rows, ok := stats["categories"].([]any)
	if !ok || len(rows) != 4 {
		return fmt.Errorf("post-AI model categories missing")
	}
	for i, want := range []float64{4, 3, 41, 5} {
		row, ok := rows[i].(map[string]any)
		failed := float64(0)
		if i == 2 {
			failed = 2
		} else if i == 3 {
			failed = 1
		}
		if !ok || row["started"] != want || row["completed"] != want-failed || row["failed"] != failed || row["active"] != float64(0) {
			return fmt.Errorf("post-AI model category differs: %d want=%v started=%v completed=%v", i, want, row["started"], row["completed"])
		}
	}
	statsJSON, _ := json.Marshal(stats)
	for _, secret := range []string{"synthetic-model-key", "smoke-chat-model", modelURL, "quoted smoke", "本地模型模拟回复", smokeModelFailureText} {
		if bytes.Contains(statsJSON, []byte(secret)) {
			return fmt.Errorf("model aggregates retained sensitive request data")
		}
	}
	alertJSON, _ := json.Marshal(updatedOverview["alerts"])
	if bytes.Contains(alertJSON, []byte("synthetic-model-key")) || bytes.Contains(alertJSON, []byte("local-smoke-only")) {
		return fmt.Errorf("overview alerts exposed credentials")
	}
	for _, path := range []string{"/api/logs/export", "/api/logs/export?level=info&q=Websocket"} {
		rsp, err := client.Get(baseURL + path)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(rsp.Body, (3<<20)+1))
		rsp.Body.Close()
		if readErr != nil || rsp.StatusCode != 200 || len(body) > 3<<20 || rsp.Header.Get("Cache-Control") != "no-store" || rsp.Header.Get("Content-Type") != "text/plain; charset=utf-8" || !strings.HasPrefix(rsp.Header.Get("Content-Disposition"), `attachment; filename="zerobot-logs-`) {
			return fmt.Errorf("runtime log export/download headers failed")
		}
		if bytes.Contains(body, []byte("synthetic-model-key")) || bytes.Contains(body, []byte("local-smoke-only")) || !bytes.Contains(body, []byte("[INFO]")) {
			return fmt.Errorf("runtime export redaction failed")
		}
		if path == "/api/logs/export" && !bytes.Contains(body, []byte("正文已隐藏")) {
			return fmt.Errorf("export exposed message bodies")
		}
		if path != "/api/logs/export" {
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "[") && (!strings.Contains(line, "[INFO]") || !strings.Contains(line, "Websocket")) {
					return fmt.Errorf("export filter mismatch")
				}
			}
		}
	}
	status, filtered, err := do("GET", "/api/logs?level=info&q=Websocket", nil, "")
	if err != nil || status != 200 || len(filtered["entries"].([]any)) == 0 {
		return fmt.Errorf("runtime log filters failed")
	}
	if err := verifyLogSummary(filtered); err != nil {
		return err
	}
	for _, item := range filtered["entries"].([]any) {
		row := item.(map[string]any)
		if row["level"] != "info" || !strings.Contains(row["message"].(string), "Websocket") {
			return fmt.Errorf("runtime log filter mismatch")
		}
	}
	// Exercise inclusive time filtering and export against actual process timestamps.
	timeRow := filtered["entries"].([]any)[0].(map[string]any)
	timeQuery := url.Values{"from": {timeRow["time"].(string)}, "until": {timeRow["time"].(string)}, "level": {"info"}, "q": {"Websocket"}}.Encode()
	status, timed, err := do("GET", "/api/logs?"+timeQuery, nil, "")
	if err != nil || status != 200 || len(timed["entries"].([]any)) == 0 {
		return fmt.Errorf("runtime time range failed")
	}
	if err := verifyLogSummary(timed); err != nil {
		return err
	}
	status, timedTail, err := do("GET", "/api/logs?"+timeQuery+"&after="+timed["cursor"].(string)+"&instance="+timed["instance"].(string)+"&limit=1", nil, "")
	if err != nil || status != 200 || len(timedTail["entries"].([]any)) != 0 {
		return fmt.Errorf("log summary incremental time fixture failed")
	}
	if err := verifyLogSummary(timedTail); err != nil {
		return err
	}
	beforeSummary, _ := json.Marshal(timed["summary"])
	afterSummary, _ := json.Marshal(timedTail["summary"])
	if !bytes.Equal(beforeSummary, afterSummary) {
		return fmt.Errorf("log summary depended on incremental cursor/limit")
	}
	status, hiddenSummary, err := do("GET", "/api/logs?q=synthetic-model-key", nil, "")
	if err != nil || status != 200 || hiddenSummary["summary"].(map[string]any)["matched"] != float64(0) {
		return fmt.Errorf("log summary inferred a raw model key")
	}
	for _, item := range timed["entries"].([]any) {
		if item.(map[string]any)["time"] != timeRow["time"] {
			return fmt.Errorf("runtime time range boundary mismatch")
		}
	}
	timeResponse, err := client.Get(baseURL + "/api/logs/export?" + timeQuery)
	if err != nil {
		return err
	}
	timeBody, timeErr := io.ReadAll(io.LimitReader(timeResponse.Body, (3<<20)+1))
	timeResponse.Body.Close()
	if timeErr != nil || timeResponse.StatusCode != 200 || !bytes.Contains(timeBody, []byte("时间范围（包含边界，UTC）")) || !bytes.Contains(timeBody, []byte("#"+timeRow["id"].(string)+" ")) {
		return fmt.Errorf("runtime time export failed")
	}
	for _, endpoint := range []string{"/api/logs", "/api/logs/export"} {
		status, _, err = do("GET", endpoint+"?from=bad-time", nil, "")
		if err != nil || status != 400 {
			return fmt.Errorf("runtime invalid time not rejected")
		}
		status, _, err = do("GET", endpoint+"?from=2026-10-09T08:00:01Z&until=2026-10-09T08:00:00Z", nil, "")
		if err != nil || status != 400 {
			return fmt.Errorf("runtime reversed time not rejected")
		}
	}
	status, issues, err := do("GET", "/api/logs?level=issues", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("runtime issues filter unavailable")
	}
	if err := verifyLogSummary(issues); err != nil {
		return err
	}
	for _, item := range issues["entries"].([]any) {
		row := item.(map[string]any)
		switch row["level"] {
		case "warning", "error", "fatal", "panic":
		default:
			return fmt.Errorf("runtime issues filter included a normal level")
		}
	}
	rsp, err := client.Get(baseURL + "/api/logs/export?level=issues")
	if err != nil {
		return err
	}
	issueBody, readIssueErr := io.ReadAll(io.LimitReader(rsp.Body, (3<<20)+1))
	rsp.Body.Close()
	if readIssueErr != nil || rsp.StatusCode != 200 || rsp.Header.Get("Content-Type") != "text/plain; charset=utf-8" || rsp.Header.Get("Cache-Control") != "no-store" || bytes.Contains(issueBody, []byte("synthetic-model-key")) || bytes.Contains(issueBody, []byte("local-smoke-only")) {
		return fmt.Errorf("runtime issues export failed")
	}
	for _, line := range strings.Split(string(issueBody), "\n") {
		if strings.HasPrefix(line, "[") && !strings.Contains(line, "[WARNING]") && !strings.Contains(line, "[ERROR]") && !strings.Contains(line, "[FATAL]") && !strings.Contains(line, "[PANIC]") {
			return fmt.Errorf("runtime issues export included a normal level")
		}
	}
	status, tail, err := do("GET", "/api/logs?after="+logs["cursor"].(string)+"&instance="+logs["instance"].(string), nil, "")
	if err != nil || status != 200 || tail["reset"] != false || tail["capacity"] != float64(1000) {
		return fmt.Errorf("runtime log cursor failed")
	}
	if err := verifyLogSummary(tail); err != nil {
		return err
	}
	status, settings, err := do("GET", "/api/connections", nil, "")
	if err != nil || status != 200 || settings["available"] != true || settings["restartRequired"] != false {
		return fmt.Errorf("isolated connection settings read failed")
	}
	if err := verifyStartupSummary(settings, "正向 OneBot 连接", 1, 1, false); err != nil {
		return err
	}
	settingsJSON, _ := json.Marshal(settings)
	if bytes.Contains(settingsJSON, []byte("local-smoke-only")) {
		return fmt.Errorf("connection settings leaked token")
	}
	revision, _ := settings["revision"].(string)
	change := map[string]any{"revision": revision, "superUsers": []string{"12345678", "23456789"}}
	status, _, err = do("POST", "/api/connections", change, "invalid")
	if err != nil || status != 403 {
		return fmt.Errorf("connection save lacked CSRF protection")
	}
	status, saved, err := do("POST", "/api/connections", change, csrf)
	if err != nil || status != 200 || saved["restartRequired"] != true || saved["backupId"] == "" {
		return fmt.Errorf("isolated connection save/backup failed")
	}
	if err := verifyStartupSummary(saved, "机器人主人", 1, 2, true); err != nil {
		return err
	}
	status, _, err = do("POST", "/api/connections", change, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("stale connection save was accepted")
	}
	// Restore a synthetic startup backup and confirm running configuration stays intact.
	restore := func(id string) error {
		status, preview, err := do("POST", "/api/backups/preview", map[string]string{"id": id}, csrf)
		if err != nil || status != 200 || preview["changed"] != true {
			return fmt.Errorf("restore preview failed: %d %v", status, err)
		}
		body := map[string]any{"id": id, "revision": preview["revision"], "expiresAt": preview["expiresAt"], "token": preview["token"], "confirm": true}
		status, _, err = do("POST", "/api/backups/restore", body, "invalid")
		if err != nil || status != 403 {
			return fmt.Errorf("restore lacked CSRF protection")
		}
		status, result, err := do("POST", "/api/backups/restore", body, csrf)
		if err != nil || status != 200 || result["changed"] != true || result["backupId"] == "" {
			return fmt.Errorf("restore commit failed: %d %v", status, err)
		}
		status, _, err = do("POST", "/api/backups/restore", body, csrf)
		if err != nil || status != 409 {
			return fmt.Errorf("stale restore ticket accepted")
		}
		return nil
	}
	if err = restore(saved["backupId"].(string)); err != nil {
		return err
	}
	status, settings, err = do("GET", "/api/connections", nil, "")
	if err != nil || status != 200 || settings["restartRequired"] != false || len(settings["superUsers"].([]any)) != 1 {
		return fmt.Errorf("startup restore did not recover saved config")
	}
	// Detect a same-count Token-only change without altering the running driver.
	if err := verifyStartupSummary(settings, "机器人主人", 1, 1, false); err != nil {
		return err
	}
	secretChange := map[string]any{"revision": settings["revision"], "connection": map[string]any{"kind": "ws", "index": 0, "tokenAction": "replace", "token": "startup-summary-only-private"}}
	status, secretSaved, err := do("POST", "/api/connections", secretChange, csrf)
	if err != nil || status != 200 || secretSaved["restartRequired"] != true {
		return fmt.Errorf("startup summary secret-only save failed")
	}
	if err := verifyStartupSummary(secretSaved, "正向 OneBot 连接", 1, 1, true); err != nil {
		return err
	}
	secretJSON, _ := json.Marshal(secretSaved)
	if bytes.Contains(secretJSON, []byte("startup-summary-only-private")) {
		return fmt.Errorf("startup summary leaked secret")
	}
	if err = restore(secretSaved["backupId"].(string)); err != nil {
		return err
	}
	status, settings, err = do("GET", "/api/connections", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("startup summary restore read failed")
	}
	if err := verifyStartupSummary(settings, "正向 OneBot 连接", 1, 1, false); err != nil {
		return err
	}
	// Add only an isolated forward driver, then restore the pre-addition fixture.
	addition := map[string]any{"revision": settings["revision"], "addConnection": map[string]any{"url": "ws://127.0.0.1:17777/new", "token": "synthetic-addition-key", "withoutToken": false}}
	status, _, err = do("POST", "/api/connections", addition, "invalid")
	if err != nil || status != 403 {
		return fmt.Errorf("addition lacked CSRF protection")
	}
	status, added, err := do("POST", "/api/connections", addition, csrf)
	if err != nil || status != 200 || added["restartRequired"] != true || added["backupId"] == "" || len(added["connections"].([]any)) != 2 || len(added["runningConnections"].([]any)) != 1 {
		return fmt.Errorf("isolated addition failed: %d %v", status, err)
	}
	if err := verifyStartupSummary(added, "正向 OneBot 连接", 1, 2, true); err != nil {
		return err
	}
	addedJSON, _ := json.Marshal(added)
	if bytes.Contains(addedJSON, []byte("synthetic-addition-key")) {
		return fmt.Errorf("addition exposed token")
	}
	status, _, err = do("POST", "/api/connections", addition, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("stale addition accepted")
	}
	duplicate := map[string]any{"revision": added["revision"], "addConnection": map[string]any{"url": "ws://127.0.0.1:17777/new", "withoutToken": true}}
	status, _, err = do("POST", "/api/connections", duplicate, csrf)
	if err != nil || status != 400 {
		return fmt.Errorf("duplicate addition accepted")
	}
	// Remove only the added synthetic driver, preserving the active fake connection.
	removal := map[string]any{"revision": added["revision"], "removeConnection": map[string]any{"kind": "ws", "index": 1, "confirm": true}}
	status, _, err = do("POST", "/api/connections", removal, "invalid")
	if err != nil || status != 403 {
		return fmt.Errorf("removal lacked CSRF protection")
	}
	unconfirmed := map[string]any{"revision": added["revision"], "removeConnection": map[string]any{"kind": "ws", "index": 1, "confirm": false}}
	status, _, err = do("POST", "/api/connections", unconfirmed, csrf)
	if err != nil || status != 400 {
		return fmt.Errorf("removal accepted without confirmation")
	}
	status, removed, err := do("POST", "/api/connections", removal, csrf)
	if err != nil || status != 200 || removed["backupId"] == "" || removed["restartRequired"] != false || len(removed["connections"].([]any)) != 1 || len(removed["runningConnections"].([]any)) != 1 {
		return fmt.Errorf("isolated removal failed: %d %v", status, err)
	}
	if err := verifyStartupSummary(removed, "正向 OneBot 连接", 1, 1, false); err != nil {
		return err
	}
	status, _, err = do("POST", "/api/connections", removal, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("stale removal accepted")
	}
	last := map[string]any{"revision": removed["revision"], "removeConnection": map[string]any{"kind": "ws", "index": 0, "confirm": true}}
	status, _, err = do("POST", "/api/connections", last, csrf)
	if err != nil || status != 400 {
		return fmt.Errorf("last connection removal accepted")
	}
	if err = restore(removed["backupId"].(string)); err != nil {
		return err
	}
	if err = restore(added["backupId"].(string)); err != nil {
		return err
	}
	status, settings, err = do("GET", "/api/connections", nil, "")
	if err != nil || status != 200 || settings["restartRequired"] != false || len(settings["connections"].([]any)) != 1 {
		return fmt.Errorf("addition restore did not preserve running fixture")
	}
	// Reverse addition is saved only in this run's startup file; no listener starts here.
	backendURL, _ := url.Parse(baseURL)
	blockedReverse := map[string]any{"revision": settings["revision"], "addConnection": map[string]any{"kind": "wss", "url": "ws://" + backendURL.Host, "token": "synthetic-reverse-api-key"}}
	status, _, err = do("POST", "/api/connections", blockedReverse, csrf)
	if err != nil || status != 400 {
		return fmt.Errorf("reverse backend port conflict accepted")
	}
	reverseAdd := map[string]any{"revision": settings["revision"], "addConnection": map[string]any{"kind": "wss", "url": "ws://127.0.0.1:17778", "token": "synthetic-reverse-api-key"}}
	status, reverseSaved, err := do("POST", "/api/connections", reverseAdd, csrf)
	if err != nil || status != 200 || reverseSaved["backupId"] == "" || reverseSaved["restartRequired"] != true || len(reverseSaved["connections"].([]any)) != 2 || len(reverseSaved["runningConnections"].([]any)) != 1 {
		return fmt.Errorf("reverse addition API failed: %d %v", status, err)
	}
	if err := verifyStartupSummary(reverseSaved, "反向 OneBot 连接", 0, 1, true); err != nil {
		return err
	}
	reverseJSON, _ := json.Marshal(reverseSaved)
	if bytes.Contains(reverseJSON, []byte("synthetic-reverse-api-key")) {
		return fmt.Errorf("reverse addition leaked token")
	}
	if err = restore(reverseSaved["backupId"].(string)); err != nil {
		return err
	}
	status, manual, err := do("POST", "/api/backups", map[string]string{"scope": "control-db"}, csrf)
	if err != nil || status != 200 || manual["backupId"] == "" {
		return fmt.Errorf("manual control snapshot failed")
	}
	if _, err = setPlugin("0", "disabled"); err != nil {
		return err
	}
	status, ai, err = do("GET", "/api/ai", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("AI restore fixture read failed")
	}
	currentModel := ai["global"].(map[string]any)["fields"].(map[string]any)["ModelName"]
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": ai["global"].(map[string]any)["revision"], "fields": map[string]any{"ModelName": "restore-temporary-model"}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("AI restore fixture write failed")
	}
	if err = restore(manual["backupId"].(string)); err != nil {
		return err
	}
	state, err = pluginState("101")
	if err != nil || state["effectiveEnabled"] != true {
		return fmt.Errorf("control restore left stale plugin cache")
	}
	status, ai, err = do("GET", "/api/ai", nil, "")
	if err != nil || status != 200 || ai["global"].(map[string]any)["fields"].(map[string]any)["ModelName"] != currentModel {
		return fmt.Errorf("control restore did not recover AI settings")
	}
	if text, err := sendControl(12345678, "翻转 abc"); err != nil || text != "ɔqɐ" {
		return fmt.Errorf("restored plugin did not reply: %v", err)
	}
	status, backups, err := do("GET", "/api/backups", nil, "")
	if err != nil || status != 200 || len(backups["backups"].([]any)) < 3 {
		return fmt.Errorf("backup list unavailable")
	}
	backupJSON, _ := json.Marshal(backups)
	if bytes.Contains(backupJSON, []byte("synthetic-model-key")) || bytes.Contains(backupJSON, []byte("local-smoke-only")) {
		return fmt.Errorf("backup metadata leaked credentials")
	}
	// Age only snapshots created inside this run's isolated backup directory.
	// Preserve nested RawMessage values and large integers/checksums.
	for i := 0; i < 4; i++ {
		status, created, err := do("POST", "/api/backups", map[string]string{"scope": "startup-config"}, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("cleanup fixture creation failed")
		}
		id, ok := created["backupId"].(string)
		if !ok || filepath.Base(id) != id || !strings.HasSuffix(id, ".json") {
			return fmt.Errorf("cleanup fixture ID invalid")
		}
		path := filepath.Join(dataDir, "backups", id)
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return fmt.Errorf("cleanup fixture invalid")
		}
		fields["createdAt"], _ = json.Marshal(time.Now().UTC().AddDate(0, 0, -30-i))
		raw, _ = json.Marshal(fields)
		if os.WriteFile(path, raw, 0600) != nil {
			return fmt.Errorf("cleanup fixture aging failed")
		}
	}
	policy := map[string]int{"keepPerScope": 3, "keepDays": 7}
	status, preview, err := do("POST", "/api/backups/cleanup-preview", policy, csrf)
	if err != nil || status != 200 || len(preview["candidates"].([]any)) == 0 {
		return fmt.Errorf("cleanup preview did not select old fixtures")
	}
	cleanupBody := map[string]any{"policy": preview["policy"], "revision": preview["revision"], "asOf": preview["asOf"], "expiresAt": preview["expiresAt"], "token": preview["token"], "confirm": false}
	status, _, err = do("POST", "/api/backups/cleanup", cleanupBody, csrf)
	if err != nil || status != 400 {
		return fmt.Errorf("cleanup lacked explicit confirmation")
	}
	cleanupBody["confirm"] = true
	status, _, err = do("POST", "/api/backups/cleanup", cleanupBody, "invalid")
	if err != nil || status != 403 {
		return fmt.Errorf("cleanup lacked CSRF protection")
	}
	startupPath := filepath.Join(filepath.Dir(dataDir), "config.json")
	startupBefore, err := os.ReadFile(startupPath)
	if err != nil {
		return err
	}
	status, cleaned, err := do("POST", "/api/backups/cleanup", cleanupBody, csrf)
	if err != nil || status != 200 || cleaned["partial"] != false || len(cleaned["deleted"].([]any)) != len(preview["candidates"].([]any)) {
		return fmt.Errorf("isolated cleanup failed: %d %v", status, err)
	}
	startupAfter, err := os.ReadFile(startupPath)
	if err != nil || !bytes.Equal(startupBefore, startupAfter) {
		return fmt.Errorf("cleanup changed current startup configuration")
	}
	status, _, err = do("POST", "/api/backups/cleanup", cleanupBody, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("cleanup replay was accepted")
	}
	status, remaining, err := do("GET", "/api/backups", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("cleanup list refresh failed")
	}
	validStartup := 0
	for _, entry := range remaining["backups"].([]any) {
		b := entry.(map[string]any)
		if b["scope"] == "startup-config" && b["valid"] == true {
			validStartup++
		}
	}
	if validStartup < 3 {
		return fmt.Errorf("cleanup lost protected backups")
	}
	if err = verifyPolicyCommands(dataDir, sendControl, sendGroup); err != nil {
		return err
	}
	// Password changes operate only on this run's synthetic credential directory.
	const originalPassword, replacementPassword = "Local-ui-test-password-only", "Local-ui-changed-password-only"
	passwordBody := func(current, next string) map[string]string {
		return map[string]string{"currentPassword": current, "newPassword": next, "confirmPassword": next}
	}
	primaryClient := client
	secondJar, _ := cookiejar.New(nil)
	client = &http.Client{Jar: secondJar, Timeout: 15 * time.Second}
	status, secondSession, err := do("POST", "/api/login", map[string]string{"password": originalPassword}, "")
	if err != nil || status != 200 {
		return fmt.Errorf("second synthetic admin login failed")
	}
	secondCSRF := secondSession["csrfToken"].(string)
	secondClient := client
	client = primaryClient
	status, _, err = do("POST", "/api/password", passwordBody("wrong", replacementPassword), csrf)
	if err != nil || status != 403 {
		return fmt.Errorf("wrong current password accepted")
	}
	status, _, err = do("GET", "/api/overview", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("wrong password killed current session")
	}
	status, _, err = do("POST", "/api/password", passwordBody(originalPassword, replacementPassword), "invalid-csrf")
	if err != nil || status != 403 {
		return fmt.Errorf("password change skipped CSRF")
	}
	status, changed, err := do("POST", "/api/password", passwordBody(originalPassword, replacementPassword), csrf)
	if err != nil || status != 200 || changed["passwordChanged"] != true || changed["authenticated"] != false {
		return fmt.Errorf("password change failed")
	}
	status, _, err = do("GET", "/api/overview", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("primary password session remained active")
	}
	client = secondClient
	status, _, err = do("POST", "/api/password", passwordBody(originalPassword, "another-test-password-only"), secondCSRF)
	if err != nil || status != 401 {
		return fmt.Errorf("secondary old password session remained active")
	}
	client = primaryClient
	status, _, err = do("POST", "/api/login", map[string]string{"password": originalPassword}, "")
	if err != nil || status != 401 {
		return fmt.Errorf("old password login remained valid")
	}
	status, changedSession, err := do("POST", "/api/login", map[string]string{"password": replacementPassword}, "")
	if err != nil || status != 200 {
		return fmt.Errorf("new password login failed")
	}
	csrf = changedSession["csrfToken"].(string)
	credentialBytes, err := os.ReadFile(filepath.Join(dataDir, "credential.json"))
	if err != nil || bytes.Contains(credentialBytes, []byte(originalPassword)) || bytes.Contains(credentialBytes, []byte(replacementPassword)) {
		return fmt.Errorf("password credential persistence wrong")
	}
	status, _, err = do("POST", "/api/password", passwordBody(replacementPassword, originalPassword), csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("synthetic password restore failed")
	}
	status, restoredSession, err := do("POST", "/api/login", map[string]string{"password": originalPassword}, "")
	if err != nil || status != 200 {
		return fmt.Errorf("restored synthetic password login failed")
	}
	csrf = restoredSession["csrfToken"].(string)
	status, _, err = do("POST", "/api/logout", map[string]any{}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("isolated logout failed")
	}
	status, _, err = do("GET", "/api/overview", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("logout left admin access enabled")
	}
	status, _, err = do("GET", "/api/logs", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("logout left log access enabled")
	}
	status, _, err = do("GET", "/api/logs/export", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("logout left log export enabled")
	}
	status, _, err = do("GET", "/api/backups", nil, "")
	if err != nil || status != 401 {
		return fmt.Errorf("logout left backup access enabled")
	}
	for _, path := range []string{"/api/backups/cleanup-preview", "/api/backups/cleanup"} {
		status, _, err = do("POST", path, map[string]any{}, csrf)
		if err != nil || status != 401 {
			return fmt.Errorf("logout left cleanup access enabled")
		}
	}
	return nil
}

// Failure injection affects only this run's synthetic admin backup directory.
// Check snapshots, truthful command feedback, and later message routing together.
func verifyVoiceCommands(do func(string, string, any, string) (int, map[string]any, error), csrf string, private func(int64, string) (string, error), group func(int64, int64, string, string, bool) (string, error)) error {
	if text, err := private(12345678, "发送AI语音preconfig"); err != nil || !strings.Contains(text, "未选择") {
		return fmt.Errorf("unconfigured voice did not stop: %q %v", text, err)
	}
	for _, c := range []struct{ cmd, want string }{
		{"设置AI语音群号0", "正数"},
		{"设置AI语音群号303", "为303"},
		{"设置AI语音模型", "请选择语音模型序号"},
		{"0", "已选择语音模型: Synthetic voice"},
		{"查看AI语音配置", "synthetic-voice-1"},
		{"发送AI语音   ", "填写"},
		{"发送AI语音private-ok", "仅支持群聊"},
		{"发送AI语音private-failure", "仅支持群聊"},
	} {
		text, err := private(12345678, c.cmd)
		if err != nil || !strings.Contains(text, c.want) {
			return fmt.Errorf("voice private command %s failed: %q %v", c.cmd, text, err)
		}
	}
	if text, err := group(12345678, 303, "member", "/响应", true); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("voice group setup failed")
	}
	for _, c := range []struct{ role, cmd, want string }{
		{"member", "发送AI语音group-ok", ""},
		{"admin", "发送AI语音group-ok", "VOICE_OK"},
		{"owner", "发送AI语音group-zero-id", "VOICE_OK"},
		{"admin", "发送AI语音group-failure", "未自动重试"},
	} {
		text, err := group(23456789, 303, c.role, c.cmd, true)
		if err != nil || (c.want == "" && text != "") || (c.want != "" && !strings.Contains(text, c.want)) {
			return fmt.Errorf("voice group command %s failed: %q %v", c.cmd, text, err)
		}
	}
	if text, err := group(12345678, 404, "member", "/响应", true); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("second voice group setup failed")
	}
	if text, err := group(12345678, 404, "member", "发送AI语音current-group", true); err != nil || text != "VOICE_OK" {
		return fmt.Errorf("voice escaped current group: %q %v", text, err)
	}
	if err := verifyChatVoiceScope(do, csrf, private, group); err != nil {
		return err
	}
	for _, gid := range []int64{303, 404} {
		if text, err := group(12345678, gid, "member", "/沉默", true); err != nil || !strings.Contains(text, "休息") {
			return fmt.Errorf("voice group cleanup failed")
		}
	}
	return nil
}

func verifyChatVoiceScope(do func(string, string, any, string) (int, map[string]any, error), csrf string, private func(int64, string) (string, error), group func(int64, int64, string, string, bool) (string, error)) error {
	status, snapshot, err := do("GET", "/api/ai", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("voice model snapshot failed")
	}
	global := snapshot["global"].(map[string]any)
	oldModel := global["fields"].(map[string]any)["ModelName"]
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": global["revision"], "fields": map[string]any{"ModelName": "smoke-voice-chat"}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("voice model fixture setup failed")
	}
	switchChat := func(mode string) error {
		status, list, err := do("GET", "/api/plugins", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("voice chat plugin read failed")
		}
		for _, item := range list["plugins"].([]any) {
			p := item.(map[string]any)
			if p["name"] != "aichat" {
				continue
			}
			state := p["state"].(map[string]any)
			status, _, err = do("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": mode, "revision": state["revision"]}, csrf)
			if err != nil || status != 200 {
				return fmt.Errorf("voice chat plugin switch failed")
			}
			return nil
		}
		return fmt.Errorf("voice chat plugin missing")
	}
	for _, target := range []string{"group=404", "private=12345678"} {
		status, value, err := do("GET", "/api/ai?"+target, nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("voice chat settings read failed")
		}
		for _, item := range value["groups"].([]any) {
			setting := item.(map[string]any)
			if setting["service"] != "aichatcfg" {
				continue
			}
			change := map[string]any{"service": "aichatcfg", "revision": setting["revision"], "rate": 0, "temperature": 50, "useAgent": false, "respondAt": true, "voiceOutput": true}
			if strings.HasPrefix(target, "group=") {
				change["groupId"] = "404"
			} else {
				change["privateId"] = "12345678"
			}
			status, _, err = do("POST", "/api/ai-group", change, csrf)
			if err != nil || status != 200 {
				return fmt.Errorf("voice chat settings write failed")
			}
		}
	}
	if text, err := private(12345678, "/全局响应"); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("voice chat response setup failed")
	}
	if err := switchChat("enabled"); err != nil {
		return err
	}
	if text, err := group(12345678, 404, "member", "VOICE_CHAT_GROUP", true); err != nil || text != "VOICE_OK" {
		return fmt.Errorf("ordinary group chat voice failed: %q %v", text, err)
	}
	if text, err := private(12345678, "VOICE_CHAT_PRIVATE"); err != nil || text != "VOICE_TEXT_OK" {
		return fmt.Errorf("ordinary private chat did not preserve text: %q %v", text, err)
	}
	if err := switchChat("disabled"); err != nil {
		return err
	}
	if text, err := private(12345678, "/全局沉默"); err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("voice chat response cleanup failed")
	}
	status, snapshot, err = do("GET", "/api/ai", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("voice model cleanup snapshot failed")
	}
	global = snapshot["global"].(map[string]any)
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": global["revision"], "fields": map[string]any{"ModelName": oldModel}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("voice model fixture cleanup failed")
	}
	return nil
}

// The fake model consumes actual serialized local responses, so an expected
// final message cannot hide a missing query result or a fabricated source ID.
func smokeVisionReply(body map[string]any) (string, error) {
	if body["model"] == "smoke-vision-model" || body["model"] == "smoke-prepare-vision" || body["model"] == "smoke-description-vision" {
		messages, _ := body["messages"].([]any)
		images := 0
		for _, raw := range messages {
			m, _ := raw.(map[string]any)
			content, _ := m["content"].([]any)
			for _, raw := range content {
				c, _ := raw.(map[string]any)
				if c["type"] != "image_url" {
					continue
				}
				images++
				u, _ := c["image_url"].(map[string]any)
				source, _ := u["url"].(string)
				mime, encoded, ok := strings.Cut(source, ";base64,")
				if !ok || (mime != "data:image/png" && mime != "data:image/jpeg") || len(encoded) > 1<<20 {
					return "", fmt.Errorf("vision did not receive a bounded embedded image")
				}
				data, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					return "", fmt.Errorf("vision received invalid image data")
				}
				cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
				if err != nil || "data:image/"+format != mime || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 1024 || cfg.Height > 1024 {
					return "", fmt.Errorf("vision received invalid image dimensions")
				}
			}
		}
		if images != 1 {
			return "", fmt.Errorf("vision request image count differs: %d", images)
		}
		if body["model"] == "smoke-vision-model" {
			return strings.Repeat("字", 1500) + "VISION_UNSEEN_TAIL", nil
		}
		return "SYNTHETIC_VISION_DESCRIPTION", nil
	}
	if body["model"] != "smoke-vision-agent" {
		return "", fmt.Errorf("unexpected vision fixture model")
	}
	records := smokeModelRecords(body)
	for i := len(records) - 1; i >= 0; i-- {
		if records[i]["post_type"] != "message" {
			continue
		}
		segments, _ := records[i]["message"].([]any)
		images := 0
		described := 0
		trigger := false
		for _, raw := range segments {
			s, _ := raw.(map[string]any)
			data, _ := s["data"].(map[string]any)
			if s["type"] == "text" && data["text"] == "VISION_IMAGE_CHECK" {
				trigger = true
			}
			if s["type"] != "image" {
				continue
			}
			images++
			source, _ := data["file"].(string)
			if strings.HasSuffix(source, "/agent-image-ok") {
				if data["__agent_desc__"] != strings.Repeat("字", 1333) || data["__agent_desc_truncated__"] != "true" {
					return "", fmt.Errorf("valid image description missing from Agent context")
				}
				described++
			} else if _, exists := data["__agent_desc__"]; exists {
				return "", fmt.Errorf("invalid image was described")
			}
		}
		if !trigger || images != 4 || described != 1 {
			return "", fmt.Errorf("current vision trigger or bounded image selection missing")
		}
		return `{"action":"send_group_msg","params":{"group_id":404,"message":"VISION_IMAGE_OK"}}` + "\n" + `{"action":"end_action","params":{}}`, nil
	}
	return "", fmt.Errorf("vision event missing")
}

func smokeDescriptionReply(body map[string]any, round int) (string, error) {
	if body["model"] == "smoke-description-vision" {
		if _, err := smokeVisionReply(body); err != nil {
			return "", err
		}
		switch round {
		case 1:
			return " \n\t ", nil
		case 2:
			return strings.Repeat("X", (256<<10)+1) + "OVERSIZED_VISION_TAIL", nil
		case 3:
			return "RECOVERED_IMAGE_DESCRIPTION", nil
		default:
			return "", fmt.Errorf("description was not cached after recovery")
		}
	}
	if body["model"] != "smoke-description-agent" || (round != 1 && round != 2) {
		return "", fmt.Errorf("unexpected description model or round")
	}
	records := smokeModelRecords(body)
	for i := len(records) - 1; i >= 0; i-- {
		if records[i]["post_type"] != "message" {
			continue
		}
		segments, _ := records[i]["message"].([]any)
		seen := map[string]bool{}
		trigger := false
		for _, raw := range segments {
			s, _ := raw.(map[string]any)
			data, _ := s["data"].(map[string]any)
			if s["type"] == "text" && data["text"] == "VISION_DESCRIPTION_CHECK" {
				trigger = true
			}
			if s["type"] != "image" {
				continue
			}
			file, _ := data["file"].(string)
			_, name, ok := strings.Cut(file, "/description-image-")
			if !ok || (name != "a" && name != "b" && name != "c") || seen[name] {
				return "", fmt.Errorf("description fixture image mismatch")
			}
			seen[name] = true
			if round == 1 && name != "c" {
				if _, ok := data["__agent_desc__"]; ok {
					return "", fmt.Errorf("blank or oversized description attached")
				}
			} else if data["__agent_desc__"] != "RECOVERED_IMAGE_DESCRIPTION" {
				return "", fmt.Errorf("recovered description missing")
			}
			if _, ok := data["__agent_desc_truncated__"]; ok {
				return "", fmt.Errorf("invalid or normal description has stale truncation flag")
			}
		}
		if !trigger || len(seen) != 3 {
			return "", fmt.Errorf("description event missing")
		}
		return `{"action":"send_group_msg","params":{"group_id":607,"message":"VISION_DESCRIPTION_OK"}}` + "\n" + `{"action":"end_action","params":{}}`, nil
	}
	return "", fmt.Errorf("description event unavailable")
}

func verifyVisionDescriptionRuntime(do func(string, string, any, string) (int, map[string]any, error), send func(int64, int64, string, string, bool) (string, error), csrf string, configure func(string, bool) error) error {
	status, snapshot, err := do("GET", "/api/ai", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("description config read failed")
	}
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": snapshot["global"].(map[string]any)["revision"], "fields": map[string]any{"AgentModelName": "smoke-description-agent", "ImageModelName": "smoke-description-vision"}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("description model setup failed")
	}
	if err = configure("group=607", true); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		text, err := send(23456789, 607, "member", "VISION_DESCRIPTION_CHECK", true)
		if err != nil || text != "VISION_DESCRIPTION_OK" {
			return fmt.Errorf("description recovery flow failed: %q %v", text, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, page, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=607&state=completed", nil, "")
	rows, _ := page["items"].([]any)
	if err != nil || status != 200 || len(rows) != 2 {
		return fmt.Errorf("description tasks not completed")
	}
	for _, raw := range rows {
		v, _ := raw.(map[string]any)
		a, _ := v["activity"].(map[string]any)
		if !smokeTaskTimingValid(v) || !smokeTaskResultsMatch(v, 1, 0, 0, 0) || !smokeTaskQueriesMatch(v, 0, 0, 0) || a["modelPreparations"] != float64(1) || a["localActions"] != float64(0) || a["platformActions"] != float64(1) {
			return fmt.Errorf("description task summary inconsistent")
		}
	}
	return configure("group=607", false)
}

const smokeModelFailureText = "SYNTHETIC_PROVIDER_FAILURE_PRIVATE"

func smokeModelReplyEnvelope(name, text string) map[string]any {
	var content any = text
	if name == "smoke-output-limit-ok" || name == "smoke-vision-model" || name == "smoke-description-vision" {
		runes := []rune(text)
		cut := len(string(runes[:len(runes)/2])) // Preserve UTF-8 at the text-part boundary.
		content = []any{map[string]any{"type": "text", "text": text[:cut]}, map[string]any{"type": "text", "text": text[cut:]}}
	}
	return map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}}
}

func smokeModelRequestFailure(kind string) (int, string, bool) {
	switch kind {
	case "request-http":
		return http.StatusServiceUnavailable, smokeModelFailureText, true
	case "request-protocol":
		return http.StatusOK, smokeModelFailureText, true
	case "request-empty":
		return http.StatusOK, "", true
	case "request-missing":
		body, _ := json.Marshal(map[string]any{"error": map[string]any{"message": smokeModelFailureText}})
		return http.StatusOK, string(body), true
	case "request-unsupported":
		parts := []any{map[string]any{"type": "text", "text": `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}`}, map[string]any{"type": "private-unknown-type", "text": smokeModelFailureText}}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": parts}}}})
		return http.StatusOK, string(body), true
	default:
		return 0, "", false
	}
}

func smokeActionOutputReply(kind string) (string, error) {
	if kind == "ok" {
		return `{"action":"send_group_msg","params":{"group_id":608,"message":[{"type":"text","data":{"text":"OUTPUT_LIMIT_"}},{"type":"text","data":{"text":"RECOVERY_OK"}}]},"extra":` + strings.Repeat("[", 63) + "null" + strings.Repeat("]", 63) + `}` + "\n" + `{"action":"end_action","params":{}}`, nil
	}
	if kind == "http" {
		return strings.Repeat("H", (2<<20)+1), nil
	}
	prefix := `{"action":"save_memory","params":{"text":"LIMIT_BATCH_MUST_NOT_SAVE"}}`
	if kind == "structure-depth" {
		return prefix + `{"action":"send_group_msg","params":{"group_id":608,"message":"OUTPUT_MUST_NOT_SEND","extra":` + strings.Repeat("[", 63) + "null" + strings.Repeat("]", 63) + `}}`, nil
	}
	if kind == "structure-values" {
		part := `{"action":"send_group_msg","params":{"group_id":608,"message":"OUTPUT_MUST_NOT_SEND","extra":[` + strings.TrimSuffix(strings.Repeat("null,", 5000), ",") + `]}}`
		return prefix + part + part, nil
	}
	if tail, ok := map[string]string{
		"syntax-closing": "]", "syntax-truncated": `{"action":`, "syntax-null": "null",
		"syntax-duplicate-action": `{"action":"send_group_msg","Action":"end_action","params":{}}`,
		"syntax-duplicate-params": `{"action":"send_group_msg","params":{"group_id":608,"group\u005fid":609,"message":"DUPLICATE_MUST_NOT_SEND"}}`,
	}[kind]; ok {
		return prefix + "\n" + `{"action":"send_group_msg","params":{"group_id":608,"message":"MALFORMED_MUST_NOT_SEND"}}` + "\n" + tail, nil
	}
	if kind == "text" {
		return prefix + "\n" + `{"action":"send_group_msg","params":{"group_id":608,"message":"` + strings.Repeat("T", 1<<20) + `"}}`, nil
	}
	if kind == "batch" {
		parts := []string{prefix}
		for i := 0; i < 31; i++ {
			parts = append(parts, fmt.Sprintf(`{"action":"send_group_msg","params":{"group_id":608,"message":"OUTPUT_MUST_NOT_SEND_%d"}}`, i))
		}
		parts = append(parts, `{"action":"end_action","params":{}}`)
		return strings.Join(parts, "\n"), nil
	}
	return "", fmt.Errorf("unknown action output fixture")
}

func smokeOutputLimitTask(task map[string]any, failed bool) bool {
	return smokeOutputTask(task, failed, "model_output_limit")
}

func smokeOutputTask(task map[string]any, failed bool, errorCode string) bool {
	if !smokeTaskTimingValid(task) || !smokeTaskQueriesMatch(task, 0, 0, 0) {
		return false
	}
	a, _ := task["activity"].(map[string]any)
	wantOK := 0
	status := "failed"
	if !failed {
		wantOK = 1
		status = "completed"
	}
	if task["status"] != status || !smokeTaskResultsMatch(task, wantOK, 0, 0, 0) || a["available"] != true || a["modelPreparations"] != float64(1) || a["fallbackPreparations"] != float64(0) || a["localActions"] != float64(0) || a["platformActions"] != float64(wantOK) {
		return false
	}
	return !failed || task["error"] == errorCode
}

func verifyAgentOutputLimits(do func(string, string, any, string) (int, map[string]any, error), send func(int64, int64, string, string, bool) (string, error), csrf string, configure func(string, bool) error) error {
	if err := configure("group=608", true); err != nil {
		return err
	}
	seen := map[string]bool{}
	byReason := map[string]map[string]bool{}
	byOutcome := map[string]map[string]bool{}
	for _, kind := range []string{"batch", "text", "http", "structure-depth", "structure-values", "syntax-closing", "syntax-truncated", "syntax-null", "syntax-duplicate-action", "syntax-duplicate-params", "request-http", "request-protocol", "request-empty", "request-missing", "request-unsupported", "ok"} {
		errorCode := "model_output_limit"
		if strings.HasPrefix(kind, "syntax-") {
			errorCode = "model_output_invalid"
		}
		if strings.HasPrefix(kind, "request-") {
			errorCode = "model_request_failed"
		}
		status, snapshot, err := do("GET", "/api/ai", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("output limits config read failed")
		}
		status, _, err = do("POST", "/api/ai", map[string]any{"revision": snapshot["global"].(map[string]any)["revision"], "fields": map[string]any{"AgentModelName": "smoke-output-limit-" + kind}}, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("output limits setup failed")
		}
		text, err := send(23456789, 608, "member", "OUTPUT_LIMIT_CHECK_"+kind, true)
		want := ""
		if kind == "ok" {
			want = "OUTPUT_LIMIT_RECOVERY_OK"
		}
		if err != nil || text != want {
			return fmt.Errorf("limited output was partially dispatched: %s %q %v", kind, text, err)
		}
		time.Sleep(100 * time.Millisecond)
		status, page, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=608", nil, "")
		rows, _ := page["items"].([]any)
		if err != nil || status != 200 || len(rows) == 0 {
			return fmt.Errorf("output task unavailable")
		}
		v, _ := rows[0].(map[string]any)
		summary, _ := json.Marshal(v)
		if bytes.Contains(summary, []byte(smokeModelFailureText)) {
			return fmt.Errorf("provider failure body entered task summary")
		}
		id, _ := v["taskId"].(string)
		if seen[id] || id == "" || !smokeOutputTask(v, kind != "ok", errorCode) {
			return fmt.Errorf("output task summary incorrect: %s status=%v error=%v", kind, v["status"], v["error"])
		}
		seen[id] = true
		actualReason, _ := v["error"].(string)
		if byReason[actualReason] == nil {
			byReason[actualReason] = map[string]bool{}
		}
		byReason[actualReason][id] = true
		actualOutcome, _ := v["outcome"].(string)
		if byOutcome[actualOutcome] == nil {
			byOutcome[actualOutcome] = map[string]bool{}
		}
		byOutcome[actualOutcome][id] = true
		status, exact, err := do("GET", "/api/agent/tasks/read?botId=87654321&type=group&id=608&taskId="+id, nil, "")
		if err != nil || status != 200 || !smokeOutputTask(exact, kind != "ok", errorCode) || exact["version"] != v["version"] {
			return fmt.Errorf("output task exact read mismatch")
		}
		started, parseErr := time.Parse(time.RFC3339Nano, fmt.Sprint(v["startedAt"]))
		if parseErr != nil {
			return fmt.Errorf("task start instant unavailable")
		}
		for _, bounds := range []struct {
			from, until time.Time
			want        int
		}{
			{started, started.Add(time.Nanosecond), 1},
			{started.Add(-time.Nanosecond), started, 0},
			{started.Add(time.Nanosecond), started.Add(2 * time.Nanosecond), 0},
		} {
			from := bounds.from.In(time.FixedZone("test", 8*60*60)).Format(time.RFC3339Nano)
			until := bounds.until.Format(time.RFC3339Nano)
			query := url.Values{"botId": {"87654321"}, "type": {"group"}, "id": {"608"}, "state": {fmt.Sprint(v["status"])}, "phase": {"finished"}, "reason": {"none"}, "from": {from}, "until": {until}}
			if actualReason != "" {
				query.Set("reason", actualReason)
			}
			code, filtered, err := do("GET", "/api/agent/tasks?"+query.Encode(), nil, "")
			items, ok := filtered["items"].([]any)
			if err != nil || code != 200 || !ok || len(items) != bounds.want || filtered["from"] != bounds.from.UTC().Format(time.RFC3339Nano) || filtered["until"] != until || filtered["more"] != false {
				return fmt.Errorf("task start range boundary mismatch: %s", kind)
			}
			if bounds.want == 1 {
				row, _ := items[0].(map[string]any)
				if row["taskId"] != id || row["version"] != v["version"] {
					return fmt.Errorf("task start range changed task")
				}
			}
		}
	}
	for _, tc := range []struct {
		state, reason, saved string
		empty                bool
	}{
		{"failed", "model_request_failed", "model_request_failed", false},
		{"failed", "model_output_limit", "model_output_limit", false},
		{"failed", "model_output_invalid", "model_output_invalid", false},
		{"completed", "none", "", false},
		{"failed", "none", "", true},
		{"completed", "model_request_failed", "model_request_failed", true},
		{"all", "other", "", true},
	} {
		status, page, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=608&state="+tc.state+"&reason="+tc.reason+"&phase=finished", nil, "")
		rows, ok := page["items"].([]any)
		want := len(byReason[tc.saved])
		if tc.empty {
			want = 0
		}
		if err != nil || status != 200 || !ok || len(rows) != want || page["more"] != false {
			return fmt.Errorf("task reason filter count mismatch: %s/%s", tc.state, tc.reason)
		}
		returned := map[string]bool{}
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			id, _ := row["taskId"].(string)
			if !byReason[tc.saved][id] || returned[id] || row["error"] != tc.saved || row["status"] != tc.state || row["phase"] != "finished" {
				return fmt.Errorf("task reason filter returned wrong task")
			}
			returned[id] = true
		}
	}
	for _, outcome := range []string{"all", "none", "other", "ok", "unknown", "in_flight", "rejected", "local_error"} {
		status, page, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=608&phase=finished&outcome="+outcome, nil, "")
		rows, ok := page["items"].([]any)
		want := byOutcome[outcome]
		if outcome == "none" {
			want = byOutcome[""]
		}
		if outcome == "all" {
			want = seen
		}
		if err != nil || status != 200 || !ok || len(rows) != len(want) || page["more"] != false {
			return fmt.Errorf("task outcome count mismatch: %s", outcome)
		}
		returned := map[string]bool{}
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			id, _ := row["taskId"].(string)
			if !want[id] || returned[id] {
				return fmt.Errorf("task outcome returned wrong task")
			}
			returned[id] = true
		}
	}
	status, private, err := do("GET", "/api/agent/tasks?botId=87654321&type=private&id=608&reason=model_request_failed&outcome=none", nil, "")
	privateRows, privateOK := private["items"].([]any)
	if err != nil || status != 200 || !privateOK || len(privateRows) != 0 {
		return fmt.Errorf("task reason filter crossed conversation scope")
	}
	status, memories, err := do("GET", "/api/agent/memories?botId=87654321&type=group&id=608", nil, "")
	rows, _ := memories["items"].([]any)
	if err != nil || status != 200 || len(rows) != 0 {
		return fmt.Errorf("rejected output saved a memory prefix")
	}
	return configure("group=608", false)
}

func smokeModelRecords(body map[string]any) []map[string]any {
	var records []map[string]any
	messages, _ := body["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		content, _ := message["content"].(string)
		for _, line := range strings.Split(content, "\n") {
			var record map[string]any
			if json.Unmarshal([]byte(line), &record) == nil && record != nil {
				records = append(records, record)
			}
		}
	}
	return records
}

func smokeModelResponse(body map[string]any, field string, count int) ([]any, error) {
	records := smokeModelRecords(body)
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		if _, ok := r["status"]; !ok {
			continue
		}
		data, _ := r["data"].(map[string]any)
		rows, ok := data[field].([]any)
		if r["status"] != "ok" || !ok || len(rows) != count {
			return nil, fmt.Errorf("latest local response lacks %d %s", count, field)
		}
		return rows, nil
	}
	return nil, fmt.Errorf("local response missing")
}

func smokeHistoryContextAnchor(body map[string]any) (map[string]any, error) {
	records := smokeModelRecords(body)
	responseIndex := -1
	for i := len(records) - 1; i >= 0; i-- {
		if _, ok := records[i]["status"]; ok {
			responseIndex = i
			break
		}
	}
	if responseIndex < 0 {
		return nil, fmt.Errorf("context response absent")
	}
	response := records[responseIndex]
	data, _ := response["data"].(map[string]any)
	rows, ok := data["sources"].([]any)
	anchor, _ := data["anchorSeq"].(string)
	if response["status"] != "ok" || !ok || len(rows) < 2 || len(rows) > 3 || !smokePositiveID(anchor) || data["context"] != float64(1) || data["searchMore"] != false {
		return nil, fmt.Errorf("context receipt invalid")
	}
	requestIndex := -1
	for i := responseIndex - 1; i >= 0; i-- {
		if records[i]["action"] == "read_history" {
			params, _ := records[i]["params"].(map[string]any)
			if params["seq"] != anchor || params["context"] != float64(1) {
				return nil, fmt.Errorf("context request mismatch")
			}
			requestIndex = i
			break
		}
	}
	if requestIndex < 0 {
		return nil, fmt.Errorf("context request absent")
	}
	known := false
	for i := requestIndex - 1; i >= 0; i-- {
		if _, ok := records[i]["status"]; !ok {
			continue
		}
		previous, _ := records[i]["data"].(map[string]any)
		sources, _ := previous["sources"].([]any)
		if len(sources) == 1 {
			source, _ := sources[0].(map[string]any)
			known = source["seq"] == anchor
		}
		break
	}
	if !known {
		return nil, fmt.Errorf("context anchor not taken from actual search result")
	}
	var result map[string]any
	newer, older, total := 0, 0, 0
	last := int64(0)
	center, _ := strconv.ParseInt(anchor, 10, 64)
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok || !smokePositiveID(row["seq"]) || row["kind"] != "event" {
			return nil, fmt.Errorf("context contains invalid event")
		}
		seq, _ := strconv.ParseInt(row["seq"].(string), 10, 64)
		if i > 0 && seq >= last {
			return nil, fmt.Errorf("context order/duplicate invalid")
		}
		last = seq
		text, _ := row["text"].(string)
		limit := 2000
		if seq == center {
			limit = 8000
			result = row
		} else if seq > center {
			newer++
		} else {
			older++
		}
		total += len(text)
		if !utf8.ValidString(text) || len(text) > limit {
			return nil, fmt.Errorf("context text bound invalid")
		}
	}
	if result == nil || newer > 1 || older > 1 || total > 16384 {
		return nil, fmt.Errorf("context center/radius invalid")
	}
	return result, nil
}

func smokeCommandID(body map[string]any, prefix, key string) (string, error) {
	fields, err := smokeCommandFields(body, prefix)
	if err != nil {
		return "", err
	}
	if !smokePositiveID(fields[key]) {
		return "", fmt.Errorf("synthetic event lacks valid %s", key)
	}
	return fields[key], nil
}

func smokeCommandFields(body map[string]any, prefix string) (map[string]string, error) {
	records := smokeModelRecords(body)
	for i := len(records) - 1; i >= 0; i-- {
		segments, _ := records[i]["message"].([]any)
		for _, raw := range segments {
			segment, _ := raw.(map[string]any)
			data, _ := segment["data"].(map[string]any)
			text, _ := data["text"].(string)
			if !strings.HasPrefix(text, prefix+" ") {
				continue
			}
			fields := map[string]string{}
			for _, part := range strings.Fields(text) {
				if key, value, ok := strings.Cut(part, "="); ok {
					fields[key] = value
				}
			}
			return fields, nil
		}
	}
	return nil, fmt.Errorf("synthetic event missing")
}

func smokeHistoryRange(body map[string]any) (from, until time.Time, err error) {
	fields, err := smokeCommandFields(body, "KNOWLEDGE_HISTORY_CHECK")
	if err != nil {
		return from, until, err
	}
	from, err = time.Parse(time.RFC3339Nano, fields["FROM"])
	if err != nil {
		return from, until, err
	}
	until, err = time.Parse(time.RFC3339Nano, fields["UNTIL"])
	if err != nil {
		return from, until, err
	}
	if !from.Before(until) {
		return from, until, fmt.Errorf("synthetic history time order invalid")
	}
	return from.UTC(), until.UTC(), nil
}

func smokePositiveID(value any) bool {
	s, ok := value.(string)
	n, e := strconv.ParseInt(s, 10, 64)
	return ok && e == nil && n > 0 && strconv.FormatInt(n, 10) == s
}

func smokeArchiveSender(text string) (string, error) {
	var event map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if decoder.Decode(&event) != nil || event["post_type"] != "message" {
		return "", fmt.Errorf("source is not a complete message event")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return "", fmt.Errorf("source contains trailing data")
	}
	var id string
	switch value := event["user_id"].(type) {
	case json.Number:
		id = string(value)
	case string:
		id = value
	}
	if !smokePositiveID(id) {
		return "", fmt.Errorf("source has no exact sender")
	}
	return id, nil
}

func smokeMemorySourceDelta(before, after map[string]any) bool {
	b, _ := before["context"].(map[string]any)
	a, _ := after["context"].(map[string]any)
	previous, ok := b["memorySources"].(float64)
	current, currentOK := a["memorySources"].(float64)
	return before["persisted"] == true && after["persisted"] == true && b["available"] == true && a["available"] == true && b["clipped"] == false && a["clipped"] == false && ok && currentOK && previous >= 0 && previous <= 9007199254740990 && previous == float64(int64(previous)) && current == previous+1
}

func smokeSavedReference(body map[string]any, latest bool) (map[string]any, error) {
	records := smokeModelRecords(body)
	for i := len(records) - 1; i >= 0; i-- {
		if records[i]["action"] != "save_memory" {
			continue
		}
		params, _ := records[i]["params"].(map[string]any)
		if params["text"] != "KNOWLEDGE_SAVED_FROM_RESULT" {
			return nil, fmt.Errorf("current save request missing")
		}
		for j := i + 1; j < len(records); j++ {
			response := records[j]
			if _, ok := response["status"]; !ok {
				continue
			}
			data, _ := response["data"].(map[string]any)
			ref, _ := data["memory"].(map[string]any)
			if response["status"] != "ok" || data["mode"] != "managed" || data["created"] != false || len(ref) != 4 || !smokePositiveID(ref["id"]) || !smokePositiveID(ref["revision"]) || !smokePositiveID(ref["sourceSeq"]) || ref["source"] != "agent" {
				return nil, fmt.Errorf("save result lacks exact reused reference")
			}
			original := false
			for k := i - 1; k >= 0; k-- {
				if records[k]["action"] != "save_memory" {
					continue
				}
				p, _ := records[k]["params"].(map[string]any)
				if p["text"] != "KNOWLEDGE_SAVED_FROM_RESULT" {
					break
				}
				for l := k + 1; l < i; l++ {
					if _, ok := records[l]["status"]; !ok {
						continue
					}
					d, _ := records[l]["data"].(map[string]any)
					first, _ := d["memory"].(map[string]any)
					original = records[l]["status"] == "ok" && d["mode"] == "managed" && d["created"] == true && len(first) == 4 && first["id"] == ref["id"] && first["revision"] == ref["revision"] && first["source"] == ref["source"] && first["sourceSeq"] == ref["sourceSeq"]
					break
				}
				break
			}
			if !original {
				return nil, fmt.Errorf("dedup did not preserve the first newly created reference")
			}
			if latest {
				for k := j + 1; k < len(records); k++ {
					if _, ok := records[k]["status"]; ok {
						return nil, fmt.Errorf("save response is not latest")
					}
				}
			}
			return ref, nil
		}
		return nil, fmt.Errorf("save response missing")
	}
	return nil, fmt.Errorf("save request missing")
}

func smokeKnowledgeReply(body map[string]any, round int) (string, error) {
	action := func(name string, params map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"action": name, "params": params})
		return string(raw)
	}
	finish := func(text string) string {
		return action("send_group_msg", map[string]any{"group_id": 303, "message": text}) + "\n" + action("end_action", map[string]any{})
	}
	switch body["model"] {
	case "smoke-knowledge-save":
		if round == 1 {
			raw, _ := json.Marshal(body)
			if bytes.Contains(raw, []byte("KNOWLEDGE_SAVED_FROM_RESULT")) {
				return "", fmt.Errorf("new memory already present before saving")
			}
			save := action("save_memory", map[string]any{"text": "KNOWLEDGE_SAVED_FROM_RESULT"})
			return save + "\n" + save, nil
		}
		ref, e := smokeSavedReference(body, round == 2)
		if e != nil {
			return "", e
		}
		if round == 2 {
			return action("read_memory", map[string]any{"id": ref["id"], "revision": ref["revision"]}), nil
		}
		if round == 3 {
			rows, e := smokeModelResponse(body, "memories", 1)
			if e != nil {
				return "", e
			}
			item, _ := rows[0].(map[string]any)
			if item["id"] != ref["id"] || item["revision"] != ref["revision"] || item["sourceSeq"] != ref["sourceSeq"] || item["source"] != ref["source"] || item["text"] != "KNOWLEDGE_SAVED_FROM_RESULT" || item["truncated"] != false {
				return "", fmt.Errorf("saved memory read did not match returned reference")
			}
			return finish("KNOWLEDGE_SAVE_OK"), nil
		}
	case "smoke-knowledge-memory":
		if round == 1 {
			raw, _ := json.Marshal(body)
			if bytes.Contains(raw, []byte("KNOWLEDGE_TAIL_ONLY_READ")) {
				return "", fmt.Errorf("old target unexpectedly present before retrieval")
			}
			return action("search_memory", map[string]any{"limit": 1, "source": "admin", "from": "2000-01-01T08:00:00.000000001+08:00", "until": "2100-01-01T00:00:00Z"}), nil
		}
		rows, e := smokeModelResponse(body, "memories", 1)
		if e != nil {
			return "", e
		}
		item, _ := rows[0].(map[string]any)
		text, _ := item["text"].(string)
		if round == 2 || round == 3 {
			records := smokeModelRecords(body)
			for i := len(records) - 1; i >= 0; i-- {
				if _, ok := records[i]["status"]; !ok {
					continue
				}
				data, _ := records[i]["data"].(map[string]any)
				if data["source"] != "admin" {
					return "", fmt.Errorf("memory source condition receipt missing")
				}
				if data["from"] != "2000-01-01T00:00:00.000000001Z" || data["until"] != "2100-01-01T00:00:00Z" {
					return "", fmt.Errorf("memory revision time receipt missing")
				}
				if _, present := data["state"]; present {
					return "", fmt.Errorf("management search state leaked into model response")
				}
				break
			}
		}
		if !smokePositiveID(item["id"]) || !smokePositiveID(item["revision"]) || item["source"] != "admin" || item["sourceSeq"] != "0" || !strings.Contains(text, "KNOWLEDGE_MANUAL_TARGET") {
			return "", fmt.Errorf("memory result identity/source/text missing")
		}
		if round == 2 && item["truncated"] == false && strings.Contains(text, "KNOWLEDGE_NEWER_DISTRACTOR") {
			records := smokeModelRecords(body)
			for i := len(records) - 1; i >= 0; i-- {
				if records[i]["action"] != "search_memory" {
					continue
				}
				params, _ := records[i]["params"].(map[string]any)
				if _, present := params["keyword"]; present || params["source"] != "admin" {
					return "", fmt.Errorf("source-only memory request changed")
				}
				break
			}
			for i := len(records) - 1; i >= 0; i-- {
				if _, ok := records[i]["status"]; !ok {
					continue
				}
				data, _ := records[i]["data"].(map[string]any)
				next, _ := data["nextBefore"].(string)
				position, _ := strconv.ParseInt(next, 10, 64)
				id, _ := strconv.ParseInt(item["id"].(string), 10, 64)
				if data["searchMore"] != true || !smokePositiveID(next) || position > id {
					return "", fmt.Errorf("memory continuation receipt invalid")
				}
				return action("search_memory", map[string]any{"keyword": "KNOWLEDGE_MANUAL_TARGET", "limit": 1, "before": next, "source": "admin", "from": data["from"], "until": data["until"]}), nil
			}
			return "", fmt.Errorf("memory continuation receipt missing")
		}
		if round == 3 && item["truncated"] == true && !strings.Contains(text, "KNOWLEDGE_TAIL_ONLY_READ") {
			// Check the actual request's exclusive boundary, not an expected ID.
			boundary := int64(0)
			records := smokeModelRecords(body)
			for i := len(records) - 1; i >= 0; i-- {
				if records[i]["action"] != "search_memory" {
					continue
				}
				params, _ := records[i]["params"].(map[string]any)
				if params["source"] != "admin" {
					return "", fmt.Errorf("continued memory source condition changed")
				}
				if params["from"] != "2000-01-01T00:00:00.000000001Z" || params["until"] != "2100-01-01T00:00:00Z" {
					return "", fmt.Errorf("continued memory time condition changed")
				}
				if smokePositiveID(params["before"]) {
					boundary, _ = strconv.ParseInt(params["before"].(string), 10, 64)
				}
				break
			}
			id, _ := strconv.ParseInt(item["id"].(string), 10, 64)
			if boundary == 0 || id >= boundary {
				return "", fmt.Errorf("memory source outside actual exclusive boundary")
			}
			return action("read_memory", map[string]any{"id": item["id"], "revision": item["revision"]}), nil
		}
		if round == 4 && item["truncated"] == false && strings.Contains(text, "KNOWLEDGE_TAIL_ONLY_READ") {
			return finish("KNOWLEDGE_MEMORY_OK"), nil
		}
	case "smoke-knowledge-history":
		before, e := smokeCommandID(body, "KNOWLEDGE_HISTORY_CHECK", "BEFORE")
		if e != nil {
			return "", e
		}
		sender, e := smokeCommandID(body, "KNOWLEDGE_HISTORY_CHECK", "SENDER")
		if e != nil {
			return "", e
		}
		from, until, e := smokeHistoryRange(body)
		if e != nil {
			return "", e
		}
		if round == 1 {
			return action("search_history", map[string]any{"keyword": "ROLE_AGENT_CHECK", "limit": 1, "before": before, "from": from.Format(time.RFC3339Nano), "until": until.Format(time.RFC3339Nano), "sender": sender, "eventType": "message"}), nil
		}
		var rows []any
		if round == 4 {
			anchor, err := smokeHistoryContextAnchor(body)
			if err != nil {
				return "", err
			}
			rows = []any{anchor}
		} else {
			rows, e = smokeModelResponse(body, "sources", 1)
		}
		if e != nil {
			return "", e
		}
		item, _ := rows[0].(map[string]any)
		stamp, parseErr := time.Parse(time.RFC3339Nano, fmt.Sprint(item["time"]))
		if parseErr != nil || stamp.Before(from) || !stamp.Before(until) {
			return "", fmt.Errorf("history result outside actual time range")
		}
		seq, _ := item["seq"].(string)
		n, _ := strconv.ParseInt(seq, 10, 64)
		boundary, _ := strconv.ParseInt(before, 10, 64)
		if round >= 3 {
			records := smokeModelRecords(body)
			for i := len(records) - 1; i >= 0; i-- {
				if records[i]["action"] != "search_history" {
					continue
				}
				params, _ := records[i]["params"].(map[string]any)
				if !smokePositiveID(params["before"]) {
					return "", fmt.Errorf("continued history lacks actual boundary")
				}
				if params["sender"] != sender || params["eventType"] != "message" {
					return "", fmt.Errorf("continued history changed sender condition")
				}
				if _, exists := params["keyword"]; exists {
					return "", fmt.Errorf("sender-only history unexpectedly retained a keyword")
				}
				boundary, _ = strconv.ParseInt(params["before"].(string), 10, 64)
				break
			}
		}
		text, _ := item["text"].(string)
		actualSender, senderErr := smokeArchiveSender(text)
		if senderErr != nil || actualSender != sender {
			return "", fmt.Errorf("history source sender differs from selected account")
		}
		if !smokePositiveID(seq) || n >= boundary || item["kind"] != "event" || (round == 2 && !strings.Contains(text, "ROLE_AGENT_CHECK")) {
			return "", fmt.Errorf("history source outside expected event boundary")
		}
		if round == 2 || round == 3 {
			records := smokeModelRecords(body)
			confirmed := false
			var applied map[string]any
			for i := len(records) - 1; i >= 0; i-- {
				if _, ok := records[i]["status"]; !ok {
					continue
				}
				data, _ := records[i]["data"].(map[string]any)
				applied = data
				confirmed = data["from"] == from.Format(time.RFC3339Nano) && data["until"] == until.Format(time.RFC3339Nano) && data["sender"] == sender && data["eventType"] == "message"
				break
			}
			if !confirmed {
				return "", fmt.Errorf("history response lacks applied UTC range")
			}
			if round == 2 {
				next, ok := applied["nextBefore"].(string)
				position, _ := strconv.ParseInt(next, 10, 64)
				if !ok || !smokePositiveID(next) || applied["searchMore"] != true || position > n || position >= boundary {
					return "", fmt.Errorf("history continuation receipt invalid")
				}
				return action("search_history", map[string]any{"limit": 1, "before": next, "from": from.Format(time.RFC3339Nano), "until": until.Format(time.RFC3339Nano), "sender": sender, "eventType": "message"}), nil
			}
			return action("read_history", map[string]any{"seq": seq, "context": 1}), nil
		}
		if round == 4 {
			return finish("KNOWLEDGE_HISTORY_OK"), nil
		}
	case "smoke-knowledge-stale":
		if round == 1 {
			id, e := smokeCommandID(body, "KNOWLEDGE_STALE_CHECK", "ID")
			if e != nil {
				return "", e
			}
			rev, e := smokeCommandID(body, "KNOWLEDGE_STALE_CHECK", "REV")
			return action("read_memory", map[string]any{"id": id, "revision": rev}), e
		}
		if round == 2 {
			if _, e := smokeModelResponse(body, "memories", 0); e != nil {
				return "", e
			}
			return finish("KNOWLEDGE_STALE_OK"), nil
		}
	}
	return "", fmt.Errorf("unexpected synthetic knowledge round %v/%d", body["model"], round)
}

func verifyAgentKnowledgeRuntime(do func(string, string, any, string) (int, map[string]any, error), send func(int64, int64, string, string, bool) (string, error), csrf string) error {
	const scopeQuery = "botId=87654321&type=group&id=303"
	const memoryPath = "/api/agent/memories?" + scopeQuery
	const taskPath = "/api/agent/tasks?" + scopeQuery
	scope := func() map[string]any { return map[string]any{"botId": "87654321", "type": "group", "id": "303"} }
	create := func(text string, created bool) (map[string]any, error) {
		body := scope()
		body["text"] = text
		status, result, e := do("POST", "/api/agent/memories/create", body, csrf)
		item, ok := result["item"].(map[string]any)
		if e != nil || status != 200 || result["saved"] != true || result["created"] != created || !ok || !smokePositiveID(item["id"]) {
			return nil, fmt.Errorf("manual memory create/dedup failed: %d %v", status, e)
		}
		return item, nil
	}
	text := "KNOWLEDGE_MANUAL_TARGET " + strings.Repeat("填", 950) + " KNOWLEDGE_TAIL_ONLY_READ"
	item, e := create(text, true)
	if e != nil {
		return e
	}
	if item["revision"] != "1" || item["source"] != "admin" || item["sourceSeq"] != "0" || item["active"] != true {
		return fmt.Errorf("manual memory provenance incorrect")
	}
	duplicate, e := create(text, false)
	if e != nil || duplicate["id"] != item["id"] || duplicate["updatedAt"] != item["updatedAt"] || duplicate["revision"] != item["revision"] {
		return fmt.Errorf("manual duplicate modified original metadata: %v", e)
	}
	for i := 0; i < 105; i++ {
		if _, e = create(fmt.Sprintf("KNOWLEDGE_FILLER_%03d", i), true); e != nil {
			return e
		}
	}
	newer, e := create("KNOWLEDGE_MANUAL_TARGET KNOWLEDGE_NEWER_DISTRACTOR", true)
	if e != nil {
		return e
	}
	readCurrent := func(query string, expected map[string]any) error {
		status, page, err := do("GET", "/api/agent/memories/current?"+query, nil, "")
		rows, ok := page["items"].([]any)
		count := 0
		if expected != nil {
			count = 1
		}
		if err != nil || status != 200 || page["mode"] != "managed" || !ok || len(rows) != count {
			return fmt.Errorf("current memory lookup mismatch: %d %v", status, err)
		}
		if expected != nil {
			item := rows[0].(map[string]any)
			for key, want := range expected {
				if item[key] != want {
					return fmt.Errorf("current memory field differs: %s", key)
				}
			}
		}
		return nil
	}
	status, firstPage, e := do("GET", memoryPath, nil, "")
	firstItems, _ := firstPage["items"].([]any)
	if e != nil || status != 200 || len(firstItems) != 50 || firstPage["more"] != true {
		return fmt.Errorf("current memory lookup paging baseline missing")
	}
	for _, raw := range firstItems {
		if raw.(map[string]any)["id"] == newer["id"] {
			return fmt.Errorf("current memory fixture must be beyond the first page")
		}
	}
	if e = readCurrent(scopeQuery+"&memoryId="+newer["id"].(string), newer); e != nil {
		return e
	}
	for _, query := range []string{"botId=87654321&type=private&id=303", "botId=87654321&type=group&id=304"} {
		if e = readCurrent(query+"&memoryId="+newer["id"].(string), nil); e != nil {
			return e
		}
	}
	read := func(query string, count int, expectedText string) error {
		status, page, e := do("GET", "/api/agent/memories/read?"+query, nil, "")
		rows, ok := page["memories"].([]any)
		if e != nil || status != 200 || page["available"] != true || !ok || len(rows) != count {
			return fmt.Errorf("precise memory read mismatch: %d %v", status, e)
		}
		if count == 1 {
			v, _ := rows[0].(map[string]any)
			if v["text"] != expectedText || v["truncated"] != false {
				return fmt.Errorf("precise memory text mismatch")
			}
		}
		return nil
	}
	id := item["id"].(string)
	stamp, e := time.Parse(time.RFC3339Nano, item["updatedAt"].(string))
	if e != nil {
		return e
	}
	for _, tc := range []struct {
		from, until string
		count       int
	}{
		{stamp.Format(time.RFC3339Nano), stamp.Add(time.Nanosecond).Format(time.RFC3339Nano), 1},
		{stamp.Add(-time.Nanosecond).Format(time.RFC3339Nano), stamp.Format(time.RFC3339Nano), 0},
	} {
		body := scope()
		body["from"], body["until"] = tc.from, tc.until
		time.Sleep(260 * time.Millisecond)
		code, page, err := do("POST", "/api/agent/memories/search", body, csrf)
		rows, ok := page["memories"].([]any)
		if err != nil || code != 200 || !ok || len(rows) != tc.count {
			return fmt.Errorf("memory time-only HTTP window mismatch: %d %v", code, err)
		}
		if tc.count == 1 && rows[0].(map[string]any)["id"] != id {
			return fmt.Errorf("memory time window selected wrong revision")
		}
		if tc.from != "" && page["from"] != tc.from {
			return fmt.Errorf("memory HTTP time receipt missing")
		}
	}
	if e = read(scopeQuery+"&memoryId="+id+"&revision=1", 1, text); e != nil {
		return e
	}
	for _, query := range []string{"botId=87654321&type=private&id=303", "botId=87654321&type=group&id=304"} {
		if e = read(query+"&memoryId="+id+"&revision=1", 0, ""); e != nil {
			return e
		}
	}
	// Walk the real keyset API and verify each row, including page boundaries.
	checkPages := func(path, field, identity string, count int, validate func(map[string]any) bool) error {
		seen := map[string]bool{}
		for pageNo := 0; pageNo < 10; pageNo++ {
			status, page, e := do("GET", path, nil, "")
			rows, ok := page[field].([]any)
			if e != nil || status != 200 || !ok || len(rows) > 50 {
				return fmt.Errorf("filtered pagination unavailable: %d %v", status, e)
			}
			for _, raw := range rows {
				v, ok := raw.(map[string]any)
				key, _ := v[identity].(string)
				if !ok || key == "" || seen[key] || !validate(v) {
					return fmt.Errorf("filtered page duplicate or wrong state")
				}
				seen[key] = true
			}
			if page["more"] != true {
				if len(seen) != count {
					return fmt.Errorf("filtered page count %d, want %d", len(seen), count)
				}
				return nil
			}
			next, _ := page["next"].(string)
			if !smokePositiveID(next) {
				return fmt.Errorf("filtered cursor missing")
			}
			parsed, _ := url.Parse(path)
			q := parsed.Query()
			cursor := "after"
			if field == "items" && identity == "taskId" {
				cursor = "before"
			}
			q.Set(cursor, next)
			path = parsed.Path + "?" + q.Encode()
		}
		return fmt.Errorf("filtered pagination failed to terminate")
	}
	if e = checkPages(memoryPath+"&state=active", "items", "id", 107, func(v map[string]any) bool { return v["active"] == true }); e != nil {
		return e
	}
	// Exercise the actual HTTP adapter's boundary, before invoking the model.
	time.Sleep(260 * time.Millisecond)
	bounded := scope()
	bounded["keyword"], bounded["before"] = "KNOWLEDGE_MANUAL_TARGET", newer["id"]
	status, matched, e := do("POST", "/api/agent/memories/search", bounded, csrf)
	matches, _ := matched["memories"].([]any)
	if e != nil || status != 200 || matched["available"] != true || len(matches) != 1 || matches[0].(map[string]any)["id"] != id || matches[0].(map[string]any)["revision"] != "1" || matches[0].(map[string]any)["truncated"] != true {
		return fmt.Errorf("admin memory boundary did not return the older target: %d %v", status, e)
	}
	run := func(model, command, reply string, models, locals int) (map[string]any, error) {
		status, cfg, e := do("GET", "/api/ai", nil, "")
		if e != nil || status != 200 {
			return nil, fmt.Errorf("knowledge model config read failed")
		}
		status, _, e = do("POST", "/api/ai", map[string]any{"revision": cfg["global"].(map[string]any)["revision"], "fields": map[string]any{"AgentModelName": model}}, csrf)
		if e != nil || status != 200 {
			return nil, fmt.Errorf("knowledge model config save failed")
		}
		got, e := send(23456789, 303, "member", command, true)
		if e != nil || got != reply {
			return nil, fmt.Errorf("knowledge Agent round trip %s: got %q, error %v", model, got, e)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			status, page, e := do("GET", taskPath, nil, "")
			rows, _ := page["items"].([]any)
			if e != nil || status != 200 || len(rows) == 0 {
				return nil, fmt.Errorf("knowledge task unavailable")
			}
			v, _ := rows[0].(map[string]any)
			if v["status"] == "completed" {
				a, _ := v["activity"].(map[string]any)
				m, _ := v["memory"].(map[string]any)
				if v["persisted"] != true || a["available"] != true || a["modelPreparations"] != float64(models) || a["localActions"] != float64(locals) || a["platformActions"] != float64(1) || a["fallbackPreparations"] != float64(0) || m["available"] != true || m["mode"] != "managed" || m["records"] != float64(100) || m["clipped"] != true || m["reason"] != "record_limit" {
					return nil, fmt.Errorf("knowledge task activity/memory summary incorrect")
				}
				if !smokeTaskResultsMatch(v, locals+1, 0, 0, 0) {
					return nil, fmt.Errorf("knowledge task settled results incorrect")
				}
				wantEmpty := 0
				if model == "smoke-knowledge-stale" {
					wantEmpty = 1
				}
				queryCount := locals
				if model == "smoke-knowledge-save" {
					queryCount = 1
				}
				if !smokeTaskQueriesMatch(v, queryCount, queryCount, wantEmpty) {
					return nil, fmt.Errorf("knowledge task query quota/coverage incorrect")
				}
				q := v["queries"].(map[string]any)
				switch model {
				case "smoke-knowledge-save":
					if q["readMemory"] != float64(1) || q["partial"] != float64(0) || q["searchHistory"] != float64(0) || q["searchMemory"] != float64(0) {
						return nil, fmt.Errorf("save/read quota counted saves as queries")
					}
				case "smoke-knowledge-memory":
					if q["searchMemory"] != float64(2) || q["readMemory"] != float64(1) || q["partial"].(float64) < 1 {
						return nil, fmt.Errorf("memory query types or partial coverage missing")
					}
				case "smoke-knowledge-history":
					if q["searchHistory"] != float64(2) || q["readHistory"] != float64(1) || q["partial"].(float64) < 1 {
						return nil, fmt.Errorf("history query types or partial coverage missing")
					}
				case "smoke-knowledge-stale":
					if q["readMemory"] != float64(1) || q["empty"] != float64(1) {
						return nil, fmt.Errorf("stale memory empty result missing")
					}
				}
				encoded, _ := json.Marshal(v)
				if bytes.Contains(encoded, []byte("KNOWLEDGE_MANUAL_TARGET")) {
					return nil, fmt.Errorf("task summary exposed memory text")
				}
				return v, nil
			}
			time.Sleep(30 * time.Millisecond)
		}
		return nil, fmt.Errorf("knowledge task did not finish")
	}
	v, e := run("smoke-knowledge-memory", "KNOWLEDGE_MEMORY_CHECK", "KNOWLEDGE_MEMORY_OK", 4, 3)
	if e != nil {
		return e
	}
	if v["context"].(map[string]any)["memorySources"] != float64(3) {
		return fmt.Errorf("retrieved memory source count incorrect")
	}
	if !smokePositiveID(v["sourceSeq"]) {
		return fmt.Errorf("knowledge task trigger source missing")
	}
	status, trigger, e := do("GET", "/api/agent/history?"+scopeQuery+"&archived=true&seq="+v["sourceSeq"].(string), nil, "")
	triggerRows, _ := trigger["entries"].([]any)
	if e != nil || status != 200 || len(triggerRows) != 1 || triggerRows[0].(map[string]any)["kind"] != "event" || !strings.Contains(triggerRows[0].(map[string]any)["text"].(string), "KNOWLEDGE_MEMORY_CHECK") {
		return fmt.Errorf("knowledge task source does not match triggering event: %d %v", status, e)
	}
	time.Sleep(260 * time.Millisecond)
	body := scope()
	body["keyword"] = "ROLE_AGENT_CHECK"
	status, archive, e := do("POST", "/api/agent/history/search", body, csrf)
	entries, _ := archive["entries"].([]any)
	before := ""
	until := time.Time{}
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if entry["kind"] == "event" {
			before, _ = entry["seq"].(string)
			until, _ = time.Parse(time.RFC3339Nano, fmt.Sprint(entry["time"]))
			break
		}
	}
	if e != nil || status != 200 || !smokePositiveID(before) || until.IsZero() {
		return fmt.Errorf("history event boundary unavailable")
	}
	sender, counts := "", map[string]int{}
	boundary, _ := strconv.ParseInt(before, 10, 64)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		seq, _ := entry["seq"].(string)
		number, _ := strconv.ParseInt(seq, 10, 64)
		if entry["kind"] != "event" || number >= boundary {
			continue
		}
		status, source, err := do("GET", "/api/agent/history?"+scopeQuery+"&seq="+seq, nil, "")
		rows, _ := source["entries"].([]any)
		if err != nil || status != 200 || len(rows) != 1 {
			return fmt.Errorf("history sender source unavailable")
		}
		row, _ := rows[0].(map[string]any)
		text, _ := row["text"].(string)
		id, err := smokeArchiveSender(text)
		if err != nil {
			return err
		}
		counts[id]++
		if counts[id] >= 2 {
			sender = id
			break
		}
	}
	if sender == "" {
		return fmt.Errorf("history fixture needs two actual messages from one sender")
	}
	v, e = run("smoke-knowledge-history", "KNOWLEDGE_HISTORY_CHECK BEFORE="+before+" FROM="+until.Add(-24*time.Hour).In(time.FixedZone("fixture", 8*3600)).Format(time.RFC3339Nano)+" UNTIL="+until.Format(time.RFC3339Nano)+" SENDER="+sender, "KNOWLEDGE_HISTORY_OK", 4, 3)
	if e != nil {
		return e
	}
	if v["context"].(map[string]any)["searchSources"].(float64) < 2 {
		return fmt.Errorf("retrieved history source count incorrect")
	}
	update := func(revision, action, newText, target string, want int) error {
		body := scope()
		body["memoryId"], body["revision"], body["action"] = id, revision, action
		if newText != "" {
			body["text"] = newText
		}
		if target != "" {
			body["targetRevision"] = target
		}
		status, _, e := do("POST", "/api/agent/memories/update", body, csrf)
		if e != nil || status != want {
			return fmt.Errorf("memory lifecycle %s status %d, want %d", action, status, want)
		}
		return nil
	}
	for _, change := range []struct {
		revision, action, text, target string
		status                         int
	}{
		{"1", "edit", "KNOWLEDGE_REVISED", "", 200},
		{"1", "edit", "must not overwrite", "", 409},
		{"2", "disable", "", "", 200},
		{"3", "revert", "", "1", 200},
	} {
		if e = update(change.revision, change.action, change.text, change.target, change.status); e != nil {
			return e
		}
	}
	for _, revision := range []string{"1", "2", "4"} {
		if e = read(scopeQuery+"&memoryId="+id+"&revision="+revision, 0, ""); e != nil {
			return e
		}
	}
	if e = readCurrent(scopeQuery+"&memoryId="+id, map[string]any{"id": id, "revision": "4", "text": text, "active": false}); e != nil {
		return e
	}
	if e = checkPages(memoryPath+"&state=disabled", "items", "id", 2, func(v map[string]any) bool { return v["active"] == false }); e != nil {
		return e
	}
	time.Sleep(260 * time.Millisecond)
	body = scope()
	body["keyword"] = "KNOWLEDGE_MANUAL_TARGET"
	status, page, e := do("POST", "/api/agent/memories/search", body, csrf)
	rows, _ := page["memories"].([]any)
	if e != nil || status != 200 || len(rows) != 1 || rows[0].(map[string]any)["id"] == id || !strings.Contains(rows[0].(map[string]any)["text"].(string), "KNOWLEDGE_NEWER_DISTRACTOR") {
		return fmt.Errorf("disabled target still searchable or newer match lost")
	}
	time.Sleep(260 * time.Millisecond)
	status, page, e = do("POST", "/api/agent/memories/search", bounded, csrf)
	rows, _ = page["memories"].([]any)
	if e != nil || status != 200 || page["available"] != true || len(rows) != 0 {
		return fmt.Errorf("admin memory boundary exposed disabled target")
	}
	for _, state := range []string{"disabled", "all", "active"} {
		time.Sleep(260 * time.Millisecond)
		query := scope()
		query["keyword"], query["state"] = "KNOWLEDGE_MANUAL_TARGET", state
		status, page, e = do("POST", "/api/agent/memories/search", query, csrf)
		rows, ok := page["memories"].([]any)
		want := 1
		if state == "all" {
			want = 2
		}
		if e != nil || status != 200 || !ok || len(rows) != want || page["state"] != state {
			return fmt.Errorf("memory search state receipt/count mismatch: %s %d %v", state, status, e)
		}
		foundDisabled := false
		for _, raw := range rows {
			match := raw.(map[string]any)
			if match["id"] == id {
				foundDisabled = true
				if match["active"] != false || match["revision"] != "4" || !strings.Contains(match["text"].(string), "KNOWLEDGE_MANUAL_TARGET") {
					return fmt.Errorf("disabled memory search did not use current revision")
				}
			} else if match["id"] != newer["id"] || match["active"] != true {
				return fmt.Errorf("memory state search returned unexpected identity/state")
			}
		}
		if foundDisabled != (state != "active") {
			return fmt.Errorf("memory search state did not isolate disabled target")
		}
	}
	time.Sleep(260 * time.Millisecond)
	query := scope()
	query["source"], query["state"], query["before"] = "admin", "disabled", newer["id"]
	status, page, e = do("POST", "/api/agent/memories/search", query, csrf)
	rows, _ = page["memories"].([]any)
	if e != nil || status != 200 || page["source"] != "admin" || page["state"] != "disabled" || len(rows) != 1 || rows[0].(map[string]any)["id"] != id {
		return fmt.Errorf("disabled source-only memory search lost scope or boundary")
	}
	if e = update("4", "restore", "", "", 200); e != nil {
		return e
	}
	if e = read(scopeQuery+"&memoryId="+id+"&revision=5", 1, text); e != nil {
		return e
	}
	if e = readCurrent(scopeQuery+"&memoryId="+id, map[string]any{"id": id, "revision": "5", "text": text, "active": true}); e != nil {
		return e
	}
	time.Sleep(260 * time.Millisecond)
	status, page, e = do("POST", "/api/agent/memories/search", query, csrf)
	rows, _ = page["memories"].([]any)
	if e != nil || status != 200 || page["available"] != true || len(rows) != 0 {
		return fmt.Errorf("restored memory remained in disabled search")
	}
	status, page, e = do("GET", memoryPath+"&memoryId="+id, nil, "")
	versions, _ := page["items"].([]any)
	if e != nil || status != 200 || len(versions) != 5 || versions[0].(map[string]any)["revision"] != "1" || versions[4].(map[string]any)["revision"] != "5" {
		return fmt.Errorf("memory lifecycle revisions missing")
	}
	for _, revision := range []int{1, 4, 5} {
		code, exact, err := do("GET", "/api/agent/memories/revision?"+scopeQuery+"&memoryId="+id+"&revision="+strconv.Itoa(revision), nil, "")
		items, ok := exact["items"].([]any)
		if err != nil || code != 200 || !ok || len(items) != 1 || exact["more"] != false {
			return fmt.Errorf("exact historical memory revision unavailable: %d %v", code, err)
		}
		got := items[0].(map[string]any)
		want := versions[revision-1].(map[string]any)
		for _, key := range []string{"id", "revision", "text", "source", "sourceSeq", "active", "updatedAt"} {
			if got[key] != want[key] {
				return fmt.Errorf("exact historical memory revision field differs: %s", key)
			}
		}
	}
	for _, path := range []string{
		"/api/agent/memories/revision?" + scopeQuery + "&memoryId=" + id + "&revision=6",
		"/api/agent/memories/revision?botId=87654321&type=private&id=303&memoryId=" + id + "&revision=1",
	} {
		code, absent, err := do("GET", path, nil, "")
		items, ok := absent["items"].([]any)
		if err != nil || code != 200 || !ok || len(items) != 0 {
			return fmt.Errorf("exact historical revision scope or missing version mismatch")
		}
	}
	for _, tc := range []struct {
		path  string
		first int
		count int
	}{
		{memoryPath + "&memoryId=" + id + "&order=desc", 5, 5},
		{memoryPath + "&memoryId=" + id + "&order=desc&after=3", 2, 2},
		{"/api/agent/memories?botId=87654321&type=private&id=303&memoryId=" + id + "&order=desc", 0, 0},
	} {
		status, history, err := do("GET", tc.path, nil, "")
		rows, ok := history["items"].([]any)
		if err != nil || status != 200 || !ok || len(rows) != tc.count || history["more"] != false {
			return fmt.Errorf("newest memory revisions unavailable: %d %v", status, err)
		}
		for i, raw := range rows {
			entry := raw.(map[string]any)
			if entry["id"] != id || entry["revision"] != strconv.Itoa(tc.first-i) {
				return fmt.Errorf("newest memory revision order or boundary changed")
			}
		}
	}
	if e = readCurrent(scopeQuery+"&memoryId="+id, map[string]any{"id": id, "revision": "5", "text": text, "active": true}); e != nil {
		return e
	}
	if _, e = run("smoke-knowledge-stale", "KNOWLEDGE_STALE_CHECK ID="+id+" REV=1", "KNOWLEDGE_STALE_OK", 2, 1); e != nil {
		return e
	}
	status, exact, e := do("GET", "/api/agent/tasks/read?"+scopeQuery+"&taskId="+v["taskId"].(string), nil, "")
	if e != nil || status != 200 || exact["taskId"] != v["taskId"] || exact["version"] != v["version"] || exact["status"] != "completed" || exact["context"].(map[string]any)["searchSources"] != v["context"].(map[string]any)["searchSources"] {
		return fmt.Errorf("exact read lost an earlier task summary: %d %v", status, e)
	}
	for _, query := range []string{"botId=87654321&type=private&id=303", "botId=87654321&type=group&id=304"} {
		status, _, e = do("GET", "/api/agent/tasks/read?"+query+"&taskId="+v["taskId"].(string), nil, "")
		if e != nil || status != 404 {
			return fmt.Errorf("exact task read scope leaked: %d %v", status, e)
		}
	}
	status, beforeSave, e := do("GET", taskPath, nil, "")
	beforeRows, _ := beforeSave["items"].([]any)
	if e != nil || status != 200 || len(beforeRows) == 0 {
		return fmt.Errorf("save/read context baseline unavailable")
	}
	baseline := beforeRows[0].(map[string]any)
	savedTask, e := run("smoke-knowledge-save", "KNOWLEDGE_SAVE_CHECK", "KNOWLEDGE_SAVE_OK", 3, 3)
	if e != nil {
		return e
	}
	if !smokeMemorySourceDelta(baseline, savedTask) {
		return fmt.Errorf("save/read context sources did not increase by one: before=%v after=%v", baseline["context"].(map[string]any)["memorySources"], savedTask["context"].(map[string]any)["memorySources"])
	}
	time.Sleep(260 * time.Millisecond)
	searchSaved := scope()
	searchSaved["keyword"] = "KNOWLEDGE_SAVED_FROM_RESULT"
	status, savedPage, e := do("POST", "/api/agent/memories/search", searchSaved, csrf)
	savedRows, _ := savedPage["memories"].([]any)
	if e != nil || status != 200 || len(savedRows) != 1 || savedRows[0].(map[string]any)["sourceSeq"] != savedTask["sourceSeq"] || savedRows[0].(map[string]any)["revision"] != "1" || savedRows[0].(map[string]any)["source"] != "agent" {
		return fmt.Errorf("saved reference task scope/provenance or dedup count mismatch")
	}
	for _, filter := range []struct {
		state string
		count int
	}{{"completed", 9}, {"cancelled", 3}, {"running", 0}} {
		if e = checkPages(taskPath+"&state="+filter.state, "items", "taskId", filter.count, func(v map[string]any) bool { return v["status"] == filter.state }); e != nil {
			return e
		}
	}
	for _, filter := range []struct {
		id, source, keyword string
		count               int
	}{
		{"303", "admin", "KNOWLEDGE_", 20}, {"303", "agent", "KNOWLEDGE_", 1},
		{"303", "legacy", "KNOWLEDGE_", 0}, {"404", "legacy", "synthetic legacy", 1},
		{"303", "admin", "", 20}, {"303", "legacy", "", 0}, {"404", "legacy", "", 1},
	} {
		time.Sleep(260 * time.Millisecond)
		query := scope()
		query["id"], query["source"], query["keyword"] = filter.id, filter.source, filter.keyword
		if filter.keyword == "" {
			delete(query, "keyword")
		}
		status, page, e := do("POST", "/api/agent/memories/search", query, csrf)
		rows, ok := page["memories"].([]any)
		if e != nil || status != 200 || !ok || len(rows) != filter.count || page["source"] != filter.source {
			return fmt.Errorf("memory source HTTP filter mismatch: %s %d %v", filter.source, status, e)
		}
		for _, raw := range rows {
			if raw.(map[string]any)["source"] != filter.source {
				return fmt.Errorf("memory source HTTP filter returned another provenance")
			}
		}
	}
	return nil
}

func smokeMemoryMatchesTrigger(memory, task, entry map[string]any) bool {
	seq := memory["sourceSeq"]
	text, _ := entry["text"].(string)
	return memory["source"] == "agent" && smokePositiveID(seq) && seq == task["sourceSeq"] && seq == entry["seq"] && entry["kind"] == "event" && strings.Contains(text, "ROLE_AGENT_CHECK")
}

func verifyAgentMemory(do func(string, string, any, string) (int, map[string]any, error), csrf string) error {
	status, page, err := do("GET", "/api/agent/memories?botId=87654321&type=group&id=303", nil, "")
	if err != nil || status != 200 || page["mode"] != "managed" {
		return fmt.Errorf("managed memory unavailable: %d %v", status, err)
	}
	items, ok := page["items"].([]any)
	if !ok || len(items) != 1 {
		return fmt.Errorf("local save_memory did not deduplicate three equal scoped writes")
	}
	item := items[0].(map[string]any)
	if !smokePositiveID(item["sourceSeq"]) || item["source"] != "agent" {
		return fmt.Errorf("memory source not captured")
	}
	status, tasks, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=303&state=completed", nil, "")
	completed, _ := tasks["items"].([]any)
	if err != nil || status != 200 || len(completed) != 5 {
		return fmt.Errorf("memory trigger task unavailable")
	}
	first := completed[len(completed)-1].(map[string]any)
	status, source, err := do("GET", "/api/agent/history?botId=87654321&type=group&id=303&archived=true&seq="+item["sourceSeq"].(string), nil, "")
	entries, _ := source["entries"].([]any)
	if err != nil || status != 200 || len(entries) != 1 || !smokeMemoryMatchesTrigger(item, first, entries[0].(map[string]any)) {
		return fmt.Errorf("memory did not retain its first task's exact trigger through repeated message IDs and dedup")
	}
	change := map[string]any{"botId": "87654321", "type": "group", "id": "303", "memoryId": item["id"], "revision": item["revision"], "action": "disable", "text": ""}
	status, _, err = do("POST", "/api/agent/memories/update", change, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("memory disable failed")
	}
	status, _, err = do("POST", "/api/agent/memories/update", change, csrf)
	if err != nil || status != 409 {
		return fmt.Errorf("memory stale version accepted")
	}
	status, versions, err := do("GET", "/api/agent/memories?botId=87654321&type=group&id=303&memoryId="+item["id"].(string), nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("memory versions unavailable")
	}
	if rows, ok := versions["items"].([]any); !ok || len(rows) != 2 {
		return fmt.Errorf("memory revisions missing")
	}
	scope := map[string]any{"botId": "87654321", "type": "group", "id": "404"}
	status, preview, err := do("POST", "/api/agent/memories/migration-preview", scope, csrf)
	if err != nil || status != 200 || preview["count"] != float64(1) {
		return fmt.Errorf("legacy migration preview failed: %d %v", status, err)
	}
	scope["ticket"] = preview["ticket"]
	for i := 0; i < 2; i++ {
		status, _, err = do("POST", "/api/agent/memories/migrate", scope, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("legacy migration/retry failed")
		}
	}
	status, migrated, err := do("GET", "/api/agent/memories?botId=87654321&type=group&id=404", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("migrated memory unavailable")
	}
	if rows, ok := migrated["items"].([]any); !ok || len(rows) != 1 {
		return fmt.Errorf("migration duplicated memory")
	}
	return nil
}

func verifyAgentArchive(do func(string, string, any, string) (int, map[string]any, error), csrf string) error {
	status, list, err := do("GET", "/api/agent/conversations?botId=87654321", nil, "")
	if err != nil || status != 200 || list["available"] != true {
		return fmt.Errorf("archive runtime list failed: %d %v", status, err)
	}
	for _, scope := range []string{"group", "private"} {
		status, page, err := do("GET", "/api/agent/history?botId=87654321&type="+scope+"&id=303", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("archive runtime read failed: %d %v", status, err)
		}
		entries, ok := page["entries"].([]any)
		if !ok || (scope == "group" && len(entries) == 0) || (scope == "private" && len(entries) != 0) {
			return fmt.Errorf("archive runtime conversation isolation failed")
		}
		encoded, _ := json.Marshal(page)
		if bytes.Contains(encoded, []byte("synthetic-model-key")) {
			return fmt.Errorf("archive exposed synthetic credential")
		}
	}
	status, page, err := do("POST", "/api/agent/history/search", map[string]any{"botId": "87654321", "type": "group", "id": "303", "keyword": "ROLE_AGENT_CHECK"}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("archive runtime search failed: %d %v", status, err)
	}
	entries, ok := page["entries"].([]any)
	if !ok || len(entries) == 0 {
		return fmt.Errorf("archive runtime search missing synthetic event")
	}
	selected := entries[0].(map[string]any)
	for _, archived := range []bool{false, true} {
		query := url.Values{"botId": {"87654321"}, "type": {"group"}, "id": {"303"}, "seq": {fmt.Sprint(selected["seq"])}, "archived": {strconv.FormatBool(archived)}}
		status, exact, err := do("GET", "/api/agent/history?"+query.Encode(), nil, "")
		rows, ok := exact["entries"].([]any)
		if err != nil || status != 200 || exact["available"] != true || !ok || len(rows) != 1 || exact["nextCursor"] != "" {
			return fmt.Errorf("archive exact lookup failed")
		}
		row, _ := rows[0].(map[string]any)
		text, textOK := row["text"].(string)
		if row["seq"] != selected["seq"] || row["kind"] != selected["kind"] || row["time"] != selected["time"] || !textOK || !strings.Contains(text, "ROLE_AGENT_CHECK") || len(text) > 8000 {
			return fmt.Errorf("archive exact lookup returned wrong source")
		}
		for _, absent := range []struct{ key, value string }{{"type", "private"}, {"seq", "9223372036854775807"}} {
			missingQuery := url.Values{}
			for key, values := range query {
				missingQuery[key] = append([]string(nil), values...)
			}
			missingQuery.Set(absent.key, absent.value)
			code, missing, err := do("GET", "/api/agent/history?"+missingQuery.Encode(), nil, "")
			items, ok := missing["entries"].([]any)
			if err != nil || code != 200 || !ok || len(items) != 0 {
				return fmt.Errorf("archive exact lookup scope or missing sequence mismatch")
			}
		}
	}
	// Read neighboring events through the final program's authenticated HTTP
	// path, using an actual search source rather than a guessed sequence.
	var anchor string
	for _, entry := range entries {
		row := entry.(map[string]any)
		if row["kind"] == "event" {
			anchor, _ = row["seq"].(string)
			break
		}
	}
	if anchor == "" {
		return fmt.Errorf("archive context search has no event source")
	}
	contextPath := "/api/agent/history?botId=87654321&type=group&id=303&seq=" + anchor + "&context=1"
	anchorSender := ""
	for radius := 1; radius <= 3; radius++ {
		status, neighbors, err := do("GET", strings.Replace(contextPath, "context=1", "context="+strconv.Itoa(radius), 1), nil, "")
		if err != nil || status != 200 || neighbors["available"] != true || neighbors["anchorSeq"] != anchor || neighbors["context"] != float64(radius) || neighbors["nextCursor"] != "" {
			return fmt.Errorf("archive source context HTTP failed: %d %v", status, err)
		}
		rows, ok := neighbors["entries"].([]any)
		if !ok || len(rows) < 2 || len(rows) > 2*radius+1 {
			return fmt.Errorf("archive source context missing neighbors")
		}
		anchorNumber, _ := strconv.ParseInt(anchor, 10, 64)
		previous := int64(0)
		center, older, newer, totalBytes := 0, 0, 0, 0
		for index, item := range rows {
			row, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("archive context invalid event")
			}
			seq, _ := row["seq"].(string)
			number, e := strconv.ParseInt(seq, 10, 64)
			text, textOK := row["text"].(string)
			limit := 2000
			switch {
			case number == anchorNumber:
				center++
				limit = 8000
				var senderErr error
				anchorSender, senderErr = smokeArchiveSender(text)
				if senderErr != nil {
					return senderErr
				}
			case number < anchorNumber:
				older++
			default:
				newer++
			}
			if e != nil || number <= 0 || strconv.FormatInt(number, 10) != seq || row["kind"] != "event" || !textOK || !utf8.ValidString(text) || len(text) > limit || (index > 0 && number >= previous) {
				return fmt.Errorf("archive context event order/type/budget invalid")
			}
			previous = number
			totalBytes += len(text)
		}
		if center != 1 || older > radius || newer > radius || totalBytes > 16384 {
			return fmt.Errorf("archive context anchor/radius mismatch")
		}
	}
	for _, path := range []string{strings.Replace(contextPath, "type=group", "type=private", 1), strings.Replace(contextPath, "seq="+anchor, "seq=9223372036854775807", 1)} {
		status, absent, e := do("GET", path, nil, "")
		rows, ok := absent["entries"].([]any)
		if e != nil || status != 200 || !ok || len(rows) != 0 {
			return fmt.Errorf("archive context absent/cross-scope anchor leaked neighbors")
		}
	}
	for _, path := range []string{contextPath + "&archived=true", contextPath + "&context=2", strings.Replace(contextPath, "context=1", "context=4", 1)} {
		status, _, e := do("GET", path, nil, "")
		if e != nil || status != 400 {
			return fmt.Errorf("archive context invalid range accepted")
		}
	}
	stamp, err := time.Parse(time.RFC3339Nano, fmt.Sprint(selected["time"]))
	if err != nil {
		return fmt.Errorf("archive runtime event has invalid creation time")
	}
	for _, tc := range []struct {
		from, until time.Time
		before      string
		found       bool
	}{
		{stamp, stamp.Add(time.Nanosecond), "", true},
		{stamp.Add(-time.Nanosecond), stamp, "", false},
		{stamp, stamp.Add(time.Nanosecond), fmt.Sprint(selected["seq"]), false},
	} {
		time.Sleep(300 * time.Millisecond) // Honor the existing search rate limit.
		body := map[string]any{"botId": "87654321", "type": "group", "id": "303", "from": tc.from.In(time.FixedZone("fixture", 8*3600)).Format(time.RFC3339Nano), "until": tc.until.Format(time.RFC3339Nano)}
		if tc.before != "" {
			body["before"] = tc.before
		}
		status, page, err = do("POST", "/api/agent/history/search", body, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("archive runtime time search failed: %d %v", status, err)
		}
		rows, ok := page["entries"].([]any)
		if !ok {
			return fmt.Errorf("archive time search entries missing")
		}
		found := false
		for _, row := range rows {
			if row.(map[string]any)["seq"] == selected["seq"] {
				found = true
			}
		}
		if found != tc.found {
			return fmt.Errorf("archive time/sequence boundary mismatch")
		}
	}
	for _, keyword := range []string{"ROLE_AGENT_CHECK", ""} {
		for _, sender := range []string{anchorSender, "9223372036854775807"} {
			time.Sleep(300 * time.Millisecond)
			status, page, err := do("POST", "/api/agent/history/search", map[string]any{"botId": "87654321", "type": "group", "id": "303", "keyword": keyword, "sender": sender}, csrf)
			rows, ok := page["entries"].([]any)
			if err != nil || status != 200 || !ok || page["sender"] != sender || (sender == anchorSender && len(rows) == 0) || (sender != anchorSender && len(rows) != 0) {
				return fmt.Errorf("archive sender HTTP search mismatch")
			}
			for _, raw := range rows {
				row, _ := raw.(map[string]any)
				text, _ := row["text"].(string)
				actual, err := smokeArchiveSender(text)
				if err != nil || actual != sender || row["kind"] != "event" {
					return fmt.Errorf("archive sender search matched a mention or tool")
				}
			}
		}
	}
	for _, eventType := range []string{"message", "notice", "request"} {
		time.Sleep(300 * time.Millisecond)
		status, page, err := do("POST", "/api/agent/history/search", map[string]any{"botId": "87654321", "type": "group", "id": "303", "eventType": eventType}, csrf)
		rows, ok := page["entries"].([]any)
		if err != nil || status != 200 || !ok || page["eventType"] != eventType || (eventType == "message" && len(rows) == 0) || (eventType != "message" && len(rows) != 0) {
			return fmt.Errorf("archive type-only HTTP search mismatch: %s %d %v", eventType, status, err)
		}
		for _, raw := range rows {
			row := raw.(map[string]any)
			var event map[string]any
			if row["kind"] != "event" || json.Unmarshal([]byte(row["text"].(string)), &event) != nil || event["post_type"] != eventType {
				return fmt.Errorf("archive event type matched a tool or different post_type")
			}
		}
	}
	status, _, err = do("GET", "/api/agent/history?botId=99999999&type=group&id=303", nil, "")
	if err != nil || status != 404 {
		return fmt.Errorf("archive unknown bot not rejected")
	}
	return nil
}

func verifyAIRolePermissions(do func(string, string, any, string) (int, map[string]any, error), private func(int64, string) (string, error), send func(int64, int64, string, string, bool) (string, error), csrf, modelURL string, inFlight func(string) bool) error {
	switchAgent := func(mode string) error {
		status, list, err := do("GET", "/api/plugins", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("role plugin snapshot failed")
		}
		for _, item := range list["plugins"].([]any) {
			p := item.(map[string]any)
			if p["name"] != "aichat" {
				continue
			}
			state := p["state"].(map[string]any)
			status, _, err = do("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": mode, "revision": state["revision"]}, csrf)
			if err != nil || status != 200 {
				return fmt.Errorf("role agent plugin switch failed")
			}
			return nil
		}
		return fmt.Errorf("role agent plugin missing")
	}
	read := func(target string) (map[string]any, error) {
		status, value, err := do("GET", "/api/ai?"+target, nil, "")
		if err != nil || status != 200 {
			return nil, fmt.Errorf("role snapshot failed: %v", err)
		}
		return value, nil
	}
	configure := func(target string, useAgent bool) error {
		value, err := read(target)
		if err != nil {
			return err
		}
		for _, item := range value["groups"].([]any) {
			c := item.(map[string]any)
			change := map[string]any{"service": c["service"], "revision": c["revision"], "temperature": 50}
			if strings.HasPrefix(target, "group=") {
				change["groupId"] = strings.TrimPrefix(target, "group=")
			} else {
				change["privateId"] = strings.TrimPrefix(target, "private=")
			}
			if c["service"] == "aichatcfg" {
				change["rate"], change["useAgent"], change["respondAt"], change["voiceOutput"] = 0, useAgent, useAgent, false
			}
			status, _, err := do("POST", "/api/ai-group", change, csrf)
			if err != nil || status != 200 {
				return fmt.Errorf("role conversation setup failed: %v", err)
			}
		}
		return nil
	}
	if err := configure("group=303", false); err != nil {
		return err
	}
	if err := configure("private=23456789", false); err != nil {
		return err
	}
	if text, err := private(12345678, "/全局响应"); err != nil || !strings.Contains(text, "工作") {
		return fmt.Errorf("role response setup failed")
	}
	for _, c := range []struct {
		name, role string
		uid, gid   int64
		allow      bool
	}{
		{"member", "member", 23456789, 303, false},
		{"admin", "admin", 23456789, 303, true},
		{"group owner", "owner", 34567890, 303, true},
		{"private non-owner", "member", 23456789, 0, false},
	} {
		target := "group=303"
		if c.gid == 0 {
			target = "private=23456789"
		}
		before, err := read(target)
		if err != nil {
			return err
		}
		text, err := send(c.uid, c.gid, c.role, "设置AI聊天温度65", c.gid > 0)
		if err != nil || (c.allow && !strings.Contains(text, "成功")) || (!c.allow && text != "") {
			return fmt.Errorf("role temperature command failed: %s %q %v", c.name, text, err)
		}
		after, err := read(target)
		if err != nil {
			return err
		}
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		if !c.allow && !bytes.Equal(beforeJSON, afterJSON) {
			return fmt.Errorf("denied setting changed state: %s", c.name)
		}
		if c.allow && after["groups"].([]any)[0].(map[string]any)["temperature"] != float64(65) {
			return fmt.Errorf("allowed setting did not persist: %s", c.name)
		}
		for _, command := range []string{"设置AI聊天模型名forbidden-role-model", "设置AI聊天不使用Agent模式", "重置AI聊天Agent", "查看AI聊天配置"} {
			if text, err = send(c.uid, c.gid, c.role, command, c.gid > 0); err != nil || text != "" {
				return fmt.Errorf("owner-only command accepted: %s %s %q %v", c.name, command, text, err)
			}
		}
		final, err := read(target)
		if err != nil {
			return err
		}
		finalJSON, _ := json.Marshal(final)
		if !bytes.Equal(afterJSON, finalJSON) {
			return fmt.Errorf("owner-only rejection changed config: %s", c.name)
		}
	}
	for _, c := range []struct {
		gid int64
		cmd string
	}{{303, "/gpt role member"}, {0, "/gpt role private"}} {
		text, err := send(23456789, c.gid, "member", c.cmd, false)
		if err != nil || !strings.Contains(text, "本地模型模拟回复") {
			return fmt.Errorf("non-owner GPT rejected: %v %q", err, text)
		}
	}
	value, err := read("group=303")
	if err != nil {
		return err
	}
	global := value["global"].(map[string]any)
	status, _, err := do("POST", "/api/ai", map[string]any{"revision": global["revision"], "fields": map[string]any{"AgentAPI": modelURL, "AgentModelName": "smoke-role-agent", "AgentType": 0}, "keys": map[string]any{"AgentKey": map[string]any{"action": "replace", "value": "synthetic-model-key"}}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("role agent setup failed: %v", err)
	}
	if err := configure("group=303", true); err != nil {
		return err
	}
	if err := switchAgent("enabled"); err != nil {
		return err
	}
	personaPath := "/api/ai-persona?botId=87654321&type=group&id=303"
	status, persona, err := do("GET", personaPath, nil, "")
	if err != nil || status != 200 || persona["mode"] != "inherit" {
		return fmt.Errorf("persona baseline unavailable: %v", err)
	}
	status, _, err = do("POST", "/api/ai-persona", map[string]any{"botId": "87654321", "type": "group", "id": "303", "revision": persona["revision"], "mode": "custom", "character": "SMOKE_CONVERSATION_PERSONA", "sex": "女"}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("persona override save failed: %v", err)
	}
	status, persona, err = do("GET", "/api/ai-persona?botId=87654321&type=private&id=303", nil, "")
	if err != nil || status != 200 || persona["mode"] != "inherit" || persona["character"] == "SMOKE_CONVERSATION_PERSONA" {
		return fmt.Errorf("persona crossed conversation type")
	}
	for _, c := range []struct {
		uid             int64
		role, cmd, want string
	}{
		{23456789, "member", "ROLE_AGENT_CHECK", "ROLE_AGENT_user"},
		{23456789, "admin", "ROLE_AGENT_CHECK", "ROLE_AGENT_admin"},
		{34567890, "owner", "ROLE_AGENT_CHECK", "ROLE_AGENT_admin"},
		{23456789, "member", "ROLE_AGENT_CROSS_INFO", "ROLE_BLOCKED_OK"},
		{23456789, "member", "ROLE_AGENT_CQ_ALL", "ROLE_BLOCKED_OK"},
	} {
		text, err := send(c.uid, 303, c.role, c.cmd, true)
		if err != nil || text != c.want {
			return fmt.Errorf("actual Agent role/action failed: %s %s got %q want %q error %v", c.role, c.cmd, text, c.want, err)
		}
		// Allow the message hook to record the API response before updating roles.
		time.Sleep(100 * time.Millisecond)
	}
	status, persona, err = do("GET", personaPath, nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("persona latest state unavailable")
	}
	status, _, err = do("POST", "/api/ai-persona", map[string]any{"botId": "87654321", "type": "group", "id": "303", "revision": persona["revision"], "mode": "inherit"}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("persona inherit save failed")
	}
	if err := verifyAgentTaskRuntime(do, send, csrf, inFlight); err != nil {
		return err
	}
	if err := verifyAgentMemory(do, csrf); err != nil {
		return err
	}
	if err := verifyAgentKnowledgeRuntime(do, send, csrf); err != nil {
		return err
	}
	value, err = read("group=404")
	if err != nil {
		return err
	}
	global = value["global"].(map[string]any)
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": global["revision"], "fields": map[string]any{"AgentModelName": "smoke-vision-agent", "ImageAPI": modelURL, "ImageType": 0, "ImageModelName": "smoke-vision-model"}, "keys": map[string]any{"ImageKey": map[string]any{"action": "replace", "value": "synthetic-model-key"}}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("vision setup failed: %d %v", status, err)
	}
	if err := configure("group=404", true); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		text, err := send(23456789, 404, "member", "VISION_IMAGE_CHECK", true)
		if err != nil || text != "VISION_IMAGE_OK" {
			return fmt.Errorf("actual bounded vision flow failed: %q %v", text, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, visionTasks, err := do("GET", "/api/agent/tasks?botId=87654321&type=group&id=404&state=completed", nil, "")
	if err != nil || status != 200 {
		return fmt.Errorf("vision task summaries unavailable")
	}
	visionRows, _ := visionTasks["items"].([]any)
	if len(visionRows) != 2 {
		return fmt.Errorf("vision task count incorrect")
	}
	for _, raw := range visionRows {
		v, _ := raw.(map[string]any)
		a, _ := v["activity"].(map[string]any)
		if !smokeTaskTimingValid(v) || !smokeTaskResultsMatch(v, 1, 0, 0, 0) || !smokeTaskQueriesMatch(v, 0, 0, 0) || a["modelPreparations"] != float64(1) || a["localActions"] != float64(0) || a["platformActions"] != float64(1) {
			return fmt.Errorf("vision task miscounted preparation, query or action")
		}
	}
	if err := configure("group=404", false); err != nil {
		return err
	}
	if err := verifyVisionDescriptionRuntime(do, send, csrf, configure); err != nil {
		return err
	}
	if err := verifyAgentOutputLimits(do, send, csrf, configure); err != nil {
		return err
	}
	if err := verifyAgentPreparationCancellation(do, send, csrf, configure, inFlight); err != nil {
		return err
	}
	value, err = read("group=404")
	if err != nil {
		return err
	}
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": value["global"].(map[string]any)["revision"], "fields": map[string]any{"ImageAPI": "", "ImageModelName": ""}, "keys": map[string]any{"ImageKey": map[string]any{"action": "clear", "confirm": true}}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("vision configuration cleanup failed")
	}
	if err := configure("group=303", false); err != nil {
		return err
	}
	if err := switchAgent("disabled"); err != nil {
		return err
	}
	value, err = read("group=303")
	if err != nil {
		return err
	}
	global = value["global"].(map[string]any)
	status, _, err = do("POST", "/api/ai", map[string]any{"revision": global["revision"], "fields": map[string]any{"AgentAPI": "", "AgentModelName": ""}, "keys": map[string]any{"AgentKey": map[string]any{"action": "clear", "confirm": true}}}, csrf)
	if err != nil || status != 200 {
		return fmt.Errorf("role agent model cleanup failed")
	}
	if text, err := private(12345678, "/全局沉默"); err != nil || !strings.Contains(text, "休息") {
		return fmt.Errorf("role response cleanup failed")
	}
	return nil
}

func smokePreparationStoppedTask(task map[string]any) bool {
	a, _ := task["activity"].(map[string]any)
	return task["status"] == "cancelled" && smokeTaskTimingValid(task) && smokeTaskResultsMatch(task, 0, 0, 0, 0) && smokeTaskQueriesMatch(task, 0, 0, 0) && a["available"] == true && a["modelPreparations"] == float64(1) && a["fallbackPreparations"] == float64(0) && a["localActions"] == float64(0) && a["platformActions"] == float64(0)
}

func verifyAgentPreparationCancellation(do func(string, string, any, string) (int, map[string]any, error), send func(int64, int64, string, string, bool) (string, error), csrf string, configure func(string, bool) error, inFlight func(string) bool) error {
	for i, phase := range []string{"download", "vision"} {
		gid := int64(505 + i)
		status, snapshot, err := do("GET", "/api/ai", nil, "")
		if err != nil || status != 200 {
			return fmt.Errorf("preparation settings read failed")
		}
		status, _, err = do("POST", "/api/ai", map[string]any{"revision": snapshot["global"].(map[string]any)["revision"], "fields": map[string]any{"AgentModelName": "smoke-prepare-agent-" + phase, "ImageModelName": "smoke-prepare-vision"}}, csrf)
		if err != nil || status != 200 {
			return fmt.Errorf("preparation model setup failed")
		}
		if err = configure("group="+strconv.FormatInt(gid, 10), true); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() {
			text, err := send(23456789, gid, "member", "PREPARE_CANCEL_"+phase, true)
			if err == nil && text != "" {
				err = fmt.Errorf("stopped preparation sent a reply: %q", text)
			}
			done <- err
		}()
		endpoint := "/api/agent/tasks?botId=87654321&type=group&id=" + strconv.FormatInt(gid, 10)
		var stopped map[string]any
		var request map[string]any
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			status, page, err := do("GET", endpoint, nil, "")
			rows, _ := page["items"].([]any)
			if err != nil || status != 200 {
				return fmt.Errorf("preparation task read failed")
			}
			if len(rows) > 0 && inFlight("prepare-"+phase) {
				v, _ := rows[0].(map[string]any)
				if v["status"] == "running" && v["phase"] == "model_wait" {
					for _, filter := range []string{"model_wait", "action_wait"} {
						code, filtered, e := do("GET", endpoint+"&state=running&reason=none&phase="+filter, nil, "")
						items, ok := filtered["items"].([]any)
						want := 0
						if filter == "model_wait" {
							want = 1
						}
						if e != nil || code != 200 || !ok || len(items) != want {
							return fmt.Errorf("live task phase filter mismatch: %s", filter)
						}
						if want == 1 {
							row, _ := items[0].(map[string]any)
							if row["taskId"] != v["taskId"] || row["version"] != v["version"] {
								return fmt.Errorf("phase filter changed task snapshot")
							}
						}
					}
					request = map[string]any{"botId": "87654321", "type": "group", "id": strconv.FormatInt(gid, 10), "taskId": v["taskId"], "version": v["version"]}
					status, stopped, err = do("POST", "/api/agent/tasks/cancel", request, csrf)
					if err != nil {
						return err
					}
					if status == 200 {
						break
					}
					if status != 409 {
						return fmt.Errorf("preparation cancel failed: %d", status)
					}
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		if stopped == nil || stopped["status"] != "cancel_requested" || !smokeTaskTimingValid(stopped) {
			return fmt.Errorf("preparation stop not admitted in flight: %s", phase)
		}
		select {
		case err := <-done:
			if err != nil {
				return err
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("stopped preparation did not settle")
		}
		status, terminal, err := do("GET", "/api/agent/tasks/read?botId=87654321&type=group&id="+strconv.FormatInt(gid, 10)+"&taskId="+stopped["taskId"].(string), nil, "")
		if err != nil || status != 200 || !smokePreparationStoppedTask(terminal) || terminal["cancelRequestedAt"] != stopped["cancelRequestedAt"] {
			return fmt.Errorf("preparation terminal summary incorrect: %s status=%v error=%v", phase, terminal["status"], err)
		}
		status, again, err := do("POST", "/api/agent/tasks/cancel", request, csrf)
		if err != nil || status != 200 || !smokePreparationStoppedTask(again) || again["cancelRequestedAt"] != stopped["cancelRequestedAt"] {
			return fmt.Errorf("preparation duplicate stop changed terminal")
		}
		if err = configure("group="+strconv.FormatInt(gid, 10), false); err != nil {
			return err
		}
	}
	return nil
}

func smokeTaskTimingValid(task map[string]any) bool {
	valid := func(value any) bool {
		stamp, ok := value.(string)
		if !ok || !strings.HasSuffix(stamp, "Z") {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, stamp)
		return err == nil
	}
	if task["persisted"] != true || !valid(task["startedAt"]) || !valid(task["phaseStartedAt"]) {
		return false
	}
	if task["phase"] == "finished" && (!valid(task["endedAt"]) || task["phaseStartedAt"] != task["endedAt"]) {
		return false
	}
	if stop, exists := task["cancelRequestedAt"]; exists && !valid(stop) {
		return false
	}
	if task["status"] == "cancel_requested" || task["status"] == "cancelled" {
		return valid(task["cancelRequestedAt"])
	}
	return true
}

func smokeTaskObservationValid(snapshot, task map[string]any) bool {
	text, _ := snapshot["observedAt"].(string)
	observed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil || !strings.HasSuffix(text, "Z") {
		return false
	}
	for _, field := range []string{"startedAt", "phaseStartedAt", "cancelRequestedAt", "endedAt"} {
		text, _ := task[field].(string)
		if text == "" {
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, text)
		if err != nil || observed.Before(stamp) {
			return false
		}
	}
	return true
}

func smokeTaskQueriesMatch(task map[string]any, used, successful, empty int) bool {
	q, _ := task["queries"].(map[string]any)
	return task["persisted"] == true && q["available"] == true && q["limit"] == float64(3) && q["used"] == float64(used) && q["successful"] == float64(successful) && q["empty"] == float64(empty) && q["errors"] == float64(0) && q["denied"] == float64(0)
}

func smokeTaskResultsMatch(task map[string]any, ok, rejected, unknown, localErrors int) bool {
	r, _ := task["results"].(map[string]any)
	return task["persisted"] == true && r["available"] == true && r["ok"] == float64(ok) && r["rejected"] == float64(rejected) && r["unknown"] == float64(unknown) && r["localErrors"] == float64(localErrors)
}

func verifyAgentTaskRuntime(do func(string, string, any, string) (int, map[string]any, error), send func(int64, int64, string, string, bool) (string, error), csrf string, inFlight func(string) bool) error {
	const endpoint = "/api/agent/tasks?botId=87654321&type=group&id=303"
	status, page, e := do("GET", endpoint, nil, "")
	if e != nil || status != 200 {
		return fmt.Errorf("task list unavailable")
	}
	items, ok := page["items"].([]any)
	if !ok || len(items) != 5 {
		return fmt.Errorf("role task count mismatch")
	}
	for _, raw := range items {
		v := raw.(map[string]any)
		if v["status"] != "completed" || v["persisted"] != true || v["context"].(map[string]any)["available"] != true {
			return fmt.Errorf("role task terminal/context missing")
		}
		if !smokeTaskObservationValid(page, v) {
			return fmt.Errorf("completed task page missing observation snapshot")
		}
		if !smokePositiveID(v["sourceSeq"]) {
			return fmt.Errorf("role task trigger source missing")
		}
		a := v["activity"].(map[string]any)
		if !smokeTaskResultsMatch(v, int(a["localActions"].(float64)+a["platformActions"].(float64)), 0, 0, 0) {
			return fmt.Errorf("role task settled results mismatch")
		}
		queryCount := 0
		if a["localActions"] == float64(2) {
			queryCount = 1
		}
		if !smokeTaskQueriesMatch(v, queryCount, queryCount, 0) {
			return fmt.Errorf("role task query summary mismatch")
		}
		status, source, e := do("GET", "/api/agent/history?botId=87654321&type=group&id=303&archived=true&seq="+v["sourceSeq"].(string), nil, "")
		rows, _ := source["entries"].([]any)
		if e != nil || status != 200 || len(rows) != 1 || rows[0].(map[string]any)["kind"] != "event" || !strings.Contains(rows[0].(map[string]any)["text"].(string), "ROLE_AGENT_") {
			return fmt.Errorf("role task trigger event mismatch: %d %v", status, e)
		}
	}
	for _, phase := range []string{"model", "action", "fallback"} {
		status, snapshot, e := do("GET", "/api/ai", nil, "")
		if e != nil || status != 200 {
			return fmt.Errorf("task model setup read failed")
		}
		fields := map[string]any{"AgentModelName": "smoke-task-" + phase + "-wait"}
		oldChatModel := snapshot["global"].(map[string]any)["fields"].(map[string]any)["ModelName"]
		if phase == "fallback" {
			fields["ModelName"] = "smoke-task-chat-wait"
		}
		status, _, e = do("POST", "/api/ai", map[string]any{"revision": snapshot["global"].(map[string]any)["revision"], "fields": fields}, csrf)
		if e != nil || status != 200 {
			return fmt.Errorf("task model setup failed")
		}
		done := make(chan error, 1)
		go func() {
			text, e := send(23456789, 303, "member", "TASK_CANCEL_"+phase, true)
			if e == nil && text != "" {
				e = fmt.Errorf("cancelled task sent unexpected reply %q", text)
			}
			done <- e
		}()
		var stopped map[string]any
		var waiting map[string]any
		var request map[string]any
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			status, page, e = do("GET", endpoint, nil, "")
			if e != nil || status != 200 {
				return fmt.Errorf("task poll failed")
			}
			rows := page["items"].([]any)
			if len(rows) > 0 {
				v := rows[0].(map[string]any)
				waitPhase := phase + "_wait"
				if phase == "fallback" {
					waitPhase = "fallback_model_wait"
				}
				// The phase starts before prompt preparation. Only cancel after the
				// fake server has received the request whose in-flight result we test.
				if v["status"] == "running" && v["phase"] == waitPhase && inFlight(phase) {
					if !smokeTaskObservationValid(page, v) {
						return fmt.Errorf("task page missing server observation snapshot")
					}
					status, exact, e := do("GET", "/api/agent/tasks/read?botId=87654321&type=group&id=303&taskId="+v["taskId"].(string), nil, "")
					if e != nil || status != 200 || exact["taskId"] != v["taskId"] || exact["status"] != "running" {
						return fmt.Errorf("in-flight task precise read failed: %d %v", status, e)
					}
					if !smokeTaskResultsMatch(exact, 0, 0, 0, 0) {
						return fmt.Errorf("pending action counted as settled")
					}
					if !smokeTaskTimingValid(exact) || exact["phaseStartedAt"] != v["phaseStartedAt"] {
						return fmt.Errorf("in-flight stage timestamp missing or changed by read")
					}
					if !smokeTaskObservationValid(exact, exact) {
						return fmt.Errorf("exact task missing server observation snapshot")
					}
					waiting = exact
					outcome := "none"
					if phase == "action" {
						outcome = "in_flight"
					}
					filterPath := endpoint + "&state=running&reason=none&phase=" + waitPhase + "&outcome=" + outcome
					code, filtered, err := do("GET", filterPath, nil, "")
					filteredRows, ok := filtered["items"].([]any)
					if err != nil || code != 200 || !ok || len(filteredRows) != 1 {
						return fmt.Errorf("in-flight outcome filter missing task")
					}
					filteredTask, _ := filteredRows[0].(map[string]any)
					if filteredTask["taskId"] != v["taskId"] || filteredTask["version"] != v["version"] {
						return fmt.Errorf("outcome read changed in-flight task")
					}
					request = map[string]any{"botId": "87654321", "type": "group", "id": "303", "taskId": v["taskId"], "version": v["version"]}
					status, stopped, e = do("POST", "/api/agent/tasks/cancel", request, csrf)
					if e != nil {
						return e
					}
					if status == 200 {
						break
					}
					if status != 409 {
						return fmt.Errorf("task cancel status %d", status)
					}
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		if stopped == nil || stopped["status"] != "cancel_requested" {
			return fmt.Errorf("task cancel did not reach in-flight %s", phase)
		}
		if !smokeTaskTimingValid(stopped) || stopped["phaseStartedAt"] != waiting["phaseStartedAt"] {
			return fmt.Errorf("stop timestamp missing or stop reset in-flight stage time")
		}
		if !smokeTaskObservationValid(stopped, stopped) {
			return fmt.Errorf("stop response missing observation snapshot")
		}
		status, again, e := do("POST", "/api/agent/tasks/cancel", request, csrf)
		if e != nil || status != 200 || again["taskId"] != stopped["taskId"] {
			return fmt.Errorf("task cancel retry not idempotent")
		}
		if !smokeTaskTimingValid(again) || again["cancelRequestedAt"] != stopped["cancelRequestedAt"] {
			return fmt.Errorf("duplicate stop changed first request time")
		}
		if !smokeTaskObservationValid(again, again) {
			return fmt.Errorf("repeated stop response missing observation snapshot")
		}
		if e = <-done; e != nil {
			return e
		}
		status, page, e = do("GET", endpoint, nil, "")
		if e != nil || status != 200 {
			return fmt.Errorf("task terminal unavailable")
		}
		v := page["items"].([]any)[0].(map[string]any)
		if v["status"] != "cancelled" || v["persisted"] != true || v["taskId"] != stopped["taskId"] {
			return fmt.Errorf("task terminal incorrect: %v", v["status"])
		}
		status, exact, e := do("GET", "/api/agent/tasks/read?botId=87654321&type=group&id=303&taskId="+v["taskId"].(string), nil, "")
		if e != nil || status != 200 || exact["version"] != v["version"] || exact["status"] != "cancelled" {
			return fmt.Errorf("exact cancelled task mismatch: %d %v", status, e)
		}
		if !smokeTaskTimingValid(v) || !smokeTaskTimingValid(exact) || v["cancelRequestedAt"] != stopped["cancelRequestedAt"] || exact["cancelRequestedAt"] != stopped["cancelRequestedAt"] || exact["phaseStartedAt"] != v["phaseStartedAt"] {
			return fmt.Errorf("terminal read lost phase or first stop time")
		}
		if !smokeTaskObservationValid(page, v) || !smokeTaskObservationValid(exact, exact) {
			return fmt.Errorf("terminal task observation snapshot missing or before task timestamps")
		}
		if phase == "action" && v["outcome"] != "ok" {
			return fmt.Errorf("in-flight receipt lost")
		}
		wantOK := 0
		if phase == "action" {
			wantOK = 1
		}
		if !smokeTaskResultsMatch(v, wantOK, 0, 0, 0) || !smokeTaskResultsMatch(exact, wantOK, 0, 0, 0) {
			return fmt.Errorf("cancelled task result counted twice or model cancellation invented an action")
		}
		if !smokeTaskQueriesMatch(v, 0, 0, 0) || !smokeTaskQueriesMatch(exact, 0, 0, 0) {
			return fmt.Errorf("cancellation invented query usage")
		}
		if phase == "fallback" {
			if v["context"].(map[string]any)["available"] != false {
				return fmt.Errorf("fallback advertised Agent context")
			}
			status, snapshot, e = do("GET", "/api/ai", nil, "")
			if e != nil || status != 200 {
				return fmt.Errorf("fallback cleanup read failed")
			}
			status, _, e = do("POST", "/api/ai", map[string]any{"revision": snapshot["global"].(map[string]any)["revision"], "fields": map[string]any{"ModelName": oldChatModel}}, csrf)
			if e != nil || status != 200 {
				return fmt.Errorf("fallback cleanup failed")
			}
		}
	}
	return nil
}

func verifyPolicyCommands(dataDir string, private func(int64, string) (string, error), group func(int64, int64, string, string, bool) (string, error)) error {
	const owner, user = int64(12345678), int64(23456789)
	backups := filepath.Join(dataDir, "backups")
	count := func() (int, error) {
		entries, err := os.ReadDir(backups)
		if err != nil {
			return 0, err
		}
		n := 0
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), "-control") {
				n++
			}
		}
		return n, nil
	}
	command := func(gid int64, cmd, want string, delta int) error {
		before, err := count()
		if err != nil {
			return err
		}
		var text string
		if gid == 0 {
			text, err = private(owner, cmd)
		} else {
			text, err = group(owner, gid, "member", cmd, true)
		}
		if err != nil || !strings.Contains(text, want) {
			return fmt.Errorf("policy command %s feedback failed: %v", cmd, err)
		}
		after, err := count()
		if err != nil {
			return err
		}
		if after-before != delta {
			return fmt.Errorf("policy command %s made %d snapshots, expected %d", cmd, after-before, delta)
		}
		return nil
	}
	route := func(uid, gid int64, want string) error {
		text, err := group(uid, gid, "member", "翻转 abc", false)
		if err != nil || text != want {
			return fmt.Errorf("policy gate for synthetic user %d/group %d failed: %v", uid, gid, err)
		}
		return nil
	}
	if err := command(101, "/响应", "工作", 1); err != nil {
		return err
	}
	if err := command(101, "/响应", "已处于响应", 0); err != nil {
		return err
	}
	for _, pair := range []struct {
		gid                  int64
		add, remove, yes, no string
	}{
		{101, "/禁止 chrev 23456789", "/允许 chrev 23456789", "已禁止", "已允许"},
		{0, "/全局禁止 chrev 23456789", "/全局允许 chrev 23456789", "已禁止", "已允许"},
		{0, "/封禁 23456789", "/解封 23456789", "已封禁", "已解封"},
	} {
		if err := route(user, 101, "ɔqɐ"); err != nil {
			return err
		}
		if err := command(pair.gid, pair.add, pair.yes, 1); err != nil {
			return err
		}
		if err := command(pair.gid, pair.add, pair.yes, 0); err != nil {
			return err
		}
		if err := route(user, 101, ""); err != nil {
			return err
		}
		if err := command(pair.gid, pair.remove, pair.no, 1); err != nil {
			return err
		}
		if err := command(pair.gid, pair.remove, pair.no, 0); err != nil {
			return err
		}
		if err := route(user, 101, "ɔqɐ"); err != nil {
			return err
		}
	}
	before, err := count()
	if err != nil {
		return err
	}
	if text, err := group(user, 101, "member", "/禁止 chrev 12345678", true); err != nil || text != "" {
		return fmt.Errorf("unauthorized policy command accepted")
	}
	after, err := count()
	if err != nil || after != before {
		return fmt.Errorf("unauthorized policy command created a backup")
	}
	for _, c := range []struct {
		gid       int64
		cmd, want string
	}{
		{101, "/禁止 chrev 34567890", "已禁止"},
		{0, "/全局禁止 chrev 45678901", "已禁止"},
		{0, "/封禁 56789012", "已封禁"},
	} {
		if err := command(c.gid, c.cmd, c.want, 1); err != nil {
			return err
		}
	}
	// The directory is inside a fresh smoke fixture, never the live admin path.
	withBlockedBackups := func(check func() error) (result error) {
		held := backups + "-held"
		if err := os.Rename(backups, held); err != nil {
			return err
		}
		defer func() {
			if err := os.Remove(backups); err != nil && !os.IsNotExist(err) {
				result = err
				return
			}
			if err := os.Rename(held, backups); err != nil {
				result = err
			}
		}()
		if err := os.WriteFile(backups, []byte("synthetic blocked backup directory"), 0600); err != nil {
			return err
		}
		return check()
	}
	failure := func(gid int64, cmd string) error {
		var text string
		var err error
		if gid == 0 {
			text, err = private(owner, cmd)
		} else {
			text, err = group(owner, gid, "member", cmd, true)
		}
		if err != nil || !strings.Contains(text, "原状态保留") || strings.Contains(text, "已禁止") || strings.Contains(text, "已允许") || strings.Contains(text, "已封禁") || strings.Contains(text, "已解封") || strings.Contains(text, "SQL") || strings.Contains(text, dataDir) {
			return fmt.Errorf("policy failure feedback leaked details or claimed success for %s", cmd)
		}
		return nil
	}
	if err := withBlockedBackups(func() error {
		for _, c := range []struct {
			gid int64
			cmd string
		}{
			{202, "/响应"}, {101, "/沉默"}, {0, "/全局响应"},
			{101, "/禁止 chrev 23456789"}, {101, "/允许 chrev 34567890"},
			{0, "/全局禁止 chrev 23456789"}, {0, "/全局允许 chrev 45678901"},
			{0, "/封禁 23456789"}, {0, "/解封 56789012"},
		} {
			if err := failure(c.gid, c.cmd); err != nil {
				return err
			}
		}
		for _, c := range []struct {
			uid, gid int64
			want     string
		}{
			{user, 101, "ɔqɐ"}, {user, 202, ""}, {34567890, 101, ""}, {45678901, 101, ""}, {56789012, 101, ""},
		} {
			if err := route(c.uid, c.gid, c.want); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, c := range []struct {
		gid       int64
		cmd, want string
	}{
		{101, "/允许 chrev 34567890", "已允许"}, {0, "/全局允许 chrev 45678901", "已允许"},
		{0, "/解封 56789012", "已解封"}, {101, "/沉默", "休息"}, {0, "/全局响应", "全部位置工作"},
	} {
		if err := command(c.gid, c.cmd, c.want, 1); err != nil {
			return err
		}
	}
	if err := withBlockedBackups(func() error {
		if err := failure(0, "/全局沉默"); err != nil {
			return err
		}
		return route(user, 202, "ɔqɐ")
	}); err != nil {
		return err
	}
	if err := command(0, "/全局沉默", "休息", 1); err != nil {
		return err
	}
	return route(user, 101, "")
}

// A second, disposable process exercises the actual reverse driver. It has no
// daily credentials, model settings or connection to NapCat / real QQ.
func verifyReverseDriver(root, parent, binary string) error {
	fixture, err := os.MkdirTemp(parent, "reverse-driver-")
	if err != nil {
		return err
	}
	if err = prepareFixture(root, fixture); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	address := listener.Addr().String()
	adminListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		listener.Close()
		return err
	}
	adminAddress := adminListener.Addr().String()
	listener.Close()
	adminListener.Close()
	target := "ws://" + address
	token := "synthetic-reverse-driver-key"
	config := map[string]any{"zero": map[string]any{"nickname": []string{"七叶叶"}, "command_prefix": "/", "super_users": []int64{12345678}, "ring_len": 4096, "latency": 233000000, "max_process_time": 240000000000, "mark_message": true}, "ws": []any{}, "wss": []map[string]string{{"URL": target, "AccessToken": token}}, "force_base64_file": false}
	raw, _ := json.Marshal(config)
	configPath := filepath.Join(fixture, "config.json")
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		return err
	}
	dataDir := filepath.Join(fixture, "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, ".local", "bin", binary), "-c", configPath, "-admin", "-admin-addr", adminAddress, "-admin-data", dataDir)
	cmd.Dir = fixture
	cmd.Env = append(os.Environ(), "DEBUG_MODE=1")
	stdout, err := os.Create(filepath.Join(fixture, "stdout.log"))
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(fixture, "stderr.log"))
	if err != nil {
		return err
	}
	defer stderr.Close()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
		}
	}()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: time.Second}
	base := "http://" + adminAddress
	ready := false
	for attempt := 0; attempt < 450; attempt++ {
		rsp, e := client.Get(base + "/api/session")
		if e == nil {
			io.Copy(io.Discard, rsp.Body)
			rsp.Body.Close()
			if rsp.StatusCode == 200 {
				ready = true
				break
			}
		}
		select {
		case e := <-exited:
			return fmt.Errorf("reverse fixture exited before admin ready: %v", e)
		case <-ctx.Done():
			return fmt.Errorf("reverse fixture startup timeout")
		default:
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		return fmt.Errorf("reverse fixture admin unavailable")
	}
	for _, check := range []struct {
		auth   string
		status int
	}{{"", 401}, {"Bearer incorrect-fixture-key", 403}} {
		req, _ := http.NewRequest("GET", "http://"+address+"/", nil)
		if check.auth != "" {
			req.Header.Set("Authorization", check.auth)
		}
		rsp, e := client.Do(req)
		if e != nil {
			return fmt.Errorf("reverse authentication probe unavailable: %w", e)
		}
		io.Copy(io.Discard, rsp.Body)
		rsp.Body.Close()
		if rsp.StatusCode != check.status {
			return fmt.Errorf("reverse driver did not reject wrong authentication: %d", rsp.StatusCode)
		}
	}
	conn, _, err := websocket.DefaultDialer.Dial(target, http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		return fmt.Errorf("reverse driver authenticated upgrade failed: %w", err)
	}
	defer conn.Close()
	if err = conn.WriteJSON(map[string]any{"self_id": 87654323, "post_type": "meta_event", "meta_event_type": "lifecycle", "sub_type": "connect", "time": time.Now().Unix()}); err != nil {
		return err
	}
	code, err := os.ReadFile(filepath.Join(dataDir, "setup-code.txt"))
	if err != nil {
		return err
	}
	setup, _ := json.Marshal(map[string]string{"setupCode": strings.TrimSpace(string(code)), "password": "reverse-fixture-password-only"})
	req, _ := http.NewRequest("POST", base+"/api/setup", bytes.NewReader(setup))
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Local-Request", "1")
	rsp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, rsp.Body)
	rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return fmt.Errorf("reverse fixture setup failed: %d", rsp.StatusCode)
	}
	connected := false
	for attempt := 0; attempt < 50; attempt++ {
		rsp, e := client.Get(base + "/api/overview")
		if e == nil {
			var overview map[string]any
			decodeErr := json.NewDecoder(rsp.Body).Decode(&overview)
			rsp.Body.Close()
			accounts, _ := overview["botAccounts"].([]any)
			if decodeErr == nil && rsp.StatusCode == 200 && overview["connectionState"] == "connected" && len(accounts) == 1 && accounts[0] == "87654323" {
				connected = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !connected {
		return fmt.Errorf("reverse handshake did not register synthetic account")
	}
	rsp, err = client.Get(base + "/api/connections")
	if err != nil {
		return err
	}
	var settings map[string]any
	err = json.NewDecoder(rsp.Body).Decode(&settings)
	rsp.Body.Close()
	if err != nil || rsp.StatusCode != 200 {
		return fmt.Errorf("reverse fixture settings unavailable")
	}
	views, _ := settings["runningConnections"].([]any)
	if len(views) != 1 || views[0].(map[string]any)["kind"] != "wss" || views[0].(map[string]any)["tokenConfigured"] != true {
		return fmt.Errorf("reverse driver startup config mismatch")
	}
	encoded, _ := json.Marshal(settings)
	if bytes.Contains(encoded, []byte(token)) {
		return fmt.Errorf("reverse fixture read exposed token")
	}
	if err = conn.WriteJSON(map[string]any{"time": time.Now().Unix(), "self_id": 87654323, "post_type": "message", "message_type": "private", "sub_type": "friend", "message_id": 1, "user_id": 12345678, "raw_message": "/help", "message": []map[string]any{{"type": "text", "data": map[string]string{"text": "/help"}}}, "sender": map[string]any{"user_id": 12345678, "nickname": "Reverse synthetic user", "sex": "unknown", "age": 0}}); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	help := false
	for i := 0; i < 30; i++ {
		var request apiRequest
		if err = conn.ReadJSON(&request); err != nil {
			return fmt.Errorf("reverse command reply missing: %w", err)
		}
		if err = conn.WriteJSON(map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{"message_id": 999, "user_id": 87654323, "nickname": "Reverse synthetic bot"}, "echo": request.Echo}); err != nil {
			return err
		}
		if request.Action == "send_private_msg" {
			text, e := smokeMessageText(request.Params.Message)
			if e != nil || request.Params.UserID != 12345678 || !strings.Contains(text, "OneBot + ZeroBot + Golang") {
				return fmt.Errorf("reverse command delivery incorrect")
			}
			help = true
			break
		}
	}
	if !help {
		return fmt.Errorf("reverse command response not observed")
	}
	evidence := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "missingTokenDenied": true, "wrongTokenDenied": true, "authenticatedUpgradePassed": true, "actualReverseAccountRegistered": true, "startupConfigAndRedactionPassed": true, "helpRoundtripPassed": true, "completedAt": time.Now().Format(time.RFC3339)}
	encoded, _ = json.MarshalIndent(evidence, "", "  ")
	return os.WriteFile(filepath.Join(fixture, "result.json"), encoded, 0600)
}
