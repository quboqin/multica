package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A model turn ending is not evidence that a production task produced images.
// Validate its exact phase and revision before accepting the daemon's completion.
func (h *Handler) creativeProductionCompletionError(ctx context.Context, task db.AgentTaskQueue, workspaceID string) (string, error) {
	var scope struct {
		Type            string `json:"type"`
		Workflow        string `json:"workflow"`
		VariantID       string `json:"variant_id"`
		CreativeOrderID string `json:"creative_order_id"`
		Revision        int    `json:"revision"`
	}
	if json.Unmarshal(task.Context, &scope) != nil || scope.Type != "creative_domain_task" || scope.Workflow != "creative_production" {
		return "", nil
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
		return "", nil
	}
	sizes, err := creativeTaskImageScopeSizes(task.Context)
	variantID, variantErr := parseUUIDString(scope.VariantID)
	orderID, orderErr := parseUUIDString(scope.CreativeOrderID)
	if err != nil || variantErr != nil || orderErr != nil || scope.Revision < 1 {
		return "creative production completed with invalid image scope", nil
	}
	var valid bool
	if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_order_variant v
JOIN creative_order_item i ON i.id=v.order_item_id JOIN creative_order o ON o.id=i.order_id
WHERE v.id=$1 AND v.revision=$2 AND o.id=$3 AND o.workspace_id=$4)`, variantID, scope.Revision, orderID, parseUUID(workspaceID)).Scan(&valid); err != nil {
		return "", err
	}
	if !valid {
		return "creative production completed with stale or mismatched image scope", nil
	}
	var missing []string
	if err := h.DB.QueryRow(ctx, `SELECT ARRAY(SELECT size FROM unnest($3::text[]) size
WHERE NOT EXISTS(SELECT 1 FROM creative_order_asset a WHERE a.variant_id=$1 AND a.revision=$2
AND a.size_key=size AND a.stage='generated' AND a.status='completed' AND a.attachment_id IS NOT NULL))`, variantID, scope.Revision, sizes).Scan(&missing); err != nil {
		return "", err
	}
	if len(missing) > 0 {
		var detail string
		if err := h.DB.QueryRow(ctx, `SELECT COALESCE((SELECT error_message FROM creative_image_operation
WHERE variant_id=$1 AND revision=$2 AND size_key=ANY($3::text[]) AND status='failed' AND error_message<>''
ORDER BY updated_at DESC LIMIT 1),'')`, variantID, scope.Revision, missing).Scan(&detail); err != nil {
			return "", err
		}
		message := fmt.Sprintf("creative production incomplete: missing generated assets for %s", strings.Join(missing, ", "))
		if detail != "" {
			message += "; " + truncateString(detail, 1500)
		}
		return message, nil
	}
	return "", nil
}
