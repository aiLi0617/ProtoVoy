package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type teamNode struct {
	ID, Name, WorkspaceID, ProjectID, ParentID, ModuleID string
}

type teamRequestLocation struct {
	ID, ProjectID, CollectionID string
}

func readTeamSnapshot(data []byte) (Project, error) {
	var snapshot struct {
		SchemaVersion int
		Tree          *struct {
			Workspaces, Projects, Modules, Collections []teamNode
			Requests                                   []teamRequestLocation
		}
		Requests []struct {
			OriginalID, ProjectID, CollectionID string
			Envelope                            json.RawMessage
		}
	}
	if json.Unmarshal(data, &snapshot) != nil || snapshot.SchemaVersion != 1 || snapshot.Tree == nil ||
		snapshot.Requests == nil || snapshot.Tree.Requests == nil || len(snapshot.Requests) != len(snapshot.Tree.Requests) ||
		len(snapshot.Requests) > maxRequests {
		return Project{}, errors.New("invalid or incomplete v1 team snapshot")
	}
	tree := snapshot.Tree
	workspaces, err := teamIndex(tree.Workspaces)
	if err != nil {
		return Project{}, err
	}
	projects, err := teamIndex(tree.Projects)
	if err != nil {
		return Project{}, err
	}
	modules := make(map[string]teamNode)
	if tree.Modules != nil {
		modules, err = teamIndex(tree.Modules)
	}
	if err != nil {
		return Project{}, err
	}
	collections, err := teamIndex(tree.Collections)
	if err != nil {
		return Project{}, err
	}
	for _, project := range projects {
		if _, ok := workspaces[project.WorkspaceID]; !ok {
			return Project{}, errors.New("team snapshot project refers to an unknown workspace")
		}
	}
	for _, module := range modules {
		if _, ok := projects[module.ProjectID]; !ok {
			return Project{}, errors.New("team snapshot module refers to an unknown project")
		}
	}
	paths := make(map[string][]string, len(collections))
	occupied := make(map[string]bool, len(collections))
	pathBytes := 0
	for _, collection := range tree.Collections {
		path, err := teamCollectionPath(collection.ID, collections, modules, projects, workspaces)
		if err != nil {
			return Project{}, err
		}
		for _, name := range path {
			pathBytes += len(name) + 1
		}
		if pathBytes > MaxBytes {
			return Project{}, errors.New("team snapshot folder paths exceed 8 MiB")
		}
		paths[collection.ID] = path
		if collection.ParentID != "" {
			occupied[collection.ParentID] = true
		}
	}
	locations := make(map[string]teamRequestLocation, len(tree.Requests))
	for _, request := range tree.Requests {
		collection, ok := collections[request.CollectionID]
		if request.ID == "" || locations[request.ID].ID != "" || !ok || collection.ProjectID != request.ProjectID {
			return Project{}, errors.New("team snapshot has an invalid request location")
		}
		locations[request.ID] = request
		occupied[request.CollectionID] = true
	}
	result := Project{Name: "Imported team snapshot", Requests: make([]Request, 0, len(snapshot.Requests))}
	for _, collection := range tree.Collections {
		if !occupied[collection.ID] {
			result.EmptyFolders = append(result.EmptyFolders, paths[collection.ID])
		}
	}
	requestPathBytes := 0
	for index, source := range snapshot.Requests {
		location, ok := locations[source.OriginalID]
		if !ok || source.ProjectID != location.ProjectID || source.CollectionID != location.CollectionID {
			return Project{}, fmt.Errorf("team snapshot request %d has a duplicate or inconsistent location", index+1)
		}
		delete(locations, source.OriginalID)
		converted, err := readLegacyRequest(source.Envelope)
		if err != nil {
			return Project{}, fmt.Errorf("team snapshot request %d: %w", index+1, err)
		}
		request := converted.Requests[0]
		path := paths[location.CollectionID]
		for _, name := range path {
			requestPathBytes += len(name) + 1
		}
		if requestPathBytes > MaxBytes {
			return Project{}, errors.New("team snapshot folder paths exceed 8 MiB")
		}
		request.CollectionPath = path
		result.Requests = append(result.Requests, request)
	}
	return result, nil
}

func teamIndex(nodes []teamNode) (map[string]teamNode, error) {
	if nodes == nil || len(nodes) > maxRequests {
		return nil, errors.New("missing or oversized team snapshot folder list")
	}
	index := make(map[string]teamNode, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Name) == "" || index[node.ID].ID != "" {
			return nil, errors.New("team snapshot contains an invalid or duplicate folder")
		}
		index[node.ID] = node
	}
	return index, nil
}

func teamCollectionPath(id string, collections, modules, projects, workspaces map[string]teamNode) ([]string, error) {
	current := collections[id]
	project := projects[current.ProjectID]
	if project.ID == "" {
		return nil, errors.New("team snapshot collection refers to an unknown project")
	}
	var path []string
	moduleID := ""
	for {
		if len(path) == maxCollectionDepth || current.ID == "" || current.ProjectID != project.ID {
			return nil, errors.New("team snapshot collection hierarchy is invalid or too deep")
		}
		if current.ModuleID != "" {
			if moduleID != "" && moduleID != current.ModuleID {
				return nil, errors.New("team snapshot collection spans different modules")
			}
			moduleID = current.ModuleID
		}
		path = append(path, current.Name)
		if current.ParentID == "" {
			break
		}
		current = collections[current.ParentID]
	}
	if moduleID != "" {
		module := modules[moduleID]
		if module.ProjectID != project.ID {
			return nil, errors.New("team snapshot collection refers to an unknown module")
		}
		path = append(path, module.Name)
	}
	path = append(path, project.Name, workspaces[project.WorkspaceID].Name)
	slices.Reverse(path)
	return path, nil
}
