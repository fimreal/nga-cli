# NGA CLI (艾泽拉斯国家地理论坛命令行工具)

[![Go](https://img.shields.io/badge/Go-1.18+-00ADD8?style=flat&logo=go)](https://golang.org)
[![SQLite](https://img.shields.io/badge/SQLite-PureGo-003B57?style=flat&logo=sqlite)](https://modernc.org/sqlite)
[![GitHub Release](https://img.shields.io/github/v/release/fimreal/nga-cli?style=flat&logo=github)](https://github.com/fimreal/nga-cli/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

NGA CLI 是一个专为终端开发者、Homelab 运维及论坛重度用户设计的轻量、极简流量、防风控的 NGA 命令行客户端。

---

## 核心特性

- **极致最小流量**：全局使用 `lite=js` 结构化轻量接口，单请求传输仅 5~15 KB（相比 Web 完整渲染节省 95% 以上流量）。
- **智能 Tail 直算**：查看帖子末尾楼层时根据总楼层直接直算目标末页页码，精准拉取尾部，无需全帖遍历。
- **SQLite 本地增量缓存**：纯 Go 嵌入式 SQLite 驱动（无 CGO 依赖），自动持久化历史不可变楼层（24h 长缓存）与活跃末页；二次读取耗时降至 **< 10ms**，网络消耗为 **0 流量**。
- **流式监听与退避抖动 (`tail -f`)**：支持持续监听帖子新回复，内置基准轮询间隔（默认 3s）、`±500ms` 随机抖动（Jitter）及无新楼层自动指数退避机制（最高退避至 10s），彻底避免在 CDN/WAF 形成机械周期特征。
- **发帖回复与引用排版 (`reply` / `post`)**：支持向帖子发表回复与新发主题；提供 `--quote <floor>` 自动抓取指定楼层组装规范的 NGA `[quote]` BBCode；正文支持从 Markdown/文本文件直接导入。
- **跨会话持久化防重机制**：基于 SQLite 本地记录内容签名，默认 30 秒内禁止重复提交相同内容，防止由于误触或连击被论坛防刷机制封禁；支持 `--force` 跳过。
- **多账号 Profile 凭证池**：支持通过 `nga profile` 管理多套登录凭证（Cookie UID/CID）、专属代理及 UA，支持全局 `--profile <name>` 随时切换身份。
- **反风控与特征拟真**：默认配置标准移动端 User-Agent，直接规避 Web 端防爬 JS 挑战（`ERROR:15`）；内置请求限速（默认 1.5s）与阻断熔断保护。
- **环境诊断与探测 (`doctor`)**：内置 DNS 解析检测、代理连通性探测以及 NGA 节点 WAF 绕过健康度诊断，协助 Homelab 网络调优。
- **纯净单二进制交付**：无任何外部运行时或动态库依赖，开箱即用。

---

## 安装与快速上手

### 1. 下载预编译二进制 (推荐)

项目通过自动化构建流水线为各大主流系统与架构提供静态编译的预编译产物，开箱即用，无需配置 Go 开发环境。

前往 **[GitHub Releases 页面](https://github.com/fimreal/nga-cli/releases)** 获取最新发行包：

| 操作系统 (OS) | CPU 架构 (Arch) | 预编译资产包 | 格式 |
| :--- | :--- | :--- | :--- |
| **Linux** | x86_64 (`amd64`) | `nga_<tag>_linux-amd64.tar.gz` | `.tar.gz` |
| | AArch64 / ARM64 (`arm64`) | `nga_<tag>_linux-arm64.tar.gz` | `.tar.gz` |
| | ARMv7 (`armv7`) | `nga_<tag>_linux-armv7.tar.gz` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`) | `nga_<tag>_darwin-arm64.tar.gz` | `.tar.gz` |
| | Intel 64-bit (`amd64`) | `nga_<tag>_darwin-amd64.tar.gz` | `.tar.gz` |
| **Windows** | x86_64 (`amd64`) | `nga_<tag>_windows-amd64.zip` | `.zip` |
| | ARM64 (`arm64`) | `nga_<tag>_windows-arm64.zip` | `.zip` |

#### Linux / macOS 快速安装
```bash
# 下载对应平台的最新 tar.gz 包并解压 (以 Linux amd64 为例)
curl -sSL -O https://github.com/fimreal/nga-cli/releases/latest/download/nga_latest_linux-amd64.tar.gz
tar -xzf nga_latest_linux-amd64.tar.gz

# 赋予执行权限并放入系统 PATH
chmod +x nga
sudo mv nga /usr/local/bin/

# 验证安装
nga --version
nga doctor
```

#### Windows 快速安装
1. 下载 `nga_<tag>_windows-amd64.zip` 或 `nga_<tag>_windows-arm64.zip`；
2. 解压获取 `nga.exe`；
3. 将解压目录加入系统环境变量 `Path`，或在当前目录通过 PowerShell / CMD 运行 `.\nga.exe doctor`。

#### 完整性校验
每个 Release 均包含由构建流水线自动生成的 `checksums.txt`，可通过 SHA256 校验包的完整性：
```bash
sha256sum -c checksums.txt
```

---

### 2. 源码构建 (开发者模式)
```bash
# 本地编译生成可执行二进制
go build -o nga .

# 或者静态编译 (用于制作 Alpine 镜像或无 glibc 极简宿主机)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o nga main.go
```

### 3. 多账号 Profile 管理 (`profile`)
```bash
# 查看所有已配置的账号 Profile
./nga profile list

# 添加或更新一个账号 Profile
./nga profile set default --uid 12345678 --cid "your_cid_token_1"

# 添加带专属代理的备用账号 Profile
./nga profile set homelab --uid 87654321 --cid "your_cid_token_2" --proxy "socks5://127.0.0.1:7890"

# 切换当前默认活跃 Profile
./nga profile use homelab

# 删除指定的 Profile
./nga profile remove alt
```

### 4. 网络与风控健康度诊断 (`doctor`)
在首次使用或配置代理后，运行诊断检查网络与 WAF 状态：
```bash
# 标准终端格式
./nga doctor

# 机器可读 JSON 输出
./nga doctor --json

# 通过本地 SOCKS5 代理诊断
./nga doctor --proxy socks5://127.0.0.1:7890
```

### 5. 读取帖子与缓存控制 (`read`)
```bash
# 读取指定帖子第 1 页 (自动使用本地 SQLite 缓存，二次读取 < 10ms)
./nga read 38888888

# 强制跳过缓存拉取最新数据
./nga read 38888888 --refresh

# 禁用全局缓存
./nga read 38888888 --no-cache

# 紧凑模式查看
./nga read 38888888 --compact

# 仅查看楼主发言
./nga read 38888888 --author

# 屏蔽指定用户的回复
./nga read 38888888 --filter-uid 60613851,43016716

# 输出完整 JSON 供管道处理 (如 jq)
./nga read 38888888 --json | jq '.posts[0]'
```

### 6. 尾部查看与流式实时监听 (`tail` / `tail -f`)
```bash
# 查看最后 10 楼 (默认)
./nga tail 38888888

# 查看最后 3 楼并紧凑排版
./nga tail 38888888 -n 3 --compact

# 流式实时监听新回复 (tail -f 模式，带防风控抖动与退避)
./nga tail 38888888 -f

# 调整基准轮询间隔并开启紧凑输出
./nga tail 38888888 -f --interval 3s --compact

# 流式监听中仅关注楼主发言更新
./nga tail 38888888 -f --author
```

### 7. 发帖与回复 (`reply` / `post`)
```bash
# 直接回复帖子
./nga reply 38888888 -m "感谢楼主分享，已收藏！"

# 引用指定楼层进行回复 (自动抓取该楼内容并组装标准 [quote] BBCode)
./nga reply 38888888 --quote 0 -m "完全赞同楼主的观点。"

# 从 Markdown 或文本文件中读取回复长文
./nga reply 38888888 -f ./reply.txt

# 使用特定 Profile 账号发送回复
./nga reply 38888888 -m "备用号测试回复" --profile homelab

# 在指定板块发布新主题帖 (例如 FID 441)
./nga post 441 -t "【测试】NGA CLI 终端发帖功能验证" -m "使用 Go 开发的轻量命令行客户端发布。"
```

---

## 最佳实践与 Agent Skill 规范

为方便 AI Agent（如 Multica、Codex、Claude 等）自主调度与自动化集成，本项目提供符合标准 Agent Skill 规范的元数据与说明文档，同时支持人类开发者作为运维速查手册：

| Skill 名称 | 规范文档路径 | 核心能力与适用场景 |
| :--- | :--- | :--- |
| **`nga-cli`** | [通用命令用法与动态交互规范](docs/skills/nga-cli/SKILL.md) | **通用 CLI 调度与自省**：全命令矩阵、JSON 结构化调用、Profile 凭据与代理切换，内置“版本探测与动态自省机制”，保证在 CLI 后续功能扩展后依然长效保鲜。 |
| **`nga-thread-monitor`** | [特定用户回复监控与流式监听规范](docs/skills/nga-thread-monitor/SKILL.md) | **大帖与流式监控**：10万+ 楼超大热帖末页数学直算、全局最小流量、特定人员/楼主过滤、流式监听防风控退避抖动及 Webhook 管道集成。 |

### Agent Skill 安装与使用

在 Multica、Codex 等规范工作区中，只需将 Skill 规范文件放置于当前工作区的 `.agents/skills/<skill-name>/SKILL.md`，Agent 平台即会自动发现并装载。

提供 **curl / PowerShell 一句话全自动安装**：

#### 1. Linux / macOS (Bash)
```bash
# 一键安装全部 Agent Skill（自动建立 .agents/skills/ 结构并拉取规范）
curl -sSL https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.sh | bash

# 仅安装指定的单个 Skill (例如 nga-cli 或 nga-thread-monitor)
curl -sSL https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.sh | bash -s -- nga-cli
```

#### 2. Windows (PowerShell)
```powershell
# 一键安装全部 Agent Skill
irm https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.ps1 | iex

# 仅安装指定的单个 Skill
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.ps1))) nga-cli
```

#### 3. 极简单行 curl（免执行脚本，直接获取规范文件）
```bash
# 安装通用命令用法 Skill
mkdir -p .agents/skills/nga-cli && curl -fsSL https://raw.githubusercontent.com/fimreal/nga-cli/main/docs/skills/nga-cli/SKILL.md -o .agents/skills/nga-cli/SKILL.md

# 安装大帖监控与特定用户回复追踪 Skill
mkdir -p .agents/skills/nga-thread-monitor && curl -fsSL https://raw.githubusercontent.com/fimreal/nga-cli/main/docs/skills/nga-thread-monitor/SKILL.md -o .agents/skills/nga-thread-monitor/SKILL.md
```

---

## 缓存与防风控机制设计

```mermaid
flowchart TD
    Cmd[read / tail / reply / post] --> Profile[Profile 凭证与代理选择]
    Profile --> DupCheck{30秒持久化防重检查?}
    DupCheck -- 命中相同内容 & 无 --force --> Block[阻止重复提交]
    DupCheck -- 通过 / 读请求 --> CachePolicy{本地 SQLite 缓存检查}
    
    CachePolicy -- 命中已封存历史页 (Page < TotalPages) --> Hit[本地直读 0 流量 (<10ms)]
    CachePolicy -- 未命中 / 活跃末页过期 / 发帖请求 --> Network[走 NGA 网络请求]
    
    Network --> Limiter[速率限制与 Jitter 调度]
    Limiter --> MobileUA[移动端 UA 伪装]
    MobileUA --> Proxy[Direct / SOCKS5 / HTTP 代理]
    Proxy --> Server[NGA 接口服务器]
    
    Server --> UpdateCache[更新 SQLite 线程元数据与页面 JSON]
    UpdateCache --> Render[终端格式化输出]
```

---

## 项目架构

```
.
├── cmd/               # CLI 路由与参数绑定 (Cobra Commands)
│   ├── root.go        # 全局参数与配置加载 (--proxy, --no-cache, --profile 等)
│   ├── read.go        # 帖子读取与过滤 (--refresh, --author, --filter-uid 等)
│   ├── tail.go        # 尾部直算与流式监听 (-f, --interval 等)
│   ├── reply.go       # 回复帖子 (--quote, -m, -f, --force 等)
│   ├── post.go        # 发新主题帖 (-t, -m, -f 等)
│   ├── profile.go     # 多账号凭据管理 (list, use, set, remove)
│   ├── doctor.go      # 网络与 WAF 连通性诊断
│   ├── config.go      # 基础配置读写
│   └── render.go      # 终端排版与卡片/紧凑渲染
├── internal/
│   ├── client/        # NGA HTTP Client (GBK 转码, 连接池, Mobile UA, Follow 调度, 发帖表单提交)
│   ├── cache/         # SQLite 本地增量缓存与发帖防重驱动 (Pure-Go 无 CGO)
│   ├── profile/       # Profile 多账号配置管理器
│   ├── parser/        # lite=js 清洗器, 控制字符修复, BBCode 转换
│   ├── limiter/       # 令牌桶限流与随机抖动 Jitter
│   └── filter/        # UID 黑白名单与楼主过滤器
├── pkg/
│   └── model/         # Thread, Post, DoctorReport 等领域模型
├── docs/
│   └── skills/        # Agent Skill 规范与最佳实践指南
├── scripts/           # Skill 一键安装脚本 (install-skills.sh / .ps1)
├── Dockerfile         # 极简多阶段构建 Alpine 镜像
├── docker-compose.yml # 容器化部署模版
├── main.go            # 命令行入口
└── README.md
```
