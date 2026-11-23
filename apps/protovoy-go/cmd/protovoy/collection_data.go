package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxCollectionDataBytes = 8 << 20
	maxCollectionDataRows  = 10_000
)

func readCollectionData(path string) ([]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open the collection data file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCollectionDataBytes+1))
	if err != nil || len(data) > maxCollectionDataBytes {
		return nil, errors.New("could not read the collection data file within 8 MiB")
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if strings.EqualFold(filepath.Ext(path), ".csv") {
		return parseCollectionCSV(data)
	}
	return parseCollectionJSON(data)
}

func parseCollectionJSON(data []byte) ([]map[string]string, error) {
	var input []map[string]json.RawMessage
	if json.Unmarshal(data, &input) != nil || len(input) == 0 || len(input) > maxCollectionDataRows {
		return nil, errors.New("collection data JSON must contain 1 to 10,000 object rows")
	}
	rows := make([]map[string]string, len(input))
	for i, record := range input {
		if record == nil {
			return nil, errors.New("each collection data JSON row must be an object")
		}
		row := make(map[string]string, len(record))
		for key, value := range record {
			if !validCollectionDataName(key) {
				return nil, errors.New("collection data contains an invalid variable name")
			}
			var text string
			if len(value) != 0 && value[0] == '"' {
				_ = json.Unmarshal(value, &text)
			} else {
				var compact bytes.Buffer
				_ = json.Compact(&compact, value)
				text = compact.String()
			}
			row[key] = text
		}
		rows[i] = row
	}
	return rows, nil
}

func parseCollectionCSV(data []byte) ([]map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	headers, err := reader.Read()
	if err != nil || len(headers) == 0 {
		return nil, errors.New("collection CSV needs a header row")
	}
	seen := make(map[string]bool, len(headers))
	for _, name := range headers {
		if !validCollectionDataName(name) || seen[name] {
			return nil, errors.New("collection CSV headers must be valid, unique variable names")
		}
		seen[name] = true
	}
	var rows []map[string]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid collection CSV data row")
		}
		if len(rows) == maxCollectionDataRows {
			return nil, errors.New("collection CSV exceeds 10,000 data rows")
		}
		row := make(map[string]string, len(headers))
		for i, name := range headers {
			row[name] = record[i]
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("collection CSV has no data rows")
	}
	return rows, nil
}

func validCollectionDataName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.ContainsAny(name, "{}\r\n")
}
