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
		"appgrowing-material-collector":      "appgrowing-collection-source-map.md",
		"ad-creative-analysis":               "ad-creative-analysis-source-map.md",
		"ad-creative-pre-adaptation":         "ad-creative-pre-adaptation-source-map.md",
		"ad-creative-market-pack-extraction": "ad-creative-market-pack-extraction-source-map.md",
		"ad-creative-plan":                   "ad-creative-plan-source-map.md",
		"ad-creative-production":             "ad-creative-production-source-map.md",
		"ad-creative-direct-edit":            "ad-creative-direct-edit-source-map.md",
		"ad-creative-prime-compose":          "ad-creative-prime-compose-source-map.md",
		"ad-creative-qc":                     "ad-creative-qc-source-map.md",
		"ad-creative-leadership":             "ad-creative-leadership-source-map.md",
		"creative-flow-diagnostician":        "creative-flow-diagnosis-source-map.md",
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
		"ad-creative-pre-adaptation": {"source_analysis", "text_replacements", "production_prompt"},
		"ad-creative-plan":           {"copy_snapshot", "input_snapshot", "source_analysis"},
		"ad-creative-production":     {"copy_snapshot", "prompt_sha256", "request_id"},
		"ad-creative-direct-edit":    {"copy_snapshot", "prompt_sha256", "delivery_mode"},
		"ad-creative-prime-compose":  {"input_snapshot", "prime_composition", "naming_rule"},
		"ad-creative-qc":             {"copy_snapshot", "compose_result", "qc-finalize"},
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
