# 项目问题排查记录（2026-10-13）

本轮以 Web / Desktop 共用 UI、首屏加载、核心入口闭环和现有自动化回归为范围。确定性缺陷已直接修复；跨模块、需要专项设计或性能基准的事项继续进入 `UX_BACKLOG.md`。

## 已修复

| 领域 | 问题 | 处理 | 验证 |
|------|------|------|------|
| 首屏加载 | 保存为浅色或跟随系统时，页面先使用默认深色令牌，React effect 执行后再切换；语言偏好也未在首屏同步到 `html lang` | Web / Desktop 在应用模块执行前加载轻量 appearance bootstrap，同步主题与语言 | Playwright 记录主题属性变化，确认浅色启动过程中未回落到 `dark`；桌面与移动用例通过 |
| Agent 恢复 | Agent 离线时直接显示 `Failed to fetch`，用户无法判断下一步 | 按超时、未配对、网络不可达、错误地址和服务异常映射为可操作提示；保留重试与设置入口 | 桌面与移动浏览器均覆盖离线恢复文案 |
| 信息泄漏 | 工作区未加载时仍显示 `default-project / default-collection` 等内部 ID | 仅在项目与集合均成功解析后显示可读保存路径 | 离线首屏不再展示默认内部 ID |
| 功能闭环 | “自动化测试”侧轨只改变导航状态，没有进入可运行页面 | 恢复 Collection Runner 注册与页面实例，侧轨入口直接打开 Runner | 桌面与移动均验证 `view=automation` / `workbench=runner` 与 Runner 可见 |
| 功能闭环 | 项目设置“克隆项目”按钮无事件 | 接入项目克隆能力，增加忙碌态、成功或失败反馈 | Web / Desktop 类型检查通过，既有项目克隆实现复用 |
| 可访问性 | AppShell 内部的项目首页、工作台首页和项目设置再次使用 `main` | 内层容器改为有名称的 `section` / `region`，只保留壳层顶级 `main` | 浏览器语义冒烟通过 |
| 响应校验 | 客户端设计校验与执行端同名结果直接拼接，失败项重复展示 | 合并时对同名设计校验去重，以当前所选响应定义的客户端结果为准 | 响应校验分隔区桌面与移动回归通过 |
| 工程基线 | UI 包生成声明时 `CurlImportDialog` 推断类型引用嵌套 React 类型路径 | 显式声明 `ReactPortal | null` 返回类型 | `packages/ui` 独立 typecheck 通过 |

## 后续优化建议

以下问题不适合在本轮通过局部修改解决，继续以现有 UIA 编号跟踪：

1. `UIA-021`：建立统一的 loading / success / retry 反馈矩阵，避免工作台各自维护状态文案。
2. `UIA-022`：为 WebSocket 长消息流和大型资源树补虚拟化与容量上限，并记录滚动锚点。
3. `UIA-023`、`UIA-040`、`UIA-043`、`UIA-046`：开展移动端专项重排，不再依赖裁切桌面布局。
4. `UIA-024`、`UIA-035`：统一项目 / 模块 / 集合 / 接口术语和设置入口的信息架构。
5. `UIA-032`、`UIA-036`：为每个工作台建立主题、语言、键盘、空态、错误态和真实功能闭环验收矩阵。
6. `UIA-041`：本轮完成工作区启动错误分类；Capture、历史和各协议执行错误仍应迁移到统一的结构化 Agent 错误模型。

## 验证结果

- TypeScript：UI、Web、Desktop 及共享包检查通过。
- 单元测试：设计系统、应用接线、请求模型、导入导出、协议 SDK 与 UI 测试通过。
- 浏览器 E2E：桌面与移动共 `81 passed / 1 skipped`；跳过项为原测试预设的移动视觉快照跳过。
- 本轮未修改 Rust 源码，因此未重复运行 Rust workspace 测试。

## 2026-10-14 修复跟进

| 领域 | 已处理问题 | 验证依据 |
|------|------------|----------|
| 可发现性 | 收藏按钮在键盘聚焦和无悬停设备上始终可见 | CSS 回归与移动端浏览器截图 |
| 表单安全 | 原生按钮显式声明 `type="button"`，并用 AST 测试阻止回归 | `buttonTypes.test.ts` |
| 未保存输入 | 新建项目、模块和目录有输入时不再被遮罩点击或 Escape 静默关闭 | 组件行为与类型检查 |
| 平台一致性 | Windows/Linux 显示 `Ctrl K` / `Ctrl ,`，Apple 平台显示 `⌘ K` / `⌘ ,` | `platform.test.ts` 与 Playwright |
| 可访问性 | 接口生命周期标签组使用真实中文 accessible name | 桌面与移动 Playwright 语义断言 |
| 抓包闭环 | 拆分代理配置文本、增加一键复制、中文状态、可见运行状态与移动布局 | 桌面/移动浏览器冒烟与截图 |
| 移动布局 | 主壳使用 `100dvh`；项目侧轨在窄屏改为底部导航，工作区保持全宽 | 412px 视口浏览器断言与截图 |
| 错误恢复 | 抓包网络、超时、鉴权和权限错误转为可执行提示，不再裸露 `Failed to fetch` | `executionError.test.ts` 与浏览器错误态 |
| 工程基线 | Windows + Node 22 直接启动 `pnpm.cmd` 会使 `validate:changed` 报 `EINVAL` | 优先复用当前 pnpm CLI，并为 `.cmd` 保留 Windows shell 兼容路径；`validate:changed --js` 已通过 |
| 更名回归 | 脚本加密辅助函数测试输入已改为 `ProtoVoy`，Base64 断言仍保留旧项目名结果 | 更新陈旧断言并纳入 Rust workspace 回归 |

本轮关闭 `UIA-007/012/016/029/034/038/042/043`。`UIA-041` 与 `UIA-046` 仍为 partial：共享错误模型尚需覆盖历史和全部协议工作台，移动任务流尚需继续覆盖 Gateway、AI 等工具页。`ISS-010/013/018` 属于依赖栈或共享 Runner 架构工作，未以局部 UI 修改虚假关闭。

跟进验证：`pnpm validate:changed --js`、`pnpm typecheck`、`pnpm test`、`cargo test --workspace --locked` 与 `cargo clippy --workspace --all-targets --locked -- -D warnings` 均通过；相关桌面/移动 Playwright 冒烟共 `8 passed`。完整 E2E 按仓库策略留到合并前执行。

## 2026-10-14 第二轮修复

| 领域 | 已处理问题 | 验证依据 |
|------|------------|----------|
| Tab 键盘模型 | Mock、WebSocket、AMQP、Kafka、MQTT 与既有协议工作台统一 roving `tabIndex` / 方向键模型 | `tabKeyboardModel.test.ts`；相关桌面与移动浏览器回归 |
| 危险操作 | 项目脚本删除接入真实 Agent 删除与共享确认；脚本复制写入 Agent；环境和网关任务删除统一危险确认 | UI 类型检查与组件回归 |
| 图标规范 | 交互按钮禁止使用 `×`、`↑`、`✓` 等字符代替共享图标；协议方向符号仍作为数据保留 | `controlIconGlyphs.test.ts` |
| 禁用态对比度 | 共享按钮、Checkbox 和 Switch 不再整体降透明度，改用语义弱化色 | `disabledContrast.test.ts`；桌面/移动主题冒烟 |
| 工作流闭环 | 私有化镜像监听真实 Docker 构建输入；安装生命周期在合并 `main` 后覆盖共享 crates/packages 与根清单 | YAML 解析及触发路径断言；ISS-016 关闭 |

本轮浏览器回归共 `66 passed`，随后禁用态设计系统改动的桌面/移动主题冒烟 `2 passed`。UIA-004、UIA-015 与 ISS-016 已关闭；UIA-010、UIA-017、UIA-026、UIA-027 保持 partial，并在 `UX_BACKLOG.md` 明确剩余范围。

## 2026-10-14 第三轮修复

集合执行开始从多端重复循环收敛到 `execution-engine`：新增共享顺序 Runner，统一 fail-fast、失败断言判定以及后置脚本变量在单次运行内向后传递的语义，且不再要求把提取变量写回持久环境。Gateway 保留原单请求 CI API，同时新增 `/v1/runner/collection`，可按顺序执行完整 envelopes 并返回 CI `exitCode`。

当前 ISS-013/ISS-018 标记为 `in_progress` 而非虚假关闭：Gateway、Desktop 与 Local Agent/Web 已接入共享编排，Web 不再在浏览器中下载资源树并逐条发起执行；Desktop/Agent 均保留执行历史，脚本提取变量只在本次集合运行中传递。CLI 迁移、数据迭代、项目包输入以及 UI 的环境选择、进度/取消仍是下一批工作。

验证：共享 Runner 变量串联和 fail-fast `2 passed`；Gateway `7 passed`；Local Agent `2 passed`；Web/Agent 集合入口桌面与移动浏览器 `2 passed`；Engine、Agent、Desktop、Gateway 严格 Clippy 通过。

安全前置审查同时发现 `quinn-proto 0.11.16` 与 `wasmtime 47.0.4` 的 10 个 RustSec 公告（含 critical）。已分别升级到 `0.11.18` 与 `48.0.4`，未加入 audit ignore；`cargo audit -D warnings`、Plugin Runtime 回归、单线程完整 Rust workspace 测试和全 workspace Clippy 均通过，记录为 ISS-022 并关闭。
