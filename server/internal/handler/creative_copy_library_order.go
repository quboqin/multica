package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type creativeCopyLibrarySelection struct {
	LibraryVersion    int                 `json:"library_version"`
	CreativeType      string              `json:"creative_type"`
	Slots             map[string][]string `json:"slots"`
	RepaymentPlanKeys []string            `json:"repayment_plan_keys"`
	VisualOnly        bool                `json:"visual_only"`
	VisualDirection   json.RawMessage     `json:"visual_direction"`
}

func freezeCreativeCopyLibraryOrder(ctx context.Context, q dbExecutor, workspaceID pgtype.UUID, input *creativeOrderInput) error {
	for index := range input.Items {
		item := &input.Items[index]
		if item.SourceKind != "copy_library" {
			continue
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(input.InputSnapshot, &envelope); err != nil {
			return err
		}
		var market creativePrimeFrozenMarketPack
		if err := json.Unmarshal(envelope["market_pack"], &market); err != nil {
			return errors.New("published market pack is required")
		}
		marketID, err := parseUUIDString(market.ID)
		if err != nil {
			return errors.New("market pack id must be a UUID")
		}
		publishedMarket, err := loadPublishedCreativeResource(ctx, q, workspaceID, marketID, "market_pack")
		if err != nil || market.Version != publishedMarket.PublishedVersion {
			return errors.New("market pack published version has changed; reload the selection")
		}
		market.Config = publishedMarket.Config
		envelope["market_pack"], err = json.Marshal(market)
		if err != nil {
			return err
		}
		input.InputSnapshot, err = json.Marshal(envelope)
		if err != nil {
			return err
		}
		libraryID, err := parseUUIDString(item.CopyLibraryID)
		if err != nil {
			return errors.New("copy_library_id must be a UUID")
		}
		boundID, err := creativeOrderCopyLibraryID(input.InputSnapshot)
		if err != nil || boundID != libraryID {
			return errors.New("selected market pack must bind this copy library")
		}
		library, err := loadPublishedCreativeResource(ctx, q, workspaceID, libraryID, "copy_library")
		if err != nil {
			return errors.New("copy library is not published in this workspace")
		}
		var selection creativeCopyLibrarySelection
		if err := json.Unmarshal(item.CopySnapshot, &selection); err != nil {
			return errors.New("invalid copy library selection")
		}
		item.CopySnapshot, err = freezeCreativeCopyLibrarySelection(selection, library)
		if err != nil {
			return err
		}
	}
	return nil
}

func freezeCreativeCopyLibrarySelection(selection creativeCopyLibrarySelection, library creativeResourceResponse) (json.RawMessage, error) {
	if selection.LibraryVersion != library.PublishedVersion || selection.LibraryVersion < 1 {
		return nil, errors.New("copy library published version has changed; reload the selection")
	}
	if selection.CreativeType != "num" && selection.CreativeType != "repayment_plan" {
		return nil, errors.New("invalid creative type")
	}
	var config composableCopyLibraryConfig
	if err := json.Unmarshal(library.Config, &config); err != nil {
		return nil, errors.New("invalid published copy library")
	}
	roles := []string{"headline", "subheadline", "benefit", "supporting", "cta", "legal"}
	allowedRoles := map[string]bool{}
	for _, role := range roles {
		allowedRoles[role] = true
	}
	for role := range selection.Slots {
		if !allowedRoles[role] {
			return nil, fmt.Errorf("unsupported copy slot %q", role)
		}
	}
	fragments := make([]creativeOrderCopySnapshotFragment, 0)
	textByRole := map[string]string{}
	selected := map[string]bool{}
	for _, role := range roles {
		lines := make([]string, 0)
		for _, id := range selection.Slots[role] {
			if selected[id] {
				return nil, errors.New("copy fragment is selected more than once")
			}
			found := false
			for _, fragment := range config.Fragments {
				if fragment.ID != id || fragment.Status != "approved" {
					continue
				}
				compatible := false
				for _, creativeType := range fragment.CreativeTypes {
					if creativeType == selection.CreativeType {
						compatible = true
					}
				}
				if !compatible || strings.TrimSpace(fragment.Text) == "" || creativeCopyFactReferencePattern.MatchString(fragment.Text) {
					return nil, errors.New("selected copy fragment is not ready for this creative type")
				}
				text := strings.TrimSpace(fragment.Text)
				fragments = append(fragments, creativeOrderCopySnapshotFragment{ID: id, Key: fragment.Key, Role: role, Text: text})
				lines = append(lines, text)
				found = true
				break
			}
			if !found {
				return nil, fmt.Errorf("copy fragment %q is not approved in the published library", id)
			}
			selected[id] = true
		}
		textByRole[role] = strings.Join(lines, "\n")
	}
	entries := make([]composableCopyLibraryRepaymentPlanEntry, 0)
	seenPlans := map[string]bool{}
	for _, key := range selection.RepaymentPlanKeys {
		if seenPlans[key] {
			return nil, errors.New("repayment plan is selected more than once")
		}
		found := false
		for _, entry := range config.RepaymentPlan.Entries {
			if entry.Key == key && entry.Status == "approved" && entry.Source != "" && entry.Principal > 0 && entry.TenorMonths > 0 && entry.MonthlyInstallment > 0 && entry.TotalInterest >= 0 && entry.TotalRepayment > 0 {
				entries = append(entries, entry)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("repayment plan %q is not approved in the published library", key)
		}
		seenPlans[key] = true
	}
	visualOnly := len(fragments) == 0 && len(entries) == 0
	planSelections := make([]creativePreAdaptationRepaymentPlanSelection, 0, len(entries))
	for _, entry := range entries {
		planSelections = append(planSelections, creativePreAdaptationRepaymentPlanSelection{
			ID: entry.ID, PlanKey: entry.Key, Principal: entry.Principal, TenorMonths: entry.TenorMonths,
			Values: creativePreAdaptationRepaymentPlanValues{
				Principal: creativeFormatRupiah(entry.Principal), Tenor: fmt.Sprintf("%d Bulan", entry.TenorMonths),
				MonthlyInstallment: creativeFormatRupiah(entry.MonthlyInstallment), TotalInterest: creativeFormatRupiah(entry.TotalInterest), TotalRepayment: creativeFormatRupiah(entry.TotalRepayment),
			},
		})
	}
	if visualOnly != selection.VisualOnly {
		return nil, errors.New("confirm visual-only exploration when no copy or repayment plan is selected")
	}
	visualDirection, err := normalizedOptionalJSONObject(selection.VisualDirection)
	if err != nil {
		return nil, errors.New("visual_direction must be an object")
	}
	return json.Marshal(map[string]any{
		"schema_version": 3, "source_kind": "copy_library", "status": "approved",
		"library_id": library.ID, "library_version": library.PublishedVersion, "library_name": library.Name,
		"id": "copy-library-selection", "composition_id": "copy-library-selection", "composition_key": "copy-library-selection",
		"creative_type": selection.CreativeType, "delivery_naming": map[string]string{"type": selection.CreativeType},
		"headline": textByRole["headline"], "subheadline": textByRole["subheadline"], "benefit": textByRole["benefit"],
		"supporting": textByRole["supporting"], "cta": textByRole["cta"], "legal_text": textByRole["legal"],
		"slots": selection.Slots, "fragments": fragments, "product_facts": []any{},
		"repayment_plan_entries": entries, "repayment_plan_labels": config.RepaymentPlan.Labels, "repayment_plan_selections": planSelections,
		"visual_only": visualOnly, "visual_direction": visualDirection,
		"omitted_copy_policy": "Do not invent or fill unselected copy slots or repayment plans. Render only frozen selected copy and numeric values; official Prime remains unchanged.",
	})
}
