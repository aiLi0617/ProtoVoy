package main

import (
	"errors"
	"syscall/js"

	"protovoy.dev/protovoy/internal/project"
)

const workspaceKey = "protovoy-go-workspace-v1"

func readWorkspace() (_ []byte, err error) {
	defer storageError(&err)
	value := js.Global().Get("localStorage").Call("getItem", workspaceKey)
	if value.IsNull() {
		return nil, nil
	}
	data := []byte(value.String())
	if len(data) > project.MaxBytes {
		return nil, errors.New("workspace exceeds 8 MiB")
	}
	return data, nil
}

func writeWorkspace(data []byte) (err error) {
	defer storageError(&err)
	js.Global().Get("localStorage").Call("setItem", workspaceKey, string(data))
	return nil
}

func storageError(err *error) {
	if failure := recover(); failure != nil {
		if _, ok := failure.(js.Error); !ok {
			panic(failure)
		}
		*err = errors.New("browser storage is unavailable or full")
	}
}

func onWorkspaceExit(flush func()) func() {
	window := js.Global()
	callback := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		flush()
		return nil
	})
	window.Call("addEventListener", "pagehide", callback)
	return func() {
		window.Call("removeEventListener", "pagehide", callback)
		callback.Release()
	}
}
