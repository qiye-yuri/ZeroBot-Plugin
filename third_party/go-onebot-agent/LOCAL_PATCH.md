# Local Agent history patch

2026-10-29：宿主可通过任务级 Config.MemoryLayers 启用分类记忆提示。参数校验、主人共享权限、触发消息作者绑定、属性版本更新及召回均由宿主实现；SDK默认调用和权限表保持兼容。

N3增加 `search_web` 本地工具定义。仅调用方任务 `Config.WebSearch` 为真时追加提示并准许解析，默认角色表不自动开放。每次调用独立捕获，关闭的后续任务不会继承共享Agent的开关。实际会话及角色、次数、HTTP和来源由宿主管理，工具不发到OneBot。

GetActionChecked 增加可选的 Config 值参数，单次构造系统提示时使用调用方的任务人物快照，不修改共享 Agent.cfg；旧调用仍使用原配置。宿主复用同一快照完成多轮任务，权限表和历史存储保持原规则。

search_memory 提示新增可选 from/until，以当前版本修订时间（含编辑、启停及恢复）筛选，具体来源或有效时间可省略关键词；继续保留 UTC 回执。原编号排序、任务共享额度和角色权限保持，完整权限表及系统提示回归同步。

search_history 提示新增 eventType=message/notice/request，省略或空串不限；具体类型可省略关键词，按保存事件顶层 post_type 匹配，工具动作请求不算事件请求。继续须保留实际类型回执，原动作权限保持，系统提示和完整权限表回归同步。

search_memory 提示同步允许指定具体 source 时省略或留空 keyword，不限来源仍须关键词；明确按记忆编号倒序而非修订时间。原动作和权限保持，完整权限表及系统提示回归同步。

search_history 提示补充：指定有效 from/until 后可省略或留空 keyword，不必提供 sender，直接回顾时间范围内事件。没有关键词、发言者及有效时间边界时仍不接受查询。动作/权限保持，完整权限表与系统提示测试同步。

search_memory 增加可选 source=agent/admin/legacy，省略或空串不限；提示按当前版本来源而非最初作者解释，并在继续检索时保留。动作和权限不变，参数、响应说明及完整权限表/系统提示测试同步。

search_history 提示支持指定 sender 后省略或留空 keyword，以查看该账号最近消息；没有 sender 时仍要求关键词。动作和权限不变，actions.yaml、权限表及系统提示完整文本测试同步。

动作整批预检新增每轮共享的 JSON 结构预算：对象/数组最多 64 层、所有值累计最多 10,000，字段名不另计，字符串不递归解释。未知字段和结束对象计入，首个有效结束后仍忽略尾部；新回复重置预算。复用 ErrActionOutputLimit，在自动记忆保存和请求/结束记录前拒绝全部前缀。系统提示同步，测试覆盖准确边界、跨动作共享、对象成员、opaque 文本、自动保存拒绝及正常恢复；旧 1,000 层兼容用例改为明确 64 层边界。

search_history 新增可选 sender 正整数字符串，仅按消息事件顶层 user_id 限定当前会话发言者，和时间/序号条件组合并沿实际回执继续。提示明确昵称、正文提及不算作者，未知旧字段跳过并标明 partial；已应用 sender 单独回传，不改动作白名单或三次共享额度。actions.yaml 与权限、系统提示测试同步。

read_history advertises an optional integer context radius (0..3), bounded event
neighbors, aggregate output limits and requested anchor metadata. Omitted/zero
radius retains exact-source reads; host scope/reset checks and shared quotas stay
in force. Prompt/table goldens match the updated action description.

The search_memory response schema also documents searchMore/nextBefore. The
continuation is a memory ID string, not a source sequence or revision; repeated
lookups retain the host's shared three-query budget. Prompt/table goldens match.

The search_history schema now documents searchMore/nextBefore continuation
receipts. Callers reuse the returned string as before with the same filters,
within the existing shared lookup quota. Partial excerpts alone do not imply
that another search window exists. Prompt and permission table goldens agree.

The search_history action schema now advertises optional absolute RFC3339
from/until strings, archive creation time, inclusive/exclusive boundaries and
applied UTC range receipts. Scope and shared lookup quotas remain host-bound;
other history/memory actions do not accept time filters. Prompt/table goldens
match the new schema.

Vision requests also use the response adapter before description normalization.
Pure text parts concatenate without markup; missing, unknown or mixed output and
response conversion panics skip the description without caching partial text.
Existing cancellation gates, HTTP bounds, UTF-8 truncation and cache metadata stay
in place. Regression cases cover OpenAI and GenAI failures and subsequent recovery.

Agent requests now use a response-only model adapter. Parse delegates to the
chosen protocol, then accepts only a nonempty set of text parts. It checks their
combined 1 MiB size before allocation and concatenates without tags/separators;
Output and OutputRaw expose one text part. Explicit text "" remains compatible,
while absent, unknown or mixed output fails as ErrModelRequestFailed. Panics
inside response parsing/extraction become a fixed content error with no panic
value; request construction, history and platform calls are outside this guard.
Each GetActionChecked clones the protocol to prevent stale response replay.
Tests cover OpenAI/GenAI multipart and OLLaMA text, missing/mixed/unknown output,
GenAI conversion panic, exact/overflow capacity, zero writes and protocol reuse.

Agent API.Request errors are wrapped as ErrModelRequestFailed. The default
Error text is fixed and contains no provider body, URL or credential. Unwrap
and Is preserve the original cause for host response-limit and timeout checks,
and As still reaches the transport error. Post-request task checks take priority;
preparation/history and vision-skip behavior remain unchanged. Tests cover 401,
503, invalid/empty protocol JSON, EOF/timeout cause preservation, safe text, zero automatic
writes and stop priority. No retries or fallback are introduced.

Before decoding action objects into APIRequest, a token walk checks decoded keys
for duplicates in every object, including nested parameter/message objects and
unknown fields. The root action/params/echo names use EqualFold to match struct
aliases; ordinary map keys remain case-sensitive. This rejects escaped duplicate
names, overwrite and Params-merge ambiguity before any automatic writes or end
handling. Separate objects can reuse keys, quoted text is not parsed again and
numbers retain UseNumber semantics. Failures reuse ErrActionOutputInvalid. The
prompt and tests cover exact/escaped/case aliases, nested arrays, independent
objects, large numbers, deep valid input and both automatic/manual zero writes.

Action preflight now decodes until EOF instead of using Decoder.More, which
can silently ignore stray closing brackets. Before any automatic write, every
item up to the first end marker must decode as an APIRequest JSON object.
Malformed JSON, non-object values, incompatible field types and invalid UTF-8
return ErrActionOutputInvalid with no executable prefix. UseNumber matches
execution parsing, preserving large IDs and number literals. Existing code
fences, blank outputs, empty objects, first-end and permission behavior remain.
Regression tests exercise malformed suffixes after save/send prefixes in both
automatic and manual modes, with zero memory/history writes.

Each raw Agent output is capped at 1 MiB, before removing code fences. A preflight
counts up to 32 JSON action objects (including empty objects and the end marker)
before any automatic request, memory or terminus write. Exceeding either bound
returns ErrActionOutputLimit without an executable prefix. Parsing still stops
at the first end marker; permission behavior remains compatible. Later syntax
validation described above replaces the previous partial parse-error behavior.
The prompt documents the limits without adding a total iteration cap. Agent API
failures no longer log provider response bodies. Tests cover exact bounds,
overflow, fenced output and automatic-save prefixes with zero writes.

Image descriptions are normalized before annotation or caching. Blank, invalid
UTF-8 and NUL-containing results are discarded. Each description is capped at
4000 UTF-8 bytes without splitting characters. The typed cache retains its
truncation bit, emitted as __agent_desc_truncated__="true" for partial text;
normal text clears stale flags. Existing annotations are checked too, and already
partial text remains partial. The system prompt explains this marker. Vision API
errors no longer log provider response text. The host separately bounds model
HTTP response reads; standalone SDK consumers configure their own HTTP client.

GetActionChecked accepts a request-local host check. It checks before preparation,
after memory loading, before event/image preprocessing, after downloads, around
vision calls and around the Agent HTTP request. The first error is retained and
returned without parsing actions or recording a terminus. Non-nil checks require
a HistoryStore; the old API and nil-check calls retain legacy behavior. Vision
preprocessing uses a local protocol clone instead of mutating its shared template.
No callback is installed on a shared Agent per task, and admitted HTTP requests
are not aborted. Tests cover memory/history boundaries, blocked download/vision,
post-response cancellation, compatibility and concurrent conversation isolation.

Agent image preprocessing now bounds each HTTP(S) fetch to 10 seconds, including
redirects and body reads. Only nonempty 200 responses are accepted; declared and
streamed/decoded bodies are limited to 8 MiB. Base64 is bounded before and after
decoding. DecodeConfig rejects edges above 8192 and areas above 16,777,216 pixels
before full decoding. Invalid inputs are skipped without a vision request or
cached description. Errors omit URLs, response text and encoded image data.
Existing resizing and perceptual caching remain; hashing still requires loading
the image, and retained historical images can be downloaded again. Tests cover
HTTP status, streaming, gzip, deadlines, closure, dimensions and real preprocessing.

The `save_memory` action documents optional host save receipts: `mode`, and for
managed storage, `created` plus `memory.id/revision/source/sourceSeq` strings.
Reused memories retain their original provenance. Legacy files have no invented
record ID. Generic SDK automatic saves can still return null; this patch adds no
new action or permission. Prompt and role-table expectations cover the update.

The `search_memory` prompt documents optional `before`, a positive decimal
memory ID string for an exclusive older-ID boundary. It is neither a history
source sequence nor a revision. The host retains the enabled/current-version
filter, conversation scope, scan/response budgets and shared three-call quota.
The existing role allowlists remain in force.

The local `read_memory` action accepts a memory ID and exact current revision.
The host only returns an enabled row in the current conversation, with bounded
redacted text and provenance; stale or disabled revisions are not readable.
It shares the three-call history/memory allowance and is never sent to OneBot.

The scoped local `search_memory` action retrieves enabled current memory with
version and provenance. The host enforces bounded scanning, redaction and a
shared three-call allowance with history search/read. Unassigned legacy text
and disabled revisions are excluded; the action is never forwarded to OneBot.

The `search_history` prompt also documents optional `before`, a positive decimal
sequence string for an exclusive older-record boundary. The host retains the
same conversation, reset boundary and shared search/read allowance.

The local `read_history` action lets permitted roles follow a source sequence
within the current conversation. The host enforces reset boundaries, event-only
reads, bounded redacted text, and a shared three-call search/read allowance.
Neither history action may be forwarded to OneBot.

M2 adds the scoped local `search_history` action to role prompts. The host must
execute it locally after authorization, never forward it to OneBot. The application
now executes memory saves in its checked action loop (manual memory mode). Optional
`CheckedMemoryStorage` propagates persistent read failures into prompt construction;
legacy implementations retain the existing interface. `SetMemoryStorage` is used
only before publishing an Agent. Prompt/permission golden tests include the new tool.

Baseline: `github.com/fumiama/go-onebot-agent` at `9c92a03776e3` (2026-04-25).
The original LICENSE and embedded prompts/permissions are retained.

An optional `HistoryStore` receives events, action attempts, results and end
markers. When enabled, prompt reconstruction reads that store instead of the
upstream 16-batch/8-item in-memory ring. Unmodified users retain the original
ring. Record methods return errors so execution can fail before an unrecorded
action. The image preprocessing hook also applies to restored typed events.

The application supplies private SQLite storage, conversation isolation,
redaction, bounded model input and retention without a turn/count limit. The
upstream action wire format remains compatible. Local decoding validates full
batches, duplicate fields and byte/structure limits before persistence or dispatch.
Existing permissions remain intact; M2 appends host-handled local history and memory
queries. The root project's scripts/test-agent.ps1 runs SDK, host, browser and
isolated integration checks together.
