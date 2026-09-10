# Creative image model settings

New Creative Orders and standalone direct-edit orders freeze
`input_snapshot.image_generation` to `gpt-image-2.5-sunburst` and `xhigh`.
`server/pkg/imagemodel` owns supported models, quality validation, the new-order
default and historical snapshot interpretation. Model settings are separate from
the agent's text model, thinking level and concurrency.

## Execution contract

| Boundary | Behavior |
| --- | --- |
| Order creation | Writes the platform default into the frozen input; existing submission keys return the original order |
| Production and adjustment dispatch | Copies the order settings into task context, including selected-size expansion and visual rework |
| First image operation | Requires its model and `input_snapshot.image_generation` to match the order |
| Image request | Sends explicit model, quality and mapped size to the existing `/images/edits` endpoint |
| Normal or late receipt | Checks model and quality against the frozen operation settings |
| Completed asset | Validates supported model/quality, prompt and request lineage; preserves quality in metadata and model-result evidence |
| Recovery | Reuses existing task/operation settings and successful assets; does not switch models on errors |

`multica image settings --input-file order.json` returns the model and quality
from an order JSON file. It also accepts a task/context or image-operation JSON
file. The production and direct-edit Skills use this result for both operation
registration and explicit `--model` / `--quality` command arguments.

An order without `image_generation` predates this contract and resolves to
Image 2/high. Existing image operations remain immutable. Unqualified CLI
commands retain the Image 2 default so updating the CLI does not change commands
already issued by older Skill snapshots. New platform tasks explicitly supply
their frozen Sunburst/xhigh settings. The CLI also accepts Flare; Image 2 rejects
`xhigh` and `max`. Both 2.5 models accept those quality settings.

The three provider canvases remain 1088x1088, 1200x624 and 800x992. Final
deliverables remain 1080x1080, 1200x628 and 800x1000. Official Prime composition,
candidate selection, adoption and downloads retain their current contracts.

Single-image and batch receipts record quality and requested provider size.
Image 2 asset metadata retains its original shape for idempotent replay; its
quality is read from the original model-result evidence when present.
Generation details show quality from actual generated-image evidence, leaving
older missing values blank. Provider usage/billing aggregation is outside this
change.

## Managed configuration

Factory template version 31 includes production Skill 117 and direct-edit Skill
34. Initialization and bootstrap update managed instructions and Skill content,
while preserving existing agent text-model, thinking-level and concurrency
settings. Newly created agents still use the platform's agent defaults.

No database migration is required for these settings. Existing order snapshots,
operation input snapshots and result-receipt JSON fields carry the contract.

## Deployment and acceptance

Wait for the configured gateway to support Sunburst/xhigh before deploying the
new order default. Deploy backend validation support, update execution-host CLI
binaries, and synchronize workspace Skills before submitting new orders. Update
the frontend to display recorded image quality. Source Skill changes alone do
not update persisted workspace Skills or already-running task instructions.

Validate single-image and batch requests, normal asset writeback and late
receipts with a simulated provider. Cover new and historical orders, missing-size
expansion, direct edits, immutable operation settings, and preservation of custom
agent settings. Once the gateway is ready, test actual output for every delivery
size and inspect the final overlaid images. Do not infer quality or latency gains
from the configuration migration alone.
