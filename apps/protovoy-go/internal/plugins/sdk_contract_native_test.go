//go:build !js

package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kjanat/wacogo/wasmtools"
)

func TestSDKComponentWorlds(t *testing.T) {
	ctx := t.Context()
	tool, err := wasmtools.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(ctx)
	wat, err := os.ReadFile("testdata/echo.wat")
	if err != nil {
		t.Fatal(err)
	}
	for _, world := range []struct{ kind, export, parameter string }{
		{"transformer", "transform", "input"},
		{"protocol", "execute", "request-json"},
		{"auth", "apply-auth", "request-json"},
		{"importer", "import", "document"},
	} {
		t.Run(world.kind, func(t *testing.T) {
			source := strings.NewReplacer(`export "transform"`, `export "`+world.export+`"`, `param "input"`, `param "`+world.parameter+`"`).Replace(string(wat))
			component, err := tool.Parse(ctx, []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(t.TempDir(), "plugins")
			manifest := []byte(fmt.Sprintf(`{"id":"sdk","name":"SDK fixture","version":"1","kind":%q}`, world.kind))
			trust := Trust{AllowUnsigned: true}
			if _, err := InstallPackageBytes(store, manifest, component, trust); err != nil {
				t.Fatal(err)
			}
			const input = `{"fixture":"测试"}`
			output, err := ExecuteInstalled(ctx, store, "sdk", trust, world.kind, input)
			if err != nil || output != input {
				t.Fatalf("%s returned %q: %v", world.export, output, err)
			}
		})
	}
}
