package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const creativeFactoryCollectionLimit = 2

// prepareCreativeFactoryCollectionParams resolves all workspace-owned values
// before a connector is invoked. The connector request is the only mutable
// boundary, so an old AutoPilot description cannot select another workspace's
// agent or raise the collection budget.
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
	root["analysis_agent_id"] = uuidToString(agent.ID)
	root["limit"] = creativeFactoryCollectionLimit
	for _, key := range []string{"max_materials", "max_results", "target_count"} {
		if _, exists := root[key]; exists {
			root[key] = creativeFactoryCollectionLimit
		}
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func capCreativeFactoryMaterials(materials []creativeMaterialInput) []creativeMaterialInput {
	if len(materials) <= creativeFactoryCollectionLimit {
		return materials
	}
	return materials[:creativeFactoryCollectionLimit]
}

func creativeFactoryCollectionLimitFromParams(params json.RawMessage) int {
	root, ok := crawlParamsObject(params)
	if !ok {
		return 0
	}
	value, ok := root["limit"]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func creativeFactoryCollectionParamsHasAgent(params json.RawMessage, agentID string) bool {
	root, ok := crawlParamsObject(params)
	if !ok {
		return false
	}
	value, _ := root["analysis_agent_id"].(string)
	return strings.TrimSpace(value) == strings.TrimSpace(agentID)
}
