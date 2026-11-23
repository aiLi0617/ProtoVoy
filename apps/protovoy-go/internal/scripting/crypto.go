//go:build !js

package scripting

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/FourCoreLabs/qjs"
)

func registerCrypto(ctx *qjs.Context) {
	bind := func(name string, apply func(string) (string, error)) {
		ctx.SetFunc(name, func(call *qjs.This) (*qjs.Value, error) {
			args := call.Args()
			if len(args) != 1 || !args[0].IsString() {
				return call.NewUndefined(), errors.New("crypto helper requires one string")
			}
			input := args[0].String()
			if len(input) > inputLimit {
				return call.NewUndefined(), errors.New("crypto input exceeds script size limit")
			}
			output, err := apply(input)
			if err != nil {
				return call.NewUndefined(), err
			}
			return call.Context().NewString(output), nil
		})
	}
	bind("__cryptoSha256", func(input string) (string, error) {
		sum := sha256.Sum256([]byte(input))
		return hex.EncodeToString(sum[:]), nil
	})
	bind("__cryptoBase64Encode", func(input string) (string, error) {
		return base64.StdEncoding.EncodeToString([]byte(input)), nil
	})
	bind("__cryptoBase64Decode", func(input string) (string, error) {
		data, err := base64.StdEncoding.Strict().DecodeString(input)
		if err != nil || !utf8.Valid(data) || base64.StdEncoding.EncodeToString(data) != input {
			return "", errors.New("invalid Base64 or UTF-8")
		}
		return string(data), nil
	})
	ctx.SetFunc("__cryptoRandomUuid", func(call *qjs.This) (*qjs.Value, error) {
		var id [16]byte
		rand.Read(id[:])
		id[6] = id[6]&0x0f | 0x40
		id[8] = id[8]&0x3f | 0x80
		return call.Context().NewString(fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])), nil
	})
}
