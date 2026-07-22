# 跨平台预览会话架构

Android/iOS 容器内以 H5 实现的业务页面遵循
[`webview-semantic-testing.md`](./webview-semantic-testing.md) 定义的统一语义自动化契约。

> 状态：Draft
>
> 目标分支：`feat/preview-sessions`
>
> 首个纵切：把外部可访问的 Web URL 发布为 issue 的预览会话

## 1. 背景

Multica 当前能让智能体在本地或云端运行时中修改代码、执行命令并回传文本日志，但人类验收仍然发生在平台之外：Web 项目需要自己找到开发服务器地址，桌面应用需要远程桌面，Android/iOS 需要模拟器或真机。

这不是某一种构建产物的预览问题，而是一个通用的“运行并接管界面”问题。APK、AAB、HTML 包和桌面安装包都是静态产物；用户真正需要的是一个正在运行、可操作、可观察、可回收的环境。

因此，本设计不新增“Android Artifact”之类的平台特例，而是引入通用的**预览会话**（Preview Session）。

## 2. 决策摘要

1. 构建产物和运行会话是不同实体。产物长期保存，会话短期租用。
2. Web、桌面、Android 和 iOS 共享会话生命周期、权限、证据和 UI；底层传输由 Provider 适配。
3. Web 优先使用 URL 代理，不把清晰的 DOM 页面降级成视频流。
4. 桌面和移动端使用画面流与输入回传，浏览器只是远程操作终端，不模拟目标平台。
5. 会话默认绑定工作区和 issue。所有访问都使用短期令牌，不公开原始端口、ADB 或 VNC 地址。
6. 人类和智能体必须能查看同一个会话，并把截图、录像、日志和反馈沉淀回 issue。
7. 第一版只发布已经可从用户浏览器访问的 HTTP(S) URL，用它验证领域模型和协作闭环；安全隧道与设备 Provider 后续接入同一模型。
8. 仓库选择与预览发布是两个独立决策：Issue 只检出最小代码工作集，但其中所有可预览前端目标默认发布到该 Issue 的统一预览区；纯后端仓库默认不创建视觉预览。

### 2.1 多仓库项目的默认预览规则

Runtime 开发上下文是仓库与 Preview Target 的事实来源，Multica 仓库登记和 Project Resource 都不是使用前提。任务启动时扫描当前工作目录中的 Git roots；`multica repo checkout` 完成后自动刷新；智能体通过其他方式 clone、复制或切换代码时运行 `multica preview detect --write`。识别结果写入 `.multica/preview/targets.json`。

检测器依据工程清单识别一个仓库中的多个目标：前端框架 `package.json` 产生 Web/H5 Target，Android application Gradle plugin 产生 Android Target，Xcode project/workspace 产生 iOS Target，Electron/Tauri 产生 Desktop Target；仅出现服务端清单且没有 UI Target 的仓库标记为 `backend_only`。Project Resource 只保留为可选职责提示和 `always` / `never` 策略覆盖。

任务仍先依据 Issue 中明确的仓库引用，再依据实际开发目录中的检测证据以及可选的 `role` 和 `capabilities` 选择最小工作集。无法唯一判断时询问用户，不通过检出项目全部仓库来消除歧义。

预览发布在成功的 runnable checkpoint 后独立判断：

| 仓库目标 | `auto` 默认行为 |
| --- | --- |
| Web / H5 | 发布或更新 Web Preview Session |
| Android | 发布或更新 Android Device Preview Session |
| iOS | 发布或更新 iOS Device Preview Session |
| Desktop | 发布或更新 Desktop Preview Session |
| 纯后端 / 数据 / 基础设施 | 不创建视觉预览，回传测试和部署结果 |

资源未声明策略时等价于 `auto`，运行时根据项目清单和构建结构识别目标。`always` 用于非标准但可预览的仓库，必须同时指定平台；`never` 用于明确禁止视觉预览的仓库。混合应用修改普通业务页时优先更新 H5；只有变更跨越原生构建、桥、权限或平台行为时才同时更新 Android/iOS 壳预览。不同平台可以产生不同 Session，但在产品上收敛到同一个 Issue 预览区，而不是要求用户管理多种 Artifact 控件。

## 3. 目标与非目标

### 3.1 目标

- 在 issue 内统一查看 Web、桌面、Android、iOS 等前端运行状态。
- 允许智能体或成员创建、更新、停止预览会话。
- 允许成员打开会话并获取 Console、Network、应用日志等可用观察数据。
- 支持截图、录像和测试报告作为持久证据。
- 支持 Provider 按能力扩展，而不是在业务层按平台堆积分支。
- 支持本地守护进程、云模拟器、云真机和第三方设备农场。

### 3.2 非目标

- 不在第一版实现通用远程桌面协议。
- 不在第一版代理任意本地端口。
- 不承诺模拟器替代真实硬件验收。
- 不把生产签名密钥、生产账号或生产数据注入开发会话。
- 不把自动化测试运行和人工预览会话合并成同一个实体。

## 4. 领域模型

```text
Agent Task / Human action
          |
          v
    Build Artifact -------------+
          |                      |
          v                      v
    Preview Target ------> Preview Session ------> Evidence
          ^                      |
          |                      +----> Human review
     Preview Provider           +----> Agent inspection
```

### 4.1 Build Artifact

不可变构建输出，例如：

- Web 静态包或部署版本；
- APK、AAB、测试 APK；
- Electron/macOS/Windows/Linux 安装包；
- source map、符号表和构建清单。

产物记录来源 commit、构建变体、校验和、创建 task 和保留策略。它可以独立存在，不要求马上启动会话。

### 4.2 Preview Target

声明期望的运行环境，例如：

```json
{
  "platform": "android",
  "profile": "pixel_9_api_36",
  "region": "ap-southeast-1",
  "required_capabilities": ["touch", "camera_injection", "logcat"]
}
```

Target 是调度意图，不等同于一台长期绑定的机器。

### 4.3 Preview Session

一次有租约的运行实例。会话负责：

- 关联工作区、issue、创建者、来源 task 和可选产物；
- 保存平台、Provider、Provider 侧引用和当前状态；
- 提供受控的打开入口；
- 声明实际可用能力；
- 记录到期、停止、失败和最后心跳时间。

第一版会话可以直接持有一个经过校验的外部 HTTP(S) URL。后续版本中，客户端只拿 Multica 的会话 URL，真实 Provider 地址和凭据不返回前端。

### 4.4 Evidence

从会话提取并长期保留的验收材料：

- 截图与标注；
- 录屏及时间点评论；
- Console、Network、logcat 和崩溃日志；
- 自动化测试报告；
- 设备、系统版本、网络配置和会话元数据。

Evidence 可以先复用 attachment 存储，后续再增加结构化索引。

### 4.5 Test Run

Test Run 是批量、可重复的自动化执行；Preview Session 是人类或智能体交互式探索。两者可以共享 Provider 和产物，但生命周期、配额和结果模型不同，不应合表。

## 5. 生命周期

```text
creating -> starting -> running <-> sleeping -> stopping -> stopped
                    \-> failed
                    \-> expired
```

状态定义：

| 状态 | 含义 |
| --- | --- |
| `creating` | 已创建数据库记录，尚未分配 Provider 资源 |
| `starting` | Provider 正在启动环境、部署产物 |
| `running` | 会话可打开 |
| `sleeping` | 会话记录仍可打开，但设备控制租约已释放；打开时重新竞争租约并唤醒 |
| `stopping` | 正在回收 Provider 资源 |
| `stopped` | 已主动停止，不再可打开 |
| `failed` | 启动或运行失败，保留稳定错误摘要 |
| `expired` | 租约到期并已回收 |

第一版外部 Web URL 在创建成功后直接进入 `running`。`sleeping` 只用于可恢复的设备会话，终态不能跳回 `running`；终态后的重新启动应创建新会话，以保留审计边界。

本地设备会话采用三层回收：同一 Workspace、同一设备只有一个 5 分钟可续期的写租约；连续 20 分钟无 Viewer 或 CLI 操作后会话投影为 `sleeping`；Session 最长 4 小时，之后投影为 `expired`。Issue Viewer 打开前、观看期间以及 CLI 执行动作前都会调用 `touch`，因此用户观看和智能体自动化共享同一租约。租约冲突返回 409，不允许静默切到其他 serial。模拟器进程是 Runtime 的预热共享容量，不属于某个 Issue Session；休眠只释放媒体、WebView 调试和设备控制资源，不关闭预热模拟器。

设备插拔只更新 Runtime inventory，不改变 Session。成员在 Issue Viewer 显式切换时，系统先锁定目标设备，再通过当前 Runtime 部署 Session artifact，最后原子更新 `preview_url` 中的 serial 并发布更新事件；部署失败回滚原绑定。这样用户和智能体始终引用同一个 Session ID，不会因 USB 重连静默观看到另一台设备。

## 6. Provider 协议

业务层只依赖统一 Provider 能力：

```text
Provision(target, artifact) -> provider_ref, capabilities
Connect(session, actor)     -> short-lived connection descriptor
Inspect(session)            -> health, logs, metadata
Control(session, action)    -> restart, rotate, clear_data, inject_media...
Capture(session, kind)      -> evidence
Stop(session)               -> terminal state
```

Provider 通过能力声明控制 UI，不通过平台名称猜测：

```text
open_url       dom          console        network
pointer        keyboard     touch          rotate
logcat         camera       camera_injection
location       microphone   screenshot     recording
```

未知能力必须被忽略，保证新 Provider 不会让旧客户端崩溃。

## 7. 平台适配

| Provider | 启动方式 | 交互通道 | 首要观察数据 |
| --- | --- | --- | --- |
| External Web | 已存在的 HTTP(S) URL | 浏览器直接打开 | 页面自身能力 |
| Managed Web | 守护进程启动 dev server，Gateway 建安全隧道 | 反向代理 URL | Console、Network、DOM |
| Desktop | VM/宿主机启动应用 | WebRTC 画面 + 鼠标键盘 | stdout、崩溃、系统日志 |
| Android AVD | 专用 KVM 节点启动模拟器并安装 APK | WebRTC + ADB 控制 | logcat、截图、设备状态 |
| Android Device | 本地 USB Bridge 或云真机 | 设备流 + ADB/Appium | 真机日志、录像、设备信息 |
| iOS Simulator | macOS 节点启动 Simulator | 设备流 + simctl/WebDriverAgent | 系统日志、崩溃、截图 |

Managed Web 不允许将任意 `localhost:port` 直接暴露到公网。守护进程主动连接 Gateway，Gateway 根据工作区、会话和短期访问令牌转发 HTTP/WebSocket。

## 8. 安全模型

### 8.1 URL 与网络

- 第一版仅接受绝对 `http://` 或 `https://` URL。
- 拒绝 `javascript:`、`data:`、`file:` 和带用户名密码的 URL。
- 第一版不由 Multica Server 抓取 URL，避免形成 SSRF 代理。
- 后续 Gateway 必须阻止云元数据地址、控制面地址和跨工作区路由。
- 会话 URL 使用短期、单会话、单用户签名令牌；原始 ADB/VNC/Provider 凭据永不返回浏览器。

### 8.2 隔离与清理

- 每个会话拥有独立租约、网络策略和存储边界。
- Provider 必须幂等停止；服务端 sweeper 回收超时会话。
- 真机结束后恢复已知干净状态；模拟器从只读快照启动。
- 截图和录像可能包含敏感信息，沿用 attachment 的工作区授权与保留策略。

### 8.3 权限

- 工作区成员可查看绑定到其可访问 issue 的会话。
- 创建和更新支持成员身份及受 task token 约束的智能体身份。
- 停止会话要求成员创建者或工作区 owner/admin；智能体还必须同时满足创建者一致、task 与会话 issue 一致。
- 创建和停止进入活动审计。Phase 0 直接跳转外部 URL，服务端无法可靠感知“打开”；Managed Web 的 `Connect` 入口落地后再审计打开动作。

## 9. 数据模型

长期模型需要以下信息：

| 字段 | 用途 |
| --- | --- |
| `id` | 会话 UUID |
| `workspace_id` | 多租户边界 |
| `issue_id` | 协作归属 |
| `creator_type`, `creator_id` | `member` 或 `agent` |
| `task_id` | 可选来源 task，由服务端从可信 task token 派生 |
| `artifact_id` | 预留，Build Artifact 落表后补外键 |
| `platform` | `web`、`desktop`、`android`、`ios` 或未来值 |
| `provider` | 第一版为 `external_web` |
| `title` | 人类可读名称 |
| `status` | 生命周期状态 |
| `preview_url` | 第一版外部 URL；后续 Provider 可为空 |
| `error_message` | 稳定、脱敏的失败摘要 |
| `expires_at` | 可选租约截止时间 |
| `last_active_at` | Viewer 或 CLI 最近一次有效操作时间 |
| `lease_expires_at` | 本地设备独占写租约截止时间 |
| `started_at`, `stopped_at` | 生命周期审计 |
| `created_at`, `updated_at` | 通用时间戳 |

Phase 0 实际落表只包含当前闭环需要的字段：工作区、issue、来源 task、平台、Provider、标题、外部 URL、状态、创建者、错误摘要、到期时间和生命周期时间戳。`provider_ref`、能力、健康心跳和结构化 metadata 等到真实 Provider 合约确定后再增加，避免先固化一个没有实现方的协议。

第一版也不提前创建 `build_artifact` 空表。等构建上传协议确定后，再以独立迁移加入产物实体及 `artifact_id` 外键。

## 10. API

第一版：

```text
GET  /api/issues/{issueId}/preview-sessions
POST /api/issues/{issueId}/preview-sessions
GET  /api/preview-sessions/{sessionId}
POST /api/preview-sessions/{sessionId}/touch
POST /api/preview-sessions/{sessionId}/stop
```

`POST` 示例：

```json
{
  "title": "Checkout redesign",
  "platform": "web",
  "provider": "external_web",
  "preview_url": "https://preview.example.com/pr-481",
  "expires_at": "2026-07-14T12:00:00Z"
}
```

Phase 0 的 stop 只停止 Multica 中的外部 URL 登记，不会假装已经能终止 URL 背后的进程。它是幂等操作，并保留 stopped 记录用于审计。Provider 回调、成员编辑和资源控制最终应拆成不同权限入口。

Phase 0 在 API 响应和客户端中把已超过 `expires_at` 的活跃记录投影为 `expired`，因此过期链接不可再打开或停止。本地设备的 `touch` 在事务内竞争设备写租约、释放过期持有者并唤醒当前会话；External Web 本身没有由 Multica 持有的运行资源。

列表按 `created_at DESC` 返回，默认包含活跃与最近终态会话。客户端不能假定枚举完整，未知平台、Provider、状态均降级为通用展示。

## 11. 实时事件

建议事件：

```text
preview_session:created
preview_session:updated
```

事件只携带会话摘要或 ID，客户端通过 TanStack Query 失效重新读取。视频、音频、输入和大日志不经过现有业务 WebSocket；它们走专用媒体/日志通道。

## 12. CLI 与智能体工作流

第一版提供：

```bash
multica preview create --issue MUL-123 --url https://preview.example.com/pr-481 --title "Checkout redesign"
multica preview list --issue MUL-123 --output json
multica preview stop <session-id>
```

典型流程：

1. 智能体修改并部署 Web 应用。
2. 智能体用 CLI 创建预览会话。
3. Issue 页面通过实时事件出现“预览”区。
4. 成员打开真实页面进行交互，提交截图或评论。
5. 下一次 Agent Task 获得会话引用、反馈和证据。

后续 Managed Web Provider 由守护进程自动发现已监听端口并注册，不要求智能体把 `localhost` URL 发布给用户。

## 13. Issue UI

Issue 详情中增加紧凑的“预览”区：

- 默认展示最新的 `running` 会话，`sleeping` 设备会话可点击唤醒；
- 提供平台、状态、来源、创建时间和到期时间；
- 主操作是“打开预览”；
- 次操作包括复制链接、刷新、停止、查看日志；
- 多会话使用列表或菜单切换，不嵌套多层卡片；
- 本地移动设备同时存在模拟器和 USB 真机时显示来源分段控件；设备插拔不自动切换，显式切换在目标部署成功后才更新当前 Session；
- `failed` 显示脱敏错误摘要，`stopped`/`expired` 保留历史但弱化；
- 没有会话时不占据大块空间。

第一版在新标签页打开外部 URL，并设置 `noopener,noreferrer`。不尝试 iframe：Multica 的 CSP、目标站的 `frame-ancestors` / `X-Frame-Options` 以及同源凭据边界都不允许把任意智能体页面当成可信子页面。后续 Managed Web 应使用隔离预览域名；设备 Provider 使用固定比例的媒体画面与平台能力工具栏。

## 14. 可观察性与配额

至少记录：

- 创建、启动、可用、停止各阶段耗时；
- Provider 成功率和稳定错误分类；
- 活跃会话数、租约超时数和回收失败数；
- 每工作区的会话分钟数、流量和存储；
- 首次打开率、反馈率及从反馈到下一次 task 的时间。

配额按 Provider 资源计量。External Web 几乎不消耗运行资源；AVD、macOS 和真机需要并发槽位及最大租约。

## 15. 分阶段交付

### 移动设备落地顺序

AdaKami 等原生 Android 项目的首个可交付路径是 **USB Device Runtime**，不等待云端模拟器。这里必须区分控制面与设备所在位置：

```text
Multica Cloud <== outbound TLS / WebRTC ==> Device Runtime Host <== USB / ADB ==> Physical Android device
```

公共 Mac 通过 daemon 的 `--device-runtime-url` 参数执行特殊挂载。daemon 自动
写入固定的 `mac-mobile-preview` 标签和 loopback Device Runtime URL；普通 Agent
Runtime 不携带该标签。Agent 在 APK 构建成功后的可运行检查点执行
`multica preview device sync`，无需传 Runtime URL。CLI 按标签找到唯一在线 Mac，
优先继承 Issue 活动 Preview 的 serial；Issue 尚未绑定设备时才选择 emulator。
显式 serial 切换成功后，CLI 停止同 Issue 的旧 Android Session，保证用户画面和
智能体控制目标一致。完整挂载命令、验证步骤和换机规则见
[`usb-device-runtime-phase-one.md`](./usb-device-runtime-phase-one.md#mac-移动预览-runtime-的特殊挂载)。

Issue Viewer 还允许成员在同一台 Mac 上的 emulator 与 USB physical 之间显式切换。发现或重连 USB 设备只显示可选项，不覆盖当前 serial；切换完成后保留原 Session ID，因此 `preview device run` 会继续解析到用户正在观看的设备。

Runtime 部署采用可取消的异步 job，并向 Issue Viewer 上报准备、安装、启动、连接和就绪阶段。artifact 以 SHA-256 去重；目标设备仍安装相同包时跳过重装。浏览器使用绑定 Preview Session 的短期令牌，Host 原生命令使用 `MULTICA_DEVICE_RUNTIME_TOKEN`，Runtime 以同一个 Session ID 维护 serial 的 5 分钟写租约。设备清单同时上报授权状态、电量、充电和温度，使离线、未授权或不健康设备在切换前即可被识别。

`Device Runtime Host` 是一台受管的公共电脑或设备柜主机。它主动连接 Multica Cloud，并枚举、锁定和控制其 USB 设备；Cloud 不直接连接办公室网络中的手机，也不要求 Host 开放入站端口。系统权限弹窗、应用弹窗和相机画面属于同一条设备媒体流，由 Host 编码后经 WebRTC 发给浏览器；输入控制反向通过 Host 转为 ADB 或设备控制协议。

移动端分两期推进：

1. **一期：USB Device Runtime。** 公共电脑上的 agent 注册 Runtime 和可用真机，安装 APK、启动应用、回传画面及 logcat，并以独占租约回收设备。它覆盖真实相机、系统权限、机型差异等模拟器无法替代的行为。
2. **二期：Cloud Emulator Provider。** 云主机自行运行 Android Emulator，以相同的 Preview Provider 协议提供高并发、低成本的常规 UI 验收。真机仍通过 USB Device Runtime 接入；二期不会让云主机直接连接物理手机。该能力在当前规划中延期，未来一年不纳入交付范围。

两种 Provider 对 Preview Session 均暴露相同的能力模型。调度器按会话 target 选择真机或模拟器，浏览器不需要感知底层运行位置。

### Phase 0：领域纵切

- `preview_session` 表及工作区/issue 授权；
- External Web Provider；
- API、CLI、Query 和 Issue UI；
- URL 校验、状态转换、过期展示；
- 创建/更新实时事件，以及创建/停止活动审计。

### Phase 1：Managed Web

- 守护进程端口发现与显式端口注册；
- Gateway 反向隧道和短期访问令牌；
- HTTP/WebSocket、HMR、Console 和 Network；
- 会话心跳、sweeper 和配额。

### Phase 2：像素流 Provider

- 通用 WebRTC 媒体与输入协议；
- Desktop Provider；
- Android AVD Provider，支持安装、重启、旋转、logcat 和媒体注入；
- iOS Simulator Provider。

### Phase 3：真机与证据闭环

- 本地 USB Device Bridge；
- 第三方设备农场和自建真机池；
- 截图标注、录屏时间点反馈；
- Test Run 和设备矩阵；
- 会话复现包与自动视觉验收。

## 16. Phase 0 验收标准

- 成员可以为自己可访问的 issue 创建 HTTP(S) Web 预览会话。
- 非 HTTP(S)、含 URL 凭据或格式错误的地址返回 400。
- 其他工作区无法读取、更新或删除该会话。
- Issue 页面在 Web 与 Desktop 中展示相同的会话状态和打开入口。
- API 响应经过 schema 解析；缺失字段和未知枚举不会让 UI 崩溃。
- 智能体可以通过 CLI 发布会话，且内置 skill 文档同步更新。
- 删除 issue 会级联删除预览会话。
- 数据库迁移具备完整 down migration。
- 全量 `make check` 通过。

## 17. 后续决策点

1. Managed Web Gateway 是合入 Cloud Fleet，还是作为独立边缘服务部署。
2. Preview Target 是项目级预设，还是首期只保存在会话 metadata 中。
3. Build Artifact 使用现有 attachment 存储还是独立对象存储索引。
4. Provider 回调采用服务间签名 HTTP、队列，还是独立控制面 WebSocket。
5. 实时协作是否允许多人同时控制，还是默认单控制者、多观察者。
6. 录屏默认关闭还是按工作区策略开启。
