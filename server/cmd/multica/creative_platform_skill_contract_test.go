package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreativePlatformSkillsStayNativeAndMapped(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skillRoot := filepath.Join(repoRoot, "scripts", "creative-platform-skills")

	sourceMaps := map[string]string{
		"appgrowing-material-collector": "appgrowing-collection-source-map.md",
		"ad-creative-analysis":          "ad-creative-analysis-source-map.md",
		"ad-creative-pre-adaptation":    "ad-creative-pre-adaptation-source-map.md",
		"ad-creative-plan":              "ad-creative-plan-source-map.md",
		"ad-creative-production":        "ad-creative-production-source-map.md",
		"ad-creative-prime-compose":     "source-map.md",
		"ad-creative-direct-edit":       "ad-creative-direct-edit-source-map.md",
		"ad-creative-qc":                "ad-creative-qc-source-map.md",
		"ad-creative-leadership":        "ad-creative-leadership-source-map.md",
		"creative-flow-diagnostician":   "creative-flow-diagnosis-source-map.md",
	}

	for skillName, sourceMapName := range sourceMaps {
		skillName, sourceMapName := skillName, sourceMapName
		t.Run(skillName, func(t *testing.T) {
			t.Parallel()

			content := readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "SKILL.md"))
			assertNoRepeatedLongSkillLines(t, content)
			for _, forbidden := range []string{
				"multica creative material get",
				"multica creative materials",
				"--help",
				"Rp80.000.000",
				"0,03%",
				"https://example.com",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("SKILL.md contains forbidden runtime probe or business example %q", forbidden)
				}
			}

			readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "references", sourceMapName))
		})
	}
}

func TestCreativePlatformSkillsUseFrozenBusinessInputs(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	skillRoot := filepath.Join(filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..")), "scripts", "creative-platform-skills")
	required := map[string][]string{
		"ad-creative-pre-adaptation": {"source_analysis", "text_replacements", "visual_direction", "recommendation_basis", "repayment_plan_selections", "numeric_layouts", "render_instruction", "完整展示字符串", "pre-adaptation-put", "min(", "我方优先、数量取小"},
		"ad-creative-plan":           {"copy_snapshot", "input_snapshot", "source_analysis"},
		"ad-creative-production":     {"copy_snapshot", "prompt_sha256", "request_id"},
		"ad-creative-direct-edit":    {"copy_snapshot", "prompt_sha256", "delivery_mode"},
		"ad-creative-qc":             {"copy_snapshot", "compose_result", "qc-finalize"},
		"ad-creative-prime-compose":  {"prime-compose", "creative_prime_backend.go", "does not edit model prompts"},
	}

	for skillName, terms := range required {
		content := readCreativePlatformContractFile(t, filepath.Join(skillRoot, skillName, "SKILL.md"))
		for _, term := range terms {
			if !strings.Contains(content, term) {
				t.Errorf("%s/SKILL.md must reference frozen/runtime contract %q", skillName, term)
			}
		}
	}
}

func TestAppGrowingCollectorContractKeepsBusinessSemanticsAndSingleSubmission(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	skillRoot := filepath.Join(root, "scripts", "creative-platform-skills", "appgrowing-material-collector")
	skill := readCreativePlatformContractFile(t, filepath.Join(skillRoot, "SKILL.md"))
	contract := readCreativePlatformContractFile(t, filepath.Join(skillRoot, "references", "appgrowing-collection-contract.md"))
	combined := skill + "\n" + contract

	for _, required := range []string{
		"selection_rules.new_materials",
		"selection_rules.volume_materials",
		"asset_type=image",
		"crawl_run_id",
		"delegate_preanalysis.py",
		"multica task fanout unavailable; upgrade CLI",
		"真实阶段、error code/message",
		"同一 task 最多创建一个 Crawl Run",
		"timeout_ms",
		"crawl_command_wait_incomplete",
		"worker_busy",
		"credential broker worker is busy",
		"crawler worker is busy",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("collector contract must contain %q", required)
		}
	}

	bootstrap := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "bootstrap-creative-platform-demo.ps1"))
	for _, required := range []string{
		"capability = 'material_collection'; version = 16",
		"Set-AgentDefinition -Name '素材_采集'",
		"-MaxConcurrentTasks 1",
	} {
		if !strings.Contains(bootstrap, required) {
			t.Errorf("bootstrap collector setup must contain %q", required)
		}
	}

	installation := readCreativePlatformContractFile(t, filepath.Join(root, "server", "internal", "handler", "creative_factory_installation.go"))
	for _, required := range []string{
		`Capability: "material_collection", Version: 16`,
		`Role: "collection", Name: "素材_采集"`,
		`MaxConcurrent: 1`,
	} {
		if !strings.Contains(installation, required) {
			t.Errorf("creative factory collector setup must contain %q", required)
		}
	}
}

func TestCreativeProductionPromptTemplateDefinesInputRolesAndVerticalCompression(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	content := readCreativePlatformContractFile(t, filepath.Join(root, "scripts", "creative-platform-skills", "ad-creative-production", "SKILL.md"))
	for _, required := range []string{
		"COMPOSITION GATE",
		"Input 2 is the current-size official Prime visual context",
		"Do not swap input roles",
		"Input 1 is the downloaded candidate reference",
		"Input 1 is the approved square base from this Variant",
		"failed",
		"unbranded base for this size",
		"compress vertically along the Y axis",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("production prompt template must contain %q", required)
		}
	}
}

func readCreativePlatformContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertNoRepeatedLongSkillLines(t *testing.T, content string) {
	t.Helper()
	seen := map[string]int{}
	for _, line := range strings.Split(content, "\n") {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if len(normalized) < 80 || strings.HasPrefix(normalized, "```") || strings.HasPrefix(normalized, "multica ") {
			continue
		}
		seen[normalized]++
		if seen[normalized] > 1 {
			t.Fatalf("SKILL.md repeats long instruction line: %q", normalized)
		}
	}
}
