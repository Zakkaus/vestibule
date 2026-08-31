package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	maxFileLines     = 600
	maxFunctionLines = 80
	maxComplexity    = 15

	baselineFile   = "scripts/baseline.txt"
	boundariesFile = "scripts/boundaries.txt"
	gocycloCommand = "github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0"
)

const (
	fileLinesKind  = "file-lines"
	functionKind   = "function-lines"
	complexityKind = "cyclomatic-complexity"
	boundaryKind   = "package-boundary"
)

type metric struct {
	kind  string
	path  string
	line  int
	name  string
	value int
}

type boundaryRule struct {
	directory string
	prefix    string
}

func main() {
	if len(os.Args) > 2 || (len(os.Args) == 2 && os.Args[1] != "--print-baseline") {
		fail(errors.New("usage: lint.sh [--print-baseline]"))
	}

	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	findings, err := collectFindings(root)
	if err != nil {
		fail(err)
	}
	if len(os.Args) == 2 {
		printBaseline(findings)
		return
	}

	baseline, err := loadBaseline(filepath.Join(root, baselineFile))
	if err != nil {
		fail(err)
	}
	failures, err := compareFindings(findings, baseline)
	if err != nil {
		fail(err)
	}
	if len(failures) == 0 {
		// A frozen size finding is debt: the phase that owns the file pays it down.
		// A frozen package-boundary finding is a violated invariant, and reporting it
		// as a plain pass is how a check ends up permanently green over a broken rule.
		// Report the two separately so the count stays visible on every run.
		if held := countHeld(baseline, boundaryKind); held > 0 {
			fmt.Printf("lint: passed, holding %d baselined %s violations — these break an invariant and are due in the phase that owns them\n", held, boundaryKind)
			return
		}
		fmt.Println("lint: passed")
		return
	}
	for _, failure := range failures {
		fmt.Fprintln(os.Stderr, failure)
	}
	os.Exit(1)
}

// countHeld reports how many baselined findings of one kind are still present.
func countHeld(baseline map[string]metric, kind string) int {
	n := 0
	for _, finding := range baseline {
		if finding.kind == kind {
			n++
		}
	}
	return n
}

func printBaseline(findings []metric) {
	fmt.Println("# Existing lint findings, frozen for the phase-zero snapshot.")
	fmt.Println("# Fields: kind<TAB>path<TAB>line<TAB>name<TAB>value")
	for _, finding := range findings {
		fmt.Printf("%s\t%s\t%d\t%s\t%d\n", finding.kind, finding.path, finding.line, finding.name, finding.value)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lint:", err)
	os.Exit(1)
}

func collectFindings(root string) ([]metric, error) {
	files, err := goFiles(root)
	if err != nil {
		return nil, err
	}
	rules, err := loadBoundaryRules(filepath.Join(root, boundariesFile))
	if err != nil {
		return nil, err
	}
	fileFindings, err := fileLineFindings(files)
	if err != nil {
		return nil, err
	}
	functionFindings, err := functionLineFindings(files)
	if err != nil {
		return nil, err
	}
	boundaryFindings, err := packageBoundaryFindings(files, rules)
	if err != nil {
		return nil, err
	}
	complexityFindings, err := cyclomaticFindings(root, files)
	if err != nil {
		return nil, err
	}

	findings := append(fileFindings, functionFindings...)
	findings = append(findings, complexityFindings...)
	findings = append(findings, boundaryFindings...)
	sortMetrics(findings)
	return findings, nil
}

func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find Go files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "testdata", "vendor":
		return true
	default:
		return false
	}
}

func fileLineFindings(files []string) ([]metric, error) {
	findings := make([]metric, 0)
	for _, path := range files {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		lines := lineCount(contents)
		if lines > maxFileLines {
			findings = append(findings, metric{kind: fileLinesKind, path: path, value: lines})
		}
	}
	return findings, nil
}

func lineCount(contents []byte) int {
	if len(contents) == 0 {
		return 0
	}
	lines := bytes.Count(contents, []byte{'\n'})
	if contents[len(contents)-1] != '\n' {
		lines++
	}
	return lines
}

func functionLineFindings(files []string) ([]metric, error) {
	fileSet := token.NewFileSet()
	findings := make([]metric, 0)
	for _, path := range files {
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			lines := fileSet.Position(function.End()).Line - fileSet.Position(function.Pos()).Line + 1
			if lines <= maxFunctionLines {
				continue
			}
			name, err := declarationName(fileSet, function)
			if err != nil {
				return nil, fmt.Errorf("format %s: %w", path, err)
			}
			findings = append(findings, metric{
				kind:  functionKind,
				path:  path,
				line:  fileSet.Position(function.Pos()).Line,
				name:  name,
				value: lines,
			})
		}
	}
	return findings, nil
}

func declarationName(fileSet *token.FileSet, function *ast.FuncDecl) (string, error) {
	if function.Recv == nil {
		return function.Name.Name, nil
	}
	var receiver bytes.Buffer
	if err := format.Node(&receiver, fileSet, function.Recv.List[0].Type); err != nil {
		return "", err
	}
	return "(" + receiver.String() + ")." + function.Name.Name, nil
}

func packageBoundaryFindings(files []string, rules []boundaryRule) ([]metric, error) {
	fileSet := token.NewFileSet()
	findings := make([]metric, 0)
	for _, path := range files {
		directory := filepath.ToSlash(filepath.Dir(path))
		if !hasRuleForDirectory(directory, rules) {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return nil, fmt.Errorf("parse import in %s: %w", path, err)
			}
			if !forbiddenImport(directory, importPath, rules) {
				continue
			}
			findings = append(findings, metric{
				kind: boundaryKind,
				path: path,
				line: fileSet.Position(imported.Pos()).Line,
				name: importPath,
			})
		}
	}
	return findings, nil
}

func hasRuleForDirectory(directory string, rules []boundaryRule) bool {
	for _, rule := range rules {
		if directoryMatches(directory, rule.directory) {
			return true
		}
	}
	return false
}

func forbiddenImport(directory, importPath string, rules []boundaryRule) bool {
	for _, rule := range rules {
		if directoryMatches(directory, rule.directory) && importMatches(importPath, rule.prefix) {
			return true
		}
	}
	return false
}

func directoryMatches(directory, target string) bool {
	return directory == target || strings.HasPrefix(directory, target+"/")
}

func importMatches(importPath, prefix string) bool {
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

func loadBoundaryRules(path string) ([]boundaryRule, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open boundary rules: %w", err)
	}
	defer file.Close()

	var rules []boundaryRule
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse boundary rules line %d", line)
		}
		rules = append(rules, boundaryRule{directory: fields[0], prefix: fields[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read boundary rules: %w", err)
	}
	return rules, nil
}

func cyclomaticFindings(root string, files []string) ([]metric, error) {
	command := exec.Command("go", "run", gocycloCommand)
	command.Args = append(command.Args, "-over", strconv.Itoa(maxComplexity), "--")
	command.Args = append(command.Args, files...)
	command.Dir = root

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil && !expectedGocycloExit(err, output.Bytes()) {
		return nil, fmt.Errorf("run gocyclo: %w: %s", err, strings.TrimSpace(diagnostics.String()))
	}

	findings := make([]metric, 0)
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		finding, err := parseGocycloLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read gocyclo output: %w", err)
	}
	return findings, nil
}

func expectedGocycloExit(err error, output []byte) bool {
	if err == nil {
		return true
	}
	var exitError *exec.ExitError
	return errors.As(err, &exitError) && exitError.ExitCode() == 1 && len(bytes.TrimSpace(output)) != 0
}

func parseGocycloLine(text string) (metric, error) {
	fields := strings.Fields(text)
	if len(fields) != 4 {
		return metric{}, fmt.Errorf("parse gocyclo output %q", text)
	}
	value, err := strconv.Atoi(fields[0])
	if err != nil {
		return metric{}, fmt.Errorf("parse gocyclo complexity %q: %w", fields[0], err)
	}
	path, line, err := gocycloLocation(fields[3])
	if err != nil {
		return metric{}, err
	}
	return metric{kind: complexityKind, path: path, line: line, name: fields[2], value: value}, nil
}

func gocycloLocation(location string) (string, int, error) {
	columnSeparator := strings.LastIndex(location, ":")
	if columnSeparator == -1 {
		return "", 0, fmt.Errorf("parse gocyclo location %q", location)
	}
	lineAndPath := location[:columnSeparator]
	lineSeparator := strings.LastIndex(lineAndPath, ":")
	if lineSeparator == -1 {
		return "", 0, fmt.Errorf("parse gocyclo location %q", location)
	}
	line, err := strconv.Atoi(lineAndPath[lineSeparator+1:])
	if err != nil {
		return "", 0, fmt.Errorf("parse gocyclo location %q: %w", location, err)
	}
	path := filepath.ToSlash(filepath.Clean(lineAndPath[:lineSeparator]))
	return path, line, nil
}

func loadBaseline(path string) (map[string]metric, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer file.Close()

	baseline := make(map[string]metric)
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		finding, err := parseBaselineLine(text)
		if err != nil {
			return nil, fmt.Errorf("parse baseline line %d: %w", line, err)
		}
		key := metricKey(finding)
		if _, exists := baseline[key]; exists {
			return nil, fmt.Errorf("duplicate baseline entry for %s", key)
		}
		baseline[key] = finding
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read baseline: %w", err)
	}
	return baseline, nil
}

func parseBaselineLine(text string) (metric, error) {
	fields := strings.Split(text, "\t")
	if len(fields) != 5 {
		return metric{}, errors.New("expected five tab-separated fields")
	}
	if !knownKind(fields[0]) {
		return metric{}, fmt.Errorf("unknown kind %q", fields[0])
	}
	line, err := strconv.Atoi(fields[2])
	if err != nil {
		return metric{}, fmt.Errorf("parse line %q: %w", fields[2], err)
	}
	value, err := strconv.Atoi(fields[4])
	if err != nil {
		return metric{}, fmt.Errorf("parse value %q: %w", fields[4], err)
	}
	return metric{kind: fields[0], path: fields[1], line: line, name: fields[3], value: value}, nil
}

func knownKind(kind string) bool {
	switch kind {
	case fileLinesKind, functionKind, complexityKind, boundaryKind:
		return true
	default:
		return false
	}
}

func compareFindings(findings []metric, baseline map[string]metric) ([]string, error) {
	failures := make([]string, 0)
	seen := make(map[string]struct{})
	for _, finding := range findings {
		key := metricKey(finding)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate finding for %s", key)
		}
		seen[key] = struct{}{}
		previous, exists := baseline[key]
		if !exists {
			failures = append(failures, newFindingMessage(finding))
			continue
		}
		if finding.kind != boundaryKind && finding.value > previous.value {
			failures = append(failures, worsenedFindingMessage(finding, previous.value))
		}
	}
	return failures, nil
}

func metricKey(finding metric) string {
	return strings.Join([]string{finding.kind, finding.path, finding.name}, "\t")
}

func newFindingMessage(finding metric) string {
	switch finding.kind {
	case fileLinesKind:
		return fmt.Sprintf("FAIL file-lines: new %s has %d lines (limit %d)", finding.path, finding.value, maxFileLines)
	case functionKind:
		return fmt.Sprintf("FAIL function-lines: new %s:%d %s has %d lines (limit %d)", finding.path, finding.line, finding.name, finding.value, maxFunctionLines)
	case complexityKind:
		return fmt.Sprintf("FAIL cyclomatic-complexity: new %s:%d %s has complexity %d (limit %d)", finding.path, finding.line, finding.name, finding.value, maxComplexity)
	default:
		return fmt.Sprintf("FAIL package-boundary: new %s:%d imports %s", finding.path, finding.line, finding.name)
	}
}

func worsenedFindingMessage(finding metric, baseline int) string {
	switch finding.kind {
	case fileLinesKind:
		return fmt.Sprintf("FAIL file-lines: worsened %s has %d lines (baseline %d; limit %d)", finding.path, finding.value, baseline, maxFileLines)
	case functionKind:
		return fmt.Sprintf("FAIL function-lines: worsened %s:%d %s has %d lines (baseline %d; limit %d)", finding.path, finding.line, finding.name, finding.value, baseline, maxFunctionLines)
	default:
		return fmt.Sprintf("FAIL cyclomatic-complexity: worsened %s:%d %s has complexity %d (baseline %d; limit %d)", finding.path, finding.line, finding.name, finding.value, baseline, maxComplexity)
	}
}

func sortMetrics(metrics []metric) {
	sort.Slice(metrics, func(left, right int) bool {
		if metrics[left].kind != metrics[right].kind {
			return metrics[left].kind < metrics[right].kind
		}
		if metrics[left].path != metrics[right].path {
			return metrics[left].path < metrics[right].path
		}
		return metrics[left].name < metrics[right].name
	})
}
