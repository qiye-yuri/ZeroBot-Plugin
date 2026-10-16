package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelrequest"
	"github.com/FloatTech/zbputils/modelconfig"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

var validationBinary = flag.String("validation-binary", "", "final isolated binary name under .local/bin; optional integration check")

type savedProviders struct {
	ModelName, AgentModelName, ImageModelName string
	API, AgentAPI, ImageAPI                   string
	Key, AgentKey, ImageKey                   string
	Type, AgentType, ImageType                int
	ModelRoutes                               *modelconfig.Routes
	ModelProfiles                             []modelconfig.Profile
	MaxN                                      uint
	TopP                                      float32
	ReasoningEffort                           string
}

func providerFixture(t *testing.T, cfg savedProviders) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data/control"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(root, "data/control/plugins.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE __resp(gid INTEGER PRIMARY KEY,ext TEXT)`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	if _, err = db.Exec(`INSERT INTO __resp VALUES(1,?),(2,'{"unrelated":true}')`, string(raw)); err != nil {
		t.Fatal(err)
	}
	return root
}

func namedFixture(address string) savedProviders {
	return savedProviders{API: "http://legacy.invalid", ModelName: "legacy-chat", Key: "legacy-secret", AgentAPI: "http://legacy.invalid", AgentModelName: "legacy-agent", AgentKey: "legacy-secret", ImageAPI: "http://legacy.invalid", ImageModelName: "legacy-image", ImageKey: "legacy-secret",
		ModelRoutes: &modelconfig.Routes{Chat: "chat", Agent: "agent", Image: "image"}, ModelProfiles: []modelconfig.Profile{
			{ID: "chat", API: address + "/chat", Model: "actual-chat", Key: "synthetic-chat"},
			{ID: "agent", API: address + "/agent", Model: "actual-agent", Key: "synthetic-agent"},
			{ID: "image", API: address + "/image", Model: "actual-image", Key: "synthetic-image"},
		}}
}

func parameterFixture(address string) savedProviders {
	cfg := namedFixture(address)
	cfg.MaxN, cfg.TopP, cfg.ReasoningEffort = 2048, .87, "high"
	for i := range cfg.ModelProfiles {
		max, top, reasoning := uint(512+i*256), float32(.21+float64(i)*.22), []string{"low", "high", ""}[i]
		cfg.ModelProfiles[i].Parameters = &modelconfig.Parameters{MaxN: &max, TopP: &top, ReasoningEffort: &reasoning}
	}
	return cfg
}

func routeFixture(address string) savedProviders {
	cfg := parameterFixture(address)
	cfg.ModelRoutes.GPT, cfg.ModelRoutes.Summary = "gpt", "summary"
	for i, role := range []string{"gpt", "summary"} {
		max, top, reasoning := uint(1280+i*256), float32(.4+float64(i)*.1), []string{"medium", ""}[i]
		cfg.ModelProfiles = append(cfg.ModelProfiles, modelconfig.Profile{ID: role, API: address + "/" + role, Model: "actual-" + role, Key: "synthetic-" + role, Parameters: &modelconfig.Parameters{MaxN: &max, TopP: &top, ReasoningEffort: &reasoning}})
	}
	return cfg
}

func TestNamedProvidersReachActualHTTPAndStaySnapshot(t *testing.T) {
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := strings.TrimPrefix(r.URL.Path, "/")
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "actual-"+role || r.Header.Get("Authorization") != "Bearer synthetic-"+role {
			t.Error("wrong request target, model or key")
			http.Error(w, "invalid", 400)
			return
		}
		calls[role]++
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"synthetic response"}}]}`)
	}))
	defer server.Close()
	root := providerFixture(t, namedFixture(server.URL))
	file := filepath.Join(root, "data/control/plugins.db")
	before, _ := os.ReadFile(file)
	snapshot, err := readProviders(root)
	if err != nil || snapshot.Mode != "named" {
		t.Fatal(snapshot.Mode, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("read wrote control database")
	}
	// A later save must not change the already selected request targets.
	db, _ := sql.Open("sqlite3", file)
	_, err = db.Exec(`UPDATE __resp SET ext='{"ModelName":"new-model"}' WHERE gid=1`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range snapshot.Providers {
		api := deepinfra.NewAPI(p.API, p.Key)
		api.SetHTTPClient(server.Client())
		protocol := model.NewOpenAI(p.ModelName, "", .2, .9, 64, "")
		response, err := modelrequest.Request(api, protocol)
		if err != nil || response != "synthetic response" {
			t.Fatal(response, err)
		}
	}
	for _, role := range []string{"chat", "agent", "image"} {
		want := 1
		if role == "chat" {
			want = 3
		}
		if calls[role] != want {
			t.Fatal(calls)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("preflight created artifacts or backups")
	}
}

func TestProviderLegacyDisabledMissingRoutesAndCaseRequirements(t *testing.T) {
	legacy := savedProviders{API: "http://localhost:11434", ModelName: "local", Type: 1}
	s, err := readProviders(providerFixture(t, legacy))
	if err != nil || s.Mode != "legacy" || !s.Providers[""].ready() {
		t.Fatal(err)
	}
	if names, err := selectedCases("gpt, chat", s.Providers); err != nil || len(names) != 2 {
		t.Fatal(names, err)
	}
	for _, name := range []string{"agent", "vision", "chat,", "unknown", "chat,chat", ""} {
		if _, err := selectedCases(name, s.Providers); err == nil {
			t.Fatal("accepted unavailable or invalid case", name)
		}
	}
	for _, tc := range []string{"disabled", "missing", "no-routes"} {
		t.Run(tc, func(t *testing.T) {
			c := namedFixture("https://example.invalid")
			switch tc {
			case "disabled":
				c.ModelRoutes.Agent = ""
				c.ModelRoutes.Image = ""
			case "missing":
				c.ModelRoutes.Agent = "missing"
			case "no-routes":
				c.ModelRoutes = nil
			}
			s, err := readProviders(providerFixture(t, c))
			if tc != "disabled" {
				if err == nil {
					t.Fatal("invalid explicit routes accepted")
				}
				return
			}
			if err != nil || s.Providers["Agent"].API != "" || s.Providers["Image"].ModelName != "" {
				t.Fatal("disabled entrance fell back to legacy", err)
			}
			if _, err = selectedCases("agent", s.Providers); err == nil {
				t.Fatal("disabled case accepted")
			}
		})
	}
}

func TestRealModelCLIHelper(t *testing.T) {
	if os.Getenv("REAL_MODEL_HELPER") == "" {
		return
	}
	if err := os.Chdir(os.Getenv("REAL_MODEL_ROOT")); err != nil {
		panic(err)
	}
	flag.CommandLine = flag.NewFlagSet("probe", flag.ExitOnError)
	os.Args = []string{"probe"}
	*execute = os.Getenv("REAL_MODEL_EXECUTE") == "1"
	*caseNames = os.Getenv("REAL_MODEL_CASES")
	main()
	os.Exit(0)
}

func TestProviderCLIReadOnlyPreflightAndFailureExit(t *testing.T) {
	for _, execute := range []bool{false, true} {
		t.Run(fmt.Sprint(execute), func(t *testing.T) {
			cfg := namedFixture("https://example.invalid")
			cfg.ModelRoutes.Agent = ""
			root := providerFixture(t, cfg)
			cmd := exec.Command(os.Args[0], "-test.run=^TestRealModelCLIHelper$")
			cmd.Env = append(os.Environ(), "REAL_MODEL_HELPER=1", "REAL_MODEL_ROOT="+root, "REAL_MODEL_CASES=agent", fmt.Sprintf("REAL_MODEL_EXECUTE=%d", map[bool]int{false: 0, true: 1}[execute]))
			output, err := cmd.CombinedOutput()
			if execute {
				if err == nil {
					t.Fatal("missing provider did not exit nonzero")
				}
			} else {
				if err != nil || !bytes.Contains(output, []byte(`"casesReady": false`)) || !bytes.Contains(output, []byte(`"mode": "named"`)) {
					t.Fatal(string(output), err)
				}
			}
			for _, secret := range []string{"synthetic-chat", "synthetic-image", "legacy-secret"} {
				if bytes.Contains(output, []byte(secret)) {
					t.Fatal("credential in CLI output")
				}
			}
			if _, err := os.Stat(filepath.Join(root, ".local")); !os.IsNotExist(err) {
				t.Fatal("preflight or refused run started a test instance")
			}
		})
	}
}

func TestRealModelIsolatedNamedRuntime(t *testing.T) {
	if *validationBinary == "" {
		t.Skip("run after the final build with -validation-binary")
	}
	if filepath.Base(*validationBinary) != *validationBinary {
		t.Fatal("binary must be a filename")
	}
	var mu sync.Mutex
	calls := map[string]int{}
	nonce := regexp.MustCompile(`REAL_(?:gpt|chat|agent|summary)_[0-9a-f]+`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := strings.TrimPrefix(r.URL.Path, "/")
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "actual-"+role || r.Header.Get("Authorization") != "Bearer synthetic-"+role {
			t.Error("isolated instance used wrong provider")
			http.Error(w, "invalid", 400)
			return
		}
		index := map[string]int{"chat": 0, "agent": 1, "image": 2, "gpt": 3, "summary": 4}[role]
		wantTop := []float64{.21, .43, .65, .4, .5}[index]
		if body["max_tokens"] != float64(512+index*256) || body["top_p"] != wantTop {
			t.Error("isolated per-entry parameters lost", role, body["max_tokens"], body["top_p"])
		}
		wantReasoning := any(nil)
		if index < 2 {
			wantReasoning = []string{"low", "high"}[index]
		}
		if role == "gpt" {
			wantReasoning = "medium"
		}
		if body["reasoning_effort"] != wantReasoning {
			t.Error("isolated reasoning lost", role, body["reasoning_effort"])
		}
		mu.Lock()
		calls[role]++
		mu.Unlock()
		raw, _ := json.Marshal(body)
		reply := nonce.FindString(string(raw))
		if role == "image" {
			reply = "图片主要颜色为红色。"
		}
		if role == "agent" {
			uid := 23456002
			if strings.Contains(string(raw), "主要颜色") {
				uid = 23456003
				reply = "红色"
			}
			req, _ := json.Marshal(map[string]any{"action": "send_private_msg", "params": map[string]any{"user_id": uid, "message": []any{map[string]any{"type": "text", "data": map[string]string{"text": reply}}}}})
			reply = string(req) + "\n" + `{"action":"end_action"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) == "scripts" {
		root = filepath.Dir(root)
	}
	if err = os.MkdirAll(filepath.Join(root, ".local/real-model-routing"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err := os.MkdirTemp(filepath.Join(root, ".local/real-model-routing"), "run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("ARTIFACTS=" + out)
	snapshot, err := readProviders(providerFixture(t, routeFixture(server.URL)))
	if err != nil {
		t.Fatal(err)
	}
	names, err := selectedCases("gpt,chat,agent,vision,summary", snapshot.Providers)
	if err != nil {
		t.Fatal(err)
	}
	previous := *binaryName
	*binaryName = *validationBinary
	defer func() { *binaryName = previous }()
	result := map[string]any{"realProvider": false, "configurationMode": snapshot.Mode, "syntheticOneBot": true, "realQQMessagesSent": false}
	err = run(root, out, snapshot.Providers, names, result)
	result["passed"] = err == nil
	raw, _ := json.MarshalIndent(result, "", "  ")
	if e := os.WriteFile(filepath.Join(out, "result.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["chat"] != 1 || calls["gpt"] != 1 || calls["summary"] != 1 || calls["agent"] != 2 || calls["image"] != 1 {
		t.Fatal("unexpected model calls", calls)
	}
}
