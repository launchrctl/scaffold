// Package scaffold implements a scaffold launchr plugin
package scaffold

import (
	"context"
	_ "embed"
	"path/filepath"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
)

//go:embed action.yaml
var actionYaml []byte

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] providing scaffold functionality.
type Plugin struct {
	m action.Manager
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 20,
	}
}

// OnAppInit implements [launchr.OnAppInitPlugin] interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.m)
	return nil
}

// DiscoverActions implements [launchr.ActionDiscoveryPlugin] interface.
func (p *Plugin) DiscoverActions(_ context.Context) ([]*action.Action, error) {
	_ = action.Definition{}

	a := action.NewFromYAML("scaffold", actionYaml)
	a.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		outputDir := a.Input().Opt("output").(string)
		runtimeType := a.Input().Opt("runtime").(string)
		id := a.Input().Opt("id").(string)
		title := a.Input().Opt("title").(string)
		containerPreset := a.Input().Opt("preset").(string)
		interactive := a.Input().Opt("interactive").(bool)
		interactive = interactive && a.Input().Streams() != nil && a.Input().Streams().In().IsTerminal()

		scaffold := scaffoldAction{
			manager:         p.m,
			outputDir:       outputDir,
			runtime:         action.DefRuntimeType(runtimeType),
			id:              id,
			title:           title,
			containerPreset: containerPreset,
			interactive:     interactive,
		}

		return scaffold.run()
	}))

	return []*action.Action{a}, nil
}

type scaffoldAction struct {
	manager action.Manager

	runtime action.DefRuntimeType
	id      string
	title   string

	outputDir       string
	interactive     bool
	containerPreset string
}

func (s *scaffoldAction) getDefaultValues() *templateValues {
	v := &templateValues{
		Definition: &action.Definition{
			Action: &action.DefAction{
				Title:       s.title,
				Description: "",
				Aliases:     []string{},
				Arguments:   []*action.DefParameter{},
				Options:     []*action.DefParameter{},
			},
			Runtime: &action.DefRuntime{
				Type:      s.runtime,
				Container: &action.DefRuntimeContainer{},
				Shell:     &action.DefRuntimeShell{},
			},
		},
		ID:              s.id,
		ContainerPreset: s.containerPreset,
	}

	return v
}

// run runs the generator based on command-line arguments
func (s *scaffoldAction) run() error {
	defaults := s.getDefaultValues()
	metadata := newMetadataCollector(s.manager, s.interactive)
	values, err := metadata.collectActionInfo(defaults)
	if err != nil {
		return err
	}

	var outputDir string
	switch values.Runtime.Type {
	case runtimePlugin:
		outputDir = filepath.Join(s.outputDir, "plugins")
	default:
		outputDir = filepath.Join(s.outputDir, "actions")
	}

	gen := newGenerator(outputDir)
	return gen.generate(values)
}
