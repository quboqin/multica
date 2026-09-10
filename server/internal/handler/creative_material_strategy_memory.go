package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

)

const crawlStrategyMemoryParamKey = "_adaptive_strategy_memory"
const crawlStrategyMemoryGlobalScope = "__global__"

func (h *Handler) materialSearchParamsWithStrategyMemory(ctx context.Context, workspaceID, projectID pgtype.UUID, connectorID, capability string, params json.RawMessage) json.RawMessage {
	connectorID = normalizedCrawlConnectorID(connectorID)
	capability = normalizedCrawlCapability(capability)
	if connectorID != "appgrowing" || capability != "material_search" {
		return params
	}
	root, ok := crawlParamsObject(params)
	if !ok {
		return params
	}
	competitors := crawlStrategyCompetitors(root)
	if len(competitors) == 0 {
		return params
	}
	memories, err := h.loadCreativeMaterialCrawlStrategyMemories(ctx, workspaceID, projectID, connectorID, capability, competitors)
	if err != nil || len(memories) == 0 {
		return params
	}
	root[crawlStrategyMemoryParamKey] = map[string]any{
		"enabled":  true,
		"version":  1,
		"memories": memories,
	}
	out, err := json.Marshal(root)
	if err != nil {
		return params
	}
	return out
}

func (h *Handler) loadCreativeMaterialCrawlStrategyMemories(ctx context.Context, workspaceID pgtype.UUID, projectID pgtype.UUID, connectorID, capability string, competitors []string) (map[string]any, error) {
	if h.TxStarter == nil {
		return nil, nil
	}
	keys := []string{crawlStrategyMemoryGlobalScope}
	for _, competitor := range competitors {
		if key := normalizeCrawlStrategyScopeKey(competitor); key != "" {
			keys = append(keys, key)
		}
	}
	keys = uniqueNonEmptyStrings(keys)
	if len(keys) == 0 {
		return nil, nil
	}
	projectKey := crawlStrategyProjectKey(projectID)
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
SELECT scope_key, memory
FROM creative_material_crawl_strategy_memory
WHERE workspace_id = $1
  AND connector_id = $2
  AND capability = $3
  AND scope_key = ANY($4::text[])
  AND project_id IN ('', $5)
ORDER BY CASE WHEN project_id = $5 AND $5 <> '' THEN 1 ELSE 0 END ASC, updated_at ASC
`, workspaceID, connectorID, capability, keys, projectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var scopeKey string
		var raw []byte
		if err := rows.Scan(&scopeKey, &raw); err != nil {
			return nil, err
		}
		var memory map[string]any
		if err := json.Unmarshal(raw, &memory); err != nil || len(memory) == 0 {
			continue
		}
		out[scopeKey] = memory
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (h *Handler) recordCreativeMaterialCrawlStrategyMemory(ctx context.Context, workspaceID, projectID pgtype.UUID, connectorID, capability string, raw json.RawMessage, runID string) error {
	if h.TxStarter == nil || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	connectorID = normalizedCrawlConnectorID(connectorID)
	capability = normalizedCrawlCapability(capability)
	if connectorID != "appgrowing" || capability != "material_search" {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	items, _ := root["learned_strategies"].([]any)
	if len(items) == 0 {
		return nil
	}
	projectKey := crawlStrategyProjectKey(projectID)
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		scopeKey, memory, ok := crawlStrategyMemoryFromLearnedStrategy(obj)
		if !ok {
			continue
		}
		memory["verified_at"] = time.Now().UTC().Format(time.RFC3339)
		rawMemory, err := json.Marshal(memory)
		if err != nil {
			continue
		}
		_, err = tx.Exec(ctx, `
INSERT INTO creative_material_crawl_strategy_memory (
  workspace_id, project_id, connector_id, capability, scope_key, memory,
  success_count, failure_count, last_error, last_run_id, last_seen_at
) VALUES (
  $1, $2, $3, $4, $5, $6::jsonb,
  1, 0, '', NULLIF($7, '')::uuid, now()
)
ON CONFLICT (workspace_id, project_id, connector_id, capability, scope_key)
DO UPDATE SET
  memory = creative_material_crawl_strategy_memory.memory || EXCLUDED.memory,
  success_count = creative_material_crawl_strategy_memory.success_count + 1,
  failure_count = 0,
  last_error = '',
  last_run_id = EXCLUDED.last_run_id,
  last_seen_at = now(),
  updated_at = now()
		`, workspaceID, projectKey, connectorID, capability, scopeKey, string(rawMemory), strings.TrimSpace(runID))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func crawlStrategyMemoryFromLearnedStrategy(obj map[string]any) (string, map[string]any, bool) {
	strategyType := stringFromAny(obj["strategy_type"])
	competitor := stringFromAny(obj["competitor"])
	scopeKey := normalizeCrawlStrategyScopeKey(stringFromAny(obj["scope_key"]))
	if scopeKey == "" {
		scopeKey = normalizeCrawlStrategyScopeKey(competitor)
	}
	if scopeKey == "" {
		scopeKey = crawlStrategyMemoryGlobalScope
	}
	value, _ := obj["value"].(map[string]any)
	memory := map[string]any{
		"strategy_type": strategyType,
	}
	if competitor != "" {
		memory["competitor"] = competitor
	}
	switch strategyType {
	case "appgrowing_brand":
		brandID := stringFromAny(value["brand_id"])
		if brandID == "" {
			return "", nil, false
		}
		memory["brand_id"] = brandID
		if brandName := stringFromAny(value["brand_name"]); brandName != "" {
			memory["brand_name"] = brandName
		}
		if source := stringFromAny(value["source"]); source != "" {
			memory["brand_source"] = source
		}
		if preferredSource := stringFromAny(value["preferred_source"]); preferredSource != "" {
			memory["preferred_source"] = preferredSource
		}
		if aliases := stringSliceFromAny(value["aliases"]); len(aliases) > 0 {
			memory["aliases"] = aliases
		}
	case "appgrowing_source":
		preferredSource := stringFromAny(value["preferred_source"])
		if preferredSource == "" {
			return "", nil, false
		}
		memory["preferred_source"] = preferredSource
	default:
		return "", nil, false
	}
	if materialsFound := numberFromAny(value["materials_found"]); materialsFound >= 0 {
		memory["materials_found"] = materialsFound
	}
	return scopeKey, memory, true
}

func crawlParamsObject(raw json.RawMessage) (map[string]any, bool) {
	if strings.TrimSpace(string(raw)) == "" {
		return map[string]any{}, true
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, true
}

func crawlStrategyCompetitors(root map[string]any) []string {
	return stringSliceFromAny(root["competitors"])
}

func normalizeCrawlStrategyScopeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func crawlStrategyProjectKey(projectID pgtype.UUID) string {
	if !projectID.Valid {
		return ""
	}
	return uuidToString(projectID)
}

func normalizedCrawlConnectorID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "appgrowing"
	}
	return value
}

func normalizedCrawlCapability(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "material_search"
	}
	return value
}

func numberFromAny(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n
		}
	}
	return -1
}
