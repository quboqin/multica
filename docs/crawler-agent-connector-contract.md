# 爬虫 Agent、Credential Broker 与 Connector 能力库契约

本文定义 Multica 中“登录态托管 + 爬虫自动化”的产品和工程边界。它是 `credential-broker-design.md` 的上层契约：前者说明登录态怎么安全保存，本文说明 Autopilot / Issue 怎么用自然语言驱动爬虫。

## 目标

用户希望在 Multica 里创建一个爬虫 Agent，然后用 Autopilot 或 Issue 写业务规则，例如：

```text
每周抓取 AppGrowing 印尼现金贷竞品素材。

竞品：
Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO

Easycash、Kredit Pintar、Adapundi 优先。

筛选：
- 新素材：占比 40%，投放天数 < 7 天，曝光估算 > 1K
- 跑量素材：占比 60%，投放天数 > 30 天，曝光估算 >= 10M

输出最多 25 条。没命中就说明原因，不要输出调试日志。
```

这段文字应该是 Autopilot 的业务配置，而不是代码。Agent 负责把它变成 connector 能执行的结构化调用。

## 四个角色

| 角色 | 负责什么 | 不负责什么 |
|---|---|---|
| Autopilot / Issue | 写人能看懂的目标、竞品、筛选规则、输出要求 | 不保存账号密码、不写 Cookie、不写站点技术参数 |
| 爬虫 Agent | 读取自然语言任务，调用 `multica crawl run`，整理结果并评论/创建 issue | 不绕过登录、不接触明文登录态 |
| Credential Broker | 管理绑定、重新授权、撤销、加密保存浏览器登录态 | 不理解“投放天数 > 30 天”这类业务规则 |
| Connector 能力库 | 声明站点能力，解析该站点业务 intent，执行抓取并标准化输出 | 不拥有 Autopilot 业务策略 |

关系如下：

```text
Autopilot / Issue
  -> 爬虫 Agent
  -> multica crawl run --connector appgrowing --intent-file appgrowing-weekly.md
  -> Credential Broker 解析工作区 active profile
  -> AppGrowing connector 解析 intent 并执行 material_search
  -> 返回素材结果或未命中原因
```

## Credential Broker

Credential Broker 是登录态托管层。它只回答三类问题：

1. 当前工作区有没有某个 connector 的可用 profile。
2. 没有或过期时，如何让工作区 owner/admin 通过远程浏览器重新授权。
3. 爬虫运行时，如何把加密登录态交给 worker 使用。

安全不变量：

- AI 上下文里只允许出现 `profile_id`、`connector_id` 和业务参数。
- 账号、密码、Cookie、storageState、Authorization header 不进入 AI 上下文。
- server 存密文，worker 持密钥解密。
- profile 是工作区共享资源：Autopilot 按工作区和 connector 使用唯一 active profile。绑定人只用于审计；登录态不进入智能体上下文，也不会暴露给成员。

## Connector 能力库

Connector 是每个站点的能力适配。AppGrowing 是第一个 connector。

一个 connector 至少声明：

| 字段 | 含义 |
|---|---|
| `id` | 稳定标识，例如 `appgrowing` |
| `login_url` | 远程浏览器首次打开的授权页面 |
| `state_domains` | 哪些域名的浏览器状态属于该站点 |
| `auth_check` | 怎么验证登录态是否仍然有效 |
| `capabilities` | 站点支持的业务动作，例如 `material_search` |
| `intent_parser` | 把人话规则转成该 capability 的结构化参数 |
| `executor` | 真正执行抓取、下载、解析和输出 |

Connector 可以用浏览器、GraphQL、REST API、页面快照或下载器。调用方不关心实现方式，只关心 capability 的输入输出契约。

## Crawl Intent

`crawl intent` 是 Autopilot/Issue 写给爬虫 Agent 的自然语言业务说明。它不是 prompt 魔法，而是一个受约束的产品契约：

- 竞品/对象列表要写清楚。
- 业务筛选条件要写成字段 + 比较符 + 阈值。
- 输出格式要写清楚。
- 不要写账号、密码、Cookie、Token。

自然语言理解属于爬虫 Agent 的职责，不属于 connector 的职责。Connector 不应该写死类似“创意 -> 素材搜索 -> 输入竞品名称 -> 筛选素材”的中文路径正则；这种路径可以保留在 Autopilot 里给人和 Agent 理解，但 connector 接收的应该是 Agent 整理后的结构化参数，例如 `competitors`、`selection_rules`、`date_range` 和可选的 `competitor_urls`。

边界如下：

```text
Autopilot/Issue：人话描述业务目标
爬虫 Agent/模型：理解人话，选择 capability，整理参数
Connector：校验参数，执行站点能力，返回标准结果
```

当前 CLI 支持三种传入方式：

```bash
multica crawl run --connector appgrowing --capability material_search --intent "竞品：Easycash..."

multica crawl run --connector appgrowing --capability material_search --intent-file appgrowing-weekly.md

cat appgrowing-weekly.md | multica crawl run --connector appgrowing --capability material_search --intent-stdin
```

底层仍会变成结构化参数传给 worker。例如 AppGrowing 的 intent 会被解析成：

```json
{
  "competitors": ["Easycash", "Kredit Pintar", "Adapundi"],
  "priority_competitors": ["Easycash"],
  "selection_rules": {
    "new_materials": {
      "ratio": 0.4,
      "duration_days_lt": 7,
      "impression_gt": 1000
    },
    "volume_materials": {
      "ratio": 0.6,
      "duration_days_gt": 30,
      "impression_gte": 10000000
    }
  },
  "limit": 25
}
```

结构化参数仍然可用，且优先级高于 intent。也就是说高级调用方可以在 `--params-json` 里覆盖 intent 解析出的字段。

## 不发版与需要发版的边界

不用发版的场景：

- 已有 connector 的新任务，例如 AppGrowing 换竞品、换筛选条件、换输出要求。
- 改 Autopilot 的执行频率、提示词、结果格式。
- 用 Issue 临时触发一次已有 capability。

不需要重新编译镜像、但需要部署管理员更新 Connector 清单并重启 worker 的场景：

- 新站点可以用声明式登录态、`profile_verify` 和 `page_extract` 完成。
- 页面操作只需要受限的 click/fill/select/wait/press/scroll 步骤。
- 页面结果可以通过文本或 href/src/title/alt/value 字段标准化。

通过 `MULTICA_CREDENTIAL_CONNECTORS_JSON` 把同一份清单同时交给 API 和 crawler worker。清单由部署管理员维护，不能由 issue 里的任意 prompt 直接改写，否则会形成 SSRF 和任意浏览器自动化入口。

仍然需要发版的场景：

- 新站点第一次接入，还没有 connector。
- 站点登录、验证码、反爬、字段结构需要新增适配。
- 新业务动作不属于已有 capability，例如从“素材搜索”扩展到“素材下载入库 + 视频转码 + 资产审核”。
- 需要把一次性的浏览器探索固化成稳定生产能力。

## AppGrowing 当前契约

Connector：`appgrowing`

Capability：`material_search`

支持的 intent 字段：

| 人话字段 | 结构化字段 |
|---|---|
| `竞品：Easycash、Kredit Pintar` | `competitors` |
| `Easycash 品牌页：https://appgrowing-global.youcloud.com/.../leaflet` | `competitor_urls.Easycash` |
| `Easycash、Kredit Pintar 优先` | `priority_competitors` |
| `新素材：占比 40%` | `selection_rules.new_materials.ratio = 0.4` |
| `投放天数 < 7 天` | `duration_days_lt = 7` |
| `曝光估算 > 1K` | `impression_gt = 1000` |
| `跑量素材：占比 60%` | `selection_rules.volume_materials.ratio = 0.6` |
| `投放天数 > 30 天` | `duration_days_gt = 30` |
| `曝光估算 >= 10M` / `10M+` | `impression_gte = 10000000` |
| `输出最多 25 条` | `limit = 25` |
| `日期范围 -29,0` | `date_range = "-29,0"` |

结果输出只应包含业务结果：素材、竞品、投放天数、曝光估算、资源链接、未命中原因。调试字段如 GraphQL operation、page snapshot、auth probe 只用于排障，不应直接给业务用户看。

## 后续演进

短期先把 AppGrowing 的 intent parser 和 `material_search` executor 做稳。

声明式 Connector 已支持把常见登录和页面提取能力放进部署配置。专用 Connector 仍建议使用更清晰的文件结构：

```text
services/crawler-worker/src/connectors/appgrowing/
  manifest.mjs
  intent.mjs
  material-search.mjs
```

长期如果需要给多个 agent runtime 复用，可以把 worker 的 capability 暴露成 MCP 工具。但 MCP 是工具入口，不替代 connector。登录态、授权、字段解析、站点适配仍由 Credential Broker 和 Connector 能力库负责。
