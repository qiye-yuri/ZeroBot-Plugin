package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FloatTech/floatbox/file"
	goba "github.com/fumiama/go-onebot-agent"
)

var (
	ErrAgentBusy         = errors.New("当前机器人会话已有任务，请稍后再试")
	ErrAgentCancelled    = errors.New("Agent 任务已请求停止")
	ErrAgentTaskStore    = errors.New("任务状态存储不可用")
	ErrAgentTaskConflict = errors.New("任务版本已变化，请重新读取")
	ErrAgentTaskMissing  = errors.New("任务不存在")
)

func taskID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

var taskInstance = taskID()

// ContextUsage describes assembled history, not provider token accounting or
// proof that the model understood every record. No message content is retained.
type ContextUsage struct {
	Records       int `json:"records"`
	SearchSources int `json:"searchSources"`
	// Nil distinguishes older summaries from a measured zero.
	MemorySources *int `json:"memorySources,omitempty"`
	HistoryBytes  int  `json:"historyBytes"`
	BudgetBytes   int  `json:"budgetBytes"`
	Clipped       bool `json:"clipped"`
	Available     bool `json:"available"`
}

// MemoryUsage describes managed memory selected during prompt preparation.
// It contains no text and is not a token count or proof of model consumption.
type MemoryUsage struct {
	Available   bool   `json:"available"`
	Mode        string `json:"mode"`
	Records     int    `json:"records"`
	Bytes       int    `json:"bytes"`
	BudgetBytes int    `json:"budgetBytes"`
	Limit       int    `json:"limit"`
	Clipped     bool   `json:"clipped"`
	Reason      string `json:"reason"`
}
type AgentTaskInfo struct {
	ID        string `json:"taskId"`
	SourceSeq string `json:"sourceSeq,omitempty"`
	Instance  string `json:"instance"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	Phase     string `json:"phase"`
	// Stage admission time, including repeated model preparations in the same phase.
	// Summary updates and cancellation do not restart the current stage.
	PhaseStartedAt string `json:"phaseStartedAt,omitempty"`
	// First admitted stop request; absent old data must not be inferred.
	CancelRequestedAt string       `json:"cancelRequestedAt,omitempty"`
	Action            string       `json:"action"`
	Outcome           string       `json:"outcome"`
	Error             string       `json:"error"`
	StartedAt         string       `json:"startedAt"`
	EndedAt           string       `json:"endedAt"`
	Persisted         bool         `json:"persisted"`
	Context           ContextUsage `json:"context"`
	Memory            MemoryUsage  `json:"memory"`
	Activity          TaskActivity `json:"activity"`
	Results           TaskResults  `json:"results"`
	Queries           TaskQueries  `json:"queries"`
}

const agentQueryLimit = 3

// TaskQueries records shared quota and response coverage, never keywords or text.
// Empty and Partial may overlap and do not establish absence from all history.
type TaskQueries struct {
	Available     bool `json:"available"`
	Used          int  `json:"used"`
	Limit         int  `json:"limit"`
	SearchHistory int  `json:"searchHistory"`
	ReadHistory   int  `json:"readHistory"`
	SearchMemory  int  `json:"searchMemory"`
	ReadMemory    int  `json:"readMemory"`
	Successful    int  `json:"successful"`
	Empty         int  `json:"empty"`
	Partial       int  `json:"partial"`
	Errors        int  `json:"errors"`
	Denied        int  `json:"denied"`
}

func (q TaskQueries) valid() bool {
	if !q.Available || q.Limit != agentQueryLimit || q.Used > q.Limit || q.Denied < 0 {
		return false
	}
	for _, n := range []int{q.Used, q.SearchHistory, q.ReadHistory, q.SearchMemory, q.ReadMemory, q.Successful, q.Empty, q.Partial, q.Errors} {
		if n < 0 || n > agentQueryLimit {
			return false
		}
	}
	return q.SearchHistory+q.ReadHistory+q.SearchMemory+q.ReadMemory == q.Used && q.Successful+q.Errors <= q.Used && q.Empty <= q.Successful && q.Partial <= q.Successful
}

// TaskResults counts settled outcomes, including an interrupted admitted action
// as unknown. It is not proof of delivery, rollback, or user receipt.
type TaskResults struct {
	Available   bool `json:"available"`
	OK          int  `json:"ok"`
	Rejected    int  `json:"rejected"`
	Unknown     int  `json:"unknown"`
	LocalErrors int  `json:"localErrors"`
}

// TaskActivity counts stage registrations, not HTTP calls or successful effects.
type TaskActivity struct {
	Available            bool `json:"available"`
	ModelPreparations    int  `json:"modelPreparations"`
	FallbackPreparations int  `json:"fallbackPreparations"`
	LocalActions         int  `json:"localActions"`
	PlatformActions      int  `json:"platformActions"`
}
type TaskStore struct {
	mu       sync.Mutex
	db       *sql.DB
	bot      int64
	active   map[int64]*AgentTask
	instance string
}
type AgentTask struct {
	store     *TaskStore
	grp       int64
	info      AgentTaskInfo
	cancelled bool
	finished  bool
	fault     bool
}

func newTaskStore(db *sql.DB, instance string) (*TaskStore, error) {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS agent_tasks (seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE NOT NULL, conversation INTEGER NOT NULL, instance TEXT NOT NULL, status TEXT NOT NULL, payload TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS agent_tasks_scope ON agent_tasks(conversation,seq)`,
	} {
		if _, e := db.Exec(q); e != nil {
			return nil, ErrAgentTaskStore
		}
	}
	// Recovery is lazy per account. Never replay a model request or platform action.
	rows, e := db.Query(`SELECT id,payload FROM agent_tasks WHERE status IN ('running','cancel_requested') AND instance<>?`, instance)
	if e != nil {
		return nil, ErrAgentTaskStore
	}
	var recovered []AgentTaskInfo
	for rows.Next() {
		var id, raw string
		var v AgentTaskInfo
		if rows.Scan(&id, &raw) != nil || json.Unmarshal([]byte(raw), &v) != nil {
			rows.Close()
			return nil, ErrAgentTaskStore
		}
		v.ID = id
		recovered = append(recovered, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, ErrAgentTaskStore
	}
	tx, e := db.Begin()
	if e != nil {
		return nil, ErrAgentTaskStore
	}
	defer tx.Rollback()
	for _, v := range recovered {
		v.Status = "interrupted"
		v.Phase = "finished"
		v.Error = "process_interrupted"
		v.EndedAt = taskNow()
		v.PhaseStartedAt = v.EndedAt // Recovery observation, not the actual process exit.
		v.Persisted = true
		if v.Outcome == "in_flight" {
			settleTaskResult(&v, "unknown")
		}
		bumpTask(&v)
		raw, _ := json.Marshal(v)
		if _, e = tx.Exec(`UPDATE agent_tasks SET status=?,payload=? WHERE id=?`, v.Status, string(raw), v.ID); e != nil {
			return nil, ErrAgentTaskStore
		}
	}
	if tx.Commit() != nil {
		return nil, ErrAgentTaskStore
	}
	return &TaskStore{db: db, instance: instance, active: map[int64]*AgentTask{}}, nil
}
func AgentTasks(bot int64) (*TaskStore, error) {
	if bot <= 0 {
		return nil, ErrAgentTaskMissing
	}
	agentsMu.Lock()
	defer agentsMu.Unlock()
	h := histories[bot]
	if h == nil {
		var e error
		h, e = openAgentHistory(filepath.Join(file.BOTPATH, "data", "aichat", "history"), bot, agentContextBytes, CurrentConfig)
		if e != nil {
			return nil, ErrAgentTaskStore
		}
		histories[bot] = h
	}
	return h.tasks, nil
}
func taskNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func bumpTask(v *AgentTaskInfo) {
	n, _ := strconv.ParseInt(v.Version, 10, 64)
	v.Version = strconv.FormatInt(n+1, 10)
}
func taskTerminal(s string) bool {
	return s == "completed" || s == "cancelled" || s == "failed" || s == "interrupted"
}
func (s *TaskStore) Begin(grp int64) (*AgentTask, error) {
	return s.BeginFrom(grp, 0)
}

// BeginFrom links a host-recorded event, never a model-selected source. Zero
// explicitly means that no source was captured; old Begin callers retain it.
func (s *TaskStore) BeginFrom(grp, sourceSeq int64) (*AgentTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if grp == 0 {
		return nil, ErrAgentTaskMissing
	}
	if s.active[grp] != nil {
		return nil, ErrAgentBusy
	}
	if sourceSeq < 0 {
		return nil, ErrAgentTaskConflict
	}
	if sourceSeq > 0 {
		var exists bool
		if e := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM history WHERE conversation=? AND seq=? AND kind='event')`, grp, sourceSeq).Scan(&exists); e != nil {
			return nil, ErrAgentTaskStore
		}
		if !exists {
			return nil, ErrAgentTaskConflict
		}
	}
	t := &AgentTask{store: s, grp: grp, info: AgentTaskInfo{ID: taskID(), SourceSeq: strconv.FormatInt(sourceSeq, 10), Instance: s.instance, Version: "1", Status: "running", Phase: "starting", StartedAt: taskNow(), Persisted: true}}
	t.info.PhaseStartedAt = t.info.StartedAt
	t.info.Activity.Available = true
	t.info.Results.Available = true
	t.info.Queries = TaskQueries{Available: true, Limit: agentQueryLimit}
	raw, _ := json.Marshal(t.info)
	if _, e := s.db.Exec(`INSERT INTO agent_tasks(id,conversation,instance,status,payload) VALUES(?,?,?,?,?)`, t.info.ID, grp, s.instance, t.info.Status, string(raw)); e != nil {
		t.fault = true
		t.finished = true
		t.info.Status = "failed"
		t.info.Persisted = false
		t.info.Error = "storage_error"
		t.info.EndedAt = taskNow()
		t.info.Phase = "finished"
		t.info.PhaseStartedAt = t.info.EndedAt
		s.active[grp] = t
		return nil, ErrAgentTaskStore
	}
	s.active[grp] = t
	return t, nil
}

// On uncertain writes the in-memory task stops and retains the scope lock until
// restart. Reads expose persisted=false rather than claiming a durable result.
func (t *AgentTask) saveLocked() error {
	bumpTask(&t.info)
	t.info.Persisted = true
	raw, _ := json.Marshal(t.info)
	result, e := t.store.db.Exec(`UPDATE agent_tasks SET status=?,payload=? WHERE id=?`, t.info.Status, string(raw), t.info.ID)
	if e == nil {
		var n int64
		n, e = result.RowsAffected()
		if n != 1 {
			e = ErrAgentTaskStore
		}
	}
	if e != nil {
		t.fault = true
		t.info.Persisted = false
		t.info.Error = "storage_error"
		return ErrAgentTaskStore
	}
	return nil
}
func (t *AgentTask) checkLocked() error {
	if t.fault {
		return ErrAgentTaskStore
	}
	if t.cancelled {
		return ErrAgentCancelled
	}
	if t.finished {
		return ErrAgentTaskConflict
	}
	return nil
}
func (t *AgentTask) Check() error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	return t.checkLocked()
}
func (t *AgentTask) Model() error {
	return t.model("model_wait")
}
func (t *AgentTask) FallbackModel() error {
	return t.model("fallback_model_wait")
}
func (t *AgentTask) model(phase string) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if e := t.checkLocked(); e != nil {
		return e
	}
	if t.info.Outcome == "in_flight" {
		return ErrAgentTaskConflict
	}
	t.info.Phase = phase
	t.info.PhaseStartedAt = taskNow()
	t.info.Context = ContextUsage{}
	t.info.Memory = MemoryUsage{}
	if phase == "fallback_model_wait" {
		t.info.Activity.FallbackPreparations++
	} else {
		t.info.Activity.ModelPreparations++
	}
	return t.saveLocked()
}

// Dispatch admission is serialized with Cancel. Once admitted the current action
// is in flight; Cancel never waits on or retries the platform call.
func (t *AgentTask) Action(name string) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if e := t.checkLocked(); e != nil {
		return e
	}
	if t.info.Outcome == "in_flight" {
		return ErrAgentTaskConflict
	}
	t.info.Phase = "action_wait"
	t.info.PhaseStartedAt = taskNow()
	t.info.Action = taskActionName(name)
	t.info.Outcome = "in_flight"
	switch name {
	case goba.SearchHistory, goba.ReadHistory, goba.SearchMemory, goba.ReadMemory, goba.SVM:
		t.info.Activity.LocalActions++
	default:
		t.info.Activity.PlatformActions++
	}
	return t.saveLocked()
}
func taskActionName(name string) string {
	if len(name) > 80 {
		return "other"
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "other"
		}
	}
	return name
}
func (t *AgentTask) Result(outcome string) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if t.fault {
		return ErrAgentTaskStore
	}
	if t.finished {
		return ErrAgentTaskConflict
	}
	if t.info.Phase != "action_wait" || t.info.Outcome != "in_flight" {
		return ErrAgentTaskConflict
	}
	switch outcome {
	case "ok", "rejected", "unknown", "local_error":
	default:
		return ErrAgentTaskConflict
	}
	settleTaskResult(&t.info, outcome)
	t.info.Phase = "between_actions"
	t.info.PhaseStartedAt = taskNow()
	return t.saveLocked()
}

func settleTaskResult(v *AgentTaskInfo, outcome string) {
	if v.Outcome != "in_flight" {
		return
	}
	v.Outcome = outcome
	if !v.Results.Available {
		return // Never infer accumulated results for an older task.
	}
	switch outcome {
	case "ok":
		v.Results.OK++
	case "rejected":
		v.Results.Rejected++
	case "unknown":
		v.Results.Unknown++
	case "local_error":
		v.Results.LocalErrors++
	}
}
func (s *TaskStore) capture(grp int64, usage ContextUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.active[grp]
	if t == nil {
		return nil
	}
	if e := t.checkLocked(); e != nil {
		return e
	}
	t.info.Context = usage
	return t.saveLocked()
}

func (s *TaskStore) captureMemory(grp int64, usage MemoryUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.active[grp]
	if t == nil || t.info.Phase != "model_wait" {
		return nil
	}
	if e := t.checkLocked(); e != nil {
		return e
	}
	t.info.Memory = usage
	return t.saveLocked()
}

func (t *AgentTask) captureQueries(usage TaskQueries) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if t.fault {
		return ErrAgentTaskStore
	}
	if t.finished || t.info.Phase != "action_wait" || t.info.Outcome != "in_flight" || !usage.valid() {
		return ErrAgentTaskConflict
	}
	switch t.info.Action {
	case goba.SearchHistory, goba.ReadHistory, goba.SearchMemory, goba.ReadMemory:
	default:
		return ErrAgentTaskConflict
	}
	// This action was admitted before cancellation. Preserve its actual usage,
	// just as Result preserves the eventual receipt after a stop request.
	t.info.Queries = usage
	return t.saveLocked()
}
func (t *AgentTask) Finish(runErr error) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if t.finished {
		if t.fault {
			return ErrAgentTaskStore
		}
		return nil
	}
	t.finished = true
	t.info.Status = "completed"
	t.info.Phase = "finished"
	t.info.EndedAt = taskNow()
	t.info.PhaseStartedAt = t.info.EndedAt
	if t.cancelled {
		t.info.Status = "cancelled"
	} else if runErr != nil {
		t.info.Status = "failed"
		t.info.Error = "execution_error"
		if errors.Is(runErr, goba.ErrModelRequestFailed) {
			t.info.Error = "model_request_failed"
		}
		if errors.Is(runErr, ErrAgentReloaded) {
			t.info.Error = "settings_changed"
		}
		if errors.Is(runErr, ErrAgentRepeatedAction) {
			t.info.Error = "repeated_action"
		}
		if errors.Is(runErr, goba.ErrActionOutputLimit) {
			t.info.Error = "model_output_limit"
		}
		if errors.Is(runErr, goba.ErrActionOutputInvalid) {
			t.info.Error = "model_output_invalid"
		}
	}
	if t.info.Outcome == "in_flight" {
		settleTaskResult(&t.info, "unknown")
	}
	if t.fault {
		t.info.Status = "failed"
		t.info.Error = "storage_error"
		return ErrAgentTaskStore
	}
	if e := t.saveLocked(); e != nil {
		t.info.Status = "failed"
		return e
	}
	delete(t.store.active, t.grp)
	return nil
}
func (s *TaskStore) List(ctx context.Context, grp int64) ([]AgentTaskInfo, error) {
	page, err := s.Page(ctx, grp, 0)
	return page.Items, err
}

type AgentTaskPage struct {
	Items []AgentTaskInfo `json:"items"`
	Next  string          `json:"next"`
	More  bool            `json:"more"`
	Limit int             `json:"limit"`
}

// Page follows immutable creation sequence numbers, never mutable status or
// timestamps. Later inserts cannot shift older pages. Status is read live.
func (s *TaskStore) Page(ctx context.Context, grp, before int64) (AgentTaskPage, error) {
	return s.PageFiltered(ctx, grp, before, "all")
}

func ValidTaskFilter(state string) bool {
	switch state {
	case "all", "running", "cancel_requested", "cancelled", "completed", "failed", "interrupted":
		return true
	}
	return false
}

func (s *TaskStore) PageFiltered(ctx context.Context, grp, before int64, state string) (AgentTaskPage, error) {
	return s.PageWithReason(ctx, grp, before, state, "all")
}

// ValidTaskReasonFilter accepts saved error categories, not provider messages.
func ValidTaskReasonFilter(reason string) bool {
	switch reason {
	case "all", "none", "other", "storage_error", "settings_changed", "repeated_action", "model_request_failed", "model_output_limit", "model_output_invalid", "process_interrupted", "execution_error":
		return true
	}
	return false
}

func taskMatchesReason(value, reason string) bool {
	switch reason {
	case "all":
		return true
	case "none":
		return value == ""
	case "other":
		return value != "" && (!ValidTaskReasonFilter(value) || value == "all" || value == "none" || value == "other")
	default:
		return value == reason
	}
}

// PageWithReason combines state and saved reason, applying the in-memory task
// before filtering so an uncertain write cannot hide its actual failure.
func (s *TaskStore) PageWithReason(ctx context.Context, grp, before int64, state, reason string) (AgentTaskPage, error) {
	return s.PageWithFilters(ctx, grp, before, state, reason, "all")
}

func ValidTaskPhaseFilter(phase string) bool {
	switch phase {
	case "all", "starting", "model_wait", "fallback_model_wait", "action_wait", "between_actions", "finished":
		return true
	}
	return false
}

// PageWithFilters applies all conditions before paging, using the live task
// when its saved phase or status is stale after a failed write.
func (s *TaskStore) PageWithFilters(ctx context.Context, grp, before int64, state, reason, phase string) (AgentTaskPage, error) {
	return s.PageInRange(ctx, grp, before, state, reason, phase, "", "")
}

// PageInRange filters task start instants before taking the creation-sequence page.
func (s *TaskStore) PageInRange(ctx context.Context, grp, before int64, state, reason, phase, from, until string) (AgentTaskPage, error) {
	return s.PageMatching(ctx, grp, before, TaskPageFilter{State: state, Reason: reason, Phase: phase, Outcome: "all", From: from, Until: until})
}

// TaskPageFilter combines read-only conditions on the saved task summary.
// State, Reason, Phase and Outcome use "all" for an unrestricted condition.
type TaskPageFilter struct {
	State, Reason, Phase, Outcome string
	From, Until                   string
}

func ValidTaskOutcomeFilter(outcome string) bool {
	switch outcome {
	case "all", "none", "other", "in_flight", "ok", "rejected", "unknown", "local_error":
		return true
	}
	return false
}

func taskMatchesOutcome(value, outcome string) bool {
	switch outcome {
	case "all":
		return true
	case "none":
		return value == ""
	case "other":
		return value != "" && (!ValidTaskOutcomeFilter(value) || value == "all" || value == "none" || value == "other")
	default:
		return value == outcome
	}
}

// PageMatching applies live task values before all conditions and pagination.
func (s *TaskStore) PageMatching(ctx context.Context, grp, before int64, filter TaskPageFilter) (AgentTaskPage, error) {
	state, reason, phase, outcome := filter.State, filter.Reason, filter.Phase, filter.Outcome
	page := AgentTaskPage{Items: []AgentTaskInfo{}, Limit: 50}
	start, end, rangeErr := ParseArchiveSearchRange(filter.From, filter.Until)
	if before < 0 || grp == 0 || !ValidTaskFilter(state) || !ValidTaskReasonFilter(reason) || !ValidTaskPhaseFilter(phase) || !ValidTaskOutcomeFilter(outcome) || rangeErr != nil {
		return page, ErrAgentTaskConflict
	}
	inRange := func(v AgentTaskInfo) bool {
		if start.IsZero() && end.IsZero() {
			return true
		}
		stamp, err := time.Parse(time.RFC3339Nano, v.StartedAt)
		return err == nil && !stamp.IsZero() && (start.IsZero() || !stamp.Before(start)) && (end.IsZero() || stamp.Before(end))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	query := `SELECT seq,payload FROM agent_tasks WHERE conversation=?`
	args := []any{grp}
	limit := 51
	if state != "all" || reason != "all" || phase != "all" || outcome != "all" {
		conditions := []string{}
		if state != "all" {
			conditions = append(conditions, `status=?`)
			args = append(args, state)
		}
		if reason != "all" {
			field := `COALESCE(json_extract(payload,'$.error'),'')`
			switch reason {
			case "none":
				conditions = append(conditions, field+`=''`)
			case "other":
				conditions = append(conditions, field+` NOT IN ('','storage_error','settings_changed','repeated_action','model_request_failed','model_output_limit','model_output_invalid','process_interrupted','execution_error')`)
			default:
				conditions = append(conditions, field+`=?`)
				args = append(args, reason)
			}
		}
		if phase != "all" {
			conditions = append(conditions, `json_extract(payload,'$.phase')=?`)
			args = append(args, phase)
		}
		if outcome != "all" {
			field := `COALESCE(json_extract(payload,'$.outcome'),'')`
			switch outcome {
			case "none":
				conditions = append(conditions, field+`=''`)
			case "other":
				conditions = append(conditions, field+` NOT IN ('','in_flight','ok','rejected','unknown','local_error')`)
			default:
				conditions = append(conditions, field+`=?`)
				args = append(args, outcome)
			}
		}
		activeID := ""
		if t := s.active[grp]; t != nil {
			activeID = t.info.ID
		}
		// Include the one in-memory task regardless of persisted status. An
		// uncertain terminal write may leave its database row marked running.
		query += ` AND ((` + strings.Join(conditions, " AND ") + `) OR id=?)`
		args = append(args, activeID)
		limit = 52
	}
	if before > 0 {
		query += ` AND seq<?`
		args = append(args, before)
	}
	query += ` ORDER BY seq DESC`
	if start.IsZero() && end.IsZero() {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	// Stream until one page plus a lookahead matches. Go compares full instants,
	// preserving offsets and nanoseconds that SQLite date functions would round.
	rows, e := s.db.QueryContext(ctx, query, args...)
	if e != nil {
		return page, ErrAgentTaskStore
	}
	defer rows.Close()
	items := []AgentTaskInfo{}
	sequences := []int64{}
	foundActive := false
	for rows.Next() {
		var raw string
		var v AgentTaskInfo
		var seq int64
		if rows.Scan(&seq, &raw) != nil || json.Unmarshal([]byte(raw), &v) != nil {
			return page, ErrAgentTaskStore
		}
		if t := s.active[grp]; t != nil && t.info.ID == v.ID {
			v = t.info
			foundActive = true
		}
		if (state != "all" && v.Status != state) || !taskMatchesReason(v.Error, reason) || (phase != "all" && v.Phase != phase) || !taskMatchesOutcome(v.Outcome, outcome) || !inRange(v) {
			continue
		}
		items = append(items, v)
		sequences = append(sequences, seq)
		if (!start.IsZero() || !end.IsZero()) && len(items) == 51 {
			break
		}
	}
	if rows.Err() != nil {
		return page, ErrAgentTaskStore
	}
	if t := s.active[grp]; before == 0 && t != nil && !foundActive && (state == "all" || t.info.Status == state) && taskMatchesReason(t.info.Error, reason) && (phase == "all" || t.info.Phase == phase) && taskMatchesOutcome(t.info.Outcome, outcome) && inRange(t.info) {
		items = append([]AgentTaskInfo{t.info}, items...)
		sequences = append([]int64{0}, sequences...)
	}
	if len(items) > 50 {
		page.More = true
		page.Next = strconv.FormatInt(sequences[49], 10)
		items = items[:50]
	}
	page.Items = items
	return page, nil
}
func (s *TaskStore) Cancel(ctx context.Context, grp int64, id, version string) (AgentTaskInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.active[grp]
	if t == nil || t.info.ID != id {
		var raw string
		var v AgentTaskInfo
		e := s.db.QueryRowContext(ctx, `SELECT payload FROM agent_tasks WHERE conversation=? AND id=?`, grp, id).Scan(&raw)
		if errors.Is(e, sql.ErrNoRows) {
			return v, ErrAgentTaskMissing
		}
		if e != nil || json.Unmarshal([]byte(raw), &v) != nil {
			return v, ErrAgentTaskStore
		}
		if !taskTerminal(v.Status) {
			return v, ErrAgentTaskConflict
		}
		return v, nil
	}
	if t.fault {
		return t.info, ErrAgentTaskStore
	}
	if t.cancelled || t.finished {
		return t.info, nil
	}
	if t.info.Version != version {
		return t.info, ErrAgentTaskConflict
	}
	t.cancelled = true
	t.info.Status = "cancel_requested"
	t.info.CancelRequestedAt = taskNow()
	if e := t.saveLocked(); e != nil {
		return t.info, e
	}
	return t.info, nil
}
