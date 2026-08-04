# Source map

| Contract | Source |
| --- | --- |
| `multica crawl run` connector, capability, analysis agent and params flags | `server/cmd/multica/cmd_crawl.go` |
| Credential crawl creates a stable Crawl Run before the broker call and imports into that same run | `server/internal/handler/credential.go`, `server/internal/handler/creative_material.go` |
| Candidate import deduplicates workspace assets and creates Crawl Run links; Issue links are optional legacy context | `server/internal/handler/creative_material.go` |
| Material search injects workspace history exclusions and current-run identity | `server/internal/handler/creative_material.go`, `server/internal/handler/credential.go` |
| AppGrowing round-robin pagination continues until the unseen target or budget is reached | `services/crawler-worker/src/index.mjs` |
| Credential profiles are managed through platform integrations | `server/internal/handler/credential.go` |
| `creative library list --run-id` filters a single Crawl Run before eligible images are converted to one native fanout manifest | `server/cmd/multica/cmd_creative_domain.go`, `references/delegate_preanalysis.py` |
| Fanout concurrency is enforced by Agent and runtime limits; active task idempotency includes the evidence pair and item key | `server/internal/service/task.go:EnqueueDirectTaskFanout` |
| Reference-analysis completion requires matching completed Source Analysis and run-candidate state; missing output fails closed | `server/internal/handler/daemon.go:referenceAnalysisCompletionError`, `server/internal/handler/daemon.go:CompleteTask` |
