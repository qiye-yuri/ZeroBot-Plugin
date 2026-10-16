# 本地工具模块补丁

M1～M3 的任务分页统一进入 PageMatching/TaskPageFilter，组合状态、错误类别、阶段、最近动作结果及开始时间。PageInRange 和原分页方法保留兼容；使用真实时刻保留时区/纳秒，结果 none/other 区分空值与旧值，当前进程故障摘要先覆盖旧磁盘状态，再应用全部条件。沿用创建序号分页，无数据库迁移；后台使用及完整复验见根项目 docs/local-admin/agent-tasks.md、agent-integration-test.md。

新增管理用 ViewRevision，复用 view 作用范围、存储模式及脱敏，按 conversation、memory_id 和 revision 精确读取已保存修订，允许旧版及当时停用状态。返回既有 MemoryView，最多一条，不修改当前记忆；旧列表/历史分页及模型 ReadEnabled 保持。无数据库迁移。

记忆新增 SearchCurrentInRange/SearchEnabledInRange，沿用原状态/来源/会话筛选和有界扫描，以当前 updated_at 匹配含 from、不含 until，支持时区及纳秒并回显 UTC。未知时间 partial，允许仅时间查询；Agent 固定启用且去除管理 state，仍占原共享额度。旧无时间入口保留，无结构迁移。

任务列表新增 PageWithFilters，在原状态及错误类别之外按当前 phase 筛选，SQL 过滤先于取页，再覆盖当前进程内存任务核对全部条件。旧 Page/PageFiltered/PageWithReason 默认全部阶段，缺失/未知旧阶段保留在全部列表；沿用创建序号、容量及会话隔离，无结构迁移。

新增管理用 ViewVersionsNewest，从最新修订按排他修订上界读取，每页 50 条，复用原 View 的模式、会话隔离、脱敏和分页返回结构；旧 View/ViewFiltered 的升序语义保持。版本历史读取不写入、不改变启停或模型精确版本权限，无数据库迁移。

历史搜索新增 SearchEventType，按事件顶层 post_type 限定 message/notice/request；类型可独立查询，扫描计数/字节限制前进规则保持，工具记录排除，未知元数据跳过且 partial。Agent 与后台 eventType 参数及回执同步，继续沿用条件，来源/上下文/记忆读取不接受类型，无存储迁移。

新增管理用 SearchCurrent，复用有界当前记忆检索，在扫描前按 active/disabled/all 筛选并返回实际启停状态及 state 回执。原 SearchEnabledFromSource 固定 active 并移除管理状态回执，Agent 参数仍拒绝 state，精确版本读取继续要求启用。仅后台扩展状态查询，无新表或迁移。

启用记忆支持仅按具体来源浏览：SearchEnabledFromSource 在有效非空 source 下允许空白关键词，宿主 search_memory 同步支持省略 keyword。原无来源搜索仍须关键词；来源、启用状态、编号边界、脱敏与容量预算和三次共享额度保持，无存储迁移。

记忆新增 ReadCurrent 供后台按编号定位当前管理条目，直接按会话和 id 查询，返回既有 MemoryView，允许停用状态，编号/版本/来源序号保持字符串，正文沿用配置脱敏。只读不新增版本，不暴露未归属旧文件；模型 ReadEnabled 的精确版本与启用要求保持。无数据库结构迁移。

历史搜索在有有效时间边界时允许空关键词，Agent 与后台规则一致，不必指定发言者；沿用原时间比较、重置、预算及三次共享额度。新增无关键词参数与实际模型任务取用来源回归，时间扫描预算用例覆盖空窗口继续。无新存储结构或执行分支。

启用记忆新增 SearchEnabledFromSource，原 SearchEnabledBefore 调用保留不限来源行为。来源在 SQL 扫描前按当前字段筛选，启用状态、版本、正文容量与编号继续保持；source 条件随搜索响应回传，精确正文读取不继承。Agent 仅 search_memory 接受来源，沿用原三次合计查询额度，无结构迁移。

历史搜索在指定正 SearchSender 时允许空关键词，沿用消息作者筛选、时间/序号和实际继续位置，扫描及返回预算保持。Agent 参数接受省略或空白 keyword；历史查询没有发言者或时间条件、记忆查询没有具体来源时仍须关键词，所有查询共用原三次额度。未增加工具或执行分支。

SDK 动作结构超限沿用 model_output_limit。真实任务回归增加深层容器、单动作大量值及跨动作共享值预算三类，均在前缀归档/保存/派发前失败，保留一次模型准备、零动作/查询/兜底及可靠终态。整机新增两类有效 JSON 坏结构与恰好 64 层正常恢复，不更改角色、会话、任务取消和重试规则。

历史 SearchSender 仅用于搜索，沿用扫描预算后按消息事件顶层 user_id 精确筛选，UseNumber/正整数字符串避免精度损失。非匹配仍计预算，未知元数据 partial，通知/工具/其他事件不算作者，继续位置保留真实处理进度。Agent sender 及成功回执与后台同步，当前范围、最近重置和三次共享查询额度保持；默认查询和来源上下文不继承此条件，无结构迁移。

任务 PageWithReason 支持状态和错误类别组合，Page/PageFiltered 维持全部类别兼容。SQL 读取 JSON error 并在分页前筛选，仅允许已知类别及 all/none/other，参数化绑定；不迁移任务结构。当前内存摘要覆盖后再次判断条件，未保存存储故障不会被旧磁盘值隐藏；大整数分页、未知/缺失类别、真实错误终态、写失败覆盖和损坏正文均由存储回归验证。

read_history 支持 context 0～3。先在同一事务和快照内核对中心事件及范围，再有界读取前后事件；不存在、工具、重置前或其他会话中心不返回邻居。最多 7 条，中心 8,000/邻居 2,000/合计 16 KiB，字符边界截断，元数据不证明中心存在。默认单条兼容，任务仍计一次 read_history；后台现复用此只读查询提供人工前后事件核对，不消耗任务额度。

启用记忆搜索新增 searchMore/nextBefore 回执，按已完整处理的记忆 id 继续，空扫描窗口也可前进，不丢弃容量边界上的未返回匹配。partial 与是否有下一窗口分开；读取旧/停用版本和旧文件不提供继续位置。任务测试验证按真实回执继续、助手重建共享额度及按准确版本读取，原存储结构和输入/扫描/返回限制保留。

历史搜索新增 SearchLimit 和 searchMore/nextBefore 回执，limit 在档案读取时生效，继续位置按完整处理行计算，保留未返回匹配。空结果可继续扫描下一窗口，截断/异常行导致的 partial 不伪装成仍有下一批。模型四轮回归验证 >2^53 序号、两次搜索后读取来源与持久任务三次额度，预算/时间/重置/范围边界保留。

search_history 增加可选 from/until，和后台共用绝对时间解析器；成功响应返回已应用 UTC 边界。仅历史搜索接受时间参数，固定 EventsOnly、最近重置与任务会话，沿用原扫描预算与三次共享额度。参数/存储错误、助手重建与回执验证由真实任务和模型往返测试覆盖。

后台只读 ArchiveQuery 增加 SearchFrom/SearchUntil 时间范围，限文本搜索，开始包含/结束排除。RFC3339Nano 解析保留纳秒与偏移时区，未知旧时间跳过且 partial；范围外记录仍计入原 5,000 条及 4 MiB 扫描预算。无模式迁移，默认查询、重置边界、源读取与模型 search_history 不变。回归包含纳秒、偏移、重置/私聊隔离及日期筛选不能绕过扫描预算。

实际模型故障任务回归增加 HTTP 200 有效 JSON 但缺失输出、文本保存前缀后含未知内容两项，验证响应适配器失败后仍为 model_request_failed，零派发/归档/查询/兜底并准确持久化；继续使用原任务类别和故障优先级，不新增网页参数。

NextAgentActionsChecked 的 EOF 结束判定排除 ErrModelRequestFailed，修正空 HTTP 协议响应被误作 end_action、进而可能触发普通聊天兜底的问题；显式结束动作兼容。新增空协议实际任务用例先复现该问题，再验证失败终态、零派发/归档/兜底；SDK 的 EOF cause 仍可通过 Is 识别，不丢失原诊断链。

TaskStore.Finish 将 SDK ErrModelRequestFailed 保存为 model_request_failed，取消、设置更新、输出限额和存储故障仍保持原优先。实际任务测试覆盖 401/503/非法响应协议的零派发、零动作归档、一次模型准备和准确持久终态；错误类别只保存固定码，不写入提供商错误正文。额外用例覆盖取消、设置失效、容量、无关错误及写失败，不改变 HTTP 交换统计或自动兜底规则。

持久任务的坏格式回归增加重复 action 别名、重复目标参数和嵌套消息字段，验证 SDK 整批拒绝后零派发/查询/动作归档，沿用 model_output_invalid。原阶段时间测试在需要区分两个时间样本前先等待系统时钟实际前进，避免 Windows 同刻时间造成严格递增断言偶发失败；不改变产品时间戳，也不承诺阶段时间一定唯一或单调。

TaskStore.Finish 将 SDK ErrActionOutputInvalid 归类为 model_output_invalid；取消和存储故障保持优先。实际持久任务验证有效保存/发送前缀后的多余括号、不完整 JSON、null 均零派发、一次模型准备、零请求/响应/结束归档，终态准确；格式错误不会进入普通聊天兜底，也不补造动作失败回执。

TaskStore.Finish 将 SDK ErrActionOutputLimit 归类为 model_output_limit；取消及存储故障仍按原优先级处理，不抹去前轮已结算动作。宿主拒绝超限输出后不派发可执行前缀、不调用普通聊天兜底，HTTP Agent 容量错误由宿主映射为同一类别。实际持久任务测试覆盖对象及字节超限的零派发、零请求/结束归档、正确终态和一次模型准备，原超过八轮运行测试保持。

NextAgentActionsChecked 将本次宿主任务检查传给 SDK GetActionChecked，原 NextAgentActions/CallAgent 保留空检查兼容与原范围/角色过滤。aichat 按当前 run.Check 和 AgentGeneration 检查准备许可，不把任务回调写到共享 Agent；取消及配置更新会在记忆/历史/图片/模型准备边界阻止后续请求，当前在途请求仍等待返回。合成持久任务验证下载中停止、识图中停止及设置代次变化，终态、零动作/查询和原故障分类保持。

AgentTaskInfo 增加可选 PhaseStartedAt/CancelRequestedAt UTC 字符串。每轮阶段登记刷新前者，摘要更新不改；首次有效 Cancel 在原范围锁内登记后者，重复/并发停止不改，过期版本不登记。Finish 与恢复使阶段时间等于 EndedAt，恢复仅表示发现旧任务，不补造旧停止时间。初次状态保存失败也进入内存 finished 阶段并保持故障标记。旧 payload 保留未知，写失败的时间仅在未保存摘要可见；合成阶段、并发、恢复和失败用例验证。

MemoryStore.SaveFromSequenceResult 返回 MemorySaveResult 和不含正文的 MemoryReference。新建在正文/首个修订提交后返回 created=true；同范围相同启用正文选择最新编号，返回原版本及来源、created=false，不改时间或历史。失败返回零结果，损坏引用拒绝成功；旧文件沿用已检查配置的追加和脱敏，只返回 legacy 模式。旧 Save/SaveFrom/SaveFromSequence 保留 error 接口。消息入口将真实元数据响应入档，入档失败不登记本地成功、不自动重试；大整数、版本、并发、隔离、旧文件和失败回滚有合成测试覆盖。

TaskQueries 持久化每任务共享额度及查询类型/响应覆盖计数，TaskStore 绑定打开数据库的 bot。NewTaskHistorySearch 校验活动任务账号和会话，重建继承原额度。HistorySearch 在请求前记录已用额度，在响应成功入档后结算 successful/empty/partial，错误归入 errors，超限只增加 denied；Execute 返回错误时 Outcome 不再保留成功。摘要保存失败停止查询，已登记动作取消后仍保存其真实结果；旧摘要不补造，重启未确认统计保留额度但不推断成功。覆盖、隔离、重建、取消、入档/摘要写失败与旧数据均由合成测试验证。

TaskResults 记录每任务已结算的 ok/rejected/unknown/localErrors，Available 区分旧摘要与实测零。Result 只接受 action_wait/in_flight 的合法结果，禁止重复结算；Model/FallbackModel/Action 不能覆盖尚未结算的动作。Finish 与其他实例恢复将未确认在途结果记为 unknown，已结束任务不再结算。字段与原 payload 原子保存，未落盘计数保持故障标记，旧 payload 不推断历史结果；测试覆盖分类、取消回执、并发重复结果、重启幂等、旧数据与写失败。

MemoryStore.SaveFromSequence 校验准确来源属于同一会话的 event，再执行归属检查、正文去重和事务保存；来源校验不被相同正文的成功复用掩盖。Save 与旧 SaveFrom 消息编号调用兼容保存，但未知来源为 0，不再以最近 5,000 条中的重复平台编号猜测。消息入口捕获一次事件序号供任务及记忆共用；初始修订持久化相同来源，重复保存不改首次元数据。合成测试覆盖大整数、重复编号、超旧查询窗口、重置及重开数据库、24 路并发、来源隔离和写失败回滚。

agentHistory.appendWithSequence 复用原序列化/脱敏写入并返回本次 INSERT 的 LastInsertId，原 Append 接口保持；事件钩子把序号绑定到相同事件指针、机器人和会话，AgentEventSequence 不向另一事件复用。TaskStore.BeginFrom 校验来源是同会话事件，初始 payload 保存 SourceSeq 字符串，Begin 委托未知来源 0，旧 payload 缺失保留未知。生命周期与重启恢复自然保留来源，不按重复消息编号猜测；合成并发、大整数、来源边界、旧任务和写失败验证覆盖此行为。

TaskStore.Read 按当前会话和 32 位小写十六进制任务编号直接读取当前摘要，独立于列表分页与筛选。优先复制匹配的内存任务，包含未落盘故障状态并复制可选 MemorySources 指针，防止调用者修改内部摘要；其他任务只读取会话/id 匹配的持久记录。取消上下文、缺失与读取/身份损坏分别上报，不递增版本或重放动作；合成测试覆盖超过首页范围的旧任务、隔离、首次/终态写失败及重启中断恢复。

MemoryStore.SearchEnabledBefore 支持排他记忆编号边界，SQL 在扫描前固定会话、active=1 及 id < before；原 SearchEnabled 委托无边界查询以保留后台与其他调用者兼容。HistorySearch 将 search_memory 的 before 严格解析为正整数字符串，仍共用三次额度和原预算；边界不读取历史修订或旧文件。测试覆盖范围/版本/重置持久语义、5,000 条与字节/片段上限、非法参数及大整数。

MemoryStore.CreateManual 新增后台录入：锁定归属和数据库，仅 managed 范围可创建，归一化/脱敏后相同启用正文复用原条目且不刷新来源和修订；新条目标记 admin/source_seq=0。正文、mode 和首个版本在同一事务内保存，失败回滚，旧文件不写入。

TaskStore.PageFiltered 支持严格状态筛选，原 Page/List 保留全部状态兼容。查询将单个当前内存任务纳入候选并用实际状态过滤；多探测一条避免排除过期数据库状态后丢失后续页。未落盘状态仅首页追加，创建序号和会话边界保持。

ContextUsage 新增可选 MemorySources，在 Modelize 实际选择的历史响应中累计可解析 memories 数组条目，与 Sources 分开统计。当前准备写入零值，旧 payload 缺失保留 nil；重复项照实计数，截断无法解析的响应不计。仅保存计数，重置和兜底沿用原清理边界。

MemoryStore.ReadEnabled 新增精确编号/修订读取当前启用记忆，固定会话并拒绝旧修订或停用条目，复用脱敏及 8,000 字节上限、超大正文占位。HistorySearch 接入 read_memory，与其他历史和记忆查询共用三次额度；宿主本地处理，TaskActivity 分类为本地动作。

MemoryStore.SearchEnabled 新增有界启用记忆检索，固定会话、当前修订和启用状态，排除旧文件，先脱敏再做字面匹配；最多扫描 5,000 条/4 MiB，返回 20 个/16 KiB 片段。HistorySearch 承接 search_memory，与历史检索/读取共用三次额度；宿主本地执行，不转发 OneBot，TaskActivity 归入本地动作。模型输入回归覆盖默认装载范围之外的记忆。

MemoryStore.ViewFiltered 支持当前记忆 all/active/disabled 筛选，在 SQL 分页前筛选状态，保持每页 50 条和会话边界。原 View 委托全部状态以保留兼容；修订历史拒绝非全部筛选，旧文件归属和模型加载行为不变。

AgentTaskInfo 新增 TaskActivity：在 Model/FallbackModel/Action 的既有任务锁与持久化边界内累计阶段登记次数。本地动作按宿主实际处理的本地动作常量分类；取消拒绝不计数，回执不重复计数。旧 payload 解码为未统计，重启恢复保留累计值；计数不代表 HTTP 调用或成功执行。

HistorySearch 支持可选 before 正整数字符串，复用 ArchiveQuery.Before 的排他序号边界，只检索当前会话的更早事件；拒绝非法或越界整数，保留 EventsOnly、重置边界、扫描预算和任务总次数。三轮回环用例验证先检索最新匹配，再定位更早匹配。

MemoryStore.SaveFromSequence 在既有锁和事务内对归一化、脱敏后的正文做同会话启用条目精确查重；重复保存直接复用，不更新时间、来源或版本，不改变停用条目及旧文件行为。存储失败继续上报；并发去重与取用预算回归使用合成数据验证。

HistorySearch 新增 read_history 本地动作，固定任务机器人及会话，以字符串序号精确读取历史事件；与 search_history 共用三次额度，复用只读档案的重置边界、脱敏及 8,000 字节截断。三轮回环测试覆盖检索片段后读取来源，执行不转发 OneBot。

任务新增 MemoryUsage，与历史上下文计数独立持久化；MemoryStore 实际加载时记录独立记忆条数、字节及条数/字节上限原因，只多探测一条判断是否截断，不改变原 100 条和 30,000 字节取用预算。旧文件模式不统计最终提示长度；每轮及兜底重置摘要，统计保存失败阻止继续准备，不保存正文。

MemoryStore 新增 Revert：在同一事务内按会话、记忆编号及当前修订号读取指定旧版本，按当前配置脱敏后生成新修订，保留旧来源序号及当前启停状态。历史版本保留，写入失败回滚，重复或过期请求拒绝覆盖；不复用浏览器中的正文。

档案查询新增 Sequence 精确来源读取：限定机器人和会话，复用只读连接、重置边界、脱敏及正文截断，不调用模型。拒绝与搜索、分页组合，超大记录只返回占位；供后台核对记忆当前及历史版本的来源。

任务历史新增 Page 查询：按固定创建序号每页读取 50 条，状态实时读取，新增任务不挤入旧页；未保存内存状态仅覆盖首页，大整数分页位置以字符串传输。

M3 新增 TaskStore：任务状态持久化、按机器人/会话互斥、版本化停止与重启中断恢复；状态写失败停止派发。历史拼装记录有界上下文摘要，不保存模型思考。插件在模型及动作边界接入，同步覆盖普通聊天兜底；详见主项目 docs/local-admin/agent-tasks.md。

M2 新增 `MemoryStore` 与任务级 `HistorySearch`：记忆按机器人分库，版本与来源持久保存；旧文件通过带摘要的显式归属迁移，备份、重复提交及失败重试保持一致。检索绑定当前会话和重置边界，仅搜索历史事件，最多三次，不命中自身工具记录。运行时记忆保存由动作执行阶段处理，读取错误通过检查接口向上传递。详见主项目 `docs/local-admin/agent-memory.md`。

新增 `chat.ReadAgentArchive` 只读查询适配器：固定机器人档案路径、只读 SQLite 连接、快照分页、重置范围、当前配置脱敏和有界搜索；不创建 Agent 或档案、不调用图片预处理和模型。调用方先校验机器人访问范围。新增档案测试含十万条合成记录及会话分页，使用说明见主项目 `docs/local-admin/agent-archive.md`。

沿用原始许可证与模块结构，根模块和本模块均通过相邻目录使用配套的SQLite、控制层、Agent及资源请求补丁。

主要修改包括AI共享配置与检查写入、会话参数并发合并、Agent事件/动作持久记录、角色和当前群边界、模型协议及消息内容处理，以及服务列表与响应命令的一致控制语义。应用层集成验证见主项目的使用与开发文档。

在本目录执行`go test ./chat ./ctxext ./control`验证相关功能。上游`TestMaru`原实现仅打印一个长度后无条件调用`t.Fail()`，没有功能断言；这里保留名称并明确跳过，不把它计为验证通过。其余功能测试正常执行。

本模块初始化可能按需读取或下载字体等资源；生成的data目录属于运行/测试缓存，不随源码上传。
