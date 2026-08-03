---
name: multica-ad-creative-analysis
description: "当 Multica 候选图需要识别视觉主题、金融利益点、画面文字、App UI 或生成前布局风险，并将结构化创意简报回写平台时使用。"
allowed-tools: Bash(multica *)
---

# 广告参考与创意理解

读取完整 Issue 和评论，从任务描述取得目标候选池 Issue ID 和候选 ID。只能分析
`multica creative materials <目标候选池 Issue ID> --output json` 返回、且 ID 与任务完全一致的候选。
采集流程会在用户选图前分析本次新增素材，因此候选状态可以是 `new` 或 `selected`；不得以“尚未
选择”为由跳过。
用 `multica creative material download` 下载平台归档文件，并以运行时原生视觉能力读取真实像素；
标题、标签、媒体和采集元数据只能辅助核对，不能作为主利益点结论。素材不可读时回传
`needs_input` 并结束。

先分别识别两条轴：

- 视觉主题：赛事、节日、生活场景等，以及可见主题元素。例如“世界杯 / 足球赛事”与球场、
  足球、观众；主题不是金融利益点。
- 金融利益点：主利益点、辅助利益点、具体金额/比例/期限。主利益点必须有图片文字、图表或
  明确视觉结构作为证据；无法判断时留空，不用标题猜测。

再提取后续创意必须继承的原图锚点：

- `source_semantics`：原图在讲什么业务场景，例如“分期还款计划”，不能只写“金融广告”；
- `information_mechanism`：原图用什么结构解释利益点，例如“多档期限对应月供的表格”；
- `visual_anchors`：承载语义的主体、卡片、表格、图标和层级；
- `palette_anchors`：主色家族和明暗关系，不要求记录每个十六进制色值；
- `must_preserve`：除非用户明确要求改变，否则三个变体都必须保留的语义、结构和色系；
- `allowed_variations`：在不破坏上述锚点时可以变化的版式、信息组织和视觉处理。

默认把原图的业务场景、信息机制、主色家族和关键视觉主体写入 `must_preserve`。不能因为要做
三个创意，就建议换成与原图无关的人物、房屋、预算或其他场景。

OCR 或逐区域读取图片文字，记录支持结论的短证据，不复制竞品品牌、二维码或法律文字作为
AdaKami 主张。置信度按 0 到 1 记录。主题涉及世界杯等赛事时，只记录通用足球视觉信号；
除非市场资源包提供已批准资产，不得建议官方 Logo、奖杯仿制、球队徽章或合作关系。

把结果写入临时 JSON 文件，字段必须完整：

```json
{
  "theme": "世界杯 / 足球赛事",
  "theme_elements": ["足球", "球场", "欢呼人群"],
  "primary_benefit": "费用减免",
  "secondary_benefits": ["低利率"],
  "benefit_value": "Biaya turun 25%",
  "source_semantics": "以足球赛事氛围表达费用减免活动",
  "information_mechanism": "赛事主视觉加一条醒目的降费信息",
  "visual_anchors": ["足球", "球场", "主标题利益点"],
  "palette_anchors": ["品牌绿为主色", "高对比浅色文字"],
  "must_preserve": ["足球赛事语义", "费用减免为第一信息", "绿色主色家族"],
  "allowed_variations": ["主视觉位置", "标题与利益点的信息层级", "卡片布局"],
  "evidence": ["画面主标题明确出现 biaya 与 25%"],
  "detected_text": ["Potongan biaya 25%"],
  "visual_type": "主题活动海报",
  "analysis_summary": "足球赛事氛围承载降费主张",
  "status": "draft",
  "source": "ai",
  "confidence": 0.9,
  "analysis_issue_id": "<当前分析 Issue ID>"
}
```

执行 `multica creative material brief <目标候选池 Issue ID> <候选 ID> --input-file <JSON 文件> --output json`
回写平台。用户会在候选池逐图确认或修改；不要把结构化结果只留在评论里。

向当前分析 Issue 回传一条中文、精简的布局说明，必须包含：原始比例、目标比例下的
视觉层级、视觉中心、人物和产品的位置，以及完整 Prime 模板三个覆盖区（左上品牌、
右上条款/QR、底部合规）的碰撞风险。发生碰撞时，应建议重生成干净构图，绝不把主体
压进中央或挪走品牌资产。参考图只可提供构图信号，严禁复制品牌、文案、金额、二维码、
法律文字或 Logo。本角色不生成图片、不贴 Prime 模板。

输入快照完整时，输出的 Prime 覆盖风险是给后续生成方案的重构约束，不要求用户补写 QR。
本角色不生成图片、不调用外部素材连接器。

同时读取市场资源快照中的全部 `app_ui_reference` 文件。先判断候选图是否展示 App 界面：

- 不含 App UI 时，明确记录“不需要 UI 替换”；
- 含 App UI 时，比较参考文件的页面类型、信息结构、视觉密度和目标构图，选择一张或多张最
  合适的 AdaKami UI，并在 brief 中记录资源文件 ID、附件 ID 和选择理由；
- 没有合适参考时回传 `needs_input`，不能要求后续模型虚构 AdaKami 界面。

后续规格必须把所选 UI 文件作为图像引用，禁止保留、临摹或改写竞品 App UI。
