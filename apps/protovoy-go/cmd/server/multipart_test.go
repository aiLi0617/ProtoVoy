package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostedHTTPBodyEncodings(t *testing.T) {
	requests := make(chan []byte, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			reader, err := r.MultipartReader()
			if err != nil {
				t.Error(err)
				return
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			if part.FormName() != "file" || part.FileName() != "report.bin" || part.Header.Get("Content-Type") != "application/octet-stream" {
				t.Error("multipart metadata was lost")
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Error(err)
			}
			requests <- data
		} else {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			requests <- data
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"method":"POST","bodyEncoding":"base64","body":"AP8="}`, http.StatusOK},
		{`{"method":"POST","multipart":[{"name":"file","fileName":"report.bin","contentType":"application/octet-stream","base64":true,"value":"AP8="}]}`, http.StatusOK},
		{`{"method":"POST","multipart":[{"name":"file\r\nX-Injected: value","value":"hello"}]}`, http.StatusBadRequest},
		{`{"method":"POST","bodyEncoding":"base64","body":"%%"}`, http.StatusBadRequest},
		{`{"method":"GET","eventStream":true,"multipart":[{"name":"file","value":"hello"}]}`, http.StatusBadRequest},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", strings.NewReader(test.body))
		r.Header.Set("Origin", "http://"+addr)
		r.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("got HTTP %d, want %d: %s", w.Code, test.status, w.Body.String())
		}
		if test.status == http.StatusOK {
			select {
			case data := <-requests:
				if string(data) != "\x00\xff" {
					t.Fatalf("unexpected bytes: %x", data)
				}
			default:
				t.Fatal("the upstream did not receive the request body")
			}
		}
	}
}
