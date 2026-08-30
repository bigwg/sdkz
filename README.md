# sdkz

> 一个命令，管理所有语言的 SDK 版本 —— 跨语言、跨平台的统一版本管理工具。

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-2ea44f.svg)](#)

---

## 为什么需要 sdkz？

开发者的机器上，往往同时装着好几种 SDK：Java、Go、Node.js、Maven、Gradle……
而它们的版本管理器却**各自为政、互不兼容**。这带来了两个层面的割裂：

### 痛点一：语言不统一

每种语言都有自己的版本管理器，记忆成本和习惯都不同：

| 语言 | 常见版本管理器 |
|------|---------------|
| Java | SDKMAN!、jenv |
| Go   | gvm、手动切换 |
| Node | nvm、fnm、n |
| Ruby | rbenv、rvm |
| Python | pyenv |

每学一门语言，就要学一套新的版本管理命令；团队里有人用 nvm、有人用 fnm，协作时心智负担很重。

### 痛点二：平台不统一

即便是同一种语言，不同 OS 上的工具也不一样：

- **Node.js**：Linux/macOS 用 `nvm`，Windows 用 `nvm-windows`（两套代码、两套命令、两套配置）
- **包管理/环境**：macOS 用 `brew`，Windows 用 `scoop`/`choco`，Linux 用 `apt`/`dnf`

同一份 `.nvmrc` 在 Windows 上常常水土不服，CI 与本地、同事与同事的环境难以对齐。

### sdkz 的目标

**用一套工具、一套命令，统一管理所有语言的 SDK 版本，且行为在 Linux / macOS / Windows 上完全一致。**

- 跨语言：Java / Go / Node.js / Maven / Gradle，一个 `sdkz` 全包
- 跨平台：同一命令、同一配置文件，三端行为一致（Windows 免管理员权限）
- 一个心智模型：`install` / `default` 对所有语言通用，无需切换工具

---

## 特性

- **跨语言统一**：Java (JDK) / Go / Node.js / Maven / Gradle 一套命令管理
- **跨平台一致**：Linux / macOS / Windows，命令、配置、行为三端对齐（Windows 免管理员）
- **Java 多发行版**：Temurin / Azul Zulu / GraalVM / Tencent Kona / Alibaba Dragonwell / SAP Machine，可指定 `--vendor`
- **持久生效**：`default` 设置全局默认版本，跨终端、跨 shell 一致生效
- **离线可用**：元数据本地缓存 + 国内镜像加速
- **安全可靠**：sha256 校验 + 原子安装 + current 指针自动降级（symlink → junction → copy）
- **单文件二进制**：Go 实现，无运行时依赖，开箱即用

---

## 快速开始

开箱即用，无需安装 Go 工具链（从 GitHub Releases 下载预编译二进制）：

```bash
# Linux / macOS / Windows (Git Bash)
curl -fsSL https://raw.githubusercontent.com/bigwg/sdkz/main/scripts/install.sh | bash

# Windows (PowerShell)
irm https://raw.githubusercontent.com/bigwg/sdkz/main/scripts/install.ps1 | iex
```

> Windows 的 Git Bash 里跑 `install.sh` 会自动转交 PowerShell 执行 `install.ps1`，无需手动切换。

脚本会自动探测平台、安装二进制、写入 shell 集成。**重新打开终端后**：

```bash
# 查看可安装版本（跨语言统一入口）
sdkz list java
sdkz list node
sdkz list go

# 安装与切换（所有语言命令一致）
sdkz install java 21        # 安装 Java 21（默认 Temurin）
sdkz install node 20        # 安装 Node.js 20
sdkz default java 21        # 全局默认 Java 21
sdkz default node 20        # 全局默认 Node.js 20
```

> 不同语言、不同平台，用的都是同一套 `install` / `default` —— 这就是 sdkz 存在的意义。

后续升级自身：`sdkz selfupdate`（无需重新跑安装脚本）。

### Windows 上的 `default`

在 Windows 上，`sdkz default` 除了更新 `current` 指针，还会**自动把 `JAVA_HOME`（及 `PATH` 中的 `bin` 目录）写入用户级环境变量**（HKCU\Environment，无需管理员权限、无需 `sdkz init`）。因此：

- **PowerShell / CMD / Git Bash 全部通用**，即使从未运行过 `sdkz init` 也能生效；
- 已打开的终端需**新开一个窗口**才能看到变更（Windows 环境变量机制限制：已启动进程不会自动继承新写入的用户级变量）。

如需当前窗口立即生效，可在已 `sdkz init` 的 shell 中执行 `default` 输出的导出语句，或新开终端。

---

## 命令一览

```
sdkz init [shell]          # shell 集成（写入 rc/profile，让 PATH 与 *_HOME 生效）
sdkz list [candidate]      # 可安装版本
sdkz ls [candidate]        # 已安装版本
sdkz install <c> [ver] [--vendor v]
sdkz uninstall <c> <ver>
sdkz default <c> <ver>     # 全局默认
sdkz current [c] / home <c> <ver> / env
sdkz mirror add|list|use cn
sdkz cache clean / doctor / selfupdate / version
```

示例：

```bash
sdkz list java --vendor zul   # 只看 Azul Zulu 的 Java 版本
sdkz install node lts         # 安装 Node.js 最新 LTS
sdkz default go 1.23          # 全局默认 Go 1.23
```

---

## 目录结构

```
~/.sdkz/                       # 可用 SDKZ_DIR 环境变量覆盖
├── candidates/<c>/<ver>/      # 已安装版本
├── candidates/<c>/current     # 当前版本指针
├── tmp/  metadata/  config.toml
```

---

## 开发

```bash
make build        # 产出 dist/sdkz（同时同步到根目录 ./sdkz）
make build-all    # 三平台交叉编译，产出 tar.gz + checksums.txt
make test         # 单测 + 离线集成测试（本地假源）
```

详细设计见 [docs/DESIGN.md](docs/DESIGN.md)。

---

## License

Apache-2.0
