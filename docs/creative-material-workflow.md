# Multica 素材工作流

## 当前边界

当前实现覆盖以下闭环：

```text
Credential Broker 绑定登录态
  -> crawler connector 抓取并标准化素材
  -> issue 候选池去重、筛选和人工选图
  -> 修图智能体整理 prompt 与 dynamic rules
  -> create_creative_job
  -> 后端持久化并自动轮询 get_creative_job
  -> 结果看板选择变体和尺寸
  -> 下载 ZIP 素材包
```

没有配置工作区 MCP 时，`create_creative_job`、`get_creative_job` 和图片结果由内置 mock provider 返回。管理员可在“设置 -> 集成 -> 工作区 MCP”配置真实 Streamable HTTP MCP；issue 交互和 API 不变。

## 工作区 MCP

工作区 MCP 是 Multica 后台连接，不是智能体运行时的 MCP 配置。它只暴露两个稳定能力：

- `create_creative_job`：提交选图、prompt 和 dynamic rules，立即返回外部 job id。
- `get_creative_job`：按 job id 查询进度，完成后返回变体及尺寸资源。

连接地址、工具名和认证请求头保存在 `workspace_mcp_connection`。认证请求头由 `MULTICA_WORKSPACE_MCP_KEY` 使用 AES-256-GCM 加密，列表和编辑接口只返回请求头名称，不返回值。

创建修图任务时，Multica 会把当时的 `mcp_connection_id` 写入 `creative_edit_job`。后台协调器始终按这个快照恢复轮询，因此切换默认连接、智能体退出或 API 重启都不会让已提交任务改用另一套服务。停用连接会阻止新任务使用它，但不妨碍已提交任务完成恢复。

## 修图智能体

从 `creative-material-editor` 模板创建智能体。试点工作区还应绑定 `ad-creative-workflow` skill，用于文案类型、品牌约束、动态规则和 MCP 参数整理。

智能体使用以下 CLI：

```bash
# 使用当前 issue 中人工选中的素材，默认 3 个变体和 3 个尺寸
multica creative create-job <issue-id> \
  --prompt-file direction.md \
  --rules-json '{"market":"idn-adakami","strategy":"instruct"}' \
  --output json

# 查询一次。加 --wait 可在 CLI 窗口内等待，但不是任务恢复所必需
multica creative get-job <issue-id> <job-id> --output json

# 完成后导出 manifest.json 和全部变体/尺寸
multica creative download <issue-id> <job-id> --output-dir ./output
```

后台协调器根据 `creative_edit_job.next_poll_at` 领取任务。领取使用数据库行锁和租约，多 API 实例不会同时占用同一批任务。服务重启时会立即扫描 `queued/running` 任务，因此智能体退出或浏览器关闭不会终止生成。

## 过程数据与审计

Creative MCP 可在 `create_creative_job` 和 `get_creative_job` 的顶层响应中返回 `process_data` 对象。Multica 会把最新的对象保存到 `creative_edit_job.process_data`；某次轮询未返回该字段时保留上一版，不会清空已有记录。单次对象上限为 512 KiB。

`process_data` 用于记录可回溯但不含凭证的生产过程，例如文案来源和选择理由、VisualBrief 摘要、模型名称、各阶段耗时、每轮 QC 结论、通过率、产物命名及相对工件名。没有真实计量数据时，token 和成本必须明确写为 `not_available`，不能估算。

该对象会出现在素材任务 API 的 `edit_jobs[].process_data`，也会被快照到人工反馈的 `process_snapshot.process_data`，并写入“全部下载”ZIP 的 `manifest.json.process_data`。API key、认证头、服务 URL、原始凭证、绝对文件路径和带签名的素材 URL 不得进入该对象；这些信息仍只存在于各自的安全配置边界。

## 素材归档

抓取结果按 `(workspace_id, connector_id, dedupe_key)` 去重。新素材由后台归档到 Multica 已配置的 Storage：

- 配置 `S3_BUCKET` 时写入 S3 或兼容对象存储。
- 未配置 S3 时写入 `LOCAL_UPLOAD_DIR`。
- 外链下载限制协议、公网地址、重定向次数和最大字节数。
- `MULTICA_CREATIVE_ARCHIVE_ALLOWED_HOSTS` 可显式允许可信内部对象存储；不要用它放开不受信任主机。

候选池优先展示 `archived_url`。原始 URL 仍保留用于来源追踪。

## 声明式 Connector

简单的新站点可以通过部署变量 `MULTICA_CREDENTIAL_CONNECTORS_JSON` 注册，不需要重新编译镜像。API 和 crawler worker 必须使用同一份 JSON。例如：

```json
[
  {
    "id": "example-ads",
    "display_name": "Example Ads",
    "login_url": "https://ads.example.com/login",
    "probe_url": "https://ads.example.com/library",
    "state_domains": ["ads.example.com"],
    "allowed_target_domains": ["ads.example.com"],
    "capabilities": ["profile_verify", "page_extract"],
    "auth_check": {
      "url": "https://ads.example.com/api/me",
      "user_id_path": ["data", "id"],
      "payload": {}
    }
  }
]
```

`page_extract` 支持受限的 `click`、`fill`、`select`、`wait_for`、`press`、`scroll` 步骤，以及文本、`href`、`src`、`title`、`alt`、`value` 提取。复杂 GraphQL、验证码、反爬和专用字段归一化仍应实现专用 capability，不能让 issue 中的任意指令执行代码或访问内网。

## 云端部署

`docker-compose.selfhost.yml` 包含 crawler worker。生产环境至少配置：

```bash
BROKER_STATE_KEY=<base64-encoded-32-byte-key>
CRAWLER_WORKER_PUBLIC_URL=https://browser.example.com
MULTICA_APP_URL=https://multica.example.com
CORS_ALLOWED_ORIGINS=https://multica.example.com
S3_BUCKET=<bucket>
AWS_ACCESS_KEY_ID=<key>
AWS_SECRET_ACCESS_KEY=<secret>
```

worker 必须通过 HTTPS 暴露远程浏览器页面。`BROKER_STATE_KEY` 只能由 worker 持有，API 数据库只保存密文。
