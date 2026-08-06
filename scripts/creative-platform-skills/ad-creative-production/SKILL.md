---
name: multica-ad-creative-production
description: "当 creative_production task 指定一个 Creative Order Variant，需要生成同内容族无品牌三尺寸底图、登记完整模型证据并委派 Prime 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告底图生产

只处理 context 指定的 Order、Variant、revision 和 `expected_sizes`；direct edit 不进入本 Skill。

```text
multica creative order get <order-id> --output json
multica creative library download <candidate-id> --output-file <reference-image> --output json
```

只使用 Variant brief、冻结 `copy_snapshot`、market snapshot 指定附件和无品牌资产。不得读取当前文案库或
市场包，不爬取/重分析素材，不执行 Prime/QC，也不得把 primed/delivered 图作为模型输入。

## 文案与提示词证据

调用模型前验证 schema-v2 copy snapshot 和最终提示词中的金融 token：

```text
python <当前 Skill 目录>/references/validate_copy_snapshot.py \
  --materials-json <order.json> --candidate-id <candidate-id> \
  --prompt-file <prompt.txt> --evidence <copy-validation.json>
```

失败时写 Variant `action_required`，不得重选文案或调用模型。

`multica image edit --output json` 以及 batch 每个 result 返回实际 provider `prompt` 与
`prompt_sha256`。保存 CLI 原始 JSON；登记 Asset 时逐尺寸原样使用对应 prompt/hash，不得从 brief、文件或
兄弟尺寸重建。

## 三尺寸生成

初次生产先用 `multica image edit` 在 `1088x1088` 模型画布生成方形，再规范化为 `1080x1080`。方形底图
可用后，立即以它为第一图像输入，用一次 `multica image edit-batch` 并发生成横竖原生重排：

```json
{
  "max_concurrency": 2,
  "jobs": [
    {"id":"landscape","inputs":[{"path":"square.png"}],"prompt_file":"landscape.txt","model":"gpt-image-2","size":"1200x624","max_attempts":3,"output_file":"landscape-model.png"},
    {"id":"portrait","inputs":[{"path":"square.png"}],"prompt_file":"portrait.txt","model":"gpt-image-2","size":"800x1008","max_attempts":3,"output_file":"portrait-model.png"}
  ]
}
```

相对路径以 manifest 目录为准。规范化横版到 `1200x628`、竖版到 `800x1000`。横竖版不等待 Prime、QC 或
兄弟 Variant。三张共享主体、批准文案、业务语义、信息层级、色系和 `asset_family_id`；不得裁切、拉伸、
加边或重新发明内容。

`max_attempts=3` 只重试传输层 408/429/5xx/网络错误，不是创意返工。每个尺寸最多一轮有证据的定向返工；
进程仍运行时不重复提交。恢复只补当前 revision 的 missing sizes，复用输入指纹一致的成功输出。

每个模型输出确定性规范化并保存 evidence：

```text
python <当前 Skill 目录>/references/normalize_image.py \
  --input <model.png> --output <delivery.png> --width <w> --height <h> \
  --evidence <normalize-evidence.json>
```

优先有界 cover-resize；连续 provider 比例失败后才允许脚本记录的 full-content edge-fade。不得用本地绘图
替代模型。hard region 只记录 `prime_clearance_status=pending_prime_qc`；原图主体进入几何区不能触发返工。
底图错字、丢失批准内容、竞品品牌、模型生成品牌/QR 或严重破图才是底图返工原因。

## Asset 与下一跳

上传 delivery 图、CLI 原始结果和规范化证据，再登记 completed `stage=generated` Asset：

```text
multica attachment upload <file> --output json
multica creative order asset-put <order-id> --input-file <asset.json> --output json
```

Asset 必须包含 variant、family、size、revision、attachment、generated lineage；metadata 保存 CLI 原样 prompt/
model，evidence 保存 request ID、attempts、prompt SHA-256、provider slot、copy/normalize evidence 和 pending Prime
状态。方形响应的 family ID 原样传给横竖版。`variant-put` 如更新状态，必须保留 item/key/brief 完整 upsert。

全部 expected sizes 齐备后，查询 `creative_order_variant_prime` source，按
`<variant-id>:r<revision>` 跳过 active/succeeded 或已有完整 primed assets 的 item。fanout context 固定
`type=creative_domain_task`、`workflow=creative_prime`，原样携带 issue/leader/order/item/variant IDs、revision、
expected sizes、reviewer ID 和 generated asset IDs。

```text
multica task by-source list --agent <prime-agent-id> \
  --kind creative_order_variant_prime --ref <variant-id> --output json
multica task fanout --agent <prime-agent-id> --input-file <manifest.json> --output json
```

提交后立即结束。生成、上传、登记或 fanout 失败时保留已成功尺寸，写真实 error code/message 并让 task
失败；不得创建或修改 Issue。
