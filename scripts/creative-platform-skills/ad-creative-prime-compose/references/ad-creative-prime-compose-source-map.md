# Creative Order Prime Compose Source Map

| Contract | Source |
| --- | --- |
| Production accepts only `prime_composition.schema_version=2`; full-template legacy mode is rejected | `references/image_prime_compose.py:validate_composition`, `references/image_prime_compose.py:compose_manifest`, `server/internal/handler/creative_market_pack_qr.go:parsePrimeCompositionConfig` |
| Image components use a complete standalone source, preserve aspect ratio, and contain it in `destination_rect` | `references/image_prime_compose.py:contain_layer`, `references/image_prime_compose.py:compose_v2`, `server/internal/handler/creative_market_pack_qr.go:requiredPrimeComponentSourceRoles` |
| Non-QR image component edge backgrounds can be converted to alpha before fixed-coordinate composition, with alpha-key evidence recorded | `references/image_prime_compose.py:transparentize_edge_background`, `references/image_prime_compose.py:compose_v2` |
| Text components render `content`; placement style is optional and defaults remain deterministic | `references/image_prime_compose.py:render_text_component`, `references/image_prime_compose.py:compose_v2` |
| Fixed Prime component coordinates are never moved; when a component declares a backdrop rule, the exact destination rectangle is quieted before compositing and the evidence is recorded per component | `references/image_prime_compose.py:component_backdrop_rule`, `references/image_prime_compose.py:prepare_fixed_prime_backdrop`, `references/image_prime_compose.py:compose_v2` |
| Busy generated content under a fixed Prime slot is recorded as `body_clearance` warning but does not block composition | `references/image_prime_compose.py:prime_slot_clearance`, `references/image_prime_compose.py:compose_v2` |
| Enabled destination rectangles compile into the hard-region contract for every size | `references/image_prime_compose.py:compile_layout_contract`, `server/internal/handler/creative_market_pack_qr.go:compilePrimeCompositionValidation` |
| A Prime package binds one variant/revision to exactly one job per expected size and emits per-job identity plus the compiled layout contract | `references/image_prime_compose.py:validate_package_manifest`, `references/image_prime_compose.py:compose_manifest` |
| Static QR uses exactly one `prime_qr` source; dynamic QR renders the approved payload; final files are decoded as evidence | `references/image_prime_compose.py:validated_qr_payload`, `references/image_prime_compose.py:compose_v2`, `server/internal/handler/creative_market_pack_qr.go:validateStaticPrimeQRConfig`, `server/internal/handler/creative_market_pack_qr.go:validatePrimeCompositionQRPolicy` |
| A legacy sheet can be split once into independent transparent/opaque assets but is never used by production composition | `references/extract_prime_components.py:extract_manifest`, `references/extract_prime_components.py:remove_edge_background` |
| `multica creative order get` returns the frozen order, variants, assets, and market input snapshot | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderGet` |
| `multica attachment download` and `upload` use authenticated workspace attachments | `server/cmd/multica/cmd_attachment.go` |
| `multica creative order asset-put` records each primed asset and its generated lineage | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderAssetPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderAsset` |
| Native QC fanout uses a shared evidence source and distinct lane/revision item keys | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:normalizeDirectTaskContext` |
| QC fanout accepts only the Variant QC evidence kind with lane-specific workflow and complete Variant trace fields | `server/internal/handler/task_fanout.go:validateCreativeTaskFanoutContext`, `server/internal/handler/task_fanout.go:validateCreativeQCTaskContext` |

Reconfirm these paths before changing the Prime delivery contract.
