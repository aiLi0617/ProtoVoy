package project

import (
	"encoding/json"
	"errors"
	"strings"
)

// ImportPluginOutput validates and normalizes a portable project from an importer.
func ImportPluginOutput(output string) (Project, []byte, error) {
	var envelope struct{ Format string }
	if len(output) > 1<<20 || json.Unmarshal([]byte(output), &envelope) != nil ||
		(envelope.Format != legacyProjectFormat && envelope.Format != multiProjectFormat) {
		return Project{}, nil, errors.New("importer plugin must return a portable ProtoVoy project")
	}
	value, err := Read(strings.NewReader(output))
	if err != nil {
		return Project{}, nil, errors.New("importer plugin returned an invalid ProtoVoy project")
	}
	for _, request := range value.Requests {
		if request.HasScripts() {
			return Project{}, nil, errors.New("importer plugin output cannot include request scripts")
		}
	}
	data, err := Encode(value)
	if err != nil {
		return Project{}, nil, errors.New("importer plugin output could not be saved as a portable project")
	}
	return value, data, nil
}
