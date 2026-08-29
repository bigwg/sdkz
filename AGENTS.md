# AGENTS.md

本文件为 **AI 编码代理**（Claude Code、CodeBuddy、Cursor、GitHub Copilot 等）提供项目上下文与协作约定。人类贡献者请参考 `README.md` 与 `docs/DESIGN.md`。

## 项目简介

`sdkz` 是一个用 Go 实现的 **跨语言、跨平台 SDK 版本管理工具**。目标是用一套命令统一管理 Java / Go / Node.js / Maven / Gradle 等多语言 SDK 的版本，且在 Linux / macOS / Windows 上行为一致。

- 仓库：`github.com/bigwg/sdkz`
- 语言：Go 1.22+
- 许可证：Apache-2.0
- 数据目录：`~/.sdkz`（可用环境变量 `SDKZ_DIR` 覆盖）

## 架构

```
cmd/sdkz/         # CLI 入口（cobra 命令定义）
  ├─ root.go          # 根命令、全局 flag、命令装配
  ├─ list_cmds.go     # list / ls 命令与展示层
  ├─ install_cmds.go  # install / uninstall
  ├─ switch_cmds.go   # use / default / current
  ├─ config_cmds.go   # 配置、镜像
  └─ misc_cmds.go     # init / selfupdate / offline / doctor 等
pkg/
  ├─ domain/      # 核心领域模型（Release、Candidate、版本解析与排序）
  ├─ catalog/     # 候选源适配与版本聚合
  │   ├─ sources/     # 各 SDK 数据源（nodejs.go、adoptium.go、github.go 等）
  │   └─ builtin.go   # 内置候选与厂商定义
  ├─ service/     # 业务逻辑（Manager：安装、切换、缓存、元数据）
  ├─ platform/    # 平台探测（OS / Arch / shell 检测）
  └─ version/     # 版本号注入（编译期 -ldflags）
scripts/          # install.sh / install.ps1 安装脚本
docs/             # 设计文档
```

### 关键约定

- **展示层对齐 SDKMAN! 风格**：`list` 默认展示全部厂商，单列表格含 `发行版 | 版本 | 状态 | 标识` 四列；多厂商按发行版分组，组头显示厂商名。
- **版本标识（Version）即安装标识**：用户输入的完整版本串（如 `21.0.12.1+1-tem`、`v22.11.0`、`go1.23.4`）同时用于展示与精确匹配，不要在展示层擅自截断。
- **LTS 标记**：Java 在 `isLTS()` 按主版本判断（8/11/17/21）；Node 等用 `Release.LTS`（由数据源从 API 解析）。修改展示层时两者不能互相覆盖。
- **数据源容错**：部分厂商拉取失败只 `Warn` 并继续展示其余厂商，只有全部失败才报错（对齐 SDKMAN 行为）。不要在 `ListRemote` 里因单一源失败而中断整体。
- **`make build` 会同时更新 `dist/sdkz` 与根目录 `./sdkz`**，运行/验证请用最新 build 产物。

## 常用命令

```bash
make build            # 构建，产出 dist/sdkz 并同步 ./sdkz
make build-all        # 三平台交叉编译（tar.gz + checksums.txt）
make test             # 单元测试
make test-integration # 本地假源集成测试（不碰真实网络）
make fmt              # gofmt -w .
make vet / make lint  # 静态检查
go run ./cmd/sdkz <args>   # 直接运行最新源码
```

本地验证示例（用根目录二进制）：

```bash
./sdkz list java
./sdkz list node
./sdkz install java 21 --vendor tem
```

## 贡献指南（代理须知）

1. **优先复用现有抽象**：新增 SDK 数据源请在 `pkg/catalog/sources/` 实现 `Source` 接口，并在 `builtin.go` 注册候选/厂商，不要绕过 catalog 层直接改 CLI。
2. **保持跨平台一致**：任何改动需保证 Linux / macOS / Windows 行为一致。涉及路径、符号链接、`*_HOME` 注入时务必考虑 Windows（junction / copy 降级逻辑）。
3. **不要破坏幂等**：`init` 注入的 shell 块带标记包裹，重复运行不得重复追加。
4. **测试**：改动核心逻辑后运行 `make test` 与 `make test-integration`；新增数据源请补充对应解析测试。
5. **版本/元数据**：`Release` 的 `Version` 字段是用户可见且用于安装匹配的权威标识，格式由数据源决定，展示层只读不写。
6. **文档同步**：命令行为变更请同步更新 `README.md` 的「命令一览」与对应 `Long` 帮助文本。

## 行为准则

- 改动前先理解上下文，使用 `replace_in_file` 做精准编辑，避免无谓重写大文件。
- 涉及破坏性行为（强推、hard reset、删除文件）前先与用户确认。
- 新增临时脚本/文件完成任务后自行清理。
