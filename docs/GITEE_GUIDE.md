# 见微知著：Gitee 首次发布与后续更新

本项目的目标仓库是 `git@gitee.com:banshangya/jianwei-zhizhu.git`，主分支为 `main`。本文解释首次提交和后续更新的操作；是否成功应以命令返回和远端分支核对为准，不把操作步骤当作已完成记录。

## 先理解四个动作

| 动作 | 实际作用 |
| --- | --- |
| `git add` | 把选定文件的当前修改放进暂存区，决定下一次提交包含什么；此时还没有上传 |
| `git commit` | 把暂存区保存为本地历史快照，记录提交说明和提交编号；仍在本机 |
| `origin` | 远程仓库地址的本地简称；不是另一份源码，也不是登录账号 |
| `git push -u origin main` | 把本地main提交发送到origin，并设置main跟踪origin/main；以后可直接git push |

保存编辑器文件、构建网页、提交Git和推送Gitee是不同操作。推送源码不会自动替你启动Gitee上的应用服务，也不会上传本机数据库。

## 首次发布包含哪些内容

首次提交应描述整个独立应用，例如“初始化见微知著独立金相教学应用”，不能只写成一次品牌文字改动。源码包含React前端、独立Go后端、测试与脚本、构建配置和交付文档。

- 提交 `src/`、`backend/`、`docs/`、`scripts/`、README、Makefile及前端配置。
- 保留 `package.json` 与 `package-lock.json`，按当前npm工作流复现依赖；Go的 `backend/go.mod` 和 `backend/go.sum` 也一起提交。
- 只提交没有真实凭据的 `.env.example`。实际 `.env`、`data/`、`backups/`、日志、`bin/`、`dist/`、`node_modules/` 和本地缓存留在本机。
- 中文品牌为“见微知著”。内部模块 `zhiwei/backend`、配置前缀 `ZHIWEI_*`、数据库和会话标识可继续使用，不需要为了网页名称改变现有运行身份。

本地应用数据和Git源码分开保存。克隆仓库不会获得已有学生记录、模型密钥或管理员密码；运行环境需要单独配置与初始化，教学数据迁移使用应用备份/恢复命令，见 [运行说明](OPERATIONS.md)。

## 首次提交与推送

在项目根目录操作。该目录已经初始化Git，无需再次 `git init`。首次流程只执行一次；已有提交后使用下一节的日常更新流程。

### 1. 核对状态与忽略规则

```sh
git status --short
git branch --show-current
git check-ignore -v .env data/ backups/ bin/ dist/ node_modules/ .pnpm-store/
git ls-files -- .env data/ backups/ bin/ dist/ node_modules/
```

分支应为 `main`。`??`表示新文件，尚未加入暂存区；它不表示上传失败。`check-ignore`应显示敏感配置和运行目录命中的忽略规则，最后一条命令正常应无输出。

`.gitignore`只阻止尚未被跟踪的文件进入版本库，不能自动移除已经提交的秘密。若发现真实配置或运行文件已被跟踪，先停止推送并从暂存/跟踪范围排除；如果秘密已经进入历史或远端，还需要更换相关凭据并专门处理历史，不能只删除工作区文件。

### 2. 构建与检查本次修改

对于品牌、前端和文档改动，执行：

```sh
npm run build
npm run lint
npm run test:ui
python3 scripts/validate_openapi.py
```

首次安装依赖先运行 `npm ci`。`build`包含类型检查和前端生产构建；`test:ui`检查静态渲染语义与事件回放，不替代浏览器视觉验收。接口文档脚本默认检查本地引用和路由；安装了完整规范校验器时会额外运行相应检查。

后端业务改动应执行相应Go测试；完整工程检查可用 `make test`，受控模型业务闭环可用 `make test-e2e`。这些命令的作用和既有结果见 [验证记录](VALIDATION.md)。不要为一次品牌改名调用真实模型。

只更新前端时，构建会更新 `dist/`。现有Go服务按请求读取该目录，刷新网页即可读取新版本，不必重启后端。Go代码或服务端配置改变时才需要重新构建相应产物并重启服务。

### 3. 暂存并审阅整个首版

确认忽略规则正确后，将本次首版文件加入暂存区：

```sh
git add .
git diff --cached --stat
git diff --cached --name-only
git diff --cached --check
git diff --cached
```

逐项核对暂存清单，确保包含源码与必要配置，排除密钥、数据库、备份和构建产物。`--check`检查空白错误，不能代替内容审阅。编辑了已经暂存的文件后，需要再次 `git add` 才会把新的内容纳入提交。

### 4. 保存本地提交

```sh
git commit -m "初始化见微知著独立金相教学应用"
git status --short
git log -1 --oneline
```

`commit`成功后会产生提交编号；本文不预先填写编号。Git作者身份用于署名，与Gitee的SSH登录权限是两件事。提交失败时按具体提示处理，不用重新生成SSH密钥解决作者信息问题。

### 5. 配置目标仓库并推送

先检查已有远程配置：

```sh
git remote get-url origin
```

若还没有origin，添加一次：

```sh
git remote add origin git@gitee.com:banshangya/jianwei-zhizhu.git
```

若origin已存在且地址正确，直接使用；若明确指向错误仓库，再改为目标地址：

```sh
git remote set-url origin git@gitee.com:banshangya/jianwei-zhizhu.git
```

核对目标仓库及其现有历史后推送：

```sh
git push -u origin main
git status -sb
git rev-parse HEAD
git ls-remote origin refs/heads/main
```

最后两条命令分别显示本地HEAD和远端main的提交编号，两者一致才能确认远端已收到该提交。`-u`建立跟踪关系，不会把远程仓库变成本地数据备份，也不会自动推送以后尚未提交的文件。

## 后续怎样更新

以后在当前分支修改文件、执行与改动匹配的检查后，先审阅工作区，再只暂存本次需要的文件：

```sh
git status --short
git diff
git add <本次修改的文件路径>
git diff --cached --stat
git diff --cached --check
git diff --cached
git commit -m "说明这次具体修改及其目的"
git push
git status -sb
```

尖括号部分需要换成实际路径，不原样输入。首次提交审阅全项目；日常更新则优先逐文件暂存，避免把尚未完成的修改一并提交。构建后的dist被忽略，不需要为发布源码强行加入。

## SSH认证与失败处理

当前电脑已经配置SSH认证。Git通过现有SSH配置和本地密钥完成认证，不需要把私钥填进仓库、代码或聊天。仓库地址中的 `git` 是SSH服务用户名；真正的Gitee账号由服务端根据公钥识别。

| 提示或情况 | 处理方式 |
| --- | --- |
| `nothing to commit` | 检查是否确实有修改，以及是否已暂存；没有新内容时无需再创建提交 |
| `remote origin already exists` | 用 `git remote get-url origin`核对地址；已有正确配置就不重复添加 |
| `Permission denied (publickey)` | 检查当前终端使用的SSH配置/agent；可用 `ssh -T git@gitee.com`核对认证身份。不要复制或公开私钥 |
| 仓库不存在、没有写入权限 | 核对目标路径、当前认证账号及仓库权限；SSH认证成功不等于对任意仓库都有写权限 |
| 首次主机校验或主机指纹变化 | 核对可信的服务端指纹来源，不关闭主机验证来绕过错误 |
| `non-fast-forward` 或要求先fetch | 远端有本地尚未纳入的提交；先取回并审阅，不使用强制推送覆盖 |

遇到非快进时先检查：

```sh
git fetch origin
git status
git log --graph --oneline --decorate --all -n 20
```

`fetch`只取回远端历史，不自动改动工作区。工作区干净且双方有共同历史时，可将本地尚未推送的提交整理到远端最新提交之后：

```sh
git pull --rebase origin main
```

如出现冲突，阅读双方修改，编辑冲突文件后 `git add <冲突文件>`，再 `git rebase --continue`；不准备继续时用 `git rebase --abort`返回开始前状态。完成整合后重新检查，再 `git push`。

如果远端有独立初始化的README等历史，与本地没有共同祖先，应先决定如何保留两边内容，再制定合并方式。不要为了让命令通过而盲目强推、覆盖远端或删除本地教学数据。
