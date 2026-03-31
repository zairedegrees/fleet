package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func setupGoProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example"), 0644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:"), 0644)
	os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	os.MkdirAll(filepath.Join(dir, "internal"), 0755)
	os.MkdirAll(filepath.Join(dir, "test"), 0755)
	return dir
}

func setupReactProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"^18.0.0"}}`), 0644)
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.MkdirAll(filepath.Join(dir, "__tests__"), 0755)
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node"), 0644)
	return dir
}

func setupMonorepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"workspaces":["packages/*"]}`), 0644)
	for _, pkg := range []string{"api", "web", "shared", "cli"} {
		pkgDir := filepath.Join(dir, pkg)
		os.MkdirAll(pkgDir, 0755)
		os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"`+pkg+`"}`), 0644)
	}
	os.MkdirAll(filepath.Join(dir, ".github"), 0755)
	return dir
}

func setupFinanceProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("pandas\nnumpy"), 0644)
	os.MkdirAll(filepath.Join(dir, "backtest"), 0755)
	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.MkdirAll(filepath.Join(dir, "notebooks"), 0755)
	return dir
}

func TestScanGoProject(t *testing.T) {
	dir := setupGoProject(t)
	result, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if !contains(result.Languages, "Go") {
		t.Error("expected Go language detected")
	}
	if !result.HasTests {
		t.Error("expected HasTests=true (test/ dir exists)")
	}
	if !result.HasInfra {
		t.Error("expected HasInfra=true (Makefile exists)")
	}
}

func TestScanReactProject(t *testing.T) {
	dir := setupReactProject(t)
	result, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if !contains(result.Languages, "JavaScript/TypeScript") {
		t.Error("expected JavaScript/TypeScript detected")
	}
	if !contains(result.Languages, "TypeScript") {
		t.Error("expected TypeScript detected")
	}
	if !contains(result.Frameworks, "React") {
		t.Error("expected React framework detected")
	}
	if !result.HasTests {
		t.Error("expected HasTests=true")
	}
	if !result.HasDocs {
		t.Error("expected HasDocs=true")
	}
	if !result.HasInfra {
		t.Error("expected HasInfra=true (Dockerfile)")
	}
}

func TestScanMonorepo(t *testing.T) {
	dir := setupMonorepo(t)
	result, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if !result.IsMonorepo {
		t.Error("expected IsMonorepo=true (4 packages)")
	}
	if !result.HasInfra {
		t.Error("expected HasInfra=true (.github)")
	}
}

func TestScanFinanceProject(t *testing.T) {
	dir := setupFinanceProject(t)
	result, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if !result.IsFinance {
		t.Error("expected IsFinance=true (backtest/ dir)")
	}
	if !result.HasData {
		t.Error("expected HasData=true (data/ + notebooks/)")
	}
	if !contains(result.Languages, "Python") {
		t.Error("expected Python detected")
	}
}

func TestScanEmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(result.Languages) != 0 {
		t.Errorf("expected no languages, got %v", result.Languages)
	}
}

func TestSuggestAgentsGoProject(t *testing.T) {
	scan := &ScanResult{
		Languages: []string{"Go"},
		HasTests:  true,
		HasInfra:  true,
	}
	suggestions := SuggestAgents(scan)

	names := suggestNames(suggestions)
	if !contains(names, "dev") {
		t.Error("expected dev agent")
	}
	if !contains(names, "auditor") {
		t.Error("expected auditor agent (tests detected)")
	}
	if !contains(names, "ops") {
		t.Error("expected ops agent (infra detected)")
	}
	if contains(names, "frontend") {
		t.Error("unexpected frontend agent for Go project")
	}
}

func TestSuggestAgentsReactProject(t *testing.T) {
	scan := &ScanResult{
		Languages:  []string{"TypeScript"},
		Frameworks: []string{"React", "Next.js"},
		HasTests:   true,
		HasDocs:    true,
		HasInfra:   true,
	}
	suggestions := SuggestAgents(scan)

	names := suggestNames(suggestions)
	if !contains(names, "dev") {
		t.Error("expected dev")
	}
	if !contains(names, "frontend") {
		t.Error("expected frontend")
	}
	if !contains(names, "ux-designer") {
		t.Error("expected ux-designer")
	}
	if !contains(names, "auditor") {
		t.Error("expected auditor")
	}
	if !contains(names, "ops") {
		t.Error("expected ops")
	}
	if !contains(names, "docs") {
		t.Error("expected docs")
	}
}

func TestSuggestAgentsFinance(t *testing.T) {
	scan := &ScanResult{
		Languages: []string{"Python"},
		IsFinance: true,
		HasData:   true,
	}
	suggestions := SuggestAgents(scan)

	names := suggestNames(suggestions)
	if !contains(names, "quant") {
		t.Error("expected quant agent")
	}
	if !contains(names, "researcher") {
		t.Error("expected researcher agent")
	}
}

func TestSuggestAgentsMonorepo(t *testing.T) {
	scan := &ScanResult{
		Languages:  []string{"TypeScript"},
		IsMonorepo: true,
	}
	suggestions := SuggestAgents(scan)

	names := suggestNames(suggestions)
	if !contains(names, "architect") {
		t.Error("expected architect agent for monorepo")
	}
}

func TestSuggestAgentsMinimal(t *testing.T) {
	scan := &ScanResult{}
	suggestions := SuggestAgents(scan)

	if len(suggestions) != 1 {
		t.Errorf("expected only dev agent for empty project, got %d", len(suggestions))
	}
	if suggestions[0].Agent.Name != "dev" {
		t.Errorf("expected dev, got %s", suggestions[0].Agent.Name)
	}
}

func TestUniqueColors(t *testing.T) {
	scan := &ScanResult{
		Languages:  []string{"TypeScript"},
		Frameworks: []string{"React"},
		HasTests:   true,
		HasDocs:    true,
		HasInfra:   true,
		HasData:    true,
		IsMonorepo: true,
		IsFinance:  true,
	}
	suggestions := SuggestAgents(scan)

	colors := make(map[string]bool)
	for _, s := range suggestions {
		if colors[s.Agent.Color] {
			// Colors can repeat if > 8 agents, but within 8 they should be unique
			if len(suggestions) <= len(palette) {
				t.Errorf("duplicate color %s", s.Agent.Color)
			}
		}
		colors[s.Agent.Color] = true
	}
}

// helpers
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func suggestNames(suggestions []AgentSuggestion) []string {
	var names []string
	for _, s := range suggestions {
		names = append(names, s.Agent.Name)
	}
	return names
}
