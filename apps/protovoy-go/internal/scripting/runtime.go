//go:build !js

package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/FourCoreLabs/qjs"
)

const (
	memoryLimit = 16 << 20
	inputLimit  = 4 << 20
	timeout     = 500 * time.Millisecond
)

func init() {
	// Bound guest linear memory separately from the JavaScript heap.
	qjs.SetMaxMemoryPages(512)
}

type Result struct {
	Request   json.RawMessage   `json:"request"`
	Variables map[string]string `json:"variables"`
	Logs      []string          `json:"logs"`
}

// Run executes one request phase without exposing host files, modules or output streams.
func Run(parent context.Context, scripts []string, request json.RawMessage, variables map[string]string, response json.RawMessage) (result Result, runErr error) {
	size := len(request) + len(response)
	for _, script := range scripts {
		size += len(script)
	}
	if size > inputLimit || !json.Valid(request) || (len(response) != 0 && !json.Valid(response)) {
		return Result{}, errors.New("invalid or oversized script input")
	}
	if variables == nil {
		variables = map[string]string{}
	}
	scope, err := json.Marshal(variables)
	if err != nil || len(scope) > inputLimit-size {
		return Result{}, errors.New("script variables exceed input limit")
	}
	if len(response) == 0 {
		response = json.RawMessage("null")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		// The binding panics if wazero closes the guest during evaluation or cleanup.
		if recover() != nil {
			result = Result{}
			switch {
			case parent.Err() != nil:
				runErr = parent.Err()
			case ctx.Err() != nil:
				runErr = context.DeadlineExceeded
			default:
				runErr = errors.New("request script sandbox failed")
			}
		}
	}()
	vm, err := qjs.New(qjs.Option{
		Context: ctx, CloseOnContextDone: true, NoHostFS: true, NoHostModules: true,
		MemoryLimit: memoryLimit, MaxStackSize: 512 << 10, Stdout: io.Discard, Stderr: io.Discard,
	})
	if err != nil {
		return Result{}, errors.New("could not initialize script sandbox")
	}
	defer vm.Close()
	registerCrypto(vm.Context())
	source := "(function(){'use strict';let request=" + string(request) + ";let variables=" + string(scope) +
		";const response=" + string(response) + `;
		const logs=[];
		const console=Object.freeze({log:(...values)=>logs.push(values.map(value=>
			typeof value==='string'?value:JSON.stringify(value)).join(' '))});
		const assert=(condition,message='script assertion failed')=>{if(!condition)throw new Error(message)};
		const crypto=Object.freeze({sha256:__cryptoSha256,base64Encode:__cryptoBase64Encode,
			base64Decode:__cryptoBase64Decode,randomUUID:__cryptoRandomUuid});
		` + strings.Join(scripts, "\n") + "\n;return JSON.stringify({request,variables,logs});})()"
	timer = time.AfterFunc(timeout, cancel)
	value, err := vm.Eval("request.js", qjs.Code(source))
	timer.Stop()
	if ctx.Err() != nil {
		if parent.Err() != nil {
			return Result{}, parent.Err()
		}
		return Result{}, context.DeadlineExceeded
	}
	if err != nil {
		return Result{}, errors.New("request script failed")
	}
	defer value.Free()
	if !value.IsString() {
		return Result{}, errors.New("invalid script result")
	}
	data := value.String()
	if len(data) > inputLimit || json.Unmarshal([]byte(data), &result) != nil ||
		len(result.Request) == 0 || result.Request[0] != '{' || result.Variables == nil || result.Logs == nil {
		return Result{}, errors.New("invalid or oversized script result")
	}
	return result, nil
}
