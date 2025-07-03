package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/launchrctl/launchr/pkg/action"
)

//go:embed templates/*
var templateFS embed.FS

const templatesFilesDir = "templates/files"
const templatesDefinitionDir = "templates/definition"

// templateManager orchestrates a template collection, preparation and delivery
type templateManager struct{}

// getTemplateSubdirectories returns all subdirectories within a given path in the embedded filesystem
func (t *templateManager) getTemplateSubdirectories(dirPath string) ([]string, error) {
	entries, err := fs.ReadDir(templateFS, filepath.Join(templatesFilesDir, dirPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded directory %s: %w", dirPath, err)
	}

	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			subDirPath := path.Join(dirPath, entry.Name())
			directories = append(directories, subDirPath)
			subDirs, err := t.getTemplateSubdirectories(subDirPath)
			if err != nil {
				return nil, err
			}
			directories = append(directories, subDirs...)
		}
	}

	directories = append(directories, dirPath)

	return directories, nil
}

func (t *templateManager) renderTemplates(output string, values *templateValues, templates []*template.Template) error {
	for _, t := range templates {
		outputPath := filepath.Clean(filepath.Join(output, strings.Replace(t.Name(), ".tmpl", "", 1)))
		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
		}

		err = t.Execute(outFile, values)
		if err != nil {
			return err
		}
		_ = outFile.Close()
	}

	return nil
}

// getDefinitionTemplate creates the action.yaml file from templates
func (t *templateManager) getDefinitionTemplate(runtimeType action.DefRuntimeType) (*template.Template, error) {
	tmpl, err := template.New("action.yaml").
		ParseFS(templateFS,
			filepath.Join(templatesDefinitionDir, "action.yaml.tmpl"),
			filepath.Join(templatesDefinitionDir, fmt.Sprintf("%s.yaml.tmpl", runtimeType)),
		)
	if err != nil {
		return nil, err
	}

	var combined string
	var names []string
	for _, t := range tmpl.Templates() {
		names = append(names, fmt.Sprintf("{{template \"%s\" .}}", t.Name()))
	}

	slices.Sort(names)
	combined = strings.Join(names, "\n")

	combinedTmpl, err := tmpl.New("action.yaml").Parse(combined)
	if err != nil {
		return nil, err
	}

	return combinedTmpl, err
}

func (t *templateManager) getRuntimeTemplates(dir string) ([]*template.Template, error) {
	tmpl := template.New("")
	var err error

	patterns := []string{filepath.Join(templatesFilesDir, dir, "*.tmpl")}

	tmpl, err = tmpl.ParseFS(templateFS, patterns...)
	if err != nil {
		return nil, err
	}

	return tmpl.Templates(), err
}
