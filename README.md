# 见微知著 · 金相组织制备与观察评测分析AI优化系统

一个可独立运行的 Go + React 应用，基于新版 Demo 实现真实的图谱上传、AI 辅助评图、学生修正、五维理解、复练及教师指导流程。

后端不依赖 yx-new-go 的代码、服务、账号、数据库或配置。AI 通过本项目自己的兼容模型 API 配置接入；没有配置模型时不会伪造分析结果。

## 快速运行

需要 Go 1.26+、Node.js 20.19+，支持 macOS / Linux；无需 Docker、PostgreSQL 或 Redis。

```sh
npm ci
cp -n .env.example .env
make build
make init-admin
make start
```

打开 `http://127.0.0.1:8080`。管理员用户名默认为 `admin`；未设置初始密码时，初始化命令生成随机密码并仅在终端显示一次。没有公开的默认密码。

登录后在“平台治理”创建教师、学生账号及班级成员，系统自动提供已发布的四项教学评分规则；管理员可复制并发布新版本。参考图谱与知识资料由管理员上传真实资料后发布；不提供自动报告回退。参考图谱与教学案例支持带图片的关键词 RAG。

需要演示教师端成绩排序时，可执行 `make seed-demo`。该命令只创建明确标为“演示/合成数据”的班级、账号、图片和报告，不调用模型，也不作为真实学生学习证据。

开发模式使用 `make dev`，前端地址为 `http://127.0.0.1:3000`，Vite 将 `/api` 代理至独立后端。后端代码变更后重新运行该命令。

更新源码后，先正常停止旧进程，再执行 `make build`、`make start`。`make start` 只运行已构建产物，不会自动重新编译；保留现有 `.env` 与数据目录，不必重复初始化账号。

## 启用真实 AI

编辑根目录 `.env`，设置支持图片输入的兼容模型服务，然后重启：

```dotenv
ZHIWEI_AI_BASE_URL=https://your-provider.example/v1
ZHIWEI_AI_MODEL=your-vision-model
ZHIWEI_AI_API_KEY=your-private-key
```

不需要提供 yx 平台地址或访问令牌。密钥不会下发给浏览器；资料和图片仅发送给你配置的模型服务。模型未配置时，可以管理资料和保存样本，发起分析会明确返回未配置状态。

先执行 `./scripts/check-model.sh --vision-only` 检查图片读取。完整联调使用 `./scripts/check-model.sh --image /absolute/path/to/photo.png`，须提供授权真实照片；在临时数据中以未知背景检查三类报告与保存链路，最多调用模型7次，不改动教学数据库，也不证明判读准确性。

调用日志写到启动终端的 stderr。默认只显示阶段、耗时和安全错误；需要本地排障正文时，显式设置 `ZHIWEI_LOG_LEVEL=debug` 与 `ZHIWEI_LOG_AI_CONTENT=true` 后重启。正文会脱敏并限量分段输出，具体配置、32 KiB限制和真实进度口径见 [AI 调用说明](docs/AI_CALL_FLOW.md)。

## 已实现的流程

- **组织图谱**：单次上传1–8张同一样本图片，材料/工艺/倍率/腐蚀剂可选，冻结样本版本和学习轮次。
- **鹰眼评图**：AI 综合对照同组多张图片，输出四项学习评分、观察证据及可选参考定位；AI 原稿、学生规则/得分/依据修正分别保存。无法判断项保留 `null`。
- **识相悟道**：成分、工艺、组织、性能、应用五维解释；区分背景、观察和推断，保留来源及学生最终判断。
- **炉火纯青**：真实历史、问题统计、个人任务、下一轮关联和前后对照；不同评分口径分别展示。
- **见微知著**：个人学习归档、搜索及历史详情，共享参考图谱单独审核发布。
- **教师工作台**：按姓名或账号检索学生、班级筛选、按成绩升降序或最新记录排序及服务端分页，查看/指导/复核分别记录，干预任务草稿、发布及逐学生完成跟踪。
- **治理**：账号、班级、评分规则、图谱和文档管理，资料版本替换、中文检索及历史引用保留。
- **运行保障**：持久任务、幂等提交、标准 SSE 回放、取消、失败重试、重启恢复及数据库和原图备份恢复。

评分用于学习反馈；不提供正式竞赛自动定分、像素级多相分割、单图精确性能预测或已验证热处理参数。

## 验证

```sh
make test
make test-e2e
```

`make test` 执行 Go race 测试、前端类型检查/构建、lint及 `npm run test:ui`。后者是React服务端渲染语义断言和标准SSE回放检查，不启动浏览器。`make test-e2e` 在临时目录启动真实 Go 服务与受控模型 HTTP 服务，验证整个业务链；结束后清理临时数据。

受控模型只用于工程验收，不会成为应用的运行时降级方案。真实模型连通性与金相专业准确性需要在独立配置有效后另行验证。具体执行结果及未完成验证见 [验证记录](docs/VALIDATION.md)。

## 代码与文档

| 位置 | 内容 |
| --- | --- |
| `backend/cmd/server` | 服务启动、管理员初始化、迁移与备份恢复命令 |
| `backend/internal/app` | 业务接口、资源权限、学习记录、教学任务与管理 |
| `backend/internal/ai` | 独立兼容模型适配、提示词、结构及业务校验 |
| `backend/internal/knowledge` | 文档提取、中文分词、安全查询和分块 |
| `backend/internal/jobs` | 持久队列、取消、事件回放与结果事务 |
| `backend/internal/observability` | 终端脱敏日志、调用关联、实际阶段及耗时 |
| `backend/internal/store` / `auth` | SQLite、版本快照及身份会话 |
| `src/App.tsx` / `src/api.ts` | 新版工作台入口、API 与类型 |
| `src/components/*Workspace.tsx` | 学生、教师和治理业务界面 |
| `scripts/e2e.py` | 黑盒端到端验收，使用受控模型服务 |
| `scripts/validate_openapi.py` | 本地接口文档、内部引用与Go路由一致性检查 |
| `scripts/ui-smoke.mjs` | React静态语义与SSE回放检查，不替代浏览器视觉验收 |

- [运行与配置](docs/OPERATIONS.md)
- [AI 调用、标注坐标、评分与终端排障](docs/AI_CALL_FLOW.md)
- [接口契约](docs/API_CONTRACT.md) / [OpenAPI 3.1](docs/openapi.json)
- [架构与数据口径](docs/ARCHITECTURE.md)
- [数据字典](docs/DATA_DICTIONARY.md)
- [启动与完整流程测试](DEMO_USAGE.md)
- [验证记录](docs/VALIDATION.md)
- [Gitee 首次发布与后续更新](docs/GITEE_GUIDE.md)

原始模拟页面和预置数据已删除。正式数据默认存放在根目录 `data/`，不进入版本库。升级迁移3会归档历史示例图谱、资料与规则，保留旧学习快照、原图和引用；历史示例不进入常规列表或新分析。

资料复用与测试说明见 [资料与分析使用指南](docs/REFERENCE_WORKFLOW.md)。
