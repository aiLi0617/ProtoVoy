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

ISS-013/ISS-018 已在后续补齐后关闭：Gateway、Desktop 与 Local Agent/Web 已接入共享编排，Web 不再在浏览器中下载资源树并逐条发起执行；Desktop/Agent 均保留执行历史，脚本提取变量只在本次集合运行中传递。CLI 支持数据迭代、并发与 JUnit；并发请求采用隔离作用域，依赖前序提取变量的场景继续使用默认顺序执行。Gateway 的无状态 CI 接口接受完整 envelopes，而不直接解包尚未稳定的项目归档格式。

验证：共享 Runner 变量串联和 fail-fast `2 passed`；Gateway `7 passed`；Local Agent `2 passed`；Web/Agent 集合入口桌面与移动浏览器 `2 passed`；Engine、Agent、Desktop、Gateway 严格 Clippy 通过。

安全前置审查同时发现 `quinn-proto 0.11.16` 与 `wasmtime 47.0.4` 的 10 个 RustSec 公告（含 critical）。已分别升级到 `0.11.18` 与 `48.0.4`，未加入 audit ignore；`cargo audit -D warnings`、Plugin Runtime 回归、单线程完整 Rust workspace 测试和全 workspace Clippy 均通过，记录为 ISS-022 并关闭。

## 2026-10-14 第四轮修复

Desktop 与 Web 的 Collection Runner 增加稳定运行 ID、已完成/总数进度、运行中锁定配置和可访问的取消操作。Desktop 与 Local Agent 都提供进度查询和取消入口；取消会终止集合调度，并保证不再启动下一条请求。环境覆盖、fail-fast 和运行内变量链均由共享 Runner 承担，ISS-013 因而闭环。

验证：Web/Desktop/UI 类型检查通过；集合环境与运行生命周期浏览器回归 `2 passed`，取消进度视觉截图已复核；变更选择器覆盖的 34 个 UI 测试文件、Rust 测试与仓库内依赖检查通过；相关 Rust 包严格 Clippy 通过。`validate:changed` 的原生 Clippy 步骤受本机旧 `cargo-clippy.exe` shim 架构不兼容影响，已使用稳定工具链对同一包集合执行等价命令并通过。

## 2026-10-15 第五轮修复

协作服务首次 Owner 引导不再要求部署者预先生成并同步一串环境变量。空库启动时，缺失、空白或仍为示例占位值的配置会被替换为 256-bit URL-safe 随机令牌，并仅在启动日志提示；已有用户时不输出。公开的 `/v1/auth/bootstrap-status` 只暴露引导是否开放、是否需要令牌以及令牌是否由服务生成，不返回凭证本身。Team 客户端据此隐藏已关闭的首次引导入口，并在自动生成场景说明从受控启动日志取值。

部署 Compose 允许引导令牌留空，仍支持生产环境显式注入独立高熵值。完成首位 Owner 创建后，既有服务端用户数门禁继续让引导接口永久失效。安全审查确认：未削弱 Local Agent 鉴权、未启用远程绑定、未记录 Authorization Header；唯一明文输出是空库首次启动所需的一次性引导凭证，符合 ISS-001 既定威胁模型。

验证：协作服务 Gradle 回归通过；UI 类型检查通过；首次引导开放/关闭两条桌面 Chromium 回归 `2 passed`，并复核自动生成提示截图。ISS-001 关闭。

## 2026-10-15 第六轮修复

字段级错误反馈完成无障碍闭环。共享 `Field` 原语的错误描述继续作为 `role=alert`，并通过稳定 ID 与控件的 `aria-describedby` 对应；回归测试新增 ID 一致性断言。此前绕开共享原语的 HTTP 状态码组合框、Mock 客户端 IP 与 UDP 目标地址也增加可见的就地恢复提示，并同步 `aria-invalid` / `aria-describedby`，避免只依赖红色边框、placeholder 或 title。

验证：UI 类型检查与共享组件测试通过；Mock 与 UDP 两条桌面 Chromium 无障碍回归 `2 passed`，视觉截图已复核。UIA-020 关闭。

## 2026-10-15 第七轮修复

模态交互完成共享焦点闭环。保存接口、新建目录、测试用例预览/创建和执行历史统一接入 `ModalFrame`；焦点管理增加模态栈，嵌套弹窗打开时只有最上层处理 Tab 与 Escape，关闭后焦点返回各自触发控件。命令面板继续使用同一焦点约束能力；响应校验配置和参数差异提示属于页面内非模态浮层，不错误标记为 `aria-modal`。

验证：`npm run validate:changed -- --js` 通过；执行历史焦点锁定、Escape 关闭与焦点恢复桌面 Chromium 回归 `2 passed`，视觉截图已复核。UIA-011 关闭。

## 2026-10-15 第八轮修复

协议与工作台入口统一使用设计系统中的共享语义图标映射。工作台页签、创建菜单、命令面板和项目自动化入口不再分别维护图标或统一回退到闪电图标；未知扩展工作台使用中性的命令图标回退。新增回归断言覆盖全部内置工作台映射，并防止再次引入通用 `bolt` 碰撞。UIA-003 关闭。

验证：设计系统与 UI 包类型检查、`WorkbenchDeck` 定向回归和 `npm run validate:changed` 通过；桌面 Chromium 直连验证 16 个工作台入口图标均不重复，截图已复核。

设计系统文档只保留规范与约束，不再自报实现完成度；顶部明确把实时状态和 UX 债指向 `UX_BACKLOG.md`，并将图标章节的历史“缺口”措辞改为验证证据链接。UIA-030 关闭。

UIA-001 已完成：gRPC、WebSocket、SSE、TCP 与 UDP 顶部命令栏的接口名称、目标地址及执行/保存/取消操作统一使用共享 `TextInput` / `Button`；协议历史回放和脚本库同步迁移输入框、文本域、选择器及按钮；HTTP 工作台的命令操作、弹窗操作和关闭控件统一接入共享 `Button` / `IconButton`。所有 `*Workbench.tsx` 均接入共享组件入口，且不再以私有样式或 `ui-button` / `ui-icon-button` 类名仿造共享按钮。页签、消息列表行、文件选择等具有专用交互语义的原生控件按设计保留。

验证：UI 类型检查与共享原语回归通过；HTTP、gRPC、WebSocket、SSE、TCP、UDP 与脚本库桌面 Chromium 回归 `7 passed`，截图已复核；随后执行 `npm run validate:changed`。

## 2026-10-15 第九轮修复

UIA-002 启动共享工作台文案收敛。协议工作台公共布局的接口名称、运行/保存/取消、请求配置、响应检查器、操作模式及请求加载提示均接入中英文资源；AMQP、Kafka、MQTT、Redis、RPC 与 SQL 的保存成功反馈统一使用翻译键。公共布局同时补齐 `Button` / `TextInput` 共享原语迁移，避免协议页通过私有原生控件绕过设计系统。各协议专属字段、空响应说明以及应用壳仍存在硬编码文案，因此 UIA-002 保持 partial。

验证：UI 类型检查、共享布局/原语与双语资源定向回归通过；英文 MQTT 公共布局桌面 Chromium 回归 `1 passed`，截图已复核。
