package main

import (
	"errors"
	"io"
	"os"
)

const maxCLIDataBytes = 8 << 20

func readCLIData(path string, input io.Reader) (string, error) {
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		input = file
	}
	body, err := io.ReadAll(io.LimitReader(input, maxCLIDataBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxCLIDataBytes {
		return "", errors.New("request data exceeds 8 MiB")
	}
	return string(body), nil
}
