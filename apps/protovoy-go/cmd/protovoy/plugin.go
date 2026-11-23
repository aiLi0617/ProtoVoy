package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"unicode/utf8"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/plugins"
	"protovoy.dev/protovoy/internal/project"
)

func runResponsePlugin(ctx context.Context, request execution.HTTPRequest, directory, trustedKeys string, output, status io.Writer) error {
	transform, err := prepareResponsePlugin(directory, trustedKeys)
	if err != nil {
		return err
	}
	_, err = transformHTTPResponse(ctx, execution.HTTPClient{}, request, transform, func(code int) error {
		_, err := fmt.Fprintf(status, "HTTP %d\n", code)
		return err
	}, func(value string) error {
		_, err := io.WriteString(output, value)
		return err
	})
	return err
}

type responseTransformer func(context.Context, string) (string, error)

func prepareResponsePlugin(directory, trustedKeys string) (responseTransformer, error) {
	keys, err := plugins.ParseTrustedKeys(trustedKeys)
	if err != nil {
		return nil, err
	}
	return plugins.PreparePackage(directory, plugins.Trust{Keys: keys}, "transformer")
}

func transformHTTPResponse(ctx context.Context, client execution.HTTPClient, request execution.HTTPRequest,
	transform responseTransformer, reportStatus func(int) error, consume func(string) error,
) (int, error) {
	for _, header := range request.Headers {
		if execution.IsSensitiveHTTPHeader(header[0]) {
			return 0, errors.New("HTTP response plugins cannot be used with credential headers")
		}
	}
	if target, err := url.Parse(request.Target); err == nil {
		if err := validateResponsePluginURL(target); err != nil {
			return 0, err
		}
	}
	client.ValidateRedirect = func(next *http.Request) error {
		return validateResponsePluginURL(next.URL)
	}
	response, err := client.Execute(ctx, request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	code := response.StatusCode
	if reportStatus != nil {
		if err := reportStatus(code); err != nil {
			return code, err
		}
	}
	input, err := readPluginInput(response.Body)
	if err != nil {
		if ctx.Err() != nil {
			return code, ctx.Err()
		}
		return code, err
	}
	if project.ContainsRecognizableBodyCredentials(input) {
		return code, errors.New("HTTP response plugin cannot process a body containing possible credentials")
	}
	value, err := transform(ctx, input)
	if err != nil {
		return code, err
	}
	return code, consume(value)
}

func validateResponsePluginURL(target *url.URL) error {
	if target.User != nil {
		return errors.New("HTTP response plugins cannot be used with URL credentials")
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return errors.New("HTTP response plugins require a valid URL query")
	}
	for key := range query {
		if execution.IsSensitiveHTTPHeader(key) {
			return errors.New("HTTP response plugins cannot be used with URL credentials")
		}
	}
	return nil
}

func removePlugin(args []string, store string, output io.Writer) error {
	if len(args) != 1 || !plugins.ValidID(args[0]) {
		return errors.New("usage: protovoy plugin remove PLUGIN_ID")
	}
	store, err := plugins.StoreDirectory(store)
	if err != nil {
		return err
	}
	result, err := plugins.RemoveInstalled(store, args[0])
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

func setPluginEnabled(args []string, enabled bool, store string, output io.Writer) error {
	if len(args) != 1 || !plugins.ValidID(args[0]) {
		return errors.New("usage: protovoy plugin enable|disable PLUGIN_ID")
	}
	store, err := plugins.StoreDirectory(store)
	if err != nil {
		return err
	}
	if err := plugins.SetEnabled(store, args[0], enabled); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(plugins.StateResult{ID: args[0], Enabled: enabled})
}

func installPlugin(args []string, trustedKeys, store string, update bool, output io.Writer) error {
	directory, trust, err := pluginOptions(args, trustedKeys)
	if err != nil {
		return err
	}
	store, err = plugins.StoreDirectory(store)
	if err != nil {
		return err
	}
	publish := plugins.InstallPackage
	if update {
		publish = plugins.UpdatePackage
	}
	pack, err := publish(store, directory, trust)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		ID        string `json:"id"`
		Directory string `json:"directory"`
		SHA256    string `json:"sha256"`
	}{pack.Manifest.ID, filepath.Join(store, pack.Manifest.ID), pack.SHA256})
}

func listPlugins(store string, output io.Writer) error {
	store, err := plugins.StoreDirectory(store)
	if err != nil {
		return err
	}
	items, err := plugins.Inventory(store)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(items)
}

func inspectPlugin(args []string, trustedKeys string, output io.Writer) error {
	directory, trust, err := pluginOptions(args, trustedKeys)
	if err != nil {
		return err
	}
	pack, err := plugins.ReadPackage(directory, trust)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Manifest plugins.Manifest `json:"manifest"`
		SHA256   string           `json:"sha256"`
		Verified bool             `json:"signatureVerified"`
	}{pack.Manifest, pack.SHA256, pack.SignatureVerified})
}

func executePlugin(ctx context.Context, args []string, trustedKeys string, input io.Reader, output io.Writer) error {
	directory, trust, err := pluginOptions(args, trustedKeys)
	if err != nil {
		return err
	}
	value, err := executePluginInput(ctx, directory, trust, "", input)
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, value)
	return err
}

func importPluginProject(ctx context.Context, args []string, trustedKeys string, input io.Reader, output io.Writer) error {
	directory, trust, err := pluginOptions(args, trustedKeys)
	if err != nil {
		return err
	}
	value, err := executePluginInput(ctx, directory, trust, "importer", input)
	if err != nil {
		return err
	}
	_, data, err := project.ImportPluginOutput(value)
	if err != nil {
		return err
	}
	_, err = output.Write(data)
	return err
}

func executePluginInput(ctx context.Context, directory string, trust plugins.Trust, kind string, input io.Reader) (string, error) {
	run, err := plugins.PreparePackage(directory, trust, kind)
	if err != nil {
		return "", err
	}
	data, err := readPluginInput(input)
	if err != nil {
		return "", err
	}
	return run(ctx, data)
}

func readPluginInput(input io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return "", fmt.Errorf("timed out reading plugin input: %w", context.DeadlineExceeded)
		}
		return "", errors.New("could not read plugin input")
	}
	if len(data) > 1<<20 || !utf8.Valid(data) {
		return "", errors.New("plugin input must be UTF-8 within 1 MiB")
	}
	return string(data), nil
}

func pluginOptions(args []string, trustedKeys string) (string, plugins.Trust, error) {
	flags := flag.NewFlagSet("plugin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	unsigned := flags.Bool("allow-unsigned", false, "allow an unsigned development package")
	permissions := map[string]bool{}
	flags.Func("grant", "grant a manifest permission (repeatable)", func(permission string) error {
		switch permission {
		case "network", "filesystem_read", "filesystem_write", "secrets_read":
			permissions[permission] = true
			return nil
		default:
			return errors.New("unknown plugin permission")
		}
	})
	if flags.Parse(args) != nil || flags.NArg() != 1 {
		return "", plugins.Trust{}, errors.New("usage: protovoy plugin inspect|install|update|run|import [-allow-unsigned] [-grant PERMISSION] PACKAGE_DIRECTORY")
	}
	keys, err := plugins.ParseTrustedKeys(trustedKeys)
	if err != nil {
		return "", plugins.Trust{}, err
	}
	return flags.Arg(0), plugins.Trust{Keys: keys, Permissions: permissions, AllowUnsigned: *unsigned}, nil
}
