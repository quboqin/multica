# 创意协作平台改造记录（2026-07-31 至 2026-08-01）

## 1. 文档目的

本文记录 2026-07-31 至 2026-08-01 完成的创意协作平台改造、真实验收流程、平台资源、技术入口、清理项和未完成事项。

本轮目标不是实现一套写死的 AdaKami 修图脚本，而是把素材采集、选图、文案管理、市场约束、智能体委派、图片生成、Prime 包装、质量检查和结果交付沉淀为用户可见、可编辑、可复用的平台能力。

相关设计文档：

- [Issue 原生创意工作流](./issue-native-creative-workflow.md)
- [Issue 原生图片工作流交接](./issue-native-image-workflow-handover.md)
- [创意素材工作流使用指南](./creative-material-workflow-guide.md)

## 2. 最终产品模型

### 2.1 用户在父 Issue 里看到什么

父 Issue 是一次素材采集和批量创作任务的业务工作台，外层只保留：

- 原生素材候选池：展示抓取结果、选择参考图、填写或选择文案、发起修图。
- 原生结果看板：按创意和尺寸聚合最终图片，支持预览、下载和继续调整。
- Leader 认为必须让业务用户知道的进度、异常和交付结论。

图片模型提示词、工具命令、参数 JSON、尝试记录和智能体之间的对话不堆在父 Issue 中。

### 2.2 Leader 和子 Issue 如何协作

- Autopilot 按计划创建父 Issue。
- 素材小队 Leader 读取父 Issue、小队名册、成员职责和成员绑定的普通平台 Skill。
- Leader 委派素材采集智能体，采集过程进入采集子 Issue，真实结果写入父 Issue 候选池。
- 用户选择 N 张素材后，平台创建 N 个互相隔离的创意工作 Issue。
- 每张图的创意工作 Issue 根据实际需要创建文案、生成、Prime 包装、QC 或返修子 Issue。
- 子 Issue 按能力和职责命名，不使用“第一阶段、第二阶段”一类固定节点。
- Leader 负责委派、判断、汇总和发布；专业智能体只处理自己的职责。
- 智能体完成不等于流程完成。Leader 必须检查产物、QC 结论和未解决问题。
- 最终图片只能发布到创建本次工作的快照父 Issue，防止结果进入错误看板。
- 父 Issue 不因一次批量生成自动关闭，用户可以继续选图、改文案和发起新修订。

### 2.3 多图交互原则

- 候选池支持多选，但每张参考图保持独立创意分支。
- 文案可批量套用，也可逐图覆盖。
- 每张图分别保存文案版本、创意要求、尺寸结果、QC 和返修历史。
- 单张失败不会阻断其他图片交付。
- 结果看板按稳定文件名前缀聚合，同一创意显示三个尺寸；新修订覆盖该尺寸的默认展示，历史附件仍可追溯。

## 3. 已落地的平台能力

### 3.1 创意工厂资源页

Web 和 Desktop 均增加“创意工厂”入口。工作区用户可在页面维护以下资源：

| 资源 | 作用 | 当前能力 |
| --- | --- | --- |
| 素材库 | 保存候选素材和复用素材 | 浏览、归档、作为工作流输入 |
| 文案库 | 管理可复用文案 | Excel 导入、在线编辑、审批、版本管理、候选池选择 |
| 市场资源包 | 描述品牌、市场、尺寸、模板、二维码和交付约束 | 页面编辑配置、上传附件、版本发布、工作流快照 |

Leader 协作方法使用平台原生 Skill，绑定到 Leader 智能体。平台不再维护专用编排资源、任务模板
或展开方式。市场资源包是平台资源，Skill 是智能体使用资源的方法。品牌或市场数据不写死在
代码里；未来增加其他市场时，用户在页面新建或复制资源包、上传对应模板和规则，再由同一套
通用 Skill 读取。

### 3.2 文案库

- 已导入原 Excel：`NEW SCRIPT DESIGN CONTENT - 2026.xlsx`。
- 已解析 423 条文案。
- 支持在线维护标题、利益点和审批状态。
- 工作流使用具体文案版本快照，之后修改文案库不会悄悄改变运行中的任务。
- 候选池可以推荐和选择文案；选择多张图时可以先批量设置，再逐图精确修改。

### 3.3 市场资源包

资源包承载可由业务维护的市场事实，包括：

- 品牌和市场标识。
- 输出尺寸和文件命名规则。
- 三个 Prime 完整贴图模板及其尺寸角色。
- 品牌安全区、二维码区域、页脚和商店标识约束。
- 二维码目标地址、批准状态和允许域名。
- 文案语言、合规要求和 QC 标准。
- 提供给智能体的附件和参考材料。

运行时使用已发布版本快照。草稿修改不会影响正在执行的 Issue；发布后新任务使用新版本。

### 3.4 候选池和结果看板

候选池与结果看板沿用并增强原有原生控件，而不是生成单独 HTML 看板。

候选池支持：

- 展示 AppGrowing 导入的真实素材。
- 按卡片选择单张或多张参考图。
- 查看素材来源和关键元数据。
- 选择文案库条目并逐图覆盖。
- 填写可选创意方向后发起修图。

结果看板支持：

- 按稳定文件名前缀聚合创意。
- 点击创意查看三个尺寸。
- 保留原始文件名。
- 单尺寸下载和三个尺寸打包下载。
- 从具体创意或尺寸发起调整。
- 同尺寸存在多个修订时默认展示最新版本。

标准文件名为：

```text
{month}_{brand}_{market}_{candidate}_{size}_v{revision}.png
```

标准尺寸为：

- `1080x1080`
- `1200x628`
- `800x1000`

## 4. AppGrowing 素材采集

### 4.1 串联方式

当前合理链路为：

```text
Autopilot 定时创建父 Issue
  -> Leader 创建并委派采集子 Issue
  -> AppGrowing 智能体理解自然语言条件
  -> 使用平台“设置 - 集成”中的已授权登录态执行真实查询
  -> 结果写入父 Issue 原生候选池
  -> 用户选图和文案
  -> Leader 为每张图建立独立创意工作 Issue
```

凭证只由平台集成和凭证代理提供，不进入 Autopilot 提示词、Issue 评论、Skill 文本或本文档。

### 4.2 查询和分页修正

采集智能体将自然语言条件转换为结构化查询，使用当前 Issue 的真实 ID 导入候选池。已处理以下不稳定点：

- 当前采集契约支持 Indonesia/印尼/印度尼西亚/ID 等中英文混写；其他市场由对应市场包扩展。
- `selection_rules` 使用包含 `new_materials` 和 `volume_materials` 的对象结构。
- 媒体枚举不确定时不猜测下推。
- 优先竞品至少比较 5 页，其他竞品至少比较 3 页，避免只看第一页造成“时好时坏”。
- 逐一检查 Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO。
- API 返回空或某一次页面失败不直接等同于授权失效。
- 没有命中时如实返回失败阶段和可操作原因，不使用测试图补位。

真实验收 ADC-39 共导入 25 条候选素材，七个竞品均获得原始查询结果。最终选择：

- Easycash：`e12968bb-af24-4a6f-aa17-05d6bec59ab2`
- Adapundi：`06c7762d-61db-43b2-b59c-763f254b1ecf`

采集子 Issue ADC-40 已完成。

## 5. Prime 完整贴图和图片生成

### 5.1 模板来源

模板来源目录：

```text
C:\Users\zhangzhenyu\ad-creative-factory\refference\Prime Template - PNG file
```

三个完整贴图：

| 角色 | 文件 | 输出尺寸 |
| --- | --- | --- |
| `prime_square` | `11-01.png` | 1080x1080 |
| `prime_landscape` | `191-01.png` | 1200x628 |
| `prime_portrait` | `45-01.png` | 800x1000 |

当前链路采用完整 Prime 叠加，不再只贴二维码局部。图片生成智能体负责安全区内的主视觉和文案，不生成品牌 Logo、二维码、二维码边框、页脚或商店标识；Prime 包装智能体使用已发布模板统一叠加这些合规元素。

### 5.2 合成脚本

平台 Skill 的参考实现：

```text
scripts/creative-platform-skills/ad-creative-prime-compose/references/image_prime_compose.py
```

当前实现：

- 清理模板内二维码槽位后重新渲染资源包批准的二维码内容。
- 保留完整 Prime 视觉和安全区。
- 对三个输出尺寸分别定位二维码槽位。
- 使用 OpenCV 多路径解码校验：全图原尺寸、全图 2 倍最近邻、二维码槽位裁剪。
- 已修复旧模板二维码残影、底部白色色块和内容碰撞问题。

当前二维码槽位：

| 尺寸 | 二维码坐标 | 清理区域 |
| --- | --- | --- |
| 1080x1080 | `(1002, 22)` | `(983, 29, 1053, 99)` |
| 1200x628 | `(1128, 13)` | `(1133, 20, 1185, 73)` |
| 800x1000 | `(727, 20)` | `(724, 22, 779, 76)` |

## 6. Skill 和智能体职责

本轮把旧 MCP 中可复用的说明、参考文件和操作约束迁移为平台 Skill 及其 references。主要职责为：

- 素材采集 Skill：解释自然语言筛选、调用 AppGrowing、写入候选池。
- 创意规划 Skill：读取参考图、文案版本和市场包，生成每个尺寸的明确创作指令。
- 图片生成 Skill：只生成主视觉和指定文案，避开 Prime 安全区，不生成二维码和固定合规元素。
- Prime 包装 Skill：读取市场包模板、覆盖完整 Prime、渲染批准二维码并按标准命名。
- QC Skill：检查尺寸、文案、遮挡、白边、品牌元素、二维码机器可读性和资源快照一致性。
- Leader 普通平台 Skill：读取小队成员能力，按图隔离工作，处理返修并发布到正确父 Issue。

Leader 编排增加了以下约束：

- 子 Issue 标记 `done` 但评论仍说明存在遮挡、白边、人物、文案或二维码问题时，不能视为通过。
- 不能通过“重新打包”绕过图片内容返修。
- 发布前必须校验目标父 Issue ID 和三尺寸文件名。
- 调整子 Issue 完成后，上层创意 Issue 只负责复核和关闭，不重复上传结果。
- 父批次 Issue 保持开放，供用户继续操作。

## 7. ADC-39 完整验收现场

父 Issue：

- 标识：ADC-39
- ID：`213dca52-6347-4543-831e-2a62b4fbd712`
- 页面：`http://localhost:3000/ad-creative-direct-pilot/issues/213dca52-6347-4543-831e-2a62b4fbd712`
- 状态：`todo`，按设计保留开放。

关键子 Issue：

| Issue | 职责 | ID | 状态 |
| --- | --- | --- | --- |
| ADC-40 | AppGrowing 真实采集 | `b1fdd...` | done |
| ADC-41 | Easycash 独立创意工作 | `e575d1e6-f441-48e9-b1e6-161207c4a308` | done |
| ADC-42 | Adapundi 独立创意工作 | `e47bb058-5bb1-4f4d-a4f3-fc27106463ea` | done |
| ADC-60 | Adapundi QC | 已创建 | done，二维码配置问题见第 11 节 |
| ADC-68 | Easycash 最终 QC | `cb4f5d8d-e151-4f93-aa92-84386a2e0683` | done |

### 7.1 选用文案

Easycash 使用文案版本 v5：

- 文案 ID：`094d5d85-f5f5-4abf-a3df-44498fdbd97e`
- 标题：`Lebih Ringan, Mulai dari 0,01%*`
- 利益点：`Proses simple`、`Tenor 3-12 Bulan`、`Jumlah Pinjaman Rp20.000.000`、`Tenor 12 BULAN`、`Total Bunga Rp720.000`

Adapundi 使用文案版本 v5：

- 文案 ID：`f16fd9bb-9511-40c9-b999-d38575ab2af9`
- 标题：`Pinjaman Fleksibel Tanpa Ribet`
- 利益点：`Limit hingga Rp80.000.000`、`Pilihan Tenor`、`3-12 Bulan`、`Bunga mulai dari`、`0,03%*`

### 7.2 已发布结果

父 Issue 的结果看板默认展示 6 张结果，解析为 2 个创意组，每组 3 个尺寸。Issue 时间线同时保留历史附件。

Adapundi：

```text
AdaKami_ID_06c7762d_1080x1080_v1.png
AdaKami_ID_06c7762d_1200x628_v1.png
AdaKami_ID_06c7762d_800x1000_v1.png
```

Easycash：

```text
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_800x1000_v1.png
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1080x1080_v2.png
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1200x628_v4.png
```

Easycash 在真实协作链路中经历了多次视觉返修，最终 ADC-68 对三个尺寸全部通过。关闭全部子任务后再次唤醒 Leader，返回 `no_action`，没有重复生成或重复上传。

2026-08-01 二维码修复后，平台保留上述历史附件，并新增以下最新修订：

```text
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_800x1000_v2.png
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1080x1080_v3.png
January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1200x628_v5.png
AdaKami_ID_06c7762d_800x1000_v2.png
AdaKami_ID_06c7762d_1080x1080_v2.png
AdaKami_ID_06c7762d_1200x628_v2.png
```

修复协作现场：

| Issue | 职责 | ID | 状态 |
| --- | --- | --- | --- |
| ADC-69 | Easycash QR 标准地址修订，Leader 编排 | `4858939b-b0a7-4aa7-90e4-75e94b0b7348` | done |
| ADC-70 | Adapundi QR 标准地址修订，Leader 编排 | `2693ce39-c9b8-48f9-adc8-3058f7cee322` | done |
| ADC-71/72/73 | Easycash 三尺寸 Prime 包装 | 逐尺寸子 Issue | done |
| ADC-74/75/76 | Adapundi 三尺寸 Prime 包装 | 逐尺寸子 Issue | done |
| ADC-77 | Easycash 独立 QC | `a9891e63-b7ca-4156-b010-7032d1ad77eb` | done / QC PASS |
| ADC-78 | Adapundi 独立 QC | `2820cb17-69c0-44ac-9ea5-db3b5fed03af` | done / QC PASS |

父 Issue 现有 12 个历史加最新 PNG 附件。结果看板按创意、尺寸和修订号选择每个尺寸的最高版本，因此仍显示 2 个创意、每个创意 3 个最新尺寸；历史版本只保留在 Issue 时间线中。

## 8. 当前平台资源实例

工作区：

- ID：`cf7160d6-8bc3-4d04-8226-6d771bd94ae7`

资源：

| 类型 | 名称/用途 | ID | 当前状态 |
| --- | --- | --- | --- |
| 文案库 | AdaKami Indonesia 文案库 | `fbfe4c32-bdbd-45c2-abad-84c767c06cb0` | 已发布 v14，423 条 |
| 市场包 | AdaKami Indonesia 市场资源包 | `518d2f40-fac3-4dc2-8e18-d78fc2a0f8c4` | 已发布 v15，服务端 QR 校验通过 |
| 市场包附件 | 当前 Prime Compose 附件 | `019fba39-9c02-7a88-a2e0-94a12353326f` | 已上传 |
| 市场包文件记录 | Prime Compose 文件 | `1bbf1b55-4c51-4985-99a3-b829c428116a` | 已关联 |
| Skill | Prime Compose | `037c7843-2210-46a6-ac64-d706144777b4` | 已发布 |
| 小队 | 创意素材小队 | `4ff69424-c0cd-4cb2-894b-32bc17132739` | 已配置 |
| Skill | Leader 编排 | `8a9dffda-9927-4817-882a-fc8994f16f3f` | 已配置 |
| Agent | 小队 Leader | `b3460435-3bec-4921-8693-94a5e2e0d59c` | 已配置 |
| Autopilot | 周期素材采集 | `25a34039-bf06-4a82-9d22-7a94ad8b950c` | 已配置 |

## 9. 已清理的旧负债

本轮删除了已被 Issue 原生工作流替代的运行时实现：

- 旧 `creative_edit_job` provider、resolver、mock、reconciler、反馈和下载接口。
- 旧创意编辑任务 UI 和 QC/反馈组件。
- Workspace MCP 连接配置、Agent 工作区 MCP 引用、设置页和类型。
- 旧固定结果导入脚本。
- 旧 MCP 需求文档和已被新设计替代的工作流文档。

数据库迁移：

- `247_retire_legacy_creative_edit_jobs`：删除旧创意编辑任务相关表。
- `248_retire_workspace_and_agent_mcp`：删除工作区 MCP 连接。
- `249_creative_platform_resources`：新增平台创意资源结构。

历史迁移文件保留是数据库迁移链的必要组成，不代表运行时仍依赖旧能力。当前运行时代码中不应再存在旧 `creative_edit_job` 或 Workspace/Agent MCP 入口。

## 10. 服务与开发环境稳定性

### 10.1 图片 daemon

- profile：`direct-image2`
- 凭证使用本机加密配置挂载，不写入仓库、Skill、Issue 或日志说明。
- 启动脚本已处理 `SecureString`/`NetworkCredential` 生命周期错误。

### 10.2 Web reconnect 黑屏

针对 YIZHI 持续 `reconnecting` 和黑屏，已完成：

- `scripts/dev-web.mjs` 默认不执行隐式 warmup，仅在 `MULTICA_WEB_WARMUP=true` 时开启。
- `.env.example` 默认关闭 warmup。
- Next on-demand warmup 只在显式配置时启用。
- 开发服务器切换到 Turbopack。
- 清理损坏的 `.next` 缓存并从离线依赖恢复 `shiki`。

最终连续请求返回 HTTP 200，响应约 195-353ms，日志无新的应用错误。

当前服务：

- Web：`http://localhost:3000`
- API：`http://localhost:8080`
- daemon：端口 `19516`

## 11. 已发现并完成修复的问题

### 11.1 二维码配置错误

用户在 Adapundi 结果中发现二维码内容错误。排查结论：

- 图片生成智能体的基础图没有二维码，生成提示词已经明确禁止生成二维码。
- 错误发生在 Prime 包装阶段，不是图片模型自行生成。
- 三个原始 Prime 模板机器解码结果均为：

```text
https://www.adakami.id/termsandconditions
```

- 旧 main 环境 MCP profile 使用的也是该地址。
- 当前市场包 v14 错误配置为另一个活动地址。
- Prime 包装按错误配置生成，QC 又只与同一份错误配置比较，形成了自证闭环，因此 ADC-60 被错误判定为通过。

### 11.2 已实施修复

二维码不能只靠自由文本提示词保证。本轮已增加以下平台硬约束：

- 市场包页面明确维护目标地址、模板标准地址、允许域名和业务批准状态。
- 发布市场包时由服务端真实读取三个 Prime 模板并机器解码二维码。
- 三个模板缺一不可，解码值必须一致。
- 目标地址必须使用 HTTPS、命中允许域名并与模板标准地址完全一致。
- 校验结果由服务端写入发布快照，不能由客户端伪造。
- 市场包附件或二维码字段变化后，旧校验立即失效，必须重新发布校验。
- QC 同时比较成品、批准快照和模板解码证据，消除同源错误自证。
- 修复配置后只重新执行 Prime 包装和 QC，不重新消耗图片生成模型。

实现结果：

- 新增 Go 二维码策略、真实模板解码器和像素级测试。
- 三个实际 Prime 源模板均被新解码器识别为标准条款地址。
- 旧 v14 在新发布接口下被拒绝，错误信息明确指出缺少批准状态。
- 创意工厂页面可维护目标地址、模板标准地址、允许域名、批准状态和批准说明，并显示三个模板的服务端解码结果。
- AdaKami Indonesia 市场包已发布 v15，`qr_validation.status=passed`，三个模板解码值一致。
- API 已切换到 `creative-platform-v5` 镜像，旁路和正式健康检查均通过。
- ADC-39 六张图只重新执行 Prime 包装和 QC，没有重新调用图片模型。
- 六张最新成品均由 OpenCV 多路径解码为 `https://www.adakami.id/termsandconditions`。
- Prime 包装 Skill 明确要求上传证据后将包装 Issue 置为 `done`，独立 QC 由 Leader 创建，避免停在 `in_review` 造成编排不唤醒。
- Prime 和 QC 智能体的并发从 1 调整为 3；bootstrap 更新已有智能体时也会持续写入该配置。

## 12. 主要代码入口

前端：

- `packages/views/creative/components/creative-studio-page.tsx`
- `packages/views/issues/components/creative-material-pool.tsx`
- `apps/web/app/[workspaceSlug]/(dashboard)/creative/`
- `apps/desktop/src/renderer/src/routes.tsx`

Core：

- `packages/core/types/creative.ts`
- `packages/core/api/client.ts`
- `packages/core/api/schemas.ts`
- `packages/core/creative/queries.ts`

服务端和数据库：

- `server/internal/handler/creative_platform.go`
- `server/internal/handler/creative_market_pack_qr.go`
- `server/internal/handler/creative_market_pack_qr_test.go`
- `server/internal/handler/creative_material.go`
- `server/cmd/multica/cmd_creative.go`
- `server/cmd/multica/cmd_crawl.go`
- `server/migrations/247_retire_legacy_creative_edit_jobs.*.sql`
- `server/migrations/248_retire_workspace_and_agent_mcp.*.sql`
- `server/migrations/249_creative_platform_resources.*.sql`

采集和 Skill：

- `services/crawler-worker/src/index.mjs`
- `services/crawler-worker/src/appgrowing-material.test.mjs`
- `scripts/creative-platform-skills/`
- `scripts/bootstrap-creative-platform-demo.ps1`
- `scripts/start-direct-image-daemon.ps1`

## 13. 已执行验证

- crawler worker：27/27 测试通过。
- Go creative downloader 测试通过。
- API Docker 构建和编译通过。
- CLI 已构建和安装，版本 `creative-cli-v1`。
- 结果文件名解析测试：3/3 通过。
- Prime Compose Python 编译和三个尺寸重组测试通过。
- QR 策略、拒绝规则、像素二维码和三个实际 Prime 模板解码测试通过。
- bootstrap PowerShell 语法解析通过。
- `packages/views` TypeScript 类型检查通过。
- 结果看板最高修订选择测试 4/4 通过。
- `git diff --check` 无格式错误，仅有既有 CRLF 提示。
- ADC-39 真实跑通采集、选图、逐图协作、三尺寸生成、返修、QC 和结果看板发布。

完整 monorepo typecheck 曾长时间无输出并产生孤儿进程，已终止，改用受影响模块的定向测试和构建验证。二维码硬校验完成后仍需执行完整 `make check`。

浏览器控制器当时没有可复用的活动会话，因此服务稳定性使用 HTTP 连续请求和日志验证，未记为自动化浏览器截图验收。

## 14. 2026-07-31 与 2026-08-01 时间线

### 2026-07-31

- 将方案从单次修图脚本收敛为 Issue 原生、AI 原生的平台工作流。
- 确定父 Issue 只展示候选池、结果看板和必要 Leader 信息。
- 建立素材库、文案库和市场包三类创意工厂资源；协作方法复用平台原生 Skill。
- 把旧 MCP 材料迁移为平台 Skill/references。
- 增加创意工厂页面和资源维护能力。
- 删除旧 `creative_edit_job` 和 Workspace/Agent MCP 运行时能力。
- 修正 AppGrowing 多竞品、多页采集行为。
- 创建并真实运行 ADC-39，导入 25 条候选素材。
- 选择 Easycash 和 Adapundi，完成两个独立创意分支。
- 完成三尺寸图片生成、Prime 完整贴图、返修、QC 和结果看板发布。

### 2026-08-01

- 修复 Web 持续 reconnect/黑屏和开发缓存问题。
- 复核服务、daemon 和 ADC-39 最终状态，消除重复唤醒和重复上传。
- 根据用户截图定位 Adapundi 二维码错误。
- 对比原 Prime 模板、旧 main MCP profile、生成基础图、市场包 v14、包装结果和 QC 证据。
- 确认根因是市场包配置错误和 QC 同源自证，不是图片模型提示词遗漏。
- 开始增加发布时的真实模板二维码硬校验。
- 形成本文档，作为后续平台迭代和验收基线。
- 发布市场包 v15，并由服务端真实解码三个 Prime 模板后写入校验快照。
- 创建 ADC-69 至 ADC-78，完成两个创意、六个尺寸的 Prime 重包装、独立 QC 和父看板发布。
- 修正多图并发配置和包装 Issue 状态契约。
- 修正结果看板同尺寸历史版本重复展示，只呈现最高修订。

## 15. 最终验收状态

二维码修复已达到以下验收条件：

- 二维码策略单元测试通过。
- 服务端发布校验真实解码三个 Prime 模板。
- 创意工厂页面可维护并查看二维码批准和校验状态。
- AdaKami Indonesia Market Pack 发布 v15，地址恢复为模板标准地址。
- ADC-39 现有 6 张结果不重新调用图片模型，只重新 Prime 包装和 QC。
- 结果看板每个创意仍为三个最新尺寸，二维码均可机器解码且值一致。
- 页面和 API 已重新部署，新修订链路 ADC-69 至 ADC-78 可供验收。

完整 `make check` 仍需在与当前代码匹配的完整测试数据库上执行。当前仓库的 handler 全套测试连接到了 schema 漂移的测试库，缺少 `stage`、`planned_at`、`attribution_fail_closed` 等既有字段；本轮使用受影响模块定向测试、实际模板测试、TypeScript 类型检查、真实 API 发布和实际 Issue 流程进行验证。

## 16. 当前工作区说明

本轮改造尚未提交，工作区包含大量互相关联的新增、修改和删除文件。它们属于同一轮平台化改造，不能通过重置或回退局部文件清理。

二维码硬校验、AdaKami 市场包 v15 发布、ADC-39 六张结果重包装、独立 QC 和结果看板更新均已完成。当前剩余动作是对整组工作区 diff 做最终人工审查后统一提交；不再有等待实现的二维码修复项。

## 17. 2026-08-01 平台模型收口

在前述实现基础上继续完成以下收口，后文若与本节冲突，以本节为准：

- 创意工厂只保留素材库、文案库、市场资源包三类业务资源。
- 删除用户侧任务模板、固定节点和 `single/per_image/per_size` 展开方式。
- 原 `素材小队 Leader 编排` 原位迁移为普通平台 Skill `创意素材协作`，配置类型为
  `creative_role / creative_leadership`，不再包含 `task_templates`。
- 父 Issue 的创意上下文不再保存专用编排 Skill ID；快照保存小队、Leader、成员职责和每个
  智能体绑定的普通平台 Skill 内容、配置及 references。
- 小队 Leader 运行简报直接展示成员 Skill 名称和描述，由 Leader 按证据缺口动态选择成员。
- 市场资源文件改为动态用途，支持页面添加、编辑、预览、下载和移除，同一用途允许多条。
- 新增 `app_ui_reference` 多参考能力，已上传 AdaKami `1080x2160` 新客首页；候选图包含竞品
  App UI 时，分析成员选择最合适参考，图像编辑以多图输入完成替换。
- 确定性合成脚本从市场资源文件移除，只保留在 Prime Skill reference 中。
- 素材库新增本地文件/URL 导入、搜索、竞品筛选、详情预览和下载。
- 当前 Autopilot 和 AppGrowing 采集契约只执行 Indonesia / Indonesian。
- 新增数据库迁移 `250_native_creative_squad_context`，同时清理
  `creative_issue_context.orchestration_skill_id`。
- API 已部署为 `creative-platform-v7`，健康检查通过；平台回灌后市场包有 5 个有效文件，
  `compose_script` 数量为 0，`app_ui_reference` 数量为 1。
- 新验收父 Issue 为 `ADC-79`，标题为“2026-08-01 印尼素材采集与修图协作验收 v6”。

本轮代码验证：core schema 测试 36/36 通过，`packages/views` TypeScript 全量检查通过，Go
`internal/handler` 与 `cmd/server` 测试通过，Leader Skill 目录通过 `quick_validate.py`。

## 18. ADC-79 完整现场验收

2026-08-01 使用平台真实运行链路完成 `ADC-79`，不是测试数据或单独 HTML 演示：

- 父 Issue：`ADC-79`，保留候选池、结果看板和必要发布结论，状态保持 `todo`，便于继续选图。
- 采集 Issue：`ADC-80`，查询 7 家竞品并逐页检查 27 页，选材 25 条，去重后向父候选池写入
  23 条真实 AppGrowing 素材，其中 7 条新增、18 条已有记录。
- 入选候选：`7c618bb5-6efe-4b55-8eaa-4be430b6e536`，Adapundi 问答式素材；候选状态保留为
  `selected`，以便用户从结果看板继续精确调整。
- 固定文案：`48b92bf2-b790-4ba6-9d83-421e85063c87` v7，已审核。
- 创意工作 Issue：`ADC-81`。Leader 通过直接子 Issue 依次委派 `ADC-82` 参考分析、`ADC-83`
  生成方案、`ADC-84` 至 `ADC-86` 三尺寸无品牌底图、`ADC-87` 至 `ADC-89` Prime 包装和
  `ADC-90` 独立终检；专业对话和附件均留在对应子 Issue。
- `ADC-90` 对三张最终 PNG 重新执行尺寸、命名、Prime 完整性、文案和二维码机器验收，三个
  尺寸全部 `QC PASS`。二维码均解码为
  `https://www.adakami.id/termsandconditions`。
- Leader 把通过的 `1200x628`、`1080x1080`、`800x1000` 成图重新上传到 `ADC-79`；三个
  新附件的 `issue_id` 均为父 Issue ID `18892d87-97c2-4ce2-b306-54a8d0120d76`，结果看板据此
  聚合为 1 张创意、3 个尺寸，并保留预览、单尺寸下载、整组下载和继续调整入口。

本次现场同时修正了以下运行问题：

- 专业任务统一使用“新建直接子 Issue + 分配智能体”触发，不再用当前 Issue 内的成员
  `@mention` 代替委派。
- 专业成员只把自己的子 Issue 置为 `done`，由平台完成事件唤醒 Leader，避免手工回执和自动
  回执造成重复执行。
- 图片模型出现 429 限流时，先核对模型请求和附件证据；本次复用已成功生成的原始输出完成
  尺寸适配，没有重复消耗模型调用。
- Prime 安全判断区分模板实际不透明图文区和外围留白缓冲区。只有批准文案、金额、按钮、主体
  或关键内容进入实际覆盖区才阻断；连续背景、空白卡片下缘或阴影进入外围缓冲区，由真实 Prime
  合成图和独立 QC 判断。
- Leader Skill 更新为 v5，读取专业子 Issue 的完整时间线和最新人工决定，不再被历史
  `approved_with_risk` 文本错误阻断。

父结果看板最终附件：

- `019fbbea-4fd7-7980-8cfd-a8c1a55dfa88`：`1200x628`
- `019fbbea-4feb-7e39-a695-c5a6d791971d`：`1080x1080`
- `019fbbea-4ff8-726a-b3f6-3be0afc66710`：`800x1000`

验收时 API、Web 和 `direct-image2` daemon 均正常。当前会话没有可用的内置浏览器实例，因此
未把页面点击或截图记为自动化浏览器验收；候选数量、Issue 层级、附件归属、图片像素和二维码
均通过平台 CLI/API、实际文件和独立 QC 证据验证。

## 19. 素材、文案和看板交互修订

### 19.1 素材归档

排查时工作区共有 34 条素材：12 条完成，8 条失败，14 条停在 `pending`。失败记录中的源站
域名解析错误来自 API 容器；14 条 `pending` 已达到 5 次尝试，旧查询不会再次领取它们。

本轮修改了归档状态机：

- 服务启动时回收超时的 `running` 任务，并把达到上限的记录标记为可见失败。
- 同一素材再次被采集时，平台使用新 URL 清零尝试次数并重新归档。
- 重试上限改为 8 次，退避时间从 15 秒增加到最多 15 分钟。
- 素材库显示归档中、归档失败和可操作错误，并提供单条与批量重试。
- 新接口 `POST /api/creative/materials/archive/retry` 只允许成员调用。

API v7 启动后先恢复 14 条卡死记录。随后用成员身份调用批量重试接口，接口返回
`scheduled_count=8`。归档器保存剩余 8 条后，数据库状态为 `completed=34`。

### 19.2 文案库一致性和推荐

当前源文件为：

```text
C:\Users\zhangzhenyu\ad-creative-factory-lean\profiles\id-adakami\catalog\source\NEW SCRIPT DESIGN CONTENT - 2026.xlsx
```

源 Excel 的 SHA-256 为
`c62e6275d963cc73462c3ee87759102a845945965d319a0e25280e4d8b67e304`，与 profile
`copy_catalog.json` 中登记的哈希一致。平台从该 profile 重新导入 423 条并发布文案库 v14。

逐条核验结果：

- 主标题、卖点、原始文案、职责、状态、标签均为 0 条差异。
- `content_keyword`、`suggested_copy_type`、源 Sheet、单元格、行列和值均为 0 条差异。
- 274 条为 `approved`，149 条为 `draft`，与 profile 判定一致。
- 一条旧绝对图片路径已改为 profile 中的 `assets/june-I42-1.png`。
- 文案库记录了源文件名、相对路径和 SHA-256，后续导入可核对来源。

创意工厂文案库增加编号、标题、卖点、职责和标签搜索。候选池推荐改为即时规则排序：平台先读取
真实图片识别出的主利益点、辅助利益点、关键数值和证据，再与 Excel 条目的利益点类型和关键词
评分。主利益点权重最高，辅助利益点次之；视觉主题只有在文案本身包含对应主题时才提供小幅加分。
素材标题、采集元数据和标签只在尚未识别主利益点时作为弱兜底，不能覆盖人工确认的创意简报。
界面展示“主利益点：额度”“辅助利益点：低利率”等匹配原因。平台每次打开工作台时使用当前查询
缓存，文案库更新后查询失效并重新读取；推荐过程不调用图片模型。没有可靠识别结果时，界面明确
提示当前只能通用排序，用户仍可触发 AI 识别、手工填写利益点、搜索文案库或精准编辑。

### 19.3 候选池

- 放大候选池现在传入 `CreativeIssueItem` 和文案操作，选择与“选择或编辑文案”使用同一套动作。
- “不采用”写入 `creative_material_issue_candidate.status=rejected`，状态只属于当前 Issue，不污染
  工作区素材库的全局标签；用户可按“不采用”筛选并恢复。
- 选图后，候选池顶部显示已选数量、已定文案数量、“逐图配置文案”和“开始修图”。用户无需滚到
  列表底部。
- 逐图文案工作台左侧展示全部入选图片，中间保留当前原图，右侧提供推荐和精准编辑。用户可用
  缩略图或前后按钮切换；保存后平台切到下一张未定文案素材。

### 19.4 结果看板

- 平台从 Leader 发布评论中的候选 ID 关联结果附件和原始素材；兼容文件名包含候选 UUID 的旧结果。
- 看板并排展示原始素材与当前尺寸成图，两侧图片都可点开查看。
- 看板移除与点击图片重复的预览按钮，保留单尺寸下载。
- “本创意 ZIP”打包当前创意的三个最新尺寸，“下载全部 ZIP”打包全部创意的最新尺寸。10 张
  创意只需下载一次，压缩包按创意前缀分目录。
- “调整这张图”使用结果对应的候选 ID，不再回退到第一张入选素材。

### 19.5 验证

- `@multica/views` 和 `@multica/core` TypeScript 检查通过。
- 创意文件名、最高修订、结果评论映射和文案推荐共 6 个测试通过。
- Go `internal/creative` 与 `internal/handler` 测试通过。
- 受影响前端文件通过 ESLint；创意模块沿用现有中文直写规则，因此检查时关闭该目录已有的
  `i18next/no-literal-string` 规则。
- API `creative-platform-v7` 的 `/readyz` 返回数据库和迁移均为 `ok`。
- 创意工厂和 ADC-79 页面由当前 Next 开发服务编译并返回 HTTP 200。
- 当前会话的浏览器运行时没有可用实例，本节未记录点击截图验收。

## 20. 创意理解、并发与后续调整

### 20.1 一张图的交付语义

当前交付语义为：

- 一张入选参考图对应一个独立创意工作 Issue；
- 一组确定的“视觉主题 × 主利益点 × 文案”生成 V01-V03 三个有明确差异的创意方向；
- 每个创意方向交付 `1080x1080`、`1200x628`、`800x1000` 三个原生尺寸；
- 每张入选参考图共交付九张成图，文件名包含 V01-V03。

用户选择多张参考图时，每张图建立独立创意分支；不同图片之间不共享文案决定、生成证据或返工
状态。同一图片的三个创意也分开保存生成、包装、QC 和返工证据；尺寸不是变体。

### 20.2 主题与利益点

数据库迁移 `251_creative_issue_item_brief` 为候选项增加版本化 `creative_brief`。字段包括视觉主题、
主题元素、主利益点、辅助利益点、关键数值、识别文字、图片证据、画面类型、置信度和分析 Issue。

两类信息职责分离：

- 主利益点决定文案推荐和信息层级，例如额度、低利率、费用减免；
- 视觉主题决定场景与视觉词汇，例如世界杯/足球赛事、斋月、发薪日；
- “世界杯 + 降费”表示足球赛事负责画面，费用减免负责主文案，不能把两者合并成一个模糊标签。

市场资源包页面可维护利益点词表和主题预设，逐图工作台仍允许自由输入。因此未来新增 Malaysia
市场时复制市场包并维护当地语言、利益点、主题、文案库和资源，不需要修改推荐代码。涉及世界杯
时默认只使用通用足球视觉，未提供批准资源时不得生成官方标志、奖杯、队徽或赞助关系。

### 20.3 并发边界

平台只保留真实依赖，不按固定阶段串行等待：

- 多张候选图的参考分析并发，页面创建子 Issue 的并发上限为 4；
- 多张图的创意工作 Issue 并发创建；
- 单个方案确认后，同时执行 V01-V03 三个 `1080x1080` 方形母版；
- 任一母版通过后，以该母版为第一参考同时执行 `1200x628` 和 `800x1000` 原生重排；
- 同一变体三张底图齐备后，一个 Prime Issue 批量包装，再由一个 QC Issue 批量验收；
- 图像编辑智能体最多 5 路并发，daemon 总任务上限为 8，允许 Leader、Prime、QC 与图像任务并行。

Leader Skill 模板为 v7，生成方案 v3，图像生产/Prime/QC v2。CLI 支持瞬时图片接口失败自动
退避，daemon 总任务并发与图像 API 并发分别配置，不再共用一个过低上限。

### 20.4 对成图不满意时

结果看板的“调整这张图”同时展示原始素材和当前成图，并要求用户选择范围：

- `仅当前尺寸`：只引用当前成图附件，只返工该尺寸；
- `整组三尺寸`：引用当前 V01/V02/V03 创意的三个最新尺寸，由 Leader 判断共享修改和尺寸适配；
- 调整 Issue 保存候选 ID、原创意工作 Issue、精确基准附件 ID、受影响附件列表和用户反馈；
- 未受影响且已经通过 QC 的尺寸继续沿用，不重复生成。

若用户改的是主题、主利益点或文案，则属于输入变更：候选修订号递增，旧工作 Issue 映射失效，
需要重新开始该创意修图，避免把旧结果误认为符合新输入。

真实验收 Issue `ADC-91` 位于 `ADC-81` 下，范围为“仅 `1200x628`”。Leader 正确回执了精确基准
附件 `019fbbea-4fd7-7980-8cfd-a8c1a55dfa88`、单尺寸范围和另外两尺寸保持不变，并直接置为
`done`；本次没有创建专业子 Issue，也没有调用图片模型。

### 20.5 本轮验证与部署

- API 镜像：`localhost/multica-direct-backend:creative-platform-v8`，`/readyz` 的数据库和迁移均为
  `ok`；迁移 251 已应用。
- Web `http://localhost:3000` 返回 HTTP 200。
- `direct-image2` daemon 已替换为新 CLI，创意简报命令可用，启动脚本不再触发
  `NetworkCredential.Dispose` 错误。
- AdaKami Indonesia 市场资源包重新发布，页面配置包含 8 个利益点和 7 个主题预设；文案库重新
  导入 423 条。
- ADC-79 已选素材依据真实像素写入创意简报：主题“临时资金需求问答”，主利益点“额度”，并
  保存画面中的额度、期限、利率和放款速度证据。
- `@multica/views`、`@multica/core` 类型检查通过；创意结果与推荐定向测试 6/6 通过；三个修改的
  Skill 均通过 `quick_validate.py`。

## 21. 性能优化前基线与下一版契约

### 21.1 ADC-92 基线

优化前的回滚样例为 `ADC-92`：

- Issue ID：`a92832f7-8866-46fa-8795-671e6602d49b`；
- 从真实候选池选择 Easycash、UATAS 两张素材；
- 每张素材生成 1 个创意方向，每个方向交付三个尺寸，共 6 张成图；
- 端到端耗时 3 小时 50 分 35 秒；
- Easycash 分支约 1 小时 42 分 49 秒；
- UATAS 分支因限流、重复唤醒和返工约 3 小时 30 分。

耗时拆解显示，图片生成的正常三尺寸调用约 10-15 分钟，主要延迟来自：选图后分析和建分支的
调度空档、Leader 重复读取和决策、包装与 QC 任务排队、失败后等待人工唤醒，以及过宽安全区
导致的误返工。Easycash 分支触发 10 次 Leader 执行；UATAS 分支触发 31 次 Leader 执行，并
产生 13 个图片任务，其中 9 个阻断、1 个取消。该行为只作为优化前事实保留，不是目标工作流。

### 21.2 当前 3 x 3 交付语义

从本次优化开始，一张入选素材对应一套创意交付：

```text
1 张参考素材
  -> 3 个有明确差异的创意变体
    -> 每个变体 3 个原生尺寸
      -> 共 9 张最终成图
```

三个创意变体共享候选素材、用户确认文案、市场资源包和合规事实，但必须在构图、视觉表达或信息
组织上形成可说明的差异。尺寸不是变体；每个变体分别原生构图 `1080x1080`、`1200x628`、
`800x1000`，不能由一个尺寸裁切或加边得到另外两个尺寸。

性能目标为单套 20-30 分钟。用户同时选择 3 张素材时，三套工作并发推进，端到端目标仍为
20-30 分钟，而不是串行累加到 60-90 分钟。图像接口当前最高允许 6 路并发，平台先使用 5 路，
保留 1 路余量；队列按素材和变体公平调度，避免一张素材占满全部执行窗口。Prime 包装和机器
检查应在对应图片生成后立即流水执行，不等待整套九张全部完成。

### 21.3 实现边界

本次不通过增加 Leader 评论或固定阶段来实现并发。执行契约写入平台 Skill；Leader 每次唤醒
批量创建所有已满足依赖且尚不存在的子 Issue，并按候选、修订、变体、尺寸和能力去重。平台
负责任务触发、并发配额、子 Issue 完成唤醒和结果聚合；Leader 处理动态委派、真实异常和最终发布。
包装、文件命名、二维码校验和可确定的机器检查继续使用 Skill references 中的确定性脚本。

### 21.4 代码与配置

- 候选池按钮按 `已选素材数 × 9` 展示预计交付量，创意工作 Issue 固定写入 3 x 3 契约。
- 结果看板按文件名中的 V01-V03 拆成独立创意组；每组保留三个最新尺寸、原图对比、ZIP 和返工入口。
- 图像编辑 CLI 新增 `--max-attempts`，只对 408、429、5xx 和网络失败退避重试并报告尝试次数。
- 图像编辑智能体并发设为 5；daemon 总任务并发设为 8，额外槽位用于 Leader、Prime 和 QC。
- 五个相关 Skill 模板已更新并通过 `quick_validate.py`；前端交付分组测试、Core/Views 类型检查和
  Go 图像重试测试通过。

### 21.5 ADC-135 真实 3 x 3 验收

真实验收入口为 `ADC-135`，创意工作 Issue 为 `ADC-140`。素材不是测试数据：Leader 委派
`ADC-137` 从 AppGrowing 连续查询 Easycash Indonesia 五页并把 10 条结果放入父 Issue 候选池，
本次选用新归档候选 `19c9a621-51c2-4dad-9529-c4544865066e`。`ADC-139` 读取真实像素后确认主利益点
为低利率，文案使用已审核 Excel v15 记录 `552fa8c4-2c84-4caf-927a-cdd4e6bf1d92`。

协作现场如下：

```text
ADC-140 创意工作
  ADC-141 生成方案
  ADC-142 / 143 / 144  V03 / V02 / V01 方形母版，同秒启动
  ADC-145 / 146        V03 两个原生重排
  ADC-147 / 148        V01 两个原生重排
  ADC-149 / 150        V02 两个原生重排
  ADC-151 / 152 / 154  V03 / V02 / V01 三尺寸 Prime 包装
  ADC-155 / 153 / 156  V03 / V02 / V01 三尺寸独立 QC
```

五路图像任务曾同时运行，daemon 同期仍能执行 Leader 和 Prime；全程未出现 429，CLI 自动重试
没有被触发。V01、V02 两张母版及 V01 横版因真实视觉复核做了定向第二次创意调用，其他调用一次
成功。返工只影响对应变体或尺寸，没有重做整套。

实测时间：

- AppGrowing 采集请求到采集子 Issue 完成约 7 分 38 秒；
- `ADC-140` 启动到方案完成 8 分 57 秒，其中方案子 Issue 本身约 5 分 7 秒；
- 三个母版同秒启动，分别约 5 分 47 秒、10 分 20 秒、12 分 34 秒完成；
- 9 张无品牌底图全部齐套时为启动后 39 分 55 秒；
- 三个 Prime 批量包装各约 4 分 38 秒至 5 分 44 秒，并与其他变体生成重叠；
- 三个 QC 各约 4 分 13 秒至 5 分 53 秒；
- 最后一组 QC 完成时为启动后 57 分 17 秒，9 张发布到结果看板时为约 61 分 9 秒。

结果看板收到 9 个 PNG，文件名均包含 `V01`、`V02` 或 `V03` 和真实尺寸；逐文件像素检查为三组
`1080x1080`、`1200x628`、`800x1000`。三个方向分别为蓝色住宅建设利率表、暖色家庭预算和
蓝白金融信息图。Prime、商店与监管素材完整，9 张二维码均经包装和独立 QC 解码为批准 payload，
未出现二维码白底、底部白带或折叠。

这次结果证明 3 x 3、五并发、变体级包装/QC 和结果看板契约可用，但 61 分钟仍未达到 20-30 分钟
目标。现场确认的额外编排成本主要来自 Leader 每轮拉取工作区全量 Issue 后筛子项，以及 Prime/QC
首次现场查询附件命令。平台新增 `multica issue children <issue-id>` 直接子项命令；Leader Skill
固定使用该命令，Prime/QC Skill 固定并行执行 `multica attachment download`。后续性能验收以本次
61 分 9 秒为新基线，重点继续压缩图片智能体调用前后准备时间和同一父 Issue 的无动作唤醒。

## 22. 结果看板来源映射与局部返工（2026-08-02）

### 22.1 平台数据能力

迁移 252 增加 `creative_delivery` 和 `creative_adjustment_request`。前者持久化每张最终成图的
候选来源、原创意工作 Issue、变体、尺寸、修订、无品牌底图、Prime 包装证据和 QC Issue；后者
记录用户选择的返工范围、精确目标附件、基准附件、反馈文字及调整子 Issue。

新增平台接口和 CLI：

- 注册交付清单：`multica creative delivery register <issue-id> --input-file <manifest.json>`；
- 创建单尺寸或整变体调整请求，并把调整子 Issue 绑定回请求；
- Issue 创建请求支持扁平基础类型 metadata，在事务提交和智能体入队前完成持久化。

这使任务分派所需的目标范围在智能体第一次启动时就已确定，不再依赖创建后补写描述或评论。

### 22.2 页面交互

结果看板优先读取结构化交付记录，并保留旧文件名解析作为历史兼容。每个创意组展示对应竞品
原图和三个当前尺寸；“调整当前创意”弹窗同时展示竞品原图与当前成图，允许选择“仅当前尺寸”
或“当前创意三尺寸”。调整完成后新修订替换当前展示，旧附件不覆盖、不删除。

### 22.3 Skill 契约

Leader Skill v8 识别 `creative_adjustment` metadata，只委派受影响范围；图像编辑 Skill v3 以
上一修订的无品牌底图为第一输入，不把带 Prime 的最终图作为模型编辑底图；Prime 与 QC Skill
v3 同时支持一张和三张的局部处理。Leader 发布成功后必须登记交付清单，平台页面不再依赖自然
语言评论建立来源关系。

### 22.4 验证与部署

- 本地 API 镜像升级为 `localhost/multica-direct-backend:creative-platform-v9`，迁移 252 已应用，
  `/readyz` 的数据库和迁移均为 `ok`。
- `direct-image2` daemon 使用新 CLI 运行，总并发仍为 8；图像编辑智能体并发仍为 5。
- `ADC-135` 已登记 9 条 R4 交付记录，精确覆盖 3 个变体和 3 个尺寸；父 Issue 仍有 10 条真实
  AppGrowing 候选。
- Core schema 测试 37/37、结果文件名与历史兼容测试 7/7 通过；Core/Views TypeScript 检查通过；
  Go 接口、Issue metadata 和真实数据库集成测试通过；四个相关 Skill 通过快速校验。

## 23. 原图语义锚点、多素材看板与第二套 3 x 3 验收（2026-08-02）

### 23.1 原图约束与返工

创意简报新增 `source_semantics`、`information_mechanism`、`visual_anchors`、`palette_anchors`、
`must_preserve` 和 `allowed_variations`。分析 Skill v3 负责从真实像素提取这些锚点；方案 Skill v4
要求三个变体共享业务语义、信息机制和主色，只在构图与信息层级上形成差异；图像编辑 Skill v4
和 QC Skill v4 分别执行锚点保留与偏题检查。

结果看板增加素材级自然语言调整。用户可直接写“V01 保留，V02、V03 重做并保留还款计划表和
绿色主色”；平台识别目标变体，分别创建结构化调整请求。`replan` 从竞品原图和已确认简报重新
规划，不沿用偏题底图；普通 `edit` 从上一版无品牌底图继续。Leader Skill v10 还会对 Prime
顶部或底部硬区的确定性碰撞自动创建一次精确安全区恢复 Issue，不重做整套。

### 23.2 多素材结果看板

结果看板按“候选素材 -> V01-V03 -> 三尺寸”分组。左侧素材列表展示竞品原图、创意数和尺寸数；
切换素材后右侧只展示该素材的原图对比和三个创意。全局 ZIP 按素材与变体建目录，单素材 ZIP
包含该素材九张图；单尺寸、整创意和自然语言多变体返工入口同时保留。

`ADC-135` 现有两张真实候选、六个创意和十八张最终图。新增候选为
`65c98abc-33cc-4156-8b60-12b064024a12`，创意工作 Issue 为 `ADC-159`：

```text
ADC-159 创意工作
  ADC-160                    V01-V03 方案
  ADC-161 / 162 / 163        V02 / V01 / V03 方形母版，同秒启动
  ADC-164-167 / 173 / 174    六个原生尺寸重排
  ADC-168                    V03 方形安全区恢复
  ADC-169 / 170 / 176        V02 / V01 / V03 Prime 包装
  ADC-171 / 172 / 178        V02 / V01 / V03 QC
  ADC-175 / 177 / 179        V01 竖图安全区恢复、重新包装与单尺寸复验
```

第二张竞品图的核心是低息还款计划表。三个成图方向均保留月供表、低利率、80JUTA、12 BULAN、
绿色/黄色主色和右侧人物关系；Prime 条款、二维码、商店和监管区完整，没有白底或底部白带。
九条 R3 结构化交付已在 `2026-08-02 04:06:08 UTC` 登记，`ADC-159` 于 `04:06:39 UTC` 完成。

### 23.3 实测耗时与结论

- 参考分析 `ADC-158` 用时 6 分 11 秒；创意工作到发布用时 88 分 3 秒。
- 方案用时 6 分钟；三个母版同秒启动，首轮分别用时 9 分 55 秒、11 分 58 秒和 10 分 45 秒。
- V03 因 Prime 硬区碰撞自动恢复；V01 只有 `800x1000` 被定向返工，另外两个已通过尺寸复用。
- 22 个专业任务累计执行约 178 分 30 秒，依靠并发压缩到 88 分钟；17 次 Leader 执行累计约
  51 分 44 秒，其中多数和专业任务重叠。
- 注入专业任务的 `issue-resources.json` 从 61,541 字节降到 17,772 字节，减少约 71%。不可变
  市场包、文案、已选素材和 Skill 标识仍完整保留，重复的 Skill 正文与 references 不再逐任务注入。

本轮语义一致性和自动局部恢复明显改善，但耗时从上一套的 61 分 9 秒上升到 88 分 3 秒，尚未达到
20-30 分钟目标。主要瓶颈仍是单次图片任务 8-13 分钟、两处真实返工和 17 次 Leader 唤醒；资源
瘦身只减少启动和读取成本，不能抵消图片调用与返工。下一轮性能优化应继续压缩 Leader 无动作
唤醒，并让 Prime/QC 更早流水，不降低语义锚点或 QC 标准。

## 24. GPT-5.6 平台模型目录（2026-08-02）

Codex 智能体模型目录提供完整 GPT-5.6 家族：通用别名 `gpt-5.6`、旗舰 `gpt-5.6-sol`、均衡型
`gpt-5.6-terra` 和高吞吐型 `gpt-5.6-luna`，`gpt-5.6-sol` 作为最新默认展示项，`gpt-5.5`
继续保留。平台优先采用本机 Codex 的结构化模型目录；本机已识别的 Sol 当前显示 `low`、
`medium`、`high`、`xhigh`、`max`、`ultra`，默认 `low`。未被本机目录枚举的 5.6 型号使用官方 API
努力程度 `none / low / medium / high / xhigh / max`，默认 `medium`。此次只增加平台可选项，没有
自动改写现有素材小队各智能体的模型。

修图结果看板采用两级横向选择和一个主对比区：第一行切换素材，第二行以缩略图切换 V01-V03，
主体只并排显示竞品原图和当前创意，每次保证两张图都有足够宽度完整展示。当前创意上方直接切换
三个尺寸；原图和结果都可点击放大，单尺寸、单素材 ZIP、全部 ZIP 和自然语言调整入口保持不变。

API 已部署为 `localhost/multica-direct-backend:creative-platform-v13`，`direct-image2` daemon 使用
同版 CLI，最大任务并发保持 8。平台端到端模型发现实测返回：Sol 为
`low / medium / high / xhigh / max / ultra`，Terra 为
`low / medium / high / xhigh / max / ultra`，Luna 为 `low / medium / high / xhigh / max`，
通用 `gpt-5.6` 为 `none / low / medium / high / xhigh / max`。`/readyz` 数据库与迁移检查均为
`ok`，前端类型检查和结果看板 12 个定向测试通过。

首版四张大图横排在约 1000 像素宽的 Issue 内容区导致图片过窄、文件名相互挤压，无法承担清晰
对比。现已将标题与操作区拆成稳定的上下两行，并取消四张大图同屏：V01-V03 只作为横向缩略
选择器，760 像素双栏主预览专门展示原图和当前结果。三个尺寸使用固定高度的
“方形/横版/竖版 + 尺寸”两行控制，不再截断 `800x1000`。
