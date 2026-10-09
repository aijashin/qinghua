# 数据字典

本文按当前 Go 实现整理。物理结构以 `backend/internal/store/store.go` 的迁移为准，业务写入以 `backend/internal/app`、任务状态以 `backend/internal/jobs` 为准。请求与响应的完整类型见 [OpenAPI](openapi.json)。

## 存储约定

- 单实例 SQLite，数据库文件为 `DATA_DIR/zhiwei.db`，原始文件保存在 `DATA_DIR/files/<服务端存储键>`。数据库启用 WAL、外键检查和5秒忙等待，当前进程最多使用一个数据库连接。
- 业务ID通常由16个随机字节生成32位十六进制字符串；内置默认规则ID为 `teaching-example-v1`。`system` 是示例资料的所有者标识，不是自动创建的登录账号。
- 时间以UTC RFC3339文本保存；应用时间可含小数秒。JSON中的未填写关联常用空字符串，部分文档版本字段在导入草稿中可以为 `null`。
- 下表的PK、UNIQUE、NOT NULL、FK仅描述真实SQL约束。JSON内的业务关联、角色权限、分数范围和状态流转主要由应用验证，不应误认为数据库都设有外键或CHECK。
- 业务实体通过统一 `entities` 表存储，当前没有分别命名为 `samples`、`reports`、`documents` 的物理表，也没有独立的通用审计表或模型训练表。

## 物理表与字段

### `schema_migrations`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `version` | `INTEGER` | PK | 已应用的物理结构版本；当前版本2，版本1建初始表，版本2增加jobs.telemetry。 |
| `applied_at` | `TEXT` | NOT NULL | 首次应用该版本的UTC时间。 |

### `users`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 账号ID。 |
| `username` | `TEXT` | NOT NULL；UNIQUE | 登录用户名，唯一；应用校验3–80 UTF-8字节且不含空白。 |
| `password_hash` | `TEXT` | NOT NULL | bcrypt密码哈希，不保存明文。 |
| `display_name` | `TEXT` | NOT NULL | 显示名称。 |
| `role` | `TEXT` | NOT NULL；CHECK角色枚举 | 角色，数据库CHECK限定student / teacher / admin。 |
| `disabled` | `INTEGER` | NOT NULL；DEFAULT 0 | 0可用，1停用；停用账号不能建立或使用会话。 |
| `created_at` | `TEXT` | NOT NULL | 创建时间。 |

### `sessions`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `token_hash` | `TEXT` | PK | 浏览器会话令牌的SHA-256十六进制摘要；原始令牌只在HttpOnly cookie中。 |
| `user_id` | `TEXT` | NOT NULL；FK → users.id | 所属账号；真实数据库外键。 |
| `csrf` | `TEXT` | NOT NULL | 与会话绑定的CSRF令牌，写请求必须提交X-CSRF-Token。 |
| `expires_at` | `TEXT` | NOT NULL | UTC失效时间；登录创建7天会话。 |

### `classes`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 班级ID。 |
| `name` | `TEXT` | NOT NULL | 班级名称，当前不设唯一约束。 |
| `created_at` | `TEXT` | NOT NULL | 创建时间。 |

### `class_members`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `class_id` | `TEXT` | PK第1列；NOT NULL；FK → classes.id | 班级ID；真实外键，删除班级级联删除成员关系。 |
| `user_id` | `TEXT` | PK第2列；NOT NULL；FK → users.id | 账号ID；真实外键，删除账号级联删除成员关系。 |

### `files`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 文件ID，也是当前实现使用的存储键。 |
| `owner_id` | `TEXT` | NOT NULL | 上传者ID或示例资料的system；无数据库外键。 |
| `name` | `TEXT` | NOT NULL | 经过basename处理的原文件名，仅用于展示和下载文件名。 |
| `mime` | `TEXT` | NOT NULL | 服务端判断的文件类型。 |
| `size` | `INTEGER` | NOT NULL | 原始文件字节数，上传上限20 MiB。 |
| `path` | `TEXT` | NOT NULL | files目录内的服务端生成存储键，不保存客户端路径或外部URL。 |
| `created_at` | `TEXT` | NOT NULL | 上传时间。 |

### `entities`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 业务实体ID；不同kind共享该主键空间。 |
| `kind` | `TEXT` | NOT NULL | 业务类型，见后文；类型值由应用路由设置，数据库不设CHECK枚举。 |
| `owner_id` | `TEXT` | NOT NULL | 实体所有者；并非每次操作人，教师复核记录另有独立owner。无数据库外键。 |
| `class_id` | `TEXT` | NOT NULL；DEFAULT '' | 关联班级ID，未关联时空字符串；无数据库外键。 |
| `status` | `TEXT` | NOT NULL | 实体当前状态，合法值按kind由应用控制。 |
| `data` | `TEXT` | NOT NULL | 实体完整JSON，包括公共元数据与业务字段；物理类型TEXT，未设置json_valid CHECK。 |
| `version` | `INTEGER` | NOT NULL；DEFAULT 1 | 实体快照版本，初始1；应用更新时递增，与data.version保持一致。 |
| `created_at` | `TEXT` | NOT NULL | 创建时间。 |
| `updated_at` | `TEXT` | NOT NULL | 最近一次实体更新的时间。 |

### `entity_versions`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `entity_id` | `TEXT` | PK第1列；NOT NULL；FK → entities.id | 对应entities.id的真实外键。 |
| `version` | `INTEGER` | PK第2列；NOT NULL | 该实体的历史版本序号，与entity_id组成主键。 |
| `data` | `TEXT` | NOT NULL | 当时完整实体JSON快照；保留背景、关联与状态。 |
| `created_at` | `TEXT` | NOT NULL | 生成该快照的UTC时间。 |

### `jobs`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 异步任务ID。 |
| `owner_id` | `TEXT` | NOT NULL | 任务发起账号，与幂等键共同唯一。 |
| `class_id` | `TEXT` | NOT NULL；DEFAULT '' | 创建任务时关联的班级ID，无关联为空。 |
| `kind` | `TEXT` | NOT NULL | assessment / understanding / identify / coaching / document_import。 |
| `sample_id` | `TEXT` | NOT NULL；DEFAULT '' | AI任务所属样本；文档导入为空。 |
| `learning_session_id` | `TEXT` | NOT NULL；DEFAULT '' | 可选学习轮次ID；决定使用哪次冻结的样本快照。 |
| `input` | `TEXT` | NOT NULL | 服务端冻结后的完整请求JSON，包含样本/规则/教学上下文或文档导入参数；不在Job API响应中返回。 |
| `status` | `TEXT` | NOT NULL | queued / running / succeeded / failed / cancelled，应用控制状态转换。 |
| `stage` | `TEXT` | NOT NULL | 简短当前阶段或终结说明。 |
| `error` | `TEXT` | NOT NULL；DEFAULT '' | 失败/取消的安全错误说明；初始为空。 |
| `result_id` | `TEXT` | NOT NULL；DEFAULT '' | 成功时写入报告或文档实体ID；无真实外键。 |
| `idempotency_key` | `TEXT` | NOT NULL | 客户端请求幂等键；同账号同键同输入复用任务，不同输入冲突。 |
| `lease` | `TEXT` | NOT NULL；DEFAULT '' | 领取任务时生成的执行令牌；提交结果时验证仍属于本次执行。 |
| `lease_until` | `TEXT` | NOT NULL；DEFAULT '' | 执行中的租约到期时间，定期续期；当前不是跨机器分布式调度机制。 |
| `next_seq` | `INTEGER` | NOT NULL；DEFAULT 0 | 最后已分配的任务内事件序号，事务递增。 |
| `created_at` | `TEXT` | NOT NULL | 任务创建时间。 |
| `updated_at` | `TEXT` | NOT NULL | 最近状态、事件或结果更新时间。 |
| `telemetry` | `TEXT` | NOT NULL；DEFAULT '{}' | 迁移2新增。安全的任务诊断JSON，包含请求关联、时间、实际阶段历史与耗时；字段见下文。原表的其它列不改名。 |

额外唯一约束：`UNIQUE(owner_id, idempotency_key)`。

### `job_events`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `job_id` | `TEXT` | PK第1列；NOT NULL；FK → jobs.id | 任务ID，真实外键；和seq共同组成主键。 |
| `seq` | `INTEGER` | PK第2列；NOT NULL | 任务内单调递增事件序号，映射到SSE的id。 |
| `event` | `TEXT` | NOT NULL | status / progress / delta / completed / error。 |
| `data` | `TEXT` | NOT NULL | 该事件的JSON负载；progress为结构化实际阶段，delta为兼容安全进度文字。 |
| `created_at` | `TEXT` | NOT NULL | 事件落库时间。 |

### `knowledge_chunks`

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 片段ID，也是报告citations允许引用的ID。 |
| `document_id` | `TEXT` | NOT NULL；FK → entities.id | 所属documents业务实体ID；物理外键指向entities.id，kind由应用保证。 |
| `title` | `TEXT` | NOT NULL | 导入时的资料标题。 |
| `content` | `TEXT` | NOT NULL | 可核对的原文片段；PDF的真实页码以正文标记保留。 |
| `position` | `INTEGER` | NOT NULL | 文档中片段顺序，从0开始。 |

### `knowledge_fts`

FTS5虚拟表；SQLite自行维护其影子表，业务代码不直接写影子表。

| 字段 | 类型 | SQL约束/默认值 | 含义 |
| --- | --- | --- | --- |
| `chunk_id` | `FTS5列` | UNINDEXED | 对应knowledge_chunks.id的逻辑关联；UNINDEXED列，无外键或唯一约束。 |
| `tokens` | `FTS5列` | — | 用于全文检索的中文单字/二元词和英文词；由应用生成，FTS5使用unicode61。 |

### 索引与实际关联

显式创建的普通索引只有 `entities_kind_owner(kind, owner_id, created_at)` 和 `jobs_queue(status, created_at)`。PK和UNIQUE约束由SQLite维护相应索引；FTS5索引由虚拟表维护。

真实外键为：`sessions.user_id → users.id`；`class_members.class_id → classes.id`、`class_members.user_id → users.id`（两者均级联删除）；`entity_versions.entity_id → entities.id`；`job_events.job_id → jobs.id`；`knowledge_chunks.document_id → entities.id`。其余ID关联是应用层逻辑关联，没有隐含的数据库级联删除。

文件表当前不保存图像宽高、内容SHA-256或外部对象存储URL；上传时会校验图像尺寸和完整解码，但不能据此假设存在这些列。模型地址和密钥从进程配置读取，不保存到业务表。

## 业务JSON实体

### 公共元数据和版本

`entities.data` 通常包含 `id, owner_id, class_id, status, version, created_at, updated_at`，与外层列一致。`owner_id` 决定归属，`class_id` 支持教师范围校验；两者不由普通PATCH随意修改。

`Store.Create/Update`、学习完成归档、文档发布和任务结果事务会维护 `entity_versions`。学生修正会新增独立 `revision` 记录，并保存原 `reports` 实体的新版本；当前 `revision` 记录本身不另外写入 `entity_versions`，它就是不可覆盖的修正历史。

三个版本计数含义不同：

| 字段 | 含义 | 并发/历史用途 |
| --- | --- | --- |
| `version` | 某一实体的修改快照序号，初始1 | 样本与规则等完整状态历史 |
| `revision_no` | 某份报告的学生修正次数，初始0 | 提交修正的 `base_version` 必须匹配，不匹配409 |
| `document_version` | 一套知识资料的新旧内容版本，初始1 | 新资料替换发布及来源版本显示 |

样本PATCH当前不使用 `base_version` 作并发控制。已存在的轮次和报告通过 `sample_snapshot`/`sample_version` 固定输入，不会随样本最新版本而改变。

### 学习与图谱

| `kind` | 核心字段 | 状态和说明 |
| --- | --- | --- |
| `samples` | `title, file_id, image_url, material, process, magnification, etchant, demo, source_atlas_id` | `active`。四项背景必须有值；不知道明确填“未知”。换图/改背景产生新版本。使用已发布图谱时复制为独立文件并保留来源标记。 |
| `learning-sessions` | `sample_id, title?, sample_snapshot, sample_version, step, completed, parent_session_id?, task_id?, personal_atlas_id?, demo, source_atlas_id` | `active/completed`；步骤为 `atlas/assessment/understanding/practice`。`parent_session_id` 关联上一轮，`task_id` 关联已发布且分配给本人的任务。 |
| `atlas` | `title, material, phase, process, magnification, etchant, description, observation, file_id?, image_url?, source, demo` | `draft/published`。教师可建草稿，管理员发布；编辑公开条目会恢复草稿。它是审核发布的教学图谱。 |
| `personal-atlas` | `title, sample_id, sample_snapshot, learning_session_id, file_id, image_url, material, process, phase, dimensions, report_ids, reports, demo, source_atlas_id` | `private`。来自完成的学习轮次。`reports` 是以 `assessment/understanding` 为键的归档报告快照，而不是报告数组；后续即时教师反馈可按 `report_ids` 查询原报告记录。 |

学习完成要求同一轮次的评图和五维理解报告都存在，版本与轮次样本相符，且分别至少保存一份采用/修正稿。完成时写入个人图谱并更新学习轮次，重复完成返回已存在的归档。`demo` 始终沿样本、轮次、报告和个人图谱传播，不因学生完成学习而变成真实实验数据。

### AI报告及来源

`reports` 的核心字段：

| 字段 | 内容 |
| --- | --- |
| `kind` | `assessment / understanding / identify / coaching` |
| `sample_id, sample_version, sample_snapshot` | 使用的样本及其不可变背景/文件快照 |
| `job_id, learning_session_id` | 生成它的异步任务与可选学习轮次 |
| `data, original_data` | 校验后的原始AI报告；学生修正和教师复核不覆盖这两份原稿 |
| `revision_no, latest_revision` | 当前学生修正序号及最新完整修正记录；初始为0与null |
| `sources` | 本次实际检索且被模型引用的 `{id,title,content}` 快照；与报告citations对应 |
| `retrieved_sources` | 本次检索到的全部候选来源，包含未引用片段，不能全部当作模型的引用证据 |
| `rubric` | 生成时的完整评分规则快照，包括规则ID、版本（如果存在）、四项和细则 |
| `model, prompt_version` | 实际配置的模型名称与代码提示词版本，当前提示词版本 `metallography-v2` |
| `latest_review?` | 最新独立教师复核记录 |
| `demo, source_atlas_id` | 沿样本保留的示例来源 |

报告状态为 `pending_review/reviewed`。这里的审核状态不等同于实验验证。`model` 是独立配置的模型标识，不证明图片识别或教学结论已达到某个准确率。

各类 `data`：

| 类型 | 字段和限制 |
| --- | --- |
| `assessment` | `summary, criteria, defects, limitations, citations, total_score, max_score, coverage, rubric_id`。`criteria` 是固定 `scratches/artifacts/etching/clarity` 四项，各25分，包含 `label, score, max_score, evidence`。`score=null` 表示无法观察；任一项不可观察时 `total_score=null`，不能当作零分。总分和覆盖率由服务端计算。 |
| `defects[]` | `name, severity, evidence, suggestion`；可选 `bbox{x,y,width,height}`。severity限定low/medium/high/uncertain，bbox归一化且在图像内，仅为观察标注。 |
| `understanding/identify` | `summary, candidates, dimensions, questions, limitations, citations`。候选包含 `name,evidence,uncertainty`；五维恰为成分、工艺、组织、性能、应用，每项 `name,content,basis`，basis为known/observed/inferred。 |
| `coaching` | `summary, questions, suggestions, verification_plan, limitations, citations`，提供有前提的训练建议与验证问题。 |

模型输出的 `citations` 只能取本次实际检索片段ID；保存报告时 `sources` 只保留被引用项，`retrieved_sources`保留全部候选。学生修正不能增加不属于原报告sources的引用。来源归档后旧片段不删除；有权访问相关历史报告的用户仍可读其引用片段。原始来源快照没有单独的“知识库版本号”字段；具体文档归属通过片段表定位，文档版本通过文档实体和知识检索响应提供，不应虚构另一套版本列。

### 学生修正与教师记录

| `kind` | 业务字段 | 写入与效果 |
| --- | --- | --- |
| `revision` | `report_id, data, reason, revision_no` | 报告所有者提交完整修正稿及理由；`base_version`是请求参数，不作为记录字段保存。服务端重算分数；规则文字可保存在评分项的 `rule_text/description/rule_description`，组织判断可含 `final_phase/phase_reason`。只影响个人记录。 |
| `review` | `report_id, decision, comment?, corrected_score?` | 教师/管理员明确复核，decision为confirmed/needs_revision；另存记录并更新报告latest_review与审核状态，原AI数据不变。 |
| `feedback` | `report_id, comment` | 教学指导，不自动确认报告。 |
| `viewed` | `report_id` | 教师已查看事件，不是复核决定。 |

四种记录的实体状态均为 `saved`，所有者是实际提交该记录的账号。当前没有单独的 `reviewed_revision_no` 字段；教师复核记录不是某个学生修订版本的强绑定签名，展示时应保留记录时间并对照修正历史。

### 练习任务与学生回答

`practice-tasks` 存 `title, instruction?, sample_id?, report_id?, due_at?, assignee_ids, assignments`。状态只有 `draft/published`；完成不改变整个任务的发布状态。

- `assignee_ids` 是分配的学生ID列表。教师可发所授班级或指定学生；学生自建只分配本人。
- `assignments` 保存完成记录 `{user_id,status:"completed",completed_at,reflection}`。学生读取时仅返回本人记录；尚未完成时响应可补充 `{user_id,status:"open"}`，这项是展示时计算的，不是另一张物理表。
- `attempt` 实体只保存 `task_id, answer` 及公共元数据，状态 `submitted`。没有保存 `learning_session_id`、`sample_id` 或自动批改分数。
- 教师任务详情通过 `learning-sessions.task_id + owner_id` 找到相关轮次，再用 `reports.learning_session_id` 查看该轮报告；不能假设attempt响应自带轮次。任务创建者可以读取其任务回答，其他教师按授课/任务授权范围读取。
- `due_at` 当前是原样保存的字符串，没有数据库日期类型或自动逾期调度。

### 评分规则和知识资料

`rubrics` 存 `title, description?, criteria, demo?`，状态 `draft/published`。`criteria` 中四个固定ID各出现一次，`max_score=25`，可带 `rule_text/description`。发布新规则版本不会改写已有报告的规则快照。

`documents` 存：

| 字段 | 内容 |
| --- | --- |
| `title, file_id, content, chunk_count, demo` | 资料标题、原文件、提取全文、片段数量、示例标志；内置示例资料可以没有原文件 |
| `document_version` | 资料内容版本，首版1，替代版本递增 |
| `series_id` | 资料系列ID；首版发布时设为该资料ID，替代版本沿用；新导入草稿可为null |
| `replaces_document_id` | 明确指向被替代的当前已发布资料；首版可为null |
| `replaced_by` | 旧资料归档后记录的新版本ID |
| `published_at` | 发布时的UTC时间 |
| `material_tags, phase_tags, process_tags` | 材料、组织、工艺标签数组；每类最多12项，每项1至64字，trim去重。未填写时为空数组。 |

导入成功创建 `draft` 文档及其全部片段/FTS索引，但检索SQL只选择 `published` 文档。发布替代版本时，同一事务将上一版置为 `archived`，新版本置为 `published`；如果上一版已被替换，返回409。旧版全文和片段仍保留，不进入新检索。

物理片段表没有独立章节、页码或向量列。目前正文提取会保留可靠的PDF `[第 n 页]` 标记；DOCX没有可靠分页时不会虚构页码。全文检索使用中文单字/二元词和英文分词，生成限定长度的安全FTS查询；最多返回6个已发布片段。导入时标签也加入FTS词，但不追加到原文content。`knowledge/search`可同时按`material_tag/phase_tag/process_tag`精确筛选，类别之间AND，或仅按标签查询；旧资料无标签时按空数组处理。个人图谱和学生修正不会自动写入该公共索引。

## 任务状态、事件与模型调用边界

AI任务创建时固定样本快照和规则。任务若关联学习轮次，使用该轮快照；后续改图或改背景不会偷偷替换输入。coaching任务还在创建时冻结`teaching_history`和`practice_goal`：前者为当前匹配样本/轮次版本的评图、五维报告最新修正以及可用的上一轮记录，条目含`phase(current/previous), kind, data, revision_no`；后者为分配任务的`title/instruction`。模型接收的history来自此服务端快照，忽略客户端提供的history。历史结论仍是学习记录，不冒充本轮新检索的引用。

任务队列默认两个worker，持久状态为：

`queued → running → succeeded / failed / cancelled`

成功写入业务结果、片段（导入时）、任务终态和completed事件在同一个事务内完成。提交前检查仍为running且lease匹配；取消或其它执行已经终结的任务不能被迟到响应覆盖。服务重启把残留running任务标记failed，排队任务继续；不会自动重放可能已经计费的模型调用。`lease_until`当前用于执行心跳记录，不应据此宣称支持多实例失效接管。

SSE使用 `job_events.seq` 作为id，支持Last-Event-ID或after回放：status数据是Job；progress是实际阶段对象；delta是兼容安全进度文字；completed含 `job_id, report_id, result_id, result_kind, finished_at, timings`（report_id是兼容别名，文档应按result_kind读取）；error含code、message及可用的finished_at/timings。浏览器断开不会取消后台任务。事件拉取每批最多200条，继续回放直到完成。

### 安全诊断JSON与计时

`jobs.telemetry`存一个JSON对象，在Job API中以以下顶层字段展开。它不是名为telemetry的新接口，也没有为每个诊断字段新建物理列。迁移前记录默认为 `{}`，没有的时间与token字段应保持缺省。

| JSON字段 | 类型与含义 |
| --- | --- |
| `origin_request_id` | 创建任务的HTTP请求关联ID；用来查终端日志，不改变权限 |
| `started_at/finished_at` | UTC RFC3339时间；未开始、未结束或旧记录可缺省 |
| `error_code` | 安全错误分类，和供用户阅读的jobs.error说明分开 |
| `progress` | 当前实际阶段 `{code,label,at,elapsed_ms,attempt?,completed?,total?}` |
| `progress_history` | 按执行顺序保存的阶段数组；包含排队及终态，浏览器重连可恢复 |
| `timings` | `{queue_ms?,total_ms,model_ms,stages?,model_calls?}`；所有耗时单位毫秒 |

阶段code包括queued、preparing、retrieving、model_waiting、model_receiving、validating、repairing、persisting、extracting、chunking、succeeded、failed、cancelled。code用于程序判断，label供界面展示；不是按时间模拟出来的完成率。文档切片时completed/total用于真实片段计数，不能用作模型推理完成比例。

`stages`的条目为 `{code,attempt?,duration_ms}`。`model_calls`的条目为 `{attempt,duration_ms,first_content_ms?,input_tokens?,output_tokens?,total_tokens?}`；服务商没有返回用量时不填写token值。`model_ms`是实际调用耗时之和，格式修复也算独立调用。`total_ms`从开始处理计至结果保存前的快照，不包含排队或最终数据库提交；`queue_ms`单独记录。`finished_at`是准备写入终态时的时间，job与成功报告共享同一次计时快照，时间戳之差与total_ms不必完全相等。运行中取消会截取当时计时，未返回模型调用只计截至取消时已消耗的时长；排队即取消、重启处理中断及旧记录没有可信计时时保持缺省，不按updated_at猜测。

成功生成的报告或导入文档在自身实体JSON中同样保存 `started_at/finished_at/timings`，不是entities表新增列。原始模型请求、回复正文不写入telemetry、job_events或报告诊断字段；仅在 `debug + ZHIWEI_LOG_AI_CONTENT=true` 时以脱敏分段日志写stderr。日志不是数据库审计记录，也不作为教材内容。

模型结果必须通过字段、评分范围、五维枚举、归一化标注和真实引用ID校验。bbox采用整图左上角原点及宽/高比例，四个坐标必须明确为数字，合法矩形不代表专业定位已验证；学生评分修正不采信客户端bbox，仅同序同名缺陷继承有效原框，其它条目移除位置；AI原稿保持不变。结构不合格最多一次修复请求；流没有正常结束、拒答、超时或取消时不会写成功报告。没有模型配置时API返回明确不可用状态。模型端点、API密钥和原始HTTP错误体不进入报告或事件记录；token用量仅在服务商返回时记录，不保存实际费用或模型权重版本，不能把model字段扩展解释为这些数据。

## 权限与统计口径

- 学生读取自己的样本、轮次、报告和个人图谱；已发布任务须在分配列表内。学生任务响应会移除同学的完成与反思内容，班级响应只保留自己的成员信息。
- 教师按当前班级成员关系读取学生记录；自己创建的任务关联学习轮次、样本、报告也在授权范围内。角色来自会话关联的users记录，不由前端切换角色决定。
- 管理员管理全部记录、账号、班级、公共资料发布。已发布公共图谱、规则和资料对登录用户可见；草稿按所有者/管理员权限读取。
- 文件有独立鉴权：文件所有者、管理员，或有权访问引用该文件的实体/历史快照者才能下载。报告引用片段同样独立检查授权，不能凭猜到ID读取私有内容。
- 数据库没有tenant、yx账号、yx数据库连接或yx运行时ID；与外部智能体平台没有运行依赖。

学情数据是请求时计算的结果，没有独立insights表。`score_groups`按规则版本及有效评分细则的 `rule_signature` 与来源demo/real分组；改变个人规则文字会产生单独组，调整分数/观察证据本身不会改变规则签名。`score_group_id`用于响应分组，`rules_modified`表示是否改变规则，均不是数据库列。完整可观察评分才进入趋势，优先使用最新学生修正。顶层average_score仅在一个可比较组时返回，否则为null；不同规则和示例/真实记录不混合成一条趋势。

`from/to`是可选RFC3339闭区间，非法或倒置返回400。响应`period{from,to,label}`明确口径，未指定时边界为空、label为“全部记录”。样本、报告、任务各按自身created_at筛选，修正计数按revision自身created_at，因此区间内修正可以涉及更早创建的报告。

| 统计字段 | 单位与口径 |
| --- | --- |
| `sample_count/report_count/pending_review_count` | 范围内授权样本、报告及其中待复核报告数 |
| `revision_count/revised_report_count` | 范围内修正记录数，以及这些修正涉及的不重复报告数 |
| `practice_count` | 范围内创建的任务定义数，含草稿 |
| `completed_practice_count` | 上述任务中已发布、有分配且全部相关分配完成的任务数；学生仅检查本人分配 |
| `assignment_count/completed_assignment_count` | 选中且当前已发布任务的分配人次/完成的人次，反映当前完成状态，不按完成时间另筛选；学生只计本人 |

`include_demo=false`排除可识别的示例记录，修正的示例归属从原报告判断。所有计数均受权限约束，不采用固定演示数值。普通实体/jobs的API列表才有limit/offset分页，内部统计查询读取全部授权记录，不将第一页误当完整班级。
