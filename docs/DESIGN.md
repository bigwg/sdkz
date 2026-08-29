# sdkz — 设计文档

> 跨平台的 SDK 版本管理工具（Go 实现，对标 SDKMAN）。
> v1 为纯 CLI；v2 通过 Wails v2 提供 GUI。GUI 与 CLI 共用同一核心用例层。

## 1. 设计目标与非目标

目标：
- 跨平台：Linux / macOS / Windows，**Windows 免管理员、免开发者模式**。
- 多候选：v1 支持 Java (JDK)、Go、Node.js、Maven、Gradle。
- 多发行版：Java 支持 Temurin / Azul Zulu / GraalVM（vendor 抽象）。
- 版本切换双作用域：`default` 全局持久化（改 current 指针，新终端生效）；
  `use` 仅当前 shell 生效（eval 输出 export，退出 shell 自动恢复）。
- 离线可用：版本元数据本地缓存，`offline` 模式不触网。
- 面向 GUI：核心 `pkg/service` 暴露结构化结果 + 进度回调 + context 取消。

非目标（v1 不做）：
- 全局环境变量注册表写入（靠 shell profile + current 指针实现持久化）。
- 用户自定义候选（设计预留扩展点，见 §9）。
- 断点续传以外的下载高级特性（多线程分片等）。

## 2. 目录布局（用户侧）

```
$SDKZ_DIR                       # 默认 ~/.sdkz，可用环境变量 SDKZ_DIR 覆盖
├── bin/sdkz                    # 主二进制（selfupdate 替换目标）
├── candidates/
│   └── java/
│       ├── 21.0.5-tem/         # 版本目录（安装后不可变）
│       ├── 17.0.10-zulu/
│       ├── .sdkz-meta.json     # current 指针模式记录
│       └── current -> 21.0.5-tem  # symlink / junction / 复制目录
├── tmp/                        # 下载(.part)与解压暂存（与 candidates 同盘，保证原子 rename）
├── metadata/
│   └── candidates-<id>.json    # 各候选版本清单缓存（离线数据源）
└── config.toml                 # 配置（镜像、offline、自更新仓库等）
```

PATH 只暴露 `$SDKZ_DIR/candidates/<c>/current/bin` 与对应 `*_HOME`，
切换版本 = 仅改 current 指针。

## 3. 代码分层

```
cmd/sdkz/           CLI 入口（cobra），仅做参数解析与格式化
pkg/domain/         领域模型：Candidate / Vendor / Version / Artifact
pkg/platform/       GOOS/GOARCH 归一化、归档格式、链接能力探测
pkg/config/         config.toml + 环境变量覆盖 + 镜像规则
pkg/catalog/        元数据：内置候选定义 + 远程源适配器 + 缓存 + 离线
  └── sources/      adoptium / zulu / graalvm / golang / nodejs / maven / gradle
pkg/download/       HTTP 下载（进度、Range 续传、镜像替换、重试）
pkg/installer/      下载→校验→解压(strip)→原子落盘 / 卸载
pkg/linker/         current 指针抽象：symlink → junction → copy 自动降级
pkg/env/            PATH / JAVA_HOME 计算、shell 集成片段、export 块、PowerShell 适配
pkg/service/        ★ 用例编排层（Manager）：CLI 与 GUI 唯一入口
pkg/selfupdate/     GitHub Releases 自更新
pkg/version/        版本信息（ldflags 注入）
desktop/            （v2）Wails 工程，仅调用 pkg/service
```

约束：
- **stdout 只输出结构化数据 / export 语句；进度条与提示一律走 stderr**。
  这是 `use` 能在 shell 函数里被安全 eval 的前提。
- `pkg/service` 所有长任务方法签名：
  `Method(ctx, ..., onProgress ProgressFunc) (Result, error)`，
  `ProgressFunc func(done, total int64, stage string)`，GUI 可直接复用。

## 4. 领域模型

```go
type Candidate struct {
    ID            string    // java / go / node / maven / gradle
    Name          string
    HomeEnv       string    // JAVA_HOME / GOROOT / MAVEN_HOME / GRADLE_HOME（node 无）
    BinDir        string    // "bin"
    Default       string    // "21" / "lts" / "latest"
    HasVendors    bool
    DefaultVendor string
    Vendors       []*Vendor
}
type Vendor struct { ID, Name, SourceID string }

// catalog
type Release struct {
    CandidateID, VendorID, Version string   // Version 为显示名，如 "21.0.5-tem"
    Artifact *Artifact                       // 当前平台产物
    LTS, Stable bool
}
type Artifact struct {
    URL           string   // 主下载地址
    FallbackURLs  []string // 404 时依次尝试（如 dlcdn → archive.apache.org）
    ChecksumURL   string   // 校验值下载地址（.sha256/.sha512/SHASUMS256.txt）
    ChecksumType  string   // sha256 | sha512
    SHA256        string   // 内联校验值（优先于 ChecksumURL）
    Ext           string   // tar.gz | zip
    Strip         int      // 解压时剥掉顶层目录数（JDK 包顶层 jdk-21.0.2+13/）
}
```

版本号解析：统一提取 `major[.minor[.patch]]`（容忍 `go1.23.4`、`v22.11.0`、`21.0.5+11` 前缀），
`Match(spec, versions)` 支持 `latest` / `lts` / `21` / `21.0` / `21.0.5` 精确，pre-release（rc/beta/ea）排 GA 之后。

## 5. 远程源适配器（sources）

| 候选 | 源 | 版本 API | 校验 |
|---|---|---|---|
| Temurin | adoptium.net | `/v3/info/available_releases` + `/v3/assets/latest/{major}/hotspot` | 内联 sha256 |
| Zulu | azul.com | `/metadata/v1/zulu/packages/?java_version={major}&...` | 内联 sha256 |
| GraalVM | github.com | `repos/graalvm/graalvm-ce-builds/releases` | 无（跳过并警告） |
| Go | go.dev | `/dl/?mode=json&include=all` | 内联 sha256 |
| Node | nodejs.org | `/dist/index.json`（含 lts 标记） | 下载 `SHASUMS256.txt` |
| Maven | dlcdn.apache.org | 目录列表 HTML | `.sha512` 文件 |
| Gradle | services.gradle.org | `/versions/all` | `.sha256` 文件 |

架构映射：`amd64→x64/x86_64/amd64`、`arm64→aarch64/arm64`，os 与各源命名对齐。
镜像替换只作用于**下载 URL**（Artifact.URL / ChecksumURL / FallbackURLs），不影响 API 请求地址。

## 6. 安装事务

1. 下载到 `tmp/<c>/<v>.<ext>.part`（`Range` 续传、3 次重试、逐镜像尝试）。
2. 校验（内联 sha256 / 校验值文件 / 跳过警告）。
3. 解压到 `tmp/<c>/<v>`（`Strip` 顶层目录；zip 防路径穿越；保留 exec 位）。
4. 同盘 `rename` 到 `candidates/<c>/<v>`（原子性；目标存在则先清理）。

卸载：从 registry（candidates 索引，v1 简化为目录扫描）找到版本目录，
若为 current 指向则拒绝（提示先 `default`/`use` 其它版本）；Windows 上被占用时
先 rename 为 `.delete-*` 再删除，失败登记 `tmp/` 下次清理。

## 7. current 指针降级链（linker）

`Set(link, target)` 依次尝试：

1. `os.Symlink`（Unix 全部成功；Windows 需开发者模式/管理员）。
2. Windows `mklink /J` 目录联接（**无需权限**）。
3. 目录复制（最差兜底；`current` 成为真实目录）。

模式记录于 `candidates/<c>/.sdkz-meta.json`，后续操作复用而不重复探测。
`Resolve(link)` 返回 `current` 实际指向的版本目录；copy 模式直接返回目录本身。
切换版本时若为 copy 模式：清空旧 `current` 内容（跳过 `.sdkz-meta.json`）再复制。

## 8. 环境集成（env）

- `sdkz init [bash|zsh|fish|pwsh]`：幂等地向 `~/.bashrc` / `~/.zshrc` /
  `~/.config/fish/config.fish` / `$PROFILE` 注入初始化块（`# >>> sdkz >>>` 标记区间替换）。
- 初始化块内容：
  - 依次检查各候选 `current/bin`，存在则前插 PATH 并导出对应 `*_HOME`。
  - 定义 `sdkz()` shell 函数：调用真实二进制；stdout 以 `# SDKZ_EXPORT` 开头时
    eval 其余行（`use` / `default` 生效到当前会话），否则原样打印。stderr 透传。
- `sdkz use <c> <v>`：不改指针，stdout 输出指向**具体版本目录**的 export 块（eval 后当前会话生效，退出即恢复）。
- `sdkz default <c> <v>`：更新 current 指针（新终端生效），若在函数会话内同时输出 export 块（立即生效）。
- `sdkz env`：输出当前所有候选的 export 块（可用于手动 eval 或 CI）。

PowerShell 注意：`*>&1` 混流后用 `Invoke-Expression` 处理 export 行（仅 use/default 触发，无进度输出，风险可控）。

## 9. 扩展点

- **镜像**：`config.toml` 中 `mirror.<host> = "base-url"`，下载 URL 以 `https://<host>/` 开头时替换前缀。
  `sdkz mirror use cn` 一键写入国内常用镜像（node→npmmirror、go→goproxy.cn、gradle→tuna、
  maven/apache→aliyun 等）。
- **用户自定义候选**（v1 预留，README 说明）：`$SDKZ_DIR/metadata/extra.toml` 定义静态 release 列表，
  走 catalog 缓存路径，不编写代码即可加入自定义私有 SDK 源。
- **GUI**：新增 `desktop/`（Wails v2）调用 `pkg/service`，进度回调直连前端。

## 10. 测试与 CI

- 单测：版本解析/匹配、平台映射、config、linker 降级、zip 穿越防护。
- 集成：`httptest` 起假源服务器 + 临时 SDKZ_DIR，跑通
  `install → use → default → uninstall` 全链路（单测内不触网）。
- CI（GitHub Actions）：三平台矩阵 build + test + vet。

## 11. 版本与发布

- `pkg/version` 经 `-ldflags "-X sdkz/pkg/version.Version=..."` 注入。
- `sdkz selfupdate`：从配置的 `self_update_repo`（owner/repo）拉取 GitHub Releases，
  校验 `checksums.txt` 后原子替换自身二进制。
