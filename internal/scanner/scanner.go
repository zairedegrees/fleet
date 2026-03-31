package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// ScanResult contains detected project characteristics.
type ScanResult struct {
	Languages  []string
	Frameworks []string
	Structure  []string // notable directories found
	HasTests   bool
	HasDocs    bool
	HasInfra   bool
	HasData    bool
	IsMonorepo bool
	IsFinance  bool
}

// languageIndicators maps file names to language names.
var languageIndicators = map[string]string{
	"go.mod":           "Go",
	"go.sum":           "Go",
	"package.json":     "JavaScript/TypeScript",
	"tsconfig.json":    "TypeScript",
	"Cargo.toml":       "Rust",
	"requirements.txt": "Python",
	"setup.py":         "Python",
	"pyproject.toml":   "Python",
	"Gemfile":          "Ruby",
	"pom.xml":          "Java",
	"build.gradle":     "Java",
	"mix.exs":          "Elixir",
	"composer.json":    "PHP",
}

// frameworkIndicators maps file patterns to framework names.
var frameworkIndicators = map[string]string{
	"next.config.js":     "Next.js",
	"next.config.ts":     "Next.js",
	"next.config.mjs":    "Next.js",
	"nuxt.config.ts":     "Nuxt",
	"nuxt.config.js":     "Nuxt",
	"vue.config.js":      "Vue",
	"angular.json":       "Angular",
	"svelte.config.js":   "Svelte",
	"remix.config.js":    "Remix",
	"astro.config.mjs":   "Astro",
	"tailwind.config.js": "Tailwind CSS",
	"tailwind.config.ts": "Tailwind CSS",
	"vite.config.ts":     "Vite",
	"vite.config.js":     "Vite",
	"webpack.config.js":  "Webpack",
}

// structureDirs are notable directories to detect.
var structureDirs = []string{
	"src", "lib", "pkg", "internal", "cmd",
	"test", "tests", "spec", "__tests__",
	"docs", "doc", "documentation",
	"deploy", "infra", "terraform", ".github", ".gitlab-ci",
	"docker", "k8s", "kubernetes",
	"data", "models", "notebooks", "ml",
}

// Scan analyzes a project directory and returns detected characteristics.
func Scan(projectDir string) (*ScanResult, error) {
	result := &ScanResult{}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}

	var topLevelDirs []string

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden dirs (except .github, .gitlab-ci)
		if strings.HasPrefix(name, ".") && name != ".github" && name != ".gitlab-ci" {
			if entry.IsDir() {
				continue
			}
		}

		// Check language indicators
		if lang, ok := languageIndicators[name]; ok {
			result.Languages = appendUnique(result.Languages, lang)
		}

		// Check framework indicators
		if fw, ok := frameworkIndicators[name]; ok {
			result.Frameworks = appendUnique(result.Frameworks, fw)
		}

		// Check directories
		if entry.IsDir() {
			topLevelDirs = append(topLevelDirs, name)
			lower := strings.ToLower(name)
			for _, sd := range structureDirs {
				if lower == sd {
					result.Structure = appendUnique(result.Structure, name)
					break
				}
			}
		}
	}

	// Derive flags from structure
	for _, dir := range result.Structure {
		lower := strings.ToLower(dir)
		switch {
		case lower == "test" || lower == "tests" || lower == "spec" || lower == "__tests__":
			result.HasTests = true
		case lower == "docs" || lower == "doc" || lower == "documentation":
			result.HasDocs = true
		case lower == "deploy" || lower == "infra" || lower == "terraform" ||
			lower == "docker" || lower == "k8s" || lower == "kubernetes" ||
			lower == ".github" || lower == ".gitlab-ci":
			result.HasInfra = true
		case lower == "data" || lower == "models" || lower == "notebooks" || lower == "ml":
			result.HasData = true
		}
	}

	// Check for README size as docs indicator
	for _, name := range []string{"README.md", "readme.md", "README.rst"} {
		info, err := os.Stat(filepath.Join(projectDir, name))
		if err == nil && info.Size() > 2048 {
			result.HasDocs = true
			break
		}
	}

	// Check for Dockerfile as infra indicator
	for _, name := range []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "Makefile"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			result.HasInfra = true
			break
		}
	}

	// Monorepo detection: multiple package dirs or workspaces
	packageDirs := 0
	for _, dir := range topLevelDirs {
		subEntries, err := os.ReadDir(filepath.Join(projectDir, dir))
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.Name() == "package.json" || se.Name() == "go.mod" || se.Name() == "Cargo.toml" {
				packageDirs++
				break
			}
		}
	}
	result.IsMonorepo = packageDirs >= 3

	// Finance detection
	for _, dir := range topLevelDirs {
		lower := strings.ToLower(dir)
		if lower == "trading" || lower == "finance" || lower == "quant" || lower == "backtest" {
			result.IsFinance = true
			break
		}
	}

	// Also check for finance-related files
	financeFiles := []string{"backtest", "trading", "portfolio", "hedge", "alpha"}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		for _, ff := range financeFiles {
			if strings.Contains(name, ff) {
				result.IsFinance = true
				break
			}
		}
	}

	// Check frontend frameworks via package.json
	pkgPath := filepath.Join(projectDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		content := string(data)
		if strings.Contains(content, "\"react\"") {
			result.Frameworks = appendUnique(result.Frameworks, "React")
		}
		if strings.Contains(content, "\"vue\"") {
			result.Frameworks = appendUnique(result.Frameworks, "Vue")
		}
		if strings.Contains(content, "\"svelte\"") {
			result.Frameworks = appendUnique(result.Frameworks, "Svelte")
		}
		if strings.Contains(content, "\"@angular/core\"") {
			result.Frameworks = appendUnique(result.Frameworks, "Angular")
		}
		if strings.Contains(content, "\"express\"") {
			result.Frameworks = appendUnique(result.Frameworks, "Express")
		}
	}

	return result, nil
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
