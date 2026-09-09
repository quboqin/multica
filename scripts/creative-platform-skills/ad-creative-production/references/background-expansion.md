# 背景扩展返工合同

仅在当前任务 `qc_visual_rework.size_strategies[<size>]=background_expansion` 时执行。初次生产、对比度修复、直接改图、model_integrated 模式均不进入此分支。

1. 下载失败尺寸上一 revision 的 **canonical generated 无品牌底图**，不能用带 Prime 的成图或未经归一化的模型原图。读取同尺寸冻结布局与同一模板 source_role。确认全部选中文案本身正确；若文字已错漏，背景扩展不能修复，写 `action_required`。
2. 准备与模型请求画布一致的输入和透明 mask：

   ```text
   python3 <skill>/references/expand_background.py prepare \
     --source <canonical-generated.png> --layout-file <layout-contract.json> --size-key <canonical-size> \
     --width <model-width> --height <model-height> --directory <expansion-dir>
   ```

   脚本仅将完整旧设计等比放进安全内容区，绝不重排字、裁字、拉伸或补文字。若需要缩到原尺寸的 65% 以下会拒绝；不要绕过。打开 `input.png` 检查正文仍可读后再调用模型。
3. 通过 `run_image_edit_job.py` 执行现有 `multica image edit`，Input 1 为 `input.png`，Input 2 为同模板当前尺寸 Prime context，`--mask` 为 `mask.png`。mask 和第一张输入严格同尺寸；保留订单冻结的 model、quality（包括 xhigh）、size 和 PNG 输出，不为了省时静默降档。模型请求回执、原图、耗时照常记录为 `visual_rework`。
4. 此分支模型提示词只要求补外围背景，例如：

   ```text
   Extend the surrounding background into the editable area of Input 1. Preserve the entire central design, all its existing lettering, numbers, style and subject identity. Continue the same lighting, colors and material naturally across the boundary without a card border or visible seam. Input 2 shows the future official overlay; use it only to keep its areas quiet and readable against the actual component colors. Do not draw any of its lettering, logos or badges. Add no new text, numbers, tables, figures, duplicated content or official components. Return a single full-bleed image on the requested canvas.
   ```

   输入角色按实际顺序明确编号，不发送坐标、文件名、JSON 或工作流指令。此背景专用提示词不执行完整新设计的 `require-redesign-guard` 或逐字重新渲染要求；批准文案校验在源正文和最终复合图上完成。不可把这一例外用于普通生成。
5. mask 不能保证像素锁定。无论模型是否改了正文，都必须用保存的正文覆盖回来：

   ```text
   python3 <skill>/references/expand_background.py compose \
     --directory <expansion-dir> --background <model-output.png> --output <expanded-generated.png>
   ```

   保存脚本输出的像素 hash 证据和 `body_preserved=true`；该合成结果再走现有 canonical normalization，保留模型原图、复合图、canonical 图三份血缘。generated metadata 写原来的 `prime_template_source_role`，evidence 附 `background_expansion` 脚本结果；模型回执仍原样保留，不能将复合图冒充 provider 原始响应。
6. 后端按相同官方模板贴片后，QC 仍逐项检查遮挡、可读性、内容完整、重复、接缝和风格。正文变小不可读、背景出现框感或仍失败时，进入 `action_required`，不得继续无限重绘或宣称已通过。
