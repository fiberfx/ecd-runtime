package jacquard

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The forbidden words and markers are built from parts, so that this file does not contain them.
var (
	pkgClause   = regexp.MustCompile(`(?m)^package ` + `ecs\b`)
	bannedWords = regexp.MustCompile(`(?i)\b(` + `wor` + `ld|ga` + `me|pla` + `yer` + `)\b`)
	portMarker  = "Port of " + "ecs/"
	portLine    = regexp.MustCompile(`(?m)^// Port of .*$`)
	attribution = "Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License."
)

// vocabIssues returns the vocabulary faults of one Go source file: the package clause of the copied
// package, a banned word, and a port marker without the attribution line. A line that begins "// Port of "
// states the path of an Ark source, not vocabulary, so a banned word on that line alone is not a fault.
func vocabIssues(name, src string) []string {
	var issues []string
	if pkgClause.MatchString(src) {
		issues = append(issues, name+": package clause of the copied package")
	}
	if w := bannedWords.FindString(portLine.ReplaceAllString(src, "")); w != "" {
		issues = append(issues, name+": banned word "+strings.ToLower(w))
	}
	if strings.Contains(src, portMarker) && !strings.Contains(src, attribution) {
		issues = append(issues, name+": port marker without the attribution line")
	}
	return issues
}

// goFiles returns every Go file under dir, as paths below dir; a missing dir fails the test.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return files
}

// TestVocabulary reads every Go file under engine/jacquard and ecd from disk (J2A-03). No file keeps the
// package clause of the copied package or a banned word, and a file that ports an Ark file carries the
// attribution line. The violating sources at the end show that each fault is detected.
func TestVocabulary(t *testing.T) {
	scanned := 0
	for _, dir := range []string{".", filepath.Join("..", "..", "ecd")} {
		files := goFiles(t, dir)
		if len(files) == 0 {
			t.Fatalf("no Go file found under %s", dir)
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("reading %s: %v", f, err)
			}
			scanned++
			for _, issue := range vocabIssues(f, string(src)) {
				t.Error(issue)
			}
		}
	}
	t.Logf("scanned %d Go files", scanned)

	bad := map[string]string{
		"package clause": "package " + "ecs\n",
		"banned word":    "package jacquard\n\n// Wor" + "ld holds the entities.\n",
		"port marker":    "package jacquard\n\n// " + portMarker + "mask256.go of the cut.\n",
		"second word":    "package jacquard\n\n// the Pla" + "yer moves.\n",
	}
	for name, src := range bad {
		if len(vocabIssues(name, src)) == 0 {
			t.Errorf("the violating source %q is not detected", name)
		}
	}
	for name, src := range map[string]string{
		"path on a port line":    "package jacquard\n\n// " + attribution + "\n// Port of ecs/wor" + "ld.go of the cut.\n",
		"port line of two files": "package jacquard\n\n// " + attribution + "\n// Port of ecs/storage.go and of ecs/wor" + "ld.go.\n",
	} {
		if issues := vocabIssues(name, src); len(issues) != 0 {
			t.Errorf("a source path on a port line is refused (%s): %v", name, issues)
		}
	}
	for name, src := range map[string]string{
		"banned word after a port line":   "package jacquard\n\n// " + attribution + "\n// Port of ecs/mask256.go.\n// the wor" + "ld moves.\n",
		"port text inside another line":   "package jacquard\n\n// " + attribution + "\nvar x = 1 // Port of ecs/wor" + "ld.go\n",
		"banned word on an indented line": "package jacquard\n\n// " + attribution + "\n\t// Port of ecs/wor" + "ld.go\n",
	} {
		if len(vocabIssues(name, src)) == 0 {
			t.Errorf("the violating source %q is not detected", name)
		}
	}
	good := "package jacquard\n\n// " + attribution + "\n// " + portMarker + "mask256.go of the cut.\n"
	if issues := vocabIssues("good", good); len(issues) != 0 {
		t.Errorf("a ported file with the attribution line is refused: %v", issues)
	}
}
