package gen_test

// init adds the facade-mode case to the goldens of TestGolden: the generated package of go/ecr/internal/llm must equal
// what Generate makes of the llm catalog in the mode ecr. The case names the file by its relative path; no Go file
// outside go/ecr holds the import path of package ecr.
func init() {
	goldens = append(goldens, golden{"ecr", "../llm/llm.catalog.json", "ecr", "../../ecr/internal/llm/llm_gen.go"})
}
