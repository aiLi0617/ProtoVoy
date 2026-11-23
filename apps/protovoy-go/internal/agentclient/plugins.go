package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"protovoy.dev/protovoy/internal/plugins"
)

func (client *Client) RemovePlugin(ctx context.Context, id string) (plugins.RemoveResult, error) {
	if !client.SupportsPlugins() || !plugins.ValidID(id) {
		return plugins.RemoveResult{}, errors.New("plugin removal requires a supported Agent and a valid ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	response, err := client.pluginRequestMethod(ctx, http.MethodDelete, "/v1/plugins/"+id, nil)
	if err != nil {
		return plugins.RemoveResult{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	var result plugins.RemoveResult
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &result) != nil || result.ID != id {
		return plugins.RemoveResult{}, errors.New("Local Agent returned invalid plugin removal metadata")
	}
	return result, nil
}

func (client *Client) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	if !client.SupportsPlugins() || !plugins.ValidID(id) {
		return errors.New("plugin state requires a supported Agent and a valid ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})
	response, err := client.pluginRequest(ctx, "/v1/plugins/"+id+"/state", body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	var result plugins.StateResult
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &result) != nil || result.ID != id || result.Enabled != enabled {
		return errors.New("Local Agent returned invalid plugin state")
	}
	return nil
}

func (client *Client) InstallPlugin(ctx context.Context, input plugins.InstallRequest) (plugins.InstallResult, error) {
	return client.uploadPlugin(ctx, "", input)
}

func (client *Client) UpdatePlugin(ctx context.Context, id string, input plugins.InstallRequest) (plugins.InstallResult, error) {
	if !client.SupportsPluginUpdates() {
		return plugins.InstallResult{}, errors.New("this Local Agent does not support plugin updates")
	}
	if !plugins.ValidID(id) {
		return plugins.InstallResult{}, errors.New("invalid plugin ID")
	}
	return client.uploadPlugin(ctx, id, input)
}

func (client *Client) uploadPlugin(ctx context.Context, id string, input plugins.InstallRequest) (plugins.InstallResult, error) {
	if !client.SupportsPlugins() || len(input.Manifest) > plugins.ManifestMaxBytes || len(input.Component) > plugins.ComponentMaxBytes || len(input.Permissions) > 4 {
		return plugins.InstallResult{}, errors.New("plugin installation requires a supported Agent and package files within their size limits")
	}
	manifest, err := plugins.ReadManifest(input.Manifest)
	if err != nil {
		return plugins.InstallResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var result plugins.InstallResult
	body, err := json.Marshal(input)
	if err != nil || len(body) > plugins.InstallMessageMaxBytes {
		return plugins.InstallResult{}, errors.New("could not encode plugin package within size limit")
	}
	path := "/v1/plugins"
	if id != "" {
		if manifest.ID != id {
			return plugins.InstallResult{}, errors.New("plugin update package does not match the selected ID")
		}
		path += "/" + id + "/update"
	}
	response, err := client.pluginRequest(ctx, path, body)
	if err != nil {
		return plugins.InstallResult{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(data) > 16384 || json.Unmarshal(data, &result) != nil {
		return plugins.InstallResult{}, errors.New("Local Agent returned invalid plugin installation metadata")
	}
	if result.ID != manifest.ID || len(result.SHA256) != 64 {
		return plugins.InstallResult{}, errors.New("Local Agent returned invalid plugin installation metadata")
	}
	return result, nil
}

func (client *Client) RunPlugin(ctx context.Context, id string, input plugins.RunRequest) (string, error) {
	if !client.SupportsPlugins() || !plugins.ValidID(id) || len(input.Input) > 1<<20 || !utf8.ValidString(input.Input) {
		return "", errors.New("plugin execution requires a supported Agent, a valid ID and UTF-8 input within 1 MiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body, err := json.Marshal(input)
	if err != nil || len(body) > plugins.MessageMaxBytes {
		return "", errors.New("could not encode plugin input within size limit")
	}
	response, err := client.pluginRequest(ctx, "/v1/plugins/"+id+"/run", body)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, plugins.MessageMaxBytes+1))
	var result plugins.RunResult
	if err != nil || len(data) > plugins.MessageMaxBytes || json.Unmarshal(data, &result) != nil || len(result.Output) > 1<<20 || !utf8.ValidString(result.Output) {
		return "", errors.New("Local Agent returned invalid or oversized plugin output")
	}
	return result.Output, nil
}

func (client *Client) pluginRequest(ctx context.Context, path string, body []byte) (*http.Response, error) {
	return client.pluginRequestMethod(ctx, http.MethodPost, path, body)
}

func (client *Client) pluginRequestMethod(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	response, err := client.requestRaw(ctx, method, path, "application/json", body, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusTooManyRequests {
		message, err := io.ReadAll(io.LimitReader(response.Body, 1025))
		if err == nil && len(message) <= 1024 && utf8.Valid(message) && strings.TrimSpace(string(message)) != "" {
			return nil, errors.New("Plugin rejected: " + strings.TrimSpace(string(message)))
		}
	}
	return nil, fmt.Errorf("Local Agent rejected the plugin request (HTTP %d)", response.StatusCode)
}

func (client *Client) SupportsPlugins() bool { return client.pluginsVersion == "1" }

func (client *Client) SupportsPluginUpdates() bool {
	return client.SupportsPlugins() && client.pluginUpdates
}

func (client *Client) Plugins(ctx context.Context) ([]plugins.Installed, error) {
	if !client.SupportsPlugins() {
		return nil, errors.New("this Local Agent does not support plugin inventory")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var result []plugins.Installed
	response, err := client.request(ctx, http.MethodGet, "/v1/plugins", "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, plugins.InventoryMaxBytes+1))
	if err != nil || len(data) > plugins.InventoryMaxBytes || json.Unmarshal(data, &result) != nil {
		return nil, errors.New("Local Agent returned invalid or oversized plugin inventory")
	}
	return result, nil
}
