package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
)

const (
	runtimePlugin    action.DefRuntimeType = "plugin"
	runtimeContainer action.DefRuntimeType = "container"
	runtimeShell     action.DefRuntimeType = "shell"
)

// generator handles generating action files from templates
type generator struct {
	dirManager  *directoryManager
	tmplManager *templateManager
}

// newGenerator creates a new generator instance
func newGenerator(prefix string) *generator {
	return &generator{
		dirManager:  newDirectoryManager(prefix),
		tmplManager: &templateManager{},
	}
}

func (g *generator) generate(values *templateValues) error {
	// Create an output directory if it doesn't exist
	actionDir, err := g.dirManager.ensureActionDir(values.ID)
	if err != nil {
		return err
	}

	launchr.Term().Info().Printfln("Generating action in %s", actionDir)

	success := false
	// Defer cleanup that will run at the end of the function
	defer func() {
		if !success && actionDir != "" {
			// Safety check: Verify actionDir is within our expected directory structure
			expectedPrefix := g.dirManager.prefix
			if !strings.HasPrefix(actionDir, expectedPrefix) {
				launchr.Term().Warning().Printfln("Not removing directory outside of expected path: %s", actionDir)
				return
			}

			// Additional check: Verify the directory exists before attempting removal
			if info, err := os.Stat(actionDir); err != nil || !info.IsDir() {
				return // Directory doesn't exist or isn't a directory, no need to remove
			}

			// Proceed with removal after all safety checks
			if err = os.RemoveAll(actionDir); err != nil {
				launchr.Term().Warning().Printfln("Failed to clean up directory %s: %v", actionDir, err)
			}
		}
	}()

	err = g.generateDefinition(actionDir, values)
	if err != nil {
		return err
	}

	err = g.generateFiles(actionDir, values)
	if err != nil {
		return err
	}

	// Mark the operation as successful
	success = true

	launchr.Term().Success().Printfln(
		"Action %s successfully generated in %s",
		values.ID,
		actionDir,
	)
	return nil
}

func (g *generator) generateDefinition(outputDir string, values *templateValues) error {
	yamlTemplate, err := g.tmplManager.getDefinitionTemplate(values.Runtime.Type)
	if err != nil {
		return fmt.Errorf("failed to generate action.yaml: %w", err)
	}

	templates := []*template.Template{yamlTemplate}
	return g.tmplManager.renderTemplates(outputDir, values, templates)
}

func (g *generator) generateFiles(output string, values *templateValues) error {
	filesDir := string(values.Runtime.Type)
	if values.Runtime.Type == runtimeContainer {
		filesDir = fmt.Sprintf("%s/%s", values.Runtime.Type, values.ContainerPreset)
	}

	dirs, err := g.tmplManager.getTemplateSubdirectories(filesDir)
	if err != nil {
		return err
	}

	dirs = append(dirs, filesDir)
	for _, d := range dirs {
		outputDir := filepath.Join(output, strings.TrimPrefix(d, filesDir))
		err = ensureDir(outputDir)
		if err != nil {
			return err
		}

		templates, err := g.tmplManager.getRuntimeTemplates(d)
		if err != nil {
			if strings.Contains(err.Error(), "template: pattern matches no files") {
				continue
			}

			return err
		}
		err = g.tmplManager.renderTemplates(outputDir, values, templates)
		if err != nil {
			return err
		}
	}

	return nil
}

// directoryManager handles the action directory creation and validation
type directoryManager struct {
	prefix string // Base prefix where actions will be stored
}

func newDirectoryManager(prefix string) *directoryManager {
	return &directoryManager{
		prefix: prefix,
	}
}

// getActionDir returns the full path for an action directory
func (dm *directoryManager) getActionDir(actionID string) string {
	// Sanitize the action ID for use as a directory name
	safeID := sanitizeForPath(actionID)
	return filepath.Join(dm.prefix, safeID)
}

// ensureActionDir ensures the action directory exists and is empty/unique
// Returns the created directory path or an error
func (dm *directoryManager) ensureActionDir(actionID string) (string, error) {
	// Get the full path for the action
	actionDir := dm.getActionDir(actionID)
	err := ensureDir(actionDir)
	return actionDir, err
}

func ensureDir(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0750); err != nil {
		return fmt.Errorf("failed to create action directory: %w", err)
	}

	return nil
}

// sanitizeForPath sanitizes a string for use as a directory name
func sanitizeForPath(s string) string {
	// Replace non-alphanumeric characters with hyphens
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)

	// Convert multiple consecutive hyphens to a single hyphen
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	// Trim hyphens from start and end
	s = strings.Trim(s, "-")

	// Ensure the result is not empty
	if s == "" {
		s = "action"
	}

	return s
}
