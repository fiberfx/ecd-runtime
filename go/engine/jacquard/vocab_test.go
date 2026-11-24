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
	attribution = "Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License."
)

// vocabIssues returns the vocabulary faults of one Go source file: the package clause of the copied
// package, a banned word, and a port marker without the attribution line.
func vocabIssues(name, src string) []string {
	var issues []string
	if pkgClause.MatchString(src) {
		issues = append(issues, name+": package clause of the copied package")
	}
	if w := bannedWords.FindString(src); w != "" {
		issues = append(issues, name+": banned word "+strings.ToLower(w))
	}
	if strings.Contains(src, portMarker) && !strings.Contains(src, attribution) {
		issues = append(issues, name+": port marker without the attribution line")
	}
	return issues
}

// goFiles returns every Go file under dir, as paths below dir, and reports a missing dir as nil.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
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
		if dir == "." && len(files) == 0 {
			t.Fatal("no Go file found in the package directory")
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
	good := "package jacquard\n\n// " + attribution + "\n// " + portMarker + "mask256.go of the cut.\n"
	if issues := vocabIssues("good", good); len(issues) != 0 {
		t.Errorf("a ported file with the attribution line is refused: %v", issues)
	}
}
