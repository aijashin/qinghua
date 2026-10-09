# 独立后端 API 契约

完整字段与响应参见 [OpenAPI 3.1](openapi.json)，前端使用的类型位于 `src/api.ts`。下面是实际实现的业务接口，不依赖外部平台。

## 通用约定

- 基路径 `/api/v1`；JSON 使用 `snake_case`，时间为 RFC3339 UTC，ID 为服务端生成的字符串。
- 单项直接返回对象；列表至少返回 `{items:[]}`。实体列表、任务列表支持 `limit`（默认100，上限500）、`offset`，并返回 `total/limit/offset`。班级、反馈、修订等子列表返回完整授权集合。
- 错误格式为 `{error:{code,message}}`。未登录返回401，无权限返回403或404；版本冲突和幂等冲突返回409。
- 登录使用同源 HttpOnly Cookie。登录后的 POST/PATCH/DELETE 必须携带 `X-CSRF-Token`，浏览器请求使用 `credentials:'include'`。页面切换不能改变服务器身份。
- HTTP响应提供服务端生成的 `X-Request-ID`，用于关联终端日志，不采信客户端指定的身份或授权。
- 所有图片、文档正文、历史引用和 SSE 都校验资源权限。学生仅可读本人学习记录；教师仅可读授权班级记录；管理员负责公共治理。

## 身份与运行状态

| 方法与路径 | 用途 |
| --- | --- |
| `GET /health` | 不需登录的存活状态 |
| `GET /auth/session` | `{user,csrf_token,ai_configured}`；未登录时 `user:null` |
| `POST /auth/login` | `{username,password}`，成功返回会话与 CSRF；登录前无需 CSRF |
| `POST /auth/logout` | 撤销当前会话 |
| `GET /system/capabilities` | 模型名称、是否配置、独立运行状态及支持文档格式，不返回密钥 |

User 包含 `id,username,display_name,role,disabled`；role 为 `student/teacher/admin`。`ai_configured:true` 只说明配置项齐全，不是模型联调通过标志。

## 上传、样本与学习轮次

- `POST /files`：multipart 的 `file` 字段；最大20MiB，图片支持 JPEG/PNG/WebP，校验真实类型、完整解码与尺寸。返回 `{id,name,mime,size,url}`。`GET /files/{id}` 受保护预览或下载。
- `GET/POST /samples`：创建字段为 `{file_id,title,material,process,magnification,etchant,class_id?}`。四项背景信息不能为空；不清楚时明确填写“未知”。新练习须使用本人上传的图片；共享参考图谱只供查阅，不复制成练习。
- `GET/PATCH /samples/{id}`：修改图片或背景形成新版本；PATCH 可带 `base_version`，版本不符返回409。班级一经创建固定。`GET /samples/{id}/versions` 查看历史。
- `GET/POST /learning-sessions`：创建字段为 `{sample_id,title?,parent_session_id?,task_id?}`，保存不可变 `sample_snapshot` 和 `sample_version`。
- `GET/PATCH /learning-sessions/{id}`：更新 `{step,completed?}`；step 为 `atlas/assessment/understanding/practice`。`completed:true` 要求本轮同版本评图与五维报告均生成且分别保存采用/修正稿，然后原子创建个人图谱归档。

历史轮次始终引用当时的图片和背景。背景更新后的新轮次不能借用旧报告通过归档校验。示例来源通过 `demo/source_atlas_id` 随快照保留。

## AI 持久任务与事件

`POST /jobs` 请求：

```json
{
  "kind": "assessment",
  "sample_id": "sample-id",
  "learning_session_id": "session-id",
  "input": {},
  "idempotency_key": "client-generated-unique-key"
}
```

kind 支持 `assessment/understanding/coaching/identify`。只有样本本人能发起分析，模型未配置返回503 `ai_not_configured`，不生成占位报告。服务端绑定样本及规则快照，传入真实图片字节。

成功创建返回202 Job，字段包括 `id,owner_id,class_id,kind,sample_id,learning_session_id,status,stage,error?,result_id?,created_at,updated_at`。status 为 `queued/running/succeeded/failed/cancelled`。新任务另提供实际 `progress/progress_history/timings`，以及可选 `started_at/finished_at/error_code/origin_request_id`；旧记录缺少这些字段时不能补成虚构计时。

- 幂等键长8–128字符，作用域为当前用户；同键同请求返回同一任务，同键不同内容返回409。
- `GET /jobs`、`GET /jobs/{id}` 查询状态；`POST /jobs/{id}/cancel` 取消自己的未完成任务，管理员可取消授权范围内任务。
- 显式重试通过 `POST /jobs` 提交新的幂等键完成，保留原失败任务。没有隐式自动重发外部模型请求的行为。
- `GET /jobs/{id}/events` 使用标准 SSE `id/event/data`，按任务内序号递增；通过 `Last-Event-ID` 或 `?after=N` 回放。断开浏览器不取消任务。
- 事件：`status` 的 data 为 Job；`progress` 为结构化实际阶段，`delta:{text}`继续兼容；`completed` 为 `{job_id,report_id,result_id,result_kind,finished_at?,timings?}`；`error` 为 `{code,message}`。文档导入成功时 `result_kind:documents`，其他分析为 `reports`。
- 两个 worker 处理持久队列。服务重启后排队任务继续，中断任务明确失败；取消后的旧模型响应不能提交报告。

模型结构化输出通过字段、分数、范围和引用校验后才保存。格式最多修复一次；无匹配资料时报告必须标识证据不足。

`progress` 的结构为 `{code,label,at,elapsed_ms,attempt?,completed?,total?}`，时间采用UTC，耗时单位为毫秒。code为 `queued/preparing/retrieving/model_waiting/model_receiving/validating/repairing/persisting/extracting/chunking/succeeded/failed/cancelled`。只报告实际发生的阶段，不通过定时器模拟百分比；`completed/total`只在有真实计数时提供。

`timings` 含可选 `queue_ms/total_ms/model_ms/stages/model_calls`。stage含 `{code,attempt?,duration_ms}`；model_call含 `{attempt,duration_ms,first_content_ms?,input_tokens?,output_tokens?,total_tokens?}`。排队与模型调用分别记录；total_ms不包含排队，token用量只采用服务商实际返回值。报告另保存本次任务 `started_at/finished_at/timings`；历史报告这些字段可缺省。运行中取消截取当时计时，含尚未返回调用已消耗的时间，不预测完整时长；排队取消或重启后的中断任务没有可信计时时保持缺省。

请求与任务可以用 `origin_request_id/job_id` 对照服务端终端日志。API和SSE不返回未校验模型正文、认证头或图片data URL。调试正文日志仅由服务端 `debug + ZHIWEI_LOG_AI_CONTENT=true` 开启，限量脱敏分段，不是新的客户端权限。详见 [AI 调用说明](AI_CALL_FLOW.md)。

## 报告、修正与教师反馈

- `GET /reports?sample_id=...`、`GET /reports/{id}`：获取原始报告及最新学生修正。报告记录样本快照、规则快照、模型、提示词版本、引用资料和生成时间。
- `POST /reports/{id}/revisions`：`{base_version,data,reason}`；data 是完整结构化结果，reason 必填，第一次 `base_version:0`。成功返回新增修订，`revision_no` 递增；409 时应重新读取后再编辑。
- `GET /reports/{id}/revisions`：查看所有保存版本。`original_data` 与 `data` 保留 AI 原稿，学生内容位于 `latest_revision.data`，不会覆盖公共规则。
- `POST /reports/{id}/viewed`：教师记录已查看。
- `POST /reports/{id}/feedback`：`{comment}`，只记录指导；GET 同路径获取历史。
- `POST /reports/{id}/review`：`{decision:'confirmed'|'needs_revision',comment,corrected_score?}`；明确保存复核决定。`GET /reports/{id}/reviews` 查看复核记录。查看、指导与复核互不替代，教师确认也不等于实验验证。

### 报告内容

评图包含 `summary,criteria,defects,limitations,total_score,max_score,coverage,citations`。criteria 固定为划痕 `scratches`、假象 `artifacts`、侵蚀程度 `etching`、组织清晰与正确度 `clarity`，每项25分；各项含 `id,label,score,max_score,evidence,rule_text?`。服务端重新计算总分，不能通过客户端伪造总分。无法判断时 `score:null` 并说明原因；有任一空项时 `total_score:null`，不进入分数趋势。`coverage` 为可判断项目占比0–1。

defects 包含 `name,severity,evidence,suggestion` 和可选 `bbox:{x,y,width,height}`。以整张图片左上角为原点，x/width按图宽归一化，y/height按图高归一化；四个坐标须明确为数字，宽高大于0且矩形不越界。前端按百分比绘制参考框，仅表示模型建议观察的位置，不是像素分割。学生评分修正不采信客户端传入的bbox：只有与原稿同序且同名的缺陷继承有效原框，其它修正条目移除定位；AI原始报告不变。

五维/识图内容包含 `summary,candidates,dimensions,questions,limitations,citations`；候选组织包含 `name,evidence,uncertainty`。维度严格为成分、工艺、组织、性能、应用，每项含 `name,content,basis`；basis 为 `known/observed/inferred`，分别表示背景、观察、推断。学生可保存 `final_phase/phase_reason`。

训练建议包含 `summary,questions,suggestions,verification_plan,limitations,citations`。创建训练建议任务时，服务端冻结本轮及上一轮授权报告、已保存的学生修订和关联任务目标，不接受客户端自报历史作为学习记录。引用 ID 必须属于本次实际检索结果；`sources` 只展示模型实际引用的片段，`retrieved_sources` 保留检索溯源。资料正文和历史引用读取仍需权限。

## 练习、图谱与班级

- `GET/POST /practice-tasks`，`GET/PATCH /practice-tasks/{id}`：字段 `{title,instruction,class_id?,assignee_ids?,sample_id?,report_id?,due_at?,status}`，status 为 `draft/published`。教师仅管理自己布置的授权班级任务；学生只能给自己建任务。
- `POST /practice-tasks/{id}/publish`：明确发布。分配列表在服务器校验；学生看不到未发布任务或他人的反思。
- `POST /practice-tasks/{id}/attempts`：`{answer}`，保存实际提交；GET 同路径读取授权提交。提交不代表自动批改。
- `POST /practice-tasks/{id}/complete`：`{reflection}`，只完成当前学生的分配，写入 `assignments`，不把全班任务直接标成完成。
- `GET /personal-atlas`、`GET /personal-atlas/{id}`：本人完成轮次的归档，保留样本、五维理解、原报告和保存时的学生修订。后续教师反馈通过报告接口实时读取。
- `GET /atlas?q=...`、`GET /atlas/{id}`：共享参考图谱。教师/管理员可创建与编辑草稿；管理员通过 `POST /atlas/{id}/publish` 发布。字段包括 `title,material,phase,process,magnification,etchant,description,observation,file_id,source`。发布须有标题、图片和来源；拒绝demo:true。
- `GET /classes` 返回有权限的班级及成员；`GET /teacher/submissions` 在教师班级权限内筛选后分页：`class_id` 精确班级、`student_id` 精确学生、`q` 按姓名或账号检索（按字面匹配，最多150字），`limit` 默认20、范围1–100，`offset` 非负整数。返回 `{items,total,limit,offset}`，报告增加 `owner_name,owner_username,class_name,sample_title`。总量是筛选后的授权报告数，按created_at/id倒序，不跨班级。非法分页返回400，越权班级返回403。

个人图谱检索与公共知识检索分开；个人修正不会自动变成公共资料，也不会自动训练模型。

## 统计口径

`GET /insights` 支持 `class_id`、`include_demo=false`，以及可选的 `from/to` RFC3339闭区间，按各类记录创建时间筛选。非法时间或倒置范围返回400。`period:{from,to,label}` 说明统计范围，默认“全部记录”。没有可比较成绩时 `average_score:null`、`score_history:[]`。

| 字段 | 统计单位 |
| --- | --- |
| `sample_count/report_count/pending_review_count` | 授权范围内样本、报告、待复核报告数量 |
| `practice_count` | 创建于范围内的可见任务定义数，教师可包含草稿 |
| `completed_practice_count` | 上述已发布任务中已完成全部当前分配的任务数；学生只看本人分配 |
| `assignment_count/completed_assignment_count` | 上述已发布任务的分配人次与已完成人次 |
| `revision_count/revised_report_count` | 在范围内保存的修订次数及涉及的不重复授权报告数 |

任务完成量表示所选创建批次的当前状态，不是按完成时间计算的新增流量。分数使用报告最新学生修订；原 AI 分数仍保留在报告中。

`score_groups[].score_group_id` 由公共规则版本、条目ID/满分/实际规则文字及样本来源计算。修改得分、证据或条目顺序不会拆组；改写规则会标记 `rules_modified:true` 并拆组。示例图像与真实样本分开统计。多个分组时不计算跨组的顶层平均分。

## 公共治理

管理员接口：

- `GET/POST /admin/users`；`PATCH /admin/users/{id}` 更新名称、角色、禁用状态或密码。
- `GET/POST /admin/classes`；`POST /admin/classes/{id}/members` `{user_id}`；`DELETE /admin/classes/{id}/members/{user_id}`。
- `GET /rubrics`、`GET /rubrics/{id}`；`POST /admin/rubrics` `{title,description,criteria,demo}`；`POST /admin/rubrics/{id}/publish`。新规则保存新实体，旧报告规则快照保留。
- `POST /admin/documents` `{file_id,title,idempotency_key?,replaces_document_id?,material_tags?,phase_tags?,process_tags?}` 返回202导入任务。三类标签为字符串数组，每类最多12项、每项最多64字，去空白并去重。支持 TXT/Markdown/DOCX/文本型PDF；扫描PDF明确提示需要OCR。导入成功先进入draft。
- `GET /documents`、`GET /documents/{id}`、`GET /documents/{id}/chunks` 预览；`POST /admin/documents/{id}/publish` 发布。只有已发布教学资料进入公共检索。
- 资料换版时指定 `replaces_document_id`，新版审核发布后旧版归档；`series_id/document_version` 保留沿革。已被报告引用的旧片段仍可由该报告有权读者访问。
- `GET /knowledge/search?q=...` 使用中文单字/二元词及拉丁词 FTS 检索；可用 `material_tag/phase_tag/process_tag` 精确过滤，类别之间为AND，可与q组合。标签参与索引但不改写引用原文。`GET /knowledge/chunks/{id}` 查看引用正文。搜索返回片段包含 `id,title,content,document_id,document_version`。

已删除示例导入功能。迁移3归档旧示例图谱/资料/规则，常规列表与检索排除示例；旧报告及引用保留。新学习只能使用本人上传图片；旧示例样本的新轮次/分析返回409。服务启动自动提供内置非示例教学规则；若规则被异常移除且没有可用版本，评图返回409 rubric_required，不调用模型。

模型超时：每次模型请求独立使用配置的等待预算，默认300秒；结构错误最多一次格式修复，修复使用新预算。取消不转换为超时，传输错误不自动重试。超时失败返回可读中文错误，并持久化error_code:timeout，不生成新报告。


## 内置规则、案例与详细进度

服务启动幂等初始化已发布的“金相制样学习评价 v1”。`is_builtin`表示系统版本，自建已发布规则优先，按`published_at`排序（历史缺失时用created_at）。原报告仍绑定当时的规则。

`POST /reports/{id}/review`增加`base_revision`，保存`revision_no/sample_version`。修订后报告回到pending_review，旧复核仍保留但不表示确认当前稿。

`POST /reports/{id}/reference-cases`：传`review_id/base_revision`创建班级案例草稿。`GET /reference-cases`支持现有分页、class_id、q；`PATCH /reference-cases/{id}`传`base_version`及标题、组织、结论、观察、说明，草稿更新返回200，发布版本复制为新草稿返回201。`POST .../publish`发布班级；`POST .../withdraw`撤回；`POST /admin/reference-cases/{id}/publish`由管理员公开。三个动作均传base_version。新版本发布事务内撤回前一版。案例的公共响应不包含owner_id/report_id/review_id/file_id和原始修正数据。

`DELETE /admin/documents/{id}`传`base_version`；状态变为deleted并记录操作者、时间。正常列表和后续RAG排除deleted，历史引用保留。发布支持可选base_version，版本变化返回409，删除/归档或空正文不能发布。

Source增加`source_kind/source_version/image_url`；document仍提供document_id/document_version。`GET /knowledge/sources/{id}`与`.../image`读取参考图谱/案例快照，校验当前可见范围或历史报告引用。旧`GET /knowledge/chunks/{id}`保持兼容。检索返回最多6个文档片段+2个参考图；class_id未提供时只召回公共资料。

JobProgress增加`stage_id/updated_at/details`，相同stage_id为详情刷新。details为固定白名单，无模型正文。客户端按stage_id合并，并拒绝较旧updated_at覆盖新快照；SSE仍按事件seq去重并回放。
