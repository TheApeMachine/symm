package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	packageDir, err := filepath.Abs(".")

	if err != nil {
		fatal(err)
	}

	if filepath.Base(packageDir) == "metallibgen" {
		packageDir = filepath.Dir(packageDir)
	}

	tempDir, err := os.MkdirTemp("", "symm-metal-*")

	if err != nil {
		fatal(err)
	}

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	generator := NewGenerator(packageDir, tempDir)

	if err := generator.Generate(); err != nil {
		fatal(err)
	}
}

type Generator struct {
	packageDir string
	tempDir    string
}

func NewGenerator(packageDir string, tempDir string) *Generator {
	return &Generator{
		packageDir: packageDir,
		tempDir:    tempDir,
	}
}

func (generator *Generator) Generate() error {
	sources, err := generator.SourceFiles()

	if err != nil {
		return err
	}

	for _, source := range sources {
		if err := generator.Run("xcrun", generator.MetalArgs(source)...); err != nil {
			return err
		}
	}

	return generator.Run("xcrun", generator.MetallibArgs(sources)...)
}

func (generator *Generator) SourceFiles() ([]string, error) {
	var sources []string

	walkError := filepath.WalkDir(generator.packageDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if entry.Name() == "metallibgen" {
				return filepath.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".metal" {
			return nil
		}

		sources = append(sources, path)

		return nil
	})

	if walkError != nil {
		return nil, walkError
	}

	sort.Strings(sources)

	if len(sources) == 0 {
		return nil, fmt.Errorf("no Metal source files under %s", generator.packageDir)
	}

	return sources, nil
}

func (generator *Generator) MetalArgs(source string) []string {
	args := []string{
		"-sdk",
		"macosx",
		"metal",
	}

	if generator.needsStrictFP(source) {
		args = append(args, "-ffp-contract=off")
	}

	return append(
		args,
		"-c",
		source,
		"-o",
		generator.AirPath(source),
	)
}

func (generator *Generator) needsStrictFP(source string) bool {
	baseName := filepath.Base(source)

	if strings.HasSuffix(baseName, "_apply.metal") {
		return true
	}

	if strings.HasSuffix(baseName, "_stats.metal") {
		return true
	}

	switch baseName {
	case "manifold.metal":
		return true
	default:
		return false
	}
}

func (generator *Generator) MetallibArgs(sources []string) []string {
	args := []string{
		"-sdk",
		"macosx",
		"metallib",
	}

	for _, source := range sources {
		args = append(args, generator.AirPath(source))
	}

	return append(
		args,
		"-o",
		filepath.Join(generator.packageDir, "kernels.metallib"),
	)
}

func (generator *Generator) AirPath(source string) string {
	relativePath, err := filepath.Rel(generator.packageDir, source)

	if err != nil {
		relativePath = filepath.Base(source)
	}

	stem := strings.TrimSuffix(relativePath, filepath.Ext(relativePath))
	stem = strings.ReplaceAll(stem, string(filepath.Separator), "_")

	return filepath.Join(generator.tempDir, stem+".air")
}

func (generator *Generator) Run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", name, args, err)
	}

	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
