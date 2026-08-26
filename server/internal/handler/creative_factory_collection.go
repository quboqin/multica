package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// prepareCreativeFactoryCollectionParams resolves all workspace-owned values
// before a connector is invoked. The connector request is the only mutable
// boundary, so an old AutoPilot description cannot select another workspace's
// agent. Collection quantity remains owned by the AutoPilot/task business
// request and is carried through params.
func (h *Handler) prepareCreativeFactoryCollectionParams(ctx context.Context, workspaceID pgtype.UUID, connectorID, capability string, params json.RawMessage) (json.RawMessage, error) {
	if normalizedCrawlConnectorID(connectorID) != "appgrowing" || normalizedCrawlCapability(capability) != "material_search" {
		return params, nil
	}
	if h == nil || h.DB == nil || h.Queries == nil {
		return nil, errors.New("creative factory is not initialized")
	}
	agent, err := h.creativeFactoryAgentByRole(ctx, workspaceID, "reference_analysis")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Existing workspaces created before installation records retain the
			// capability-bound agent until the owner enables the factory again.
			agent, err = h.resolveReferenceAnalysisAgent(ctx, workspaceID, pgtype.UUID{})
		}
		if err != nil {
			return nil, fmt.Errorf("creative factory is not initialized: %w", err)
		}
	}
	root, ok := crawlParamsObject(params)
	if !ok {
		return nil, errors.New("material_search params must be a JSON object")
	}
	if _, hasLimit := root["limit"]; !hasLimit {
		if target := creativeFactoryCollectionTargetFromObject(root); target > 0 {
			root["limit"] = target
		}
	}
	root["analysis_agent_id"] = uuidToString(agent.ID)
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func capCreativeFactoryMaterials(materials []creativeMaterialInput, params json.RawMessage) []creativeMaterialInput {
	target := creativeFactoryCollectionTargetFromParams(params)
	if target <= 0 || len(materials) <= target {
		return materials
	}
	return materials[:target]
}

func creativeFactoryCollectionTargetFromParams(params json.RawMessage) int {
	root, ok := crawlParamsObject(params)
	if !ok {
		return 0
	}
	return creativeFactoryCollectionTargetFromObject(root)
}

func creativeFactoryCollectionTargetFromObject(root map[string]any) int {
	for _, key := range []string{"limit", "max_materials", "max_results", "max_outputs", "max_output", "output_limit", "target_count", "material_limit"} {
		if target := positiveCreativeFactoryCollectionTarget(root[key]); target > 0 {
			return target
		}
	}
	return 0
}

func positiveCreativeFactoryCollectionTarget(value any) int {
	switch number := value.(type) {
	case float64:
		if number > 0 {
			return int(number)
		}
	case int:
		if number > 0 {
			return number
		}
	case int32:
		if number > 0 {
			return int(number)
		}
	case int64:
		if number > 0 {
			return int(number)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(number))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func creativeFactoryCollectionParamsHasAgent(params json.RawMessage, agentID string) bool {
	root, ok := crawlParamsObject(params)
	if !ok {
		return false
	}
	value, _ := root["analysis_agent_id"].(string)
	return strings.TrimSpace(value) == strings.TrimSpace(agentID)
}
