package scaffold

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
	"github.com/launchrctl/launchr/pkg/jsonschema"
)

// metadataCollector handles the action data collection.
type metadataCollector struct {
	actionManager action.Manager
	interactive   bool
}

type templateValues struct {
	*action.Definition
	ID              string
	ContainerPreset string
}

// newMetadataCollector creates a new form generator
func newMetadataCollector(manager action.Manager, interactive bool) *metadataCollector {
	return &metadataCollector{
		actionManager: manager,
		interactive:   interactive,
	}
}

func (m *metadataCollector) validate(values *templateValues) error {
	if values.ID == "" {
		return fmt.Errorf("ID can't be empty")
	}

	values.ID = sanitizeForPath(values.ID)
	err := isValidName("action ID", values.ID)
	if err != nil {
		return err
	}

	_, ok := m.actionManager.Get(values.ID)
	if ok {
		return fmt.Errorf("action with ID '%s' already exists", values.ID)
	}

	return nil
}

// collectActionInfo interactively collects action information
func (m *metadataCollector) collectActionInfo(values *templateValues) (*templateValues, error) {
	if m.interactive {
		launchr.Term().Info().Printfln("Running in interactive mode. Please fill the following fields. Press enter to skip a question.")
		err := m.attachInteractiveForm(values)
		if err != nil {
			return nil, err
		}
	}

	if values.Runtime.Type == runtimeContainer {
		values.Runtime.Container.Image = fmt.Sprintf("%s:latest", values.ID)
	}

	err := m.validate(values)
	if err != nil {
		return values, err
	}

	return values, nil
}

func (m *metadataCollector) attachInteractiveForm(values *templateValues) error {
	var aliasesStr string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Description("Human-readable title for the action").
				Placeholder("My Action").
				Value(&values.Action.Title).
				Validate(func(str string) error {
					if strings.TrimSpace(str) == "" {
						return errors.New("title can't be empty")
					}

					return nil
				}),
			huh.NewText().
				Title("Description").
				Description("Detailed description of what the action does").
				Placeholder("This action...").
				Lines(2).
				Value(&values.Action.Description),
			huh.NewInput().
				Title("Aliases").
				Description("Comma-separated list of alternative names").
				Placeholder("myaction, ma").
				Value(&aliasesStr),
			huh.NewSelect[action.DefRuntimeType]().
				Title("Runtime").
				Description("Runtime type for the action").
				Options(
					huh.NewOption("Plugin", runtimePlugin),
					huh.NewOption("Container", runtimeContainer),
					huh.NewOption("Shell", runtimeShell),
				).
				Value(&values.Runtime.Type),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Working directory").
				Description("Runtime type for the action").
				Options(
					huh.NewOption("Actions base dir", "{{ .actions_base_dir }})"),
					huh.NewOption("Current working dir", "{{ .current_working_dir }}"),
				).
				Value(&values.WD),
			huh.NewSelect[string]().
				Title("- Choose container files preset").
				Options(
					huh.NewOption("Golang", "go"),
					huh.NewOption("Python", "py"),
					huh.NewOption("Shell", "sh"),
				).
				Value(&values.ContainerPreset),
		).WithHideFunc(func() bool { return values.Runtime.Type != runtimeContainer }),

		huh.NewGroup(
			huh.NewInput().
				Title("Action ID").
				Description("Unique identifier for the action").
				Placeholder("my-action").
				Validate(func(str string) error {
					err := isValidName("action ID", str)
					if err != nil {
						return err
					}

					safeID := sanitizeForPath(str)
					_, ok := m.actionManager.Get(safeID)
					if ok {
						return fmt.Errorf("action with ID '%s' already exists", safeID)
					}

					return nil
				}).
				Value(&values.ID),
		),
	)

	err := form.Run()
	if err != nil {
		return err
	}

	// Parse aliases
	if aliasesStr != "" {
		aliases := strings.Split(aliasesStr, ",")
		aliases = slices.Compact(aliases)
		for _, alias := range aliases {
			values.Action.Aliases = append(values.Action.Aliases, strings.TrimSpace(alias))
		}
	}

	// Collect arguments
	addArgs := false
	// Ask if the user wants to add more parameters
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Would you like to add arguments?").
				Value(&addArgs),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("form error: %w", err)
	}

	if addArgs {
		err = m.collectParameters("Arguments", &values.Action.Arguments)
		if err != nil {
			return err
		}
	}

	// Collect options
	addOpts := false
	// Ask if the user wants to add more parameters
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Would you like to add options?").
				Value(&addOpts),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("form error: %w", err)
	}

	if addOpts {
		err = m.collectParameters("Options", &values.Action.Options)
		if err != nil {
			return err
		}
	}

	// Collect runtime-specific configuration
	err = m.collectRuntimeData(values)
	if err != nil {
		return err
	}

	return err
}

func (m *metadataCollector) collectRuntimeData(values *templateValues) error {
	switch values.Runtime.Type {
	case runtimeContainer:
		container, err := m.collectContainerConfig()
		if err != nil {
			return err
		}
		values.Runtime.Container = container
	case runtimeShell:
		shell, err := m.collectShellConfig()
		if err != nil {
			return err
		}
		values.Runtime.Shell = shell
	}

	return nil
}

// collectParameters collects parameters (arguments or options)
func (m *metadataCollector) collectParameters(paramType string, params *action.ParametersList) error {
	var addMore = true

	for addMore {
		param := &action.DefParameter{
			Items: &action.DefArrayItems{Type: jsonschema.String},
		}
		var defaultStr string

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Parameter Name").
					Description(fmt.Sprintf("Name for this %s", paramType)).
					Placeholder("name").
					Value(&param.Name).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("name can't be empty")
						}

						err := isValidName("parameter", str)
						if err != nil {
							return err
						}

						for _, p := range *params {
							if p.Name == str {
								return fmt.Errorf("parameter with name '%s' already exists", str)
							}
						}

						return nil
					}),
				huh.NewInput().
					Title("Title").
					Description("Human-readable title").
					Placeholder("Name").
					Value(&param.Title),
				huh.NewText().
					Title("Description").
					Description("Detailed description").
					Lines(2).
					Placeholder("The name of...").
					Value(&param.Description),
				huh.NewSelect[jsonschema.Type]().
					Title("Type").
					Description("Data type for this parameter").
					Options(
						huh.NewOption("String", jsonschema.String),
						huh.NewOption("Number", jsonschema.Number),
						huh.NewOption("Integer", jsonschema.Integer),
						huh.NewOption("Boolean", jsonschema.Boolean),
						huh.NewOption("Array", jsonschema.Array),
					).
					Value(&param.Type),
				huh.NewSelect[bool]().
					Title("Required").
					Description("Is this parameter required?").
					Options(
						huh.NewOption("Yes", true),
						huh.NewOption("No", false),
					).
					Value(&param.Required),
			),
			huh.NewGroup(
				huh.NewSelect[jsonschema.Type]().
					Title("Items Type").
					Description("Data type of array items").
					Options(
						huh.NewOption("String", jsonschema.String),
						huh.NewOption("Number", jsonschema.Number),
						huh.NewOption("Integer", jsonschema.Integer),
						huh.NewOption("Boolean", jsonschema.Boolean),
					).
					Value(&param.Items.Type),
				huh.NewInput().
					Title("Default Value (optional)").
					Validate(func(v string) error {
						if v == "" {
							// do not validate an empty string.
							return nil
						}
						_, err := castParamStrToType(v, param)
						return err
					}).
					Value(&defaultStr),
			).WithHideFunc(func() bool { return param.Type != jsonschema.Array }),
			huh.NewGroup(
				huh.NewInput().
					Title("Default Value (optional)").
					Validate(func(v string) error {
						if v == "" {
							// do not validate an empty string.
							return nil
						}
						_, err := castParamStrToType(v, param)
						return err
					}).
					Value(&defaultStr),
			).WithHideFunc(func() bool { return param.Type == jsonschema.Array }),
		)

		err := form.Run()
		if err != nil {
			return fmt.Errorf("form error: %w", err)
		}

		// Set default value if provided
		if defaultStr != "" {
			param.Default, err = castParamStrToType(defaultStr, param)
			if err != nil {
				return err
			}
		} else {
			param.Default, err = jsonschema.EnsureType(param.Type, nil)
			if err != nil {
				return err
			}
			// explicitly set the number as '0.0' as otherwise there will be an action definition error.
			if param.Type == jsonschema.Number {
				param.Default = "0.0"
			}
		}

		if param.Type != jsonschema.Array {
			param.Items = nil
		}

		// Normalize parameter name
		param.Name = strings.ToLower(param.Name)

		// Add parameter to list
		*params = append(*params, param)

		// Ask if the user wants to add more parameters
		form = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Add another %s?", paramType)).
					Value(&addMore),
			),
		)

		err = form.Run()
		if err != nil {
			return fmt.Errorf("form error: %w", err)
		}
	}

	return nil
}

// collectContainerConfig collects container-specific configuration
func (m *metadataCollector) collectContainerConfig() (*action.DefRuntimeContainer, error) {
	config := &action.DefRuntimeContainer{
		Env: make(action.EnvSlice, 0),
	}

	var envStr string
	var extraHostsStr string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Image").
				Description("Docker image to use").
				Placeholder("actionid:latest").
				Value(&config.Image).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("image can't be empty")
					}

					return nil
				}),
			huh.NewText().
				Title("Environment Variables").
				Description("KEY=VALUE pairs, one per line").
				Value(&envStr),
			huh.NewInput().
				Title("Extra Hosts").
				Description("Extra hosts to add (comma-separated)").
				Value(&extraHostsStr),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, fmt.Errorf("form error: %w", err)
	}

	// Parse environment variables
	if envStr != "" {
		envLines := strings.Split(envStr, "\n")
		for _, line := range envLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				config.Env = append(config.Env, line)
			}
		}
	}

	// Parse extra hosts
	if extraHostsStr != "" {
		config.ExtraHosts = strings.Split(extraHostsStr, ",")
		for i, host := range config.ExtraHosts {
			config.ExtraHosts[i] = strings.TrimSpace(host)
		}
	}

	return config, nil
}

// collectShellConfig collects shell-specific configuration
func (m *metadataCollector) collectShellConfig() (*action.DefRuntimeShell, error) {
	config := &action.DefRuntimeShell{
		Env: make(action.EnvSlice, 0),
	}

	var envStr string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Environment Variables").
				Description("KEY=VALUE pairs, one per line").
				Value(&envStr),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, fmt.Errorf("form error: %w", err)
	}

	// Parse environment variables
	if envStr != "" {
		envLines := strings.Split(envStr, "\n")
		for _, line := range envLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				config.Env = append(config.Env, line)
			}
		}
	}

	return config, nil
}

func castParamStrToType(v string, pdef *action.DefParameter) (any, error) {
	var err error
	if pdef.Type != jsonschema.Array {
		return jsonschema.ConvertStringToType(v, pdef.Type)
	}
	items := strings.Split(v, ",")
	res := make([]any, len(items))
	for i, item := range items {
		res[i], err = jsonschema.ConvertStringToType(item, pdef.Items.Type)
		if err != nil {
			return nil, err
		}

		if pdef.Items.Type == jsonschema.Number && res[i] == 0 {
			res[i] = "0.0"
		}
	}
	return res, nil
}

func isValidName(subject, name string) error {
	if name == "" {
		return fmt.Errorf("%s cannot be empty", subject)
	}

	// Check for whitespace
	if strings.ContainsAny(name, " \t\n\r\f\v") {
		return fmt.Errorf("%s cannot contain whitespace", subject)
	}

	// The first character must be a letter
	if !isLetter(rune(name[0])) {
		return fmt.Errorf("%s must start with a letter", subject)
	}

	// Check remaining characters
	for i, char := range name {
		if !isLetter(char) && char != '_' && !isDigit(char) {
			return fmt.Errorf("%s, invalid character '%c' at position %d: argument name can only contain letters and underscores", subject, char, i)
		}
	}

	return nil
}

// isLetter checks if a character is a letter (a-z, A-Z)
func isLetter(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

// isDigit checks if a character is a digit (0-9)
func isDigit(char rune) bool {
	return char >= '0' && char <= '9'
}
