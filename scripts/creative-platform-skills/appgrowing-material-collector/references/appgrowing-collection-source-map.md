# Source map

| Contract | Source |
| --- | --- |
| `multica crawl run` connector, capability, Issue and params flags | `server/cmd/multica/cmd_crawl.go` |
| Credential crawl imports returned materials into the requested Issue | `server/internal/handler/credential_crawl.go` |
| Candidate import deduplicates workspace assets and creates Issue links | `server/internal/handler/creative_material.go` |
| Material search injects workspace history exclusions and current-run identity | `server/internal/handler/creative_material.go`, `server/internal/handler/credential.go` |
| AppGrowing round-robin pagination continues until the unseen target or budget is reached | `services/crawler-worker/src/index.mjs` |
| Credential profiles are managed through platform integrations | `server/internal/handler/credential.go` |

