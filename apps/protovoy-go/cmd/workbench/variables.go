package main

import (
	"fmt"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"protovoy.dev/protovoy/internal/project"
)

var (
	variablesID      = tree.ID(page, "variables")
	globalVarsID     = tree.ID(page, "global-vars")
	collectionVarsID = tree.ID(page, "collection-vars")
	newEnvID         = tree.ID(page, "new-environment")
	importEnvID      = tree.ID(page, "import-environments")
	saveVarsID       = tree.ID(page, "save-variables")
	discardVarsID    = tree.ID(page, "discard-variables")
	useEnvID         = tree.ID(page, "use-environment")
	clearEnvID       = tree.ID(page, "clear-environment")
	removeEnvID      = tree.ID(page, "remove-environment")
	envNameID        = tree.ID(page, "environment-name")
	valuesID         = tree.ID(page, "variable-values")
	envOptions       = tree.Sub(page, tree.Name("environment-options"))
)

const (
	globalVars     = -3
	collectionVars = -2
	newEnvironment = -1
)

type variablesPanel struct {
	scopes                 project.VariableScopes
	open                   bool
	initialized            bool
	editing                int // negative values are scope constants; others are environment indices
	name                   flex.Field
	values                 requestEditor
	picker                 flex.SelectState
	savedName, savedValues string
	message                string
	revision               uint64
	picking                bool
	imported               *environmentImport
}

type environmentImport struct {
	result project.EnvironmentImport
	err    error
}

func (w *workbench) importEnvironments() {
	panel := &w.variables
	if panel.dirty() {
		panel.message = "Save variables or discard edits before importing"
		return
	}
	panel.picking = true
	chooseLocalFile(app.OpenFileOptions{
		Title: "Import legacy environments", Filters: []app.FileFilter{{Name: "JSON", Pattern: "*.json"}},
	}, project.MaxEnvironmentImportBytes, func(data []byte, err error) {
		value := environmentImport{err: err}
		if err == nil && data != nil {
			value.result, value.err = project.ReadLegacyEnvironments(data)
		}
		w.mu.Lock()
		panel.imported = &value
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (panel *variablesPanel) finishImport(value environmentImport) {
	panel.picking = false
	if value.err != nil {
		panel.message = value.err.Error()
		return
	}
	imported := value.result.Environments
	if len(imported) == 0 {
		return
	}
	if panel.dirty() {
		panel.message = "Save variables or discard edits before importing"
		return
	}
	if len(panel.scopes.Environments)+len(imported) > project.MaxEnvironments {
		panel.message = "Import would exceed the workspace environment limit"
		return
	}
	for _, incoming := range imported {
		for _, existing := range panel.scopes.Environments {
			if strings.EqualFold(existing.Name, incoming.Name) {
				panel.message = "An imported environment name already exists in this workspace"
				return
			}
		}
	}
	first := len(panel.scopes.Environments)
	panel.scopes.Environments = append(panel.scopes.Environments, imported...)
	panel.revision++
	panel.edit(first)
	panel.message = fmt.Sprintf("Imported %d environments", len(imported))
	if value.result.SecretRefs != 0 || value.result.Defaults != 0 {
		panel.message += fmt.Sprintf(". Migrate %d keychain references and %d HTTP defaults separately.",
			value.result.SecretRefs, value.result.Defaults)
	}
}

func (panel *variablesPanel) edit(scope int) {
	panel.initialized = true
	panel.editing = scope
	panel.message = ""
	var name string
	var values map[string]string
	switch scope {
	case globalVars:
		values = panel.scopes.Global
	case collectionVars:
		values = panel.scopes.Collection
	case newEnvironment:
	default:
		env := panel.scopes.Environments[scope]
		name, values = env.Name, env.Variables
	}
	panel.savedName = name
	panel.savedValues = project.FormatVariables(values)
	panel.name.SetText(name)
	panel.values = newRequestEditor()
	panel.values.input.Editor = text.NewEditor(panel.savedValues)
}

func (panel *variablesPanel) dirty() bool {
	return panel.name.Text() != panel.savedName || panel.values.input.Editor.String() != panel.savedValues
}

func (panel *variablesPanel) choose(scope int) {
	if panel.dirty() {
		panel.message = "Save variables or discard edits before switching scope"
		return
	}
	panel.edit(scope)
}

func (panel *variablesPanel) save() {
	values, err := project.ParseVariables(panel.values.input.Editor.String())
	if err != nil {
		panel.message = err.Error()
		return
	}
	switch panel.editing {
	case globalVars:
		panel.scopes.Global = values
	case collectionVars:
		panel.scopes.Collection = values
	default:
		name := strings.TrimSpace(panel.name.Text())
		if name == "" || len(name) > 128 {
			panel.message = "Enter an environment name of 1 to 128 bytes"
			return
		}
		for i, env := range panel.scopes.Environments {
			if i != panel.editing && strings.EqualFold(env.Name, name) {
				panel.message = "An environment with that name already exists"
				return
			}
		}
		env := project.Environment{Name: name, Variables: values}
		if panel.editing == newEnvironment {
			if len(panel.scopes.Environments) == project.MaxEnvironments {
				panel.message = "The workspace has reached its environment limit"
				return
			}
			panel.editing = len(panel.scopes.Environments)
			panel.scopes.Environments = append(panel.scopes.Environments, env)
		} else {
			previous := panel.scopes.Environments[panel.editing]
			env.HTTPDefaults = previous.HTTPDefaults
			if previous.Name == panel.scopes.Active {
				panel.scopes.Active = name
			}
			panel.scopes.Environments[panel.editing] = env
		}
	}
	panel.revision++
	panel.edit(panel.editing)
	panel.message = "Variables saved"
}

func (panel *variablesPanel) activate(name string) {
	if name != panel.scopes.Active {
		panel.scopes.Active = name
		panel.revision++
	}
}

func (panel *variablesPanel) remove() {
	if panel.dirty() {
		panel.message = "Discard edits before removing this environment"
		return
	}
	envs := panel.scopes.Environments
	if envs[panel.editing].Name == panel.scopes.Active {
		panel.activate("")
	}
	copy(envs[panel.editing:], envs[panel.editing+1:])
	envs[len(envs)-1] = project.Environment{}
	panel.scopes.Environments = envs[:len(envs)-1]
	panel.revision++
	panel.edit(globalVars)
	panel.message = "Environment removed"
}

func (w *workbench) drawVariables(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.variables
	f.Row().Gap(8).Children(func() {
		if f.Button(globalVarsID, "Global").Clicked() {
			panel.choose(globalVars)
		}
		if f.Button(collectionVarsID, "Collection").Clicked() {
			panel.choose(collectionVars)
		}
		if f.Button(newEnvID, "New environment").Clicked() {
			panel.choose(newEnvironment)
		}
	})
	if panel.picking {
		f.Text("Opening environment file…")
	} else if f.Button(importEnvID, "Import legacy environments").Clicked() {
		w.importEnvironments()
	}
	if len(panel.scopes.Environments) != 0 {
		names := make([]string, len(panel.scopes.Environments))
		for i, env := range panel.scopes.Environments {
			names[i] = env.Name
		}
		if selected, ok := f.Select(envOptions, &panel.picker, names, panel.editing).Placeholder("Select environment").Picked(); ok {
			panel.choose(selected)
		}
	}
	switch panel.editing {
	case globalVars:
		f.Text("Global variables")
	case collectionVars:
		f.Text("Collection variables")
	default:
		f.Field(&panel.name, envNameID, "Environment name", "Environment name")
	}
	panel.values.draw(f, canvas, valuesID, "Variables, one name=value per line", "name=value").Height(180)
	if panel.editing >= 0 {
		if count := len(panel.scopes.Environments[panel.editing].HTTPDefaults); count > 0 {
			f.Wrap(fmt.Sprintf("Imported HTTP defaults: %d", count)).Muted()
		}
	}
	f.Row().Gap(8).Children(func() {
		if f.Button(saveVarsID, "Save variables").Clicked() {
			panel.save()
		}
		if f.Button(discardVarsID, "Discard variable edits").Clicked() {
			panel.edit(panel.editing)
		}
	})
	f.Row().Gap(8).Children(func() {
		if panel.editing >= 0 {
			if f.Button(useEnvID, "Use environment").Clicked() {
				panel.activate(panel.scopes.Environments[panel.editing].Name)
			}
			if f.Button(removeEnvID, "Remove environment").Clicked() {
				panel.remove()
			}
		}
		if panel.scopes.Active != "" && f.Button(clearEnvID, "No environment").Clicked() {
			panel.activate("")
		}
	})
	if panel.message != "" {
		f.Wrap(panel.message).Muted()
	}
}
