//go:build !js

package execution

import (
	"errors"
	"io"
	"os"
)

func ReadCAFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open CA file")
	}
	defer file.Close()
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("could not read CA file within 1 MiB")
	}
	return data, nil
}
