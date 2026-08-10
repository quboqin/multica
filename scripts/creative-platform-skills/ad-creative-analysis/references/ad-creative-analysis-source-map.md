# Creative Reference Analysis Source Map

| Contract | Source |
| --- | --- |
| `multica creative library download` retrieves the candidate's real archived image or current-run source fallback | `server/cmd/multica/cmd_creative_domain.go:runCreativeLibraryDownload`, `server/internal/handler/creative_material.go` |
| `multica creative source-analysis put` persists versioned structured analysis against a candidate | `server/cmd/multica/cmd_creative_domain.go:runCreativeSourceAnalysisPut`, `server/internal/handler/creative_domain.go:CreateCreativeSourceAnalysis` |
| `multica creative source-analysis list` reads candidate analyses for write-back verification | `server/cmd/multica/cmd_creative_domain.go:runCreativeSourceAnalysisList`, `server/internal/handler/creative_domain.go` |
| `text_blocks.semantic_kind` records a value's business meaning from its visible label, unit, and relationship in the candidate image; `text_blocks.visual_bounds` records its normalized source-image rectangle; `visual_regions` groups every changeable source block into exactly one copy or repayment-number component for confirmation-page inspection; none may be inferred from a copy-library fragment | `scripts/creative-platform-skills/ad-creative-analysis/SKILL.md`, candidate pixels downloaded by `multica creative library download` |
| Reference-analysis tasks are candidate items grouped under a Crawl Run evidence source | `references/../../appgrowing-material-collector/references/delegate_preanalysis.py`, `server/internal/service/task.go:EnqueueDirectTaskFanout` |

Market and brand selection intentionally have no source here: those inputs belong to the frozen Creative Order and Planner.
