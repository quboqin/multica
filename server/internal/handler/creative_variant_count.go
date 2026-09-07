package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	creativeDefaultVariantCount = 3
	creativeMaxVariantCount     = 10
	creativeReserveCount        = 2
	creativeMaxCandidateCount   = creativeMaxVariantCount + creativeReserveCount
)

type creativeVariantCountContract struct {
	Target     int
	Candidates int
	Configured bool
}

func creativeOrderVariantCounts(raw json.RawMessage) (creativeVariantCountContract, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
		return creativeVariantCountContract{}, errors.New("input_snapshot must be an object")
	}
	target := creativeDefaultVariantCount
	if value, exists := snapshot["target_variant_count"]; exists {
		if err := json.Unmarshal(value, &target); err != nil || string(value) == "null" || target < 1 || target > creativeMaxVariantCount {
			return creativeVariantCountContract{}, errors.New("target_variant_count must be an integer between 1 and 10")
		}
	}
	_, configured := snapshot["target_variant_count"]
	counts := creativeVariantCountContract{Target: target, Candidates: target + creativeReserveCount, Configured: configured}
	if value, exists := snapshot["candidate_count"]; exists {
		var candidates int
		if json.Unmarshal(value, &candidates) != nil || candidates != counts.Candidates {
			return counts, errors.New("candidate_count does not match the frozen target variant count")
		}
	}
	return counts, nil
}

func freezeCreativeOrderVariantCount(input *creativeOrderInput) error {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(input.InputSnapshot, &snapshot); err != nil {
		return err
	}
	delete(snapshot, "candidate_count")
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	counts, err := creativeOrderVariantCounts(raw)
	if err != nil {
		return err
	}
	copyOnly := len(input.Items) > 0
	for _, item := range input.Items {
		if item.SourceKind != "copy_library" {
			copyOnly = false
		}
		if item.SourceKind != "copy_library" && counts.Target != creativeDefaultVariantCount {
			return errors.New("custom variant counts are only available for copy-library orders")
		}
	}
	snapshot["target_variant_count"] = json.RawMessage(strconv.Itoa(counts.Target))
	snapshot["candidate_count"] = json.RawMessage(strconv.Itoa(counts.Candidates))
	if copyOnly {
		snapshot["reserve_promotion_mode"] = json.RawMessage(`"allow"`)
	}
	input.InputSnapshot, err = json.Marshal(snapshot)
	return err
}

func (c creativeVariantCountContract) keyPattern() string {
	keys := make([]string, c.Candidates)
	for i := range keys {
		keys[i] = fmt.Sprintf("C%02d", i+1)
	}
	return "^(" + strings.Join(keys, "|") + ")$"
}

func loadCreativeOrderVariantCounts(ctx context.Context, q dbExecutor, orderID, workspaceID pgtype.UUID) (creativeVariantCountContract, error) {
	var raw string
	if err := q.QueryRow(ctx, `SELECT input_snapshot::text FROM creative_order WHERE id = $1 AND ($2::uuid IS NULL OR workspace_id = $2)`, orderID, workspaceID).Scan(&raw); err != nil {
		return creativeVariantCountContract{}, err
	}
	return creativeOrderVariantCounts(json.RawMessage(raw))
}

func (c creativeVariantCountContract) allowsKey(key string) bool {
	if len(key) != 3 || key[0] != 'C' {
		return false
	}
	index, err := strconv.Atoi(key[1:])
	return err == nil && index >= 1 && index <= c.Candidates && key == fmt.Sprintf("C%02d", index)
}

func (c creativeVariantCountContract) validateSelection(selected, reserves int) error {
	if selected != c.Target || reserves < 0 || reserves > creativeReserveCount {
		return fmt.Errorf("candidate selection requires %d selected variants and up to %d reserves", c.Target, creativeReserveCount)
	}
	return nil
}

func (c creativeVariantCountContract) validateRank(state string, rank int) error {
	if rank == 0 && (state == "candidate" || state == "rejected") {
		return nil
	}
	if state == "selected" && rank >= 1 && rank <= c.Target {
		return nil
	}
	if state == "reserve" && rank > c.Target && rank <= c.Candidates {
		return nil
	}
	return errors.New("candidate selection rank does not match the order target")
}
