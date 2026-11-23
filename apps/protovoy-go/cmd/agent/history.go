package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"protovoy.dev/protovoy/internal/agenthistory"
)

const historyLimit = 200
const historyStamp = "20060102T150405.000000000"
const historyBodyExtension = ".pvhistory"
const historyMetadataLimit = 2048

type historyStore struct {
	root          *os.Root
	mu            sync.Mutex
	responseLimit int
}

func openHistory(dir string) (*historyStore, error) {
	if dir == "" {
		base := os.Getenv("PROTOVOY_CONFIG_DIR")
		if base == "" {
			config, err := os.UserConfigDir()
			if err != nil {
				return nil, err
			}
			base = filepath.Join(config, "protovoy")
		}
		dir = filepath.Join(base, "go-agent-history")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &historyStore{root: root}, nil
}

func (store *historyStore) save(record agenthistory.Record, body []byte) error {
	extension := ".json"
	if record.ResponseBodyBytes == nil && len(body) != 0 {
		return errors.New("history response metadata is missing")
	}
	if record.ResponseBodyBytes != nil {
		if *record.ResponseBodyBytes != len(body) || len(body) > agenthistory.MaxResponseBytes {
			return errors.New("invalid history response size")
		}
		extension = historyBodyExtension
	}
	name := "execution-" + record.FinishedAt.UTC().Format(historyStamp) + "-" + record.ID + extension
	temp := name + ".tmp"
	store.mu.Lock()
	defer store.mu.Unlock()
	file, err := store.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer store.root.Remove(temp)
	writeErr := json.NewEncoder(file).Encode(record)
	if writeErr == nil && len(body) != 0 {
		_, writeErr = file.Write(body)
	}
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := store.root.Rename(temp, name); err != nil {
		return err
	}
	names, err := store.names()
	if err != nil {
		return err
	}
	for _, old := range names[:max(0, len(names)-historyLimit)] {
		if err := store.root.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (store *historyStore) names() ([]string, error) {
	entries, err := fs.ReadDir(store.root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		rest, owned := strings.CutPrefix(name, "execution-")
		extension := filepath.Ext(rest)
		stamp, id, split := strings.Cut(strings.TrimSuffix(rest, extension), "-")
		if _, err := time.Parse(historyStamp, stamp); owned && entry.Type().IsRegular() &&
			(extension == ".json" || extension == historyBodyExtension) && split && err == nil && validID(id) {
			names = append(names, name)
		}
	}
	return names, nil
}

func (store *historyStore) read(name string) (agenthistory.Record, bool) {
	file, err := store.root.Open(name)
	if err != nil {
		return agenthistory.Record{}, false
	}
	defer file.Close()
	return readHistoryMetadata(name, bufio.NewReaderSize(file, historyMetadataLimit+1))
}

func historyRecordMatches(name, id string) bool {
	return strings.HasSuffix(strings.TrimSuffix(name, filepath.Ext(name)), "-"+id)
}

func readHistoryMetadata(name string, reader *bufio.Reader) (agenthistory.Record, bool) {
	var record agenthistory.Record
	data, err := reader.ReadSlice('\n')
	if (err != nil && (err != io.EOF || filepath.Ext(name) == historyBodyExtension)) ||
		len(data) > historyMetadataLimit || json.Unmarshal(data, &record) != nil ||
		!validID(record.ID) || !historyRecordMatches(name, record.ID) {
		return record, false
	}
	if record.ResponseBodyBytes != nil && (filepath.Ext(name) != historyBodyExtension ||
		*record.ResponseBodyBytes < 0 || *record.ResponseBodyBytes > agenthistory.MaxResponseBytes) {
		return record, false
	}
	return record, true
}

type historyFilter struct {
	limit           int
	id, requestID   string
	protocol, state string
	status          int
}

func (filter historyFilter) matches(record agenthistory.Record) bool {
	return (filter.id == "" || record.ID == filter.id) &&
		(filter.requestID == "" || record.RequestID == filter.requestID) &&
		(filter.protocol == "" || record.ProtocolID == filter.protocol) &&
		(filter.state == "" || record.State == filter.state) &&
		(filter.status == 0 || record.Status != nil && *record.Status == filter.status)
}

func (store *historyStore) list(filter historyFilter) ([]agenthistory.Record, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	names, err := store.names()
	if err != nil {
		return nil, err
	}
	records := make([]agenthistory.Record, 0, min(len(names), filter.limit))
	for _, name := range slices.Backward(names) {
		if filter.id != "" && !historyRecordMatches(name, filter.id) {
			continue
		}
		if record, ok := store.read(name); ok && filter.matches(record) {
			records = append(records, record)
			if len(records) == filter.limit {
				break
			}
		}
	}
	return records, nil
}

func (store *historyStore) response(id string) (agenthistory.Record, []byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	names, err := store.names()
	if err != nil {
		return agenthistory.Record{}, nil, err
	}
	for _, name := range slices.Backward(names) {
		if filepath.Ext(name) != historyBodyExtension || !historyRecordMatches(name, id) {
			continue
		}
		file, err := store.root.Open(name)
		if err != nil {
			return agenthistory.Record{}, nil, err
		}
		defer file.Close()
		reader := bufio.NewReaderSize(file, historyMetadataLimit+1)
		record, ok := readHistoryMetadata(name, reader)
		if !ok || record.ResponseBodyBytes == nil {
			return agenthistory.Record{}, nil, errors.New("invalid history response record")
		}
		data := make([]byte, *record.ResponseBodyBytes)
		if _, err := io.ReadFull(reader, data); err != nil {
			return agenthistory.Record{}, nil, err
		}
		if _, err := reader.ReadByte(); err != io.EOF {
			return agenthistory.Record{}, nil, errors.New("invalid history response length")
		}
		return record, data, nil
	}
	return agenthistory.Record{}, nil, os.ErrNotExist
}

func parseHistoryFilter(query url.Values) (historyFilter, error) {
	filter := historyFilter{
		limit: 30, protocol: query.Get("protocolId"), state: query.Get("state"), requestID: query.Get("requestId"),
	}
	if text := query.Get("limit"); text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil || limit < 1 || limit > historyLimit {
			return filter, errors.New("history limit must be between 1 and 200")
		}
		filter.limit = limit
	}
	if text := query.Get("status"); text != "" {
		status, err := strconv.Atoi(text)
		if err != nil || status < 100 || status > 999 {
			return filter, errors.New("invalid HTTP history status")
		}
		filter.status = status
	}
	return filter, nil
}

func (agent *agentService) listHistory(w http.ResponseWriter, r *http.Request) {
	filter, err := parseHistoryFilter(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	records, err := agent.history.list(filter)
	if err != nil {
		http.Error(w, "could not read execution history", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (agent *agentService) historyItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		http.Error(w, "invalid execution ID", http.StatusBadRequest)
		return
	}
	records, err := agent.history.list(historyFilter{id: id, limit: 1})
	if err != nil {
		http.Error(w, "could not read execution history", http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, records[0])
}

func (agent *agentService) historyBody(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		http.Error(w, "invalid execution ID", http.StatusBadRequest)
		return
	}
	record, data, err := agent.history.response(id)
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "response body was not captured", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read history response", http.StatusInternalServerError)
		return
	}
	contentType := "application/octet-stream"
	if record.ResponseBodyText {
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
