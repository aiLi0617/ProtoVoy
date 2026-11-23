package main

import (
	"errors"
	"fmt"
	"strings"
	"syscall/js"
	"time"

	"github.com/vivaui/viva/app"
)

func chooseLocalFile(options app.OpenFileOptions, limit int, complete func([]byte, error)) {
	document := js.Global().Get("document")
	input := document.Call("createElement", "input")
	input.Set("type", "file")
	extensions := make([]string, 0, len(options.Filters))
	for _, filter := range options.Filters {
		extensions = append(extensions, strings.TrimPrefix(filter.Pattern, "*"))
	}
	input.Set("accept", strings.Join(extensions, ","))
	input.Set("hidden", true)
	document.Get("body").Call("appendChild", input)
	var selected, cancelled js.Func
	cleanup := func() {
		input.Call("removeEventListener", "change", selected)
		input.Call("removeEventListener", "cancel", cancelled)
		input.Call("remove")
		selected.Release()
		cancelled.Release()
	}
	selected = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		files := input.Get("files")
		if files.Length() == 0 {
			cleanup()
			complete(nil, nil)
			return nil
		}
		file := files.Index(0)
		cleanup()
		if file.Get("size").Float() > float64(limit) {
			complete(nil, fmt.Errorf("selected file exceeds %d KiB", limit>>10))
			return nil
		}
		go func() {
			buffer, err := awaitPromise(file.Call("arrayBuffer"))
			if err != nil {
				complete(nil, errors.New("could not read the selected file"))
				return
			}
			bytes := js.Global().Get("Uint8Array").New(buffer)
			data := make([]byte, bytes.Length())
			js.CopyBytesToGo(data, bytes)
			complete(data, nil)
		}()
		return nil
	})
	cancelled = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		cleanup()
		complete(nil, nil)
		return nil
	})
	input.Call("addEventListener", "change", selected)
	input.Call("addEventListener", "cancel", cancelled)
	input.Call("click")
}

func saveProjectFile(name string, data []byte) (bool, error) {
	return saveExportFile(name, "json", "JSON", "application/json", data)
}

func saveExportFile(name, _, _, mediaType string, data []byte) (bool, error) {
	bytes := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(bytes, data)
	blob := js.Global().Get("Blob").New([]any{bytes}, map[string]any{"type": mediaType})
	urlAPI := js.Global().Get("URL")
	url := urlAPI.Call("createObjectURL", blob)
	document := js.Global().Get("document")
	link := document.Call("createElement", "a")
	link.Set("href", url)
	link.Set("download", name)
	link.Set("hidden", true)
	document.Get("body").Call("appendChild", link)
	link.Call("click")
	link.Call("remove")
	time.AfterFunc(time.Second, func() { urlAPI.Call("revokeObjectURL", url) })
	return true, nil
}
