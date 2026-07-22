# USB Device Runtime 一期决策

> 状态：已确认
>
> 范围：未来一年

## 结论

Multica 的 Android 互动预览先只建设 USB Device Runtime。公共电脑作为受管 Runtime Host，通过 USB/ADB 连接少量公共测试手机；Multica Cloud 负责认证、会话、调度、信令和媒体转发。未来一年不建设云端 Android 模拟器池，也不以大规模真机农场为目标。

## 运行拓扑

```text
Developer browser
      |
      v
Multica Cloud control plane / media gateway
      ^
      | outbound TLS, WebRTC media and control
      v
Public computer device-agent
      |
      | USB / ADB
      v
Dedicated Android test device
```

Cloud 不直接连接物理手机。device-agent 主动建立出站连接，因此公共电脑不需要暴露入站端口，也不需要让云端进入公司内网。

## 会话行为

1. CI 或开发者构建不可变 APK artifact。
2. 成员在 issue 中创建 Android Preview Session，选择目标设备能力。
3. 调度器为会话锁定一台空闲设备，agent 下载已授权 artifact 并使用 ADB 安装、启动应用。
4. agent 将设备画面编码为 WebRTC 媒体流；浏览器的触控、键盘、旋转和会话控制反向发送给 agent。
5. Android 系统权限框、应用弹窗、相机预览和崩溃页面都属于同一段设备画面，不需要单独的“弹窗传输”协议。
6. 会话结束后 agent 停止应用、清理应用数据、收集 logcat 和截图证据，并释放设备。

当前一期落地使用可续期租约控制有限设备资源：每个 serial 同时只允许一个 Issue 持有 5 分钟写租约；Issue Viewer 打开和每分钟心跳、`preview device sync/run/snapshot` 都会续租。20 分钟无操作后 Session 进入可恢复的 `sleeping`，4 小时后 Session 到期；历史记录继续保留在 Issue 中但不占控制权。最后一个 WebRTC Viewer 离开 3 秒后 scrcpy producer 停止，WebView 调试连接空闲 20 分钟后关闭。Runtime 中的 emulator 是预热共享容量，不随单个 Issue Session 启停；USB 真机拔出时保留绑定 serial 并显示离线，重连同一 serial 后恢复，换机必须显式重新 `sync --serial`。

Issue Viewer 从当前 Device Runtime 每秒接收设备清单。只有 emulator 和 USB physical 同时可用时，标题栏才显示“模拟器 / USB 真机”分段控件；USB 插入、拔出或重连不会自动改变当前绑定。用户显式点击目标后，服务端先预占目标 serial 的租约，Runtime 再把当前 Session 的 artifact 部署到目标设备，部署成功后才发布新的 Session 绑定并切换画面；失败则回滚到原 serial，同一个 Issue 仍只保留一张活动 Preview 卡片。

部署在 Runtime 内以异步 job 执行，阶段为准备、安装、启动、连接和就绪。Issue 标题栏只显示当前阶段，不增加调试按钮；关闭 Preview 或切换目标会取消仍在进行的 job。同一 serial 已安装相同 SHA-256 artifact 且包仍存在时跳过 APK 安装，只执行启动和连接，因此重复打开或再次同步不会承担重装耗时。Runtime 每 10 秒采集一次 ADB 状态、电量、充电和温度；unauthorized/offline 设备不可选择，低电量和高温以 warning 展示并保留原因。

## 安全与运维边界

- 测试手机和公共电脑均为受管资产，不使用个人设备或个人账号。
- 设备按会话独占；其他成员可观察时不获得控制权，除非会话显式移交。
- artifact 通过短期签名下载地址获取，agent 校验摘要后安装；APK 不通过控制 WebSocket 传输。
- agent 只接受已签名且属于当前工作区/会话的控制命令。
- 受管 Host 必须设置 `MULTICA_DEVICE_RUNTIME_TOKEN`（或 `device serve --access-token`）；Issue 控制台另外签发绑定 Preview Session、4 小时过期的浏览器令牌。Runtime 再按 session ID 校验 5 分钟设备写租约，避免旧 iframe 或另一个 Issue 操作当前设备。
- 设备必须具备健康检查、电量/USB 状态上报、超时回收和应用数据清理。

## 非目标

- 不在未来一年建设云端 Android Emulator Provider。
- 不让云主机直接访问 USB 手机。
- 不承诺一开始支持多机型矩阵、自动化测试农场或第三方设备农场。
- 不把本机 Emulator 演示夹具视为一期产品能力。

## 一期验收

- 公共电脑可注册为 Device Runtime，并持续上报已连接 Android 设备。
- Issue 可显式选择在线模拟器或 USB 真机；设备插拔不自动切换，部署成功后同一 Preview Session 才更新绑定。
- agent 能安装指定 APK、启动应用、回传实时画面和 logcat。
- 浏览器可对会话发送触控和键盘输入，系统权限弹窗可见且可操作。
- 停止或超时后，设备恢复干净状态并回到可调度池。

## AdaKami 运行 Case

> 更新：AdaKami 业务页面由 Android/iOS WebView 加载 H5。开发预览不再使用
> `https://h5.adakami.my/` 作为代码变更验收目标；stage 壳通过启动参数加载当前
> task checkout 的 H5 开发服务。DOM 语义动作走 WebView 调试协议，权限、相机和
> 系统页面继续走原生设备适配器。完整契约见
> [`webview-semantic-testing.md`](./webview-semantic-testing.md)。

为验证 Android artifact 与 ADB 控制链路，已在预览专用 Android 15 Runtime 运行 `oversea/financing/adakami.my` 的 `stage` APK。

| 检查项 | 结果 |
| --- | --- |
| 源码构建 | `:app:assembleStage` 成功，生成 `app-stage.apk` |
| 安装 | `adb install -r -g` 成功 |
| 启动 | `com.adakami.my/.MainActivity` 成为当前 Activity |
| 实际页面 | 应用 WebView 打开 AdaKami MY 手机号/OTP 登录页 |
| 画面回传 | 通过 `adb screencap` 获取设备真实屏幕 |
| 控制回传 | 通过 `adb input tap` 与 `adb input text 123456789` 聚焦并填入手机号 |
| 运行日志 | 能读取 `com.adakami.my` 相关 logcat |

该 case 最初使用 `emulator-5554` 作为本机夹具，之后已在 USB 真机 `1283955535012434`（Infinix X6531B，Android 14）重复安装、启动、H5 反向端口映射、WebView 语义输入和 WebRTC 展示流程。真机的相机、系统权限和硬件差异仍需按一期验收标准逐项验证。

## 本地控制台

一期本地运行时由 CLI 提供，默认只监听 `127.0.0.1`，不会暴露 ADB 到局域网或公网：

```powershell
multica device serve `
  --adb C:\Android\Sdk\platform-tools\adb.exe `
  --access-token $env:MULTICA_DEVICE_RUNTIME_TOKEN `
  --artifact E:\artifacts\adakami-stage.apk `
  --package com.adakami.my `
  --activity com.adakami.my/.MainActivity `
  --scrcpy-server C:\tools\scrcpy-server-v3.3.4 `
  --scrcpy-version 3.3.4
```

打开 `http://127.0.0.1:18080` 后，控制台会发现所有 `adb devices -l` 返回的设备。`emulator-*` 会标记为演示夹具，其他 serial 均标记为 physical；两者均可执行安装、启动、截图、触控、文本输入、logcat 与清理，真机验收只选择 physical 设备。

### Mac 移动预览 Runtime 的特殊挂载

移动设备预览宿主不是普通 Agent Runtime。公共 Mac 必须在 daemon 启动时显式传入 `--device-runtime-url`，该参数是“挂载为移动预览宿主”的开关：

```bash
# 1. 先启动仅监听本机 loopback 的 Device Runtime。
multica device serve \
  --adb "$HOME/Library/Android/sdk/platform-tools/adb" \
  --access-token "$MULTICA_DEVICE_RUNTIME_TOKEN" \
  --artifact /opt/multica/artifacts/app-stage.apk \
  --package com.adakami.my \
  --activity com.adakami.my/.MainActivity \
  --scrcpy-server /opt/multica/scrcpy/scrcpy-server-v3.3.4 \
  --scrcpy-version 3.3.4 \
  --port 18081

# 2. 再把这台 Mac 注册为唯一的移动预览宿主。
multica daemon start \
  --device-name "Mac Mobile Preview Host" \
  --runtime-name "Mac Mobile Preview" \
  --device-runtime-url http://127.0.0.1:18081
```

用户不传标签名。只要 `--device-runtime-url` 有效，daemon 注册请求就自动携带固定 metadata：

```json
{
  "label": "mac-mobile-preview",
  "device_runtime_url": "http://127.0.0.1:18081"
}
```

固定标签在代码中定义，不提供通用 `--label` 配置，避免不同机器注册成相近但不相等的标签。未传 `--device-runtime-url` 的 daemon 是普通开发 Runtime，不会参与 Android/iOS Preview 选择。该 URL 必须是无凭据、无 query、无 fragment 的 loopback HTTP URL；Device Runtime 不应直接暴露到局域网或公网。

Device Runtime 与 daemon/CLI 必须共享 `MULTICA_DEVICE_RUNTIME_TOKEN`。该 Host 密钥只用于本机原生调用，不写入 Preview URL；Issue iframe 加载控制台时由 Runtime 生成短期浏览器令牌，并自动把当前 Session ID 带到设备 API。更换 Host 时重新生成密钥并重启两侧进程。

同一 daemon 可能为 Codex、Claude、Gemini 等 provider 生成多条 runtime row，它们共享同一个 `daemon_id`，Preview 解析时按 `daemon_id` 聚合为一台 Mac。一个 Workspace 同时只允许一台不同 `daemon_id` 的 `mac-mobile-preview` 宿主在线；零台时部署报“找不到宿主”，多台时部署拒绝随机选择。

挂载后的验证方式：

1. 在 Multica Runtimes 接口或页面确认 metadata 包含 `label=mac-mobile-preview` 和正确的 loopback URL。
2. 执行 `multica preview device sync --issue <issue> ...` 时不传 `--url`。
3. 命令应自动发现 18081、列出设备、部署 artifact，并创建带 `serial` 的 Preview Session URL。
4. Issue 中只应保留一张运行中的同平台设备 Preview；切换 serial/UDID 后旧 Session 自动停止。

Mac 关机或断网后，其 Runtime 会在心跳超时后离线，新部署停止选择该宿主；已有 Issue 和历史 Session 不丢失。替换 Mac 时，在旧 Mac 离线后用相同启动方式注册新 Mac。替换手机时不需要重新注册 Mac；Android 使用新 serial，后续 iOS 使用新 UDID，下一次 sync 会替换 Issue 的活动设备绑定。

当前 `multica device serve` 已实现 Android ADB/scrcpy。`mac-mobile-preview` 标签和宿主发现契约按 Android/iOS 共用设计，但 iOS Simulator、USB 真机、xcodebuild 安装启动和视频控制适配仍属于后续实现；文档中的标签不代表 iOS adapter 已完成。

### 当前传输实现与体验边界

当前 Runtime 使用与版本严格匹配的 scrcpy Server 在 Android 设备端编码原始 H.264。Host 为每个 Android serial 维持一个共享 scrcpy producer，通过独立 ADB forward socket 读取带 PTS、关键帧和 codec config 元数据的视频包，再分发到多个 Pion WebRTC H.264 subscriber；每个浏览器使用独立 `RTCPeerConnection` 和 UDP socket。新观察者会从缓存的最近 GOP 开始播放，浏览器刷新只替换 subscriber，不会重启设备侧编码器。订阅端按 `50ms` 的时长上限保留视频队列；消费者变慢时丢弃旧 GOP 并等待新的关键帧，不回放数秒前的画面。设备端限制 60fps，并要求编码器在静态页面上持续提交前一帧，降低稀疏 H5 更新被编码器积压的概率。最后一个观察者离开 3 秒后，producer 才停止并清理 ADB forward。

控制由浏览器 `POST` 到 Runtime。运行中的 scrcpy producer 会建立第二条常驻 control socket，`tap`、`swipe`、`text`、`keyevent` 和旋转优先使用 scrcpy 二进制控制协议，不再为每次点击执行 `DisplaySize` 和 `adb shell input`。浏览器提交触点时携带 WebRTC 源视频分辨率，control socket 直接以该尺寸注入触点；连接断开时 Runtime 自动回退到带 30 秒屏幕尺寸缓存的 ADB 路径。嵌入模式把触控与文本/按键拆成独立队列，避免文字批量发送阻塞点击。系统键盘、权限框、应用弹窗和崩溃页面都属于同一条 H.264 画面。

AdaKami 本机 Emulator 验收得到 `576x1280` 视频；Issue 内已验证手机号输入、条款页跳转、刷新和反馈。正式 `18081` Runtime 的 30 次无业务 tap 请求测得控制响应 P50 `2.8ms`、P95 `128.1ms`；20/20 次快速 WebRTC 重连、5 次跨 idle grace 的完整停止/重启及双观察者退出保活均通过，最终 session、subscriber 和 ADB forward 均清零。当前无窗口 Emulator 仍不代表真机画面性能：本机尝试 `-gpu host` 虽启用了 Intel Iris 硬件加速，但出现 System UI 无响应且渲染 P50 恶化到 `65ms`，已回退至稳定模式。一期公共电脑接入实体手机后，应以目标机型验证持续 24-30fps、click-to-photon 输入延迟和长会话稳定性。

### 控制台布局

独立控制台采用开发工具式三栏工作台：左侧是 Android 系统动作，中间是有最大尺寸约束的设备画布，右侧是设备、部署、文本输入和 logcat。Issue 通过 `?embed=1` 加载时，Runtime 隐藏全部调试栏，只保留可点击的视频画面；Issue viewer 只提供刷新、全屏、反馈和关闭。设备画布根据实际视频比例居中渲染，不拉伸，也不会随浏览器无限放大。该结构参考了 Xcode Device Hub 的设备画布与检查器，以及 AWS Device Farm Remote Access 的会话动作与日志工作流。

## 触发方式

一期只识别一个固定 Runtime 标签：`mac-mobile-preview`。公共 Mac 同时承载 Android ADB/scrcpy 与后续 iOS Simulator/USB 真机适配；daemon 使用 `--device-runtime-url http://127.0.0.1:18081` 注册时，把该标签和本机 Device Runtime URL 写入 metadata。Android/iOS Preview 先按标签找到唯一在线 Mac，再按平台和 serial/UDID 选择设备。零台或多台同标签 Runtime 在线时直接失败，不做隐式随机选择。

日常开发不直接访问控制台 URL。Agent 修改 Android 项目并完成仓库规定的 APK 构建后，在可运行检查点同步到模拟器：

```powershell
multica preview device sync `
  --issue DRT-1 `
  --artifact (Resolve-Path app/build/outputs/apk/stage/app-stage.apk) `
  --scenario examples/preview-scenarios/adakami-login.json
```

`sync` 未传 `--url` 时解析唯一在线的 `mac-mobile-preview` Runtime。未传 `--serial` 时优先继承 Issue 最新运行中 Android Preview 已绑定的设备；Issue 尚未绑定设备时才默认选择在线 emulator。显式传 `--serial` 可切换设备，目标 Session 成功运行后，CLI 自动停止同 Issue、同 Runtime 的其他 Android Session，保证智能体控制目标和用户画面不会分叉。Agent 必须传当前 checkout 生成的绝对 APK 路径，不能依赖 Runtime 启动时的默认 artifact，否则独立工作目录可能重新安装旧包。

当 `--web-url` 使用 `localhost` 或 loopback IP 时，Device Runtime 会自动为目标 serial 建立同端口的 `adb reverse`，USB 真机不需要人工配置 H5 隧道。Preview Session URL 保存 `serial` 查询参数，Issue iframe 再追加 `embed=1`，因此模拟器和多台真机同时在线时仍固定展示本次 `sync` 部署的设备，而不依赖 `adb devices` 返回顺序。

服务端拒绝不带 serial 或携带多个 serial 的 `local_device` Session。`preview device create` 只用于已经明确设备的诊断场景，日常部署必须使用 `preview device sync`。绑定设备离线后，viewer 清除旧视频并显示断开状态，不会选择设备列表中的 emulator 或其他真机；Runtime 每秒检查一次设备列表，同一 serial 恢复后重新建立 WebRTC。Issue Preview 卡片和弹窗标题栏直接显示绑定 serial，用户无需从智能体评论判断当前设备。

当 Runtime 同时发现 emulator 和 USB physical 时，Issue Viewer 标题栏显示设备来源分段控件。它不把“发现真机”解释成“切换真机”；只有成员点击后才调用 Session 设备切换接口。切换采用“目标租约预占 -> Runtime 部署当前 artifact -> Session 确认发布”的两阶段顺序，防止页面先切到未安装应用的设备。目标租约冲突、设备离线或部署失败时保留原画面。

代码保存和 Git commit 本身不会触发部署。触发点是 Agent 完成一次成功构建、确认应用可启动后的“可运行检查点”，避免未完成代码频繁重装。若提供 `--scenario`，部署后立即按 JSON 中的 `tap`、`swipe`、`text`、`key` 和 `wait` 步骤操作同一台设备。用户在 Issue 中 @ 智能体并要求点击、输入、登录、跳页或执行测试 case 时，Agent 使用 `multica preview device run --issue <issue> --scenario <scenario.json>` 立即操作，不重新安装未变化的 APK。`run` 只能从该 Issue 唯一运行中的 Android Preview 解析 Runtime 和 serial；缺少绑定、设备离线或存在多个活动设备时会失败，不允许回退到 `emulator-5554` 等默认设备。为了让操作可观察，演示场景在首步前短暂停顿，并在可见动作之间保留约 300-500ms。

首版场景使用带源分辨率的坐标动作，Runtime 会缩放到实际屏幕。这能先跑通“改代码 -> 构建 -> 部署 -> 用户实时观看 Agent 操作 -> Issue 反馈”的自动化闭环，但不等同于 Appium/UiAutomator2 的语义控件定位。发标页等布局变化频繁的长期回归场景，后续应增加 `resource-id`、文本和 accessibility selector 驱动，并在找不到元素时把失败状态和截图回写 Issue。

### 自动化闭环边界

一期本机形态的控制面与媒体面是两条链路：CLI 通过 Runtime HTTP API 部署和执行动作，Issue viewer 通过 WebRTC 订阅同一台设备的 H.264 producer。Agent 与用户因此共享设备状态，用户可以在 Agent 操作过程中接管点击，反馈后 Agent 再执行新场景。当前不会在多个 Agent 之间调度或锁定模拟器；同一公共电脑只运行一个开发闭环时无需引入设备农场调度。

### 延迟预算与 20ms 目标

延迟必须分成“控制命令完成”和“浏览器看到新像素”两个指标。USB 真机用 25 次 H5 输入变更、100ms 至 3s 空闲间隔复测后，WebView 控制响应为 P50 `31ms`、P95 `34ms`；浏览器 click-to-photon 为 P50 `150ms`、P95 `349ms`。期间 RTP 丢包和解码丢帧均为 0，浏览器 jitter buffer 平均 `27ms`。降低分辨率、强制 MTK C2/OMX 编码器、全关键帧和 CBR 均未产生稳定收益，已回退到实测最优组合。

因此，P50 `20ms` 不能作为 USB 真机视频链路的承诺：60Hz 显示器单帧已经占 `16.7ms`，手机合成、MediaCodec 硬编、USB、WebRTC 和浏览器解码仍在其后。P50 `20ms` 应用于 Web/H5 直预览的本地交互路径；Issue 涉及 H5 页面时直接嵌入该任务的 H5 dev server，只有权限、相机、推送、原生导航和壳集成等场景才切换到 Android/iOS 设备流。两条路径使用同一 Preview Session 契约，但分别记录 Web 交互延迟和 Device click-to-photon，不能用 HTTP 200 耗时替代用户可见延迟。

## 优化清单与当前结果

### 已完成（一期 P0）

- 部署 job 上报 queued/preparing/installing/starting/connecting/ready，支持关闭 Viewer 时取消；ADB 子进程被取消后最终状态为 `canceled`，不误报 `failed`。
- Runtime 计算 APK SHA-256；同一 serial 已安装相同 artifact 时跳过安装。2026-07-17 在线复测返回 `install_skipped=true`。
- Issue 浏览器使用绑定 Session 的 4 小时短期令牌，Host/CLI 使用 `MULTICA_DEVICE_RUNTIME_TOKEN`；跨 Session 读取 job 返回 404，控制设备返回 409。
- Runtime 维护 5 分钟 serial 写租约；设备切换失败回滚旧租约，CLI `run/snapshot` 始终携带当前 Issue Session ID。
- 设备清单上报 ADB 授权状态、电量、充电和温度；未授权、离线和不健康设备在切换前禁用或提示原因。
- Preview 弹窗保留 iframe/WebRTC 连接并取消开合动画。10 次关闭重开复测 P50 `80ms`、P95 `101ms`，连接 ID 和视频时间轴保持连续。
- `pagehide` 使用带令牌的 keepalive fetch 关闭 WebRTC Session。5 次完整启动/关闭均得到 `starts=stops`，最终 session、subscriber 和 producer 为 0。
- Issue 中完成 emulator -> USB -> emulator -> USB 切换，6 次两阶段 Session 更新均为 200，Session ID 不变，两端都获得非黑首帧。
- USB 真机连续 30 次输入复测：控制响应 P50 `2ms`、P95 `3ms`；像素变化 P50 `333ms`、P95 `387ms`、最大 `735ms`，未出现 2-3 秒停顿。

### 后续优化（一期 P1）

- 将已安装 artifact hash 持久化到 Host，Runtime 重启后仍可跳过相同 APK；当前内存缓存会在进程重启后丢失。
- 把首帧、控制响应、click-to-photon、丢包、解码丢帧和 producer 启停计数写入统一遥测，按 serial/机型/编码器形成趋势和告警。
- 增加 USB 拔插、ADB unauthorized/offline、Host 睡眠/唤醒和 8 小时长会话压力测试；重连仍必须保持原 serial，不自动切换 emulator。
- 对不同 SoC 的 MediaCodec 参数做按机型 profile；当前 Infinix/MTK 在静态 H5 小区域变化时仍有约 300ms 编码可见延迟。
- 为自动化结果生成截图、步骤、断言和失败原因证据，并直接回写 Issue，形成可重复点击的验收 case。

### 后续生态能力

- H5/纯 Web 改动默认走 Issue 内直接 Web Preview，原生权限、相机、推送、桥接和壳导航才使用 Device Runtime。
- 增加 iOS Simulator/USB iPhone adapter，复用 `mac-mobile-preview` Host 标签、Preview Session、租约和证据协议。
- 云端 emulator pool、TURN/媒体网关和多设备并发调度继续作为后续 Provider，不进入未来一年少量公共设备的一期范围。
