//go:build !js

package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kjanat/wacogo"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
)

// ExecutePackage rechecks trust immediately before executing a fresh component.
func ExecutePackage(parent context.Context, directory string, trust Trust, kind, input string) (output string, err error) {
	run, err := PreparePackage(directory, trust, kind)
	if err != nil {
		return "", err
	}
	return run(parent, input)
}

// PreparePackage captures a verified, enabled snapshot for a later invocation.
// An empty kind uses the kind declared by the verified manifest.
func PreparePackage(directory string, trust Trust, kind string) (func(context.Context, string) (string, error), error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, errors.New("could not open plugin package")
	}
	defer root.Close()
	pack, err := readExecutablePackage(root, trust, "")
	if err != nil {
		return nil, err
	}
	if kind == "" {
		kind = pack.Manifest.Kind
	}
	if pack.Manifest.Kind != kind {
		return nil, errors.New("plugin kind mismatch")
	}
	return func(ctx context.Context, input string) (string, error) {
		return executePackage(ctx, pack, kind, input)
	}, nil
}

// ExecuteInstalled confines all package reads to the selected store and checks
// identity before passing the verified snapshot to the sandbox.
func ExecuteInstalled(parent context.Context, store, id string, trust Trust, kind, input string) (string, error) {
	return ExecuteInstalledSelection(parent, store, id, trust, RunRequest{Kind: kind, Input: input})
}

func ExecuteInstalledSelection(parent context.Context, store, id string, trust Trust, request RunRequest) (string, error) {
	if !ValidID(id) {
		return "", errors.New("invalid plugin ID")
	}
	root, err := os.OpenRoot(store)
	if err != nil {
		return "", errors.New("could not open plugin store")
	}
	defer root.Close()
	child, err := openInstalledDirectory(root, id)
	if err != nil {
		return "", err
	}
	defer child.Close()
	pack, err := readExecutablePackage(child, trust, request.ExpectedRevision)
	if err != nil {
		return "", err
	}
	if pack.Manifest.ID != id {
		return "", errors.New("installed plugin identity does not match its directory")
	}
	if request.ExpectedVersion != "" && pack.Manifest.Version != request.ExpectedVersion {
		return "", errors.New("installed plugin version changed; refresh the plugin list and select it again")
	}
	return executePackage(parent, pack, request.Kind, request.Input)
}

func executePackage(parent context.Context, pack Package, kind, input string) (output string, err error) {
	if err := parent.Err(); err != nil {
		return "", err
	}
	if pack.Manifest.Kind != kind || len(input) > 1<<20 || !utf8.ValidString(input) {
		return "", errors.New("plugin kind mismatch or input is not UTF-8 within 1 MiB")
	}
	export := map[string]string{"transformer": "transform", "auth": "apply-auth", "importer": "import", "protocol": "execute"}[kind]
	if export == "" {
		return "", errors.New("unsupported plugin kind")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	defer func() {
		if recover() != nil {
			output, err = "", errors.New("plugin execution exceeded sandbox limits")
		}
		if parentErr := parent.Err(); parentErr != nil {
			output, err = "", parentErr
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			output, err = "", fmt.Errorf("plugin execution timed out: %w", context.DeadlineExceeded)
		}
	}()
	budget := &memoryBudget{limit: 32 << 20}
	ctx = experimental.WithMemoryAllocator(ctx, budget)
	var lock sync.Mutex
	var modules, tables uint64
	ctx = wacogo.WithCoreTableAllocator(ctx, func(definitions []wacogo.CoreTable) (func(), error) {
		lock.Lock()
		defer lock.Unlock()
		var entries uint64
		for _, table := range definitions {
			if !table.Max.Valid || uint64(table.Max.Value) > 65536-entries {
				return nil, errors.New("plugin table capacity exceeds limits")
			}
			entries += uint64(table.Max.Value)
		}
		if modules >= 16 || entries > 65536-tables {
			return nil, errors.New("plugin module or table budget exceeded")
		}
		modules++
		tables += entries
		return func() {
			lock.Lock()
			modules--
			tables -= entries
			lock.Unlock()
		}, nil
	})
	engine := wacogo.NewEngine(ctx,
		wacogo.WithRuntimeConfig(wazero.NewRuntimeConfig().WithMemoryLimitPages(512).WithCloseOnContextDone(true)),
		wacogo.WithLiftLimits(wacogo.LiftLimits{MaxBytes: 1 << 20, MaxValues: 1024}))
	defer engine.Close(context.Background())
	component, err := engine.LoadComponent(ctx, bytes.NewReader(pack.Component))
	if err != nil {
		return "", errors.New("invalid plugin Component")
	}
	instance, err := component.Instantiate(ctx)
	if err != nil {
		return "", errors.New("plugin Component could not be instantiated")
	}
	defer instance.Close(context.Background())
	function := instance.ExportedFunc(export)
	if function == nil {
		return "", errors.New("plugin export is missing")
	}
	result, err := function.Call(ctx, wacogo.ValString(input))
	if err != nil || len(result) != 1 {
		return "", errors.New("plugin function failed")
	}
	value, ok := result[0].(wacogo.ValString)
	if !ok || len(value) > 1<<20 || !utf8.ValidString(string(value)) {
		return "", errors.New("invalid or oversized plugin result")
	}
	return string(value), nil
}
