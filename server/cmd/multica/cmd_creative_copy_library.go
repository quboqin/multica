package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var creativeCopyLibraryCmd = &cobra.Command{
	Use:   "copy-library",
	Short: "Read and maintain creative copy libraries",
}

var creativeCopyLibraryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspace copy libraries",
	RunE:  runCreativeCopyLibraryList,
}

var creativeCopyLibraryGetCmd = &cobra.Command{
	Use:   "get <library-id-or-name>",
	Short: "Get one copy library",
	Args:  exactArgs(1),
	RunE:  runCreativeCopyLibraryGet,
}

var creativeCopyLibraryAddFragmentCmd = &cobra.Command{
	Use:   "add-fragment <library-id-or-name>",
	Short: "Add one reviewed copy fragment to a copy library draft",
	Args:  exactArgs(1),
	RunE:  runCreativeCopyLibraryAddFragment,
}

var creativeCopyLibraryUpdateFragmentCmd = &cobra.Command{
	Use:   "update-fragment <library-id-or-name> <fragment-key-or-id>",
	Short: "Update one copy fragment in a copy library draft",
	Args:  exactArgs(2),
	RunE:  runCreativeCopyLibraryUpdateFragment,
}

var creativeCopyLibraryUpsertRepaymentPlanCmd = &cobra.Command{
	Use:   "upsert-repayment-plan <library-id-or-name>",
	Short: "Add or update one reviewed repayment plan row",
	Args:  exactArgs(1),
	RunE:  runCreativeCopyLibraryUpsertRepaymentPlan,
}

type creativeCopyLibraryResourceCLI struct {
	ID               string         `json:"id"`
	WorkspaceID      string         `json:"workspace_id"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Status           string         `json:"status"`
	Version          int            `json:"version"`
	PublishedVersion int            `json:"published_version"`
	Config           map[string]any `json:"config"`
	PublishedConfig  map[string]any `json:"published_config,omitempty"`
	CreatedBy        string         `json:"created_by"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
}

type creativeCopyLibraryListResponseCLI struct {
	Resources []creativeCopyLibraryResourceCLI `json:"resources"`
}

type creativeCopyGroupDefaults struct {
	Role  string
	Types []string
	Usage string
}

type creativeCopyFragmentPatch struct {
	Text          string
	Name          string
	Group         string
	Status        string
	Key           string
	SemanticGroup string
	Tags          []string
}

type creativeRepaymentPlanPatch struct {
	Key                string
	Principal          int
	TenorMonths        int
	MonthlyInstallment int
	TotalInterest      int
	TotalRepayment     int
	Source             string
	Status             string
	Changed            map[string]bool
}

var creativeCopyGroupDefaultsByKey = map[string]creativeCopyGroupDefaults{
	"standard_headline":  {Role: "headline", Types: []string{"num", "repayment_plan"}, Usage: "core"},
	"core_benefit":       {Role: "benefit", Types: []string{"num", "repayment_plan"}, Usage: "core"},
	"other_benefit":      {Role: "supporting", Types: []string{"num", "repayment_plan"}, Usage: "fallback"},
	"call_to_action":     {Role: "cta", Types: []string{"num", "repayment_plan"}, Usage: "fallback"},
	"repayment_headline": {Role: "headline", Types: []string{"num", "repayment_plan"}, Usage: "core"},
}

func init() {
	creativeCmd.AddCommand(creativeCopyLibraryCmd)
	creativeCopyLibraryCmd.AddCommand(
		creativeCopyLibraryListCmd,
		creativeCopyLibraryGetCmd,
		creativeCopyLibraryAddFragmentCmd,
		creativeCopyLibraryUpdateFragmentCmd,
		creativeCopyLibraryUpsertRepaymentPlanCmd,
	)
	for _, command := range []*cobra.Command{
		creativeCopyLibraryListCmd,
		creativeCopyLibraryGetCmd,
		creativeCopyLibraryAddFragmentCmd,
		creativeCopyLibraryUpdateFragmentCmd,
		creativeCopyLibraryUpsertRepaymentPlanCmd,
	} {
		command.Flags().String("output", "json", "Output format: json")
	}
	for _, command := range []*cobra.Command{creativeCopyLibraryAddFragmentCmd, creativeCopyLibraryUpdateFragmentCmd} {
		command.Flags().String("text", "", "Final reviewed ad copy text")
		command.Flags().String("name", "", "Human-readable meaning or label")
		command.Flags().String("group", "core_benefit", "Copy group: standard_headline, core_benefit, other_benefit, call_to_action, repayment_headline")
		command.Flags().String("status", "approved", "Review status: draft, approved, disabled")
		command.Flags().String("key", "", "Stable fragment key")
		command.Flags().String("semantic-group", "", "Business meaning group")
		command.Flags().StringSlice("tag", nil, "Copy tag (can be repeated or comma-separated)")
		command.Flags().Bool("publish", false, "Publish the copy library after saving the draft")
	}
	_ = creativeCopyLibraryAddFragmentCmd.MarkFlagRequired("text")

	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().String("key", "", "Stable repayment plan key")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Int("principal", 0, "Principal amount")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Int("tenor-months", 0, "Tenor in months")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Int("monthly-installment", 0, "Monthly installment")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Int("total-interest", 0, "Total interest")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Int("total-repayment", 0, "Total repayment")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().String("source", "业务审核计划表", "Source or approval basis")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().String("status", "approved", "Review status: draft, approved, disabled")
	creativeCopyLibraryUpsertRepaymentPlanCmd.Flags().Bool("publish", false, "Publish the copy library after saving the draft")
}

func runCreativeCopyLibraryList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	result, err := listCreativeCopyLibraries(ctx, client)
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runCreativeCopyLibraryGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	resource, err := resolveCreativeCopyLibrary(ctx, client, args[0])
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, resource)
}

func runCreativeCopyLibraryAddFragment(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	resource, err := resolveCreativeCopyLibrary(ctx, client, args[0])
	if err != nil {
		return err
	}
	patch, err := creativeCopyFragmentPatchFromFlags(cmd, false)
	if err != nil {
		return err
	}
	config, err := addCreativeCopyLibraryFragment(resource.Config, patch)
	if err != nil {
		return err
	}
	publish, _ := cmd.Flags().GetBool("publish")
	updated, err := saveCreativeCopyLibraryDraft(ctx, client, resource, config, publish)
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, updated)
}

func runCreativeCopyLibraryUpdateFragment(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	resource, err := resolveCreativeCopyLibrary(ctx, client, args[0])
	if err != nil {
		return err
	}
	patch, err := creativeCopyFragmentPatchFromFlags(cmd, true)
	if err != nil {
		return err
	}
	config, err := updateCreativeCopyLibraryFragment(resource.Config, args[1], patch)
	if err != nil {
		return err
	}
	publish, _ := cmd.Flags().GetBool("publish")
	updated, err := saveCreativeCopyLibraryDraft(ctx, client, resource, config, publish)
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, updated)
}

func runCreativeCopyLibraryUpsertRepaymentPlan(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	resource, err := resolveCreativeCopyLibrary(ctx, client, args[0])
	if err != nil {
		return err
	}
	patch, err := creativeRepaymentPlanPatchFromFlags(cmd)
	if err != nil {
		return err
	}
	config, err := upsertCreativeCopyLibraryRepaymentPlan(resource.Config, patch)
	if err != nil {
		return err
	}
	publish, _ := cmd.Flags().GetBool("publish")
	updated, err := saveCreativeCopyLibraryDraft(ctx, client, resource, config, publish)
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, updated)
}

func listCreativeCopyLibraries(ctx context.Context, client *cli.APIClient) (creativeCopyLibraryListResponseCLI, error) {
	var result creativeCopyLibraryListResponseCLI
	if err := client.GetJSON(ctx, "/api/creative/resources?kind=copy_library", &result); err != nil {
		return result, err
	}
	for index := range result.Resources {
		result.Resources[index].Config = normalizeCreativeCopyLibraryConfig(result.Resources[index].Config)
	}
	return result, nil
}

func resolveCreativeCopyLibrary(ctx context.Context, client *cli.APIClient, ref string) (creativeCopyLibraryResourceCLI, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return creativeCopyLibraryResourceCLI{}, fmt.Errorf("copy library id or name is required")
	}
	result, err := listCreativeCopyLibraries(ctx, client)
	if err != nil {
		return creativeCopyLibraryResourceCLI{}, err
	}
	var matched []creativeCopyLibraryResourceCLI
	for _, resource := range result.Resources {
		if resource.ID == ref || strings.EqualFold(resource.Name, ref) {
			matched = append(matched, resource)
		}
	}
	if len(matched) == 0 {
		return creativeCopyLibraryResourceCLI{}, fmt.Errorf("copy library %q was not found", ref)
	}
	if len(matched) > 1 {
		return creativeCopyLibraryResourceCLI{}, fmt.Errorf("copy library name %q is ambiguous; use the library id", ref)
	}
	return matched[0], nil
}

func saveCreativeCopyLibraryDraft(ctx context.Context, client *cli.APIClient, resource creativeCopyLibraryResourceCLI, config map[string]any, publish bool) (creativeCopyLibraryResourceCLI, error) {
	payload := map[string]any{
		"name":        resource.Name,
		"description": resource.Description,
		"config":      config,
	}
	var updated creativeCopyLibraryResourceCLI
	if err := client.PutJSON(ctx, "/api/creative/resources/"+url.PathEscape(resource.ID), payload, &updated); err != nil {
		return updated, err
	}
	if publish {
		if err := client.PostJSON(ctx, "/api/creative/resources/"+url.PathEscape(resource.ID)+"/publish", nil, &updated); err != nil {
			return updated, err
		}
	}
	updated.Config = normalizeCreativeCopyLibraryConfig(updated.Config)
	return updated, nil
}

func creativeCopyFragmentPatchFromFlags(cmd *cobra.Command, update bool) (creativeCopyFragmentPatch, error) {
	text, _ := cmd.Flags().GetString("text")
	name, _ := cmd.Flags().GetString("name")
	group, _ := cmd.Flags().GetString("group")
	status, _ := cmd.Flags().GetString("status")
	key, _ := cmd.Flags().GetString("key")
	semanticGroup, _ := cmd.Flags().GetString("semantic-group")
	tags, _ := cmd.Flags().GetStringSlice("tag")
	if !update && strings.TrimSpace(text) == "" {
		return creativeCopyFragmentPatch{}, fmt.Errorf("--text is required")
	}
	if update && !cmd.Flags().Changed("group") {
		group = ""
	}
	if update && !cmd.Flags().Changed("status") {
		status = ""
	}
	if strings.TrimSpace(group) != "" {
		if _, err := creativeCopyGroupDefaultsFor(group); err != nil {
			return creativeCopyFragmentPatch{}, err
		}
	} else if !update {
		return creativeCopyFragmentPatch{}, fmt.Errorf("--group is required")
	}
	if strings.TrimSpace(status) != "" {
		if _, err := normalizeCreativeCopyLibraryStatus(status); err != nil {
			return creativeCopyFragmentPatch{}, err
		}
	} else if !update {
		return creativeCopyFragmentPatch{}, fmt.Errorf("--status is required")
	}
	return creativeCopyFragmentPatch{
		Text:          strings.TrimSpace(text),
		Name:          strings.TrimSpace(name),
		Group:         strings.TrimSpace(group),
		Status:        strings.TrimSpace(status),
		Key:           strings.TrimSpace(key),
		SemanticGroup: strings.TrimSpace(semanticGroup),
		Tags:          normalizeCreativeCopyLibraryTags(tags),
	}, nil
}

func creativeRepaymentPlanPatchFromFlags(cmd *cobra.Command) (creativeRepaymentPlanPatch, error) {
	key, _ := cmd.Flags().GetString("key")
	principal, _ := cmd.Flags().GetInt("principal")
	tenorMonths, _ := cmd.Flags().GetInt("tenor-months")
	monthlyInstallment, _ := cmd.Flags().GetInt("monthly-installment")
	totalInterest, _ := cmd.Flags().GetInt("total-interest")
	totalRepayment, _ := cmd.Flags().GetInt("total-repayment")
	source, _ := cmd.Flags().GetString("source")
	status, _ := cmd.Flags().GetString("status")
	if _, err := normalizeCreativeCopyLibraryStatus(status); err != nil {
		return creativeRepaymentPlanPatch{}, err
	}
	changed := map[string]bool{}
	for _, name := range []string{"key", "principal", "tenor-months", "monthly-installment", "total-interest", "total-repayment", "source", "status"} {
		changed[name] = cmd.Flags().Changed(name)
	}
	return creativeRepaymentPlanPatch{
		Key: strings.TrimSpace(key), Principal: principal, TenorMonths: tenorMonths,
		MonthlyInstallment: monthlyInstallment, TotalInterest: totalInterest, TotalRepayment: totalRepayment,
		Source: strings.TrimSpace(source), Status: strings.TrimSpace(status), Changed: changed,
	}, nil
}

func addCreativeCopyLibraryFragment(input map[string]any, patch creativeCopyFragmentPatch) (map[string]any, error) {
	config := normalizeCreativeCopyLibraryConfig(input)
	defaults, err := creativeCopyGroupDefaultsFor(patch.Group)
	if err != nil {
		return nil, err
	}
	status, err := normalizeCreativeCopyLibraryStatus(patch.Status)
	if err != nil {
		return nil, err
	}
	fragments := creativeCopyLibraryObjectSlice(config["fragments"])
	ids := creativeCopyLibraryFieldSet(fragments, "id")
	keys := creativeCopyLibraryFieldSet(fragments, "key")
	seed := firstNonEmpty(patch.Text, patch.Name, patch.Group)
	key := patch.Key
	if key == "" {
		key = creativeCopyLibraryUniqueIdentity(keys, "copy-"+patch.Group, seed)
	} else if keys[key] {
		return nil, fmt.Errorf("copy fragment key %q already exists", key)
	}
	id := creativeCopyLibraryUniqueIdentity(ids, "fragment", firstNonEmpty(key, seed))
	name := patch.Name
	if name == "" {
		name = patch.Text
	}
	fragments = append(fragments, map[string]any{
		"id":             id,
		"key":            key,
		"name":           name,
		"content_group":  patch.Group,
		"creative_types": stringAnySlice(defaults.Types),
		"role":           defaults.Role,
		"semantic_group": patch.SemanticGroup,
		"text":           patch.Text,
		"tags":           stringAnySlice(normalizeCreativeCopyLibraryTags(patch.Tags)),
		"usage":          defaults.Usage,
		"status":         status,
	})
	config["fragments"] = fragments
	return config, nil
}

func updateCreativeCopyLibraryFragment(input map[string]any, ref string, patch creativeCopyFragmentPatch) (map[string]any, error) {
	config := normalizeCreativeCopyLibraryConfig(input)
	fragments := creativeCopyLibraryObjectSlice(config["fragments"])
	ref = strings.TrimSpace(ref)
	index := -1
	for i, fragment := range fragments {
		if creativeCopyLibraryString(fragment["id"]) == ref || creativeCopyLibraryString(fragment["key"]) == ref {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("copy fragment %q was not found", ref)
	}
	fragment := cloneCreativeCopyLibraryMap(fragments[index])
	if patch.Text != "" {
		fragment["text"] = patch.Text
	}
	if patch.Name != "" {
		fragment["name"] = patch.Name
	}
	if patch.Key != "" && patch.Key != creativeCopyLibraryString(fragment["key"]) {
		keys := creativeCopyLibraryFieldSet(fragments, "key")
		if keys[patch.Key] {
			return nil, fmt.Errorf("copy fragment key %q already exists", patch.Key)
		}
		fragment["key"] = patch.Key
	}
	if patch.Group != "" {
		defaults, err := creativeCopyGroupDefaultsFor(patch.Group)
		if err != nil {
			return nil, err
		}
		fragment["content_group"] = patch.Group
		fragment["creative_types"] = stringAnySlice(defaults.Types)
		fragment["role"] = defaults.Role
		fragment["usage"] = defaults.Usage
	}
	if patch.Status != "" {
		status, err := normalizeCreativeCopyLibraryStatus(patch.Status)
		if err != nil {
			return nil, err
		}
		fragment["status"] = status
	}
	if patch.SemanticGroup != "" {
		fragment["semantic_group"] = patch.SemanticGroup
	}
	if len(patch.Tags) > 0 {
		fragment["tags"] = stringAnySlice(normalizeCreativeCopyLibraryTags(patch.Tags))
	}
	fragments[index] = fragment
	config["fragments"] = fragments
	return config, nil
}

func upsertCreativeCopyLibraryRepaymentPlan(input map[string]any, patch creativeRepaymentPlanPatch) (map[string]any, error) {
	config := normalizeCreativeCopyLibraryConfig(input)
	status, err := normalizeCreativeCopyLibraryStatus(patch.Status)
	if err != nil {
		return nil, err
	}
	plan := cloneCreativeCopyLibraryMap(creativeCopyLibraryMap(config["repayment_plan"]))
	entries := creativeCopyLibraryObjectSlice(plan["entries"])
	index := -1
	for i, entry := range entries {
		if patch.Key != "" && creativeCopyLibraryString(entry["key"]) == patch.Key {
			index = i
			break
		}
		if patch.Principal > 0 && patch.TenorMonths > 0 &&
			creativeCopyLibraryInt(entry["principal"]) == patch.Principal &&
			creativeCopyLibraryInt(entry["tenor_months"]) == patch.TenorMonths {
			index = i
			break
		}
	}
	if index < 0 {
		if patch.Key == "" && (patch.Principal <= 0 || patch.TenorMonths <= 0) {
			return nil, fmt.Errorf("new repayment plan requires --key or both --principal and --tenor-months")
		}
		if err := validateCreativeRepaymentPlanAmounts(patch, true); err != nil {
			return nil, err
		}
		ids := creativeCopyLibraryFieldSet(entries, "id")
		key := patch.Key
		if key == "" {
			key = fmt.Sprintf("plan-%d-%d", patch.Principal, patch.TenorMonths)
		}
		if creativeCopyLibraryFieldSet(entries, "key")[key] {
			return nil, fmt.Errorf("repayment plan key %q already exists", key)
		}
		entry := map[string]any{
			"id":                  creativeCopyLibraryUniqueIdentity(ids, "plan", key),
			"key":                 key,
			"principal":           patch.Principal,
			"tenor_months":        patch.TenorMonths,
			"monthly_installment": patch.MonthlyInstallment,
			"total_interest":      patch.TotalInterest,
			"total_repayment":     patch.TotalRepayment,
			"source":              firstNonEmpty(patch.Source, "业务审核计划表"),
			"status":              status,
		}
		entries = append(entries, entry)
	} else {
		entry := cloneCreativeCopyLibraryMap(entries[index])
		if patch.Changed["key"] && patch.Key != creativeCopyLibraryString(entry["key"]) {
			if creativeCopyLibraryFieldSet(entries, "key")[patch.Key] {
				return nil, fmt.Errorf("repayment plan key %q already exists", patch.Key)
			}
			entry["key"] = patch.Key
		}
		if patch.Changed["principal"] {
			entry["principal"] = patch.Principal
		}
		if patch.Changed["tenor-months"] {
			entry["tenor_months"] = patch.TenorMonths
		}
		if patch.Changed["monthly-installment"] {
			entry["monthly_installment"] = patch.MonthlyInstallment
		}
		if patch.Changed["total-interest"] {
			entry["total_interest"] = patch.TotalInterest
		}
		if patch.Changed["total-repayment"] {
			entry["total_repayment"] = patch.TotalRepayment
		}
		if patch.Changed["source"] {
			entry["source"] = patch.Source
		}
		if patch.Changed["status"] {
			entry["status"] = status
		}
		if err := validateCreativeRepaymentPlanEntry(entry); err != nil {
			return nil, err
		}
		entries[index] = entry
	}
	plan["entries"] = entries
	config["repayment_plan"] = plan
	return config, nil
}

func normalizeCreativeCopyLibraryConfig(input map[string]any) map[string]any {
	config := cloneCreativeCopyLibraryMap(input)
	config["schema_version"] = 4
	if creativeCopyLibraryString(config["locale"]) == "" {
		config["locale"] = "id-ID"
	}
	source := cloneCreativeCopyLibraryMap(creativeCopyLibraryMap(config["source"]))
	if creativeCopyLibraryString(source["sync_status"]) != "synced" && creativeCopyLibraryString(source["sync_status"]) != "failed" {
		source["sync_status"] = "pending"
	}
	if _, ok := source["name"]; !ok {
		source["name"] = ""
	}
	if _, ok := source["url"]; !ok {
		source["url"] = ""
	}
	if _, ok := source["note"]; !ok {
		source["note"] = ""
	}
	config["source"] = source
	config["fragments"] = creativeCopyLibraryObjectSlice(config["fragments"])
	config["recipes"] = creativeCopyLibraryObjectSlice(config["recipes"])
	plan := cloneCreativeCopyLibraryMap(creativeCopyLibraryMap(config["repayment_plan"]))
	labels := cloneCreativeCopyLibraryMap(creativeCopyLibraryMap(plan["labels"]))
	labelDefaults := map[string]string{
		"principal":           "Jumlah Pinjaman",
		"tenor":               "Tenor",
		"monthly_installment": "Cicilan per Bulan",
		"total_interest":      "Total Bunga",
		"total_repayment":     "Total Pembayaran",
	}
	for key, value := range labelDefaults {
		if creativeCopyLibraryString(labels[key]) == "" {
			labels[key] = value
		}
	}
	plan["labels"] = labels
	plan["entries"] = creativeCopyLibraryObjectSlice(plan["entries"])
	config["repayment_plan"] = plan
	return config
}

func creativeCopyGroupDefaultsFor(group string) (creativeCopyGroupDefaults, error) {
	group = strings.TrimSpace(group)
	defaults, ok := creativeCopyGroupDefaultsByKey[group]
	if !ok {
		return creativeCopyGroupDefaults{}, fmt.Errorf("unsupported copy group %q", group)
	}
	return defaults, nil
}

func normalizeCreativeCopyLibraryStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "approved"
	}
	switch status {
	case "draft", "approved", "disabled":
		return status, nil
	default:
		return "", fmt.Errorf("unsupported copy status %q", status)
	}
}

func normalizeCreativeCopyLibraryTags(tags []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, tag := range tags {
		for _, item := range strings.Split(tag, ",") {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func validateCreativeRepaymentPlanAmounts(patch creativeRepaymentPlanPatch, requireAll bool) error {
	if requireAll || patch.Changed["principal"] {
		if patch.Principal <= 0 {
			return fmt.Errorf("--principal must be positive")
		}
	}
	if requireAll || patch.Changed["tenor-months"] {
		if patch.TenorMonths <= 0 {
			return fmt.Errorf("--tenor-months must be positive")
		}
	}
	if requireAll || patch.Changed["monthly-installment"] {
		if patch.MonthlyInstallment <= 0 {
			return fmt.Errorf("--monthly-installment must be positive")
		}
	}
	if requireAll || patch.Changed["total-interest"] {
		if patch.TotalInterest < 0 {
			return fmt.Errorf("--total-interest cannot be negative")
		}
	}
	if requireAll || patch.Changed["total-repayment"] {
		if patch.TotalRepayment <= 0 {
			return fmt.Errorf("--total-repayment must be positive")
		}
	}
	return nil
}

func validateCreativeRepaymentPlanEntry(entry map[string]any) error {
	if creativeCopyLibraryString(entry["id"]) == "" || creativeCopyLibraryString(entry["key"]) == "" {
		return fmt.Errorf("repayment plan requires id and key")
	}
	patch := creativeRepaymentPlanPatch{
		Principal:          creativeCopyLibraryInt(entry["principal"]),
		TenorMonths:        creativeCopyLibraryInt(entry["tenor_months"]),
		MonthlyInstallment: creativeCopyLibraryInt(entry["monthly_installment"]),
		TotalInterest:      creativeCopyLibraryInt(entry["total_interest"]),
		TotalRepayment:     creativeCopyLibraryInt(entry["total_repayment"]),
		Changed:            map[string]bool{},
	}
	if err := validateCreativeRepaymentPlanAmounts(patch, true); err != nil {
		return err
	}
	if creativeCopyLibraryString(entry["source"]) == "" {
		return fmt.Errorf("repayment plan requires source")
	}
	if _, err := normalizeCreativeCopyLibraryStatus(creativeCopyLibraryString(entry["status"])); err != nil {
		return err
	}
	return nil
}

func creativeCopyLibraryUniqueIdentity(existing map[string]bool, prefix, seed string) string {
	hash := sha256.Sum256([]byte(prefix + "\n" + seed))
	base := prefix + "-" + hex.EncodeToString(hash[:])[:8]
	candidate := base
	for index := 2; existing[candidate]; index++ {
		candidate = fmt.Sprintf("%s-%d", base, index)
	}
	existing[candidate] = true
	return candidate
}

func creativeCopyLibraryFieldSet(rows []map[string]any, field string) map[string]bool {
	result := map[string]bool{}
	for _, row := range rows {
		if value := creativeCopyLibraryString(row[field]); value != "" {
			result[value] = true
		}
	}
	return result
}

func creativeCopyLibraryObjectSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if row := creativeCopyLibraryMap(item); row != nil {
				result = append(result, cloneCreativeCopyLibraryMap(row))
			}
		}
		return result
	case []map[string]any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			result = append(result, cloneCreativeCopyLibraryMap(item))
		}
		return result
	default:
		return []map[string]any{}
	}
}

func creativeCopyLibraryMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if row, ok := value.(map[string]any); ok {
		return row
	}
	return nil
}

func cloneCreativeCopyLibraryMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func creativeCopyLibraryString(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func creativeCopyLibraryInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func stringAnySlice(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
