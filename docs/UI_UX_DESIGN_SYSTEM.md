# UI/UX 设计系统

> 规范文档（tokens、组件、图标、无障碍）。**实施状态与 UX 债**见 [UX_BACKLOG.md](./UX_BACKLOG.md)；工程 bug 见 [ISSUES.md](./ISSUES.md)。
>
> 最近更新：2026-09-17

## 1. 产品与设计方向

本地优先的多协议开发者工具。界面采用“IDE + 高密度数据工作台”模式，强调紧凑、稳定和键盘友好。Hoppscotch 仅作为操作效率参考，不复制其品牌或具体组件样式。

- 使用清晰层级和稳定结构，避免装饰性大卡片、复杂渐变和营销式大标题；
- 采用 8px 基础间距，工作台主要间距为 8–16px；
- 主操作必须明确，次要操作不能争夺视觉焦点；
- 请求上下文、执行状态和响应指标保持在稳定位置；
- 状态不能只依赖颜色，必须配合文字或图标；
- Web 和 Desktop 统一使用 `@protovoy/ui`；令牌与基础控件来自 `@protovoy/design-system`。

## 2. 信息架构

共享壳层包含紧凑顶部栏、资源管理器、协议导航和主工作区。

| 分组 | 工作台 |
|---|---|
| API | HTTP、GraphQL、gRPC、SOAP/RPC |
| 实时通信 | WebSocket、SSE、TCP/UDP |
| 消息 | MQTT、AMQP、Kafka |
| 数据 | Redis、SQL |
| 工具 | Mock、Runner、Gateway、Capture、Plugins、AI |
| 账户/协作 | 顶栏 CollaborationHub（Team、Comments、SSO），不在协议轨 |

## 3. 设计令牌

令牌集中在 `packages/design-system`（CSS 变量 + 主题 API）。业务与工作台禁止硬编码色值。

| 令牌 | 深色基准 | 用途 |
|---|---:|---|
| `--protovoy-bg` | `#0b0d10` | 应用画布和代码表面 |
| `--protovoy-nav` | `#101216` | 顶部栏与导航 |
| `--protovoy-project-rail` | 深色导航压暗面 | 项目功能侧轨 |
| `--protovoy-workspace` | `#111419` | 主工作区 |
| `--protovoy-bg-elevated` | `#171a20` | 输入框、列表行和普通面板 |
| `--protovoy-overlay` | `#1d2128` | 对话框和命令面板 |

迁移期可短暂保留 `--protovoy-*` 别名；新代码只写 `--protovoy-*`。

- UI 使用 IBM Plex Sans 等无衬线字体；代码使用 JetBrains Mono 等等宽字体；
- 微交互 150–250ms；支持 `prefers-reduced-motion`；
- 层级令牌：`--z-header` / `dropdown` / `drag` / `dialog` / `toast`。

主题：`applyTheme(mode)`（`dark` | `light` | `system`）由 design-system 提供；持久化由 `@protovoy/ui` store 负责。

## 4. 图标系统

产品 UI 图标：**Lucide**，经 `@protovoy/design-system` 的 `<Icon name="…" />` 使用。

| 场景 | 允许来源 |
|---|---|
| 导航、工具栏、表单、状态 | Lucide via shared `Icon` |
| 第三方品牌 Logo | 官方 SVG / Simple Icons |
| 产品标志 | 仓库品牌资源（`protovoy-mark.svg`） |
| Lucide 缺少的协议图标 | 同网格自定义 SVG，经评审 |

禁止 emoji / 字符字形充当控件图标（`•••`、`×`、`✓` 等）。Icon-only 控件必须有可访问名称。

协议语义映射建议：HTTP=`Globe2`、gRPC=`Network`、WebSocket=`RadioTower`、Redis/SQL=`Database`、Runner=`Play`、Gateway=`Route`、Capture=`ScanSearch`、Plugins=`PlugZap`、AI=`Bot`/`Sparkles`。缺口见 [UIA-003](./UX_BACKLOG.md)。

## 5. 公共组件

- Primitives（`Button`、`Field`、`Dialog`、`Tabs`、…）→ `@protovoy/design-system`
- Shell / workbenches → `@protovoy/ui`（`AppShell`、`WorkbenchDeck`、`WorkbenchFrame`、`SplitPane`）

`useAppStore` 可持久化主题、导航、收藏等；禁止存请求正文、令牌或敏感数据。

## 6. 工作台要求

- 表单可见 label，错误可 `aria-describedby`；
- 超过约 300ms 的操作有加载反馈；
- 空页有解释与下一步；
- 破坏性操作统一确认；
- 长列表虚拟化；
- `focus-visible` 与 hover 齐全。

## 7. 禁止事项

不得新增营销式 Hero、重度玻璃拟态或霓虹风格、页面私有颜色体系、hover 缩放、超长动效、破坏小屏的固定宽度、永久挂载的隐藏重型编辑器、无反馈的异步或破坏性操作，以及任意放大的 z-index。

不得在本文件用 ✅/🟡 自报实施完成度；完成证据挂到 [UX_BACKLOG.md](./UX_BACKLOG.md) / [ISSUES.md](./ISSUES.md)。
