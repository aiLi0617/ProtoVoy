package execution

import (
	"bytes"
	"encoding/base64"
	"errors"
	"mime"
	"mime/multipart"
	"net/textproto"
	"slices"
	"strings"
	"unicode"
)

type MultipartPart struct {
	Name        string  `json:"name"`
	Value       string  `json:"value"`
	FileName    *string `json:"fileName,omitempty"`
	ContentType *string `json:"contentType,omitempty"`
	Base64      bool    `json:"base64,omitempty"`
}

// EncodeBody prepares the wire body at the execution host, after variables
// have been resolved and before any network requests.
func (payload HTTPPayload) EncodeBody() (HTTPPayload, error) {
	if payload.EventStream && (payload.Body != "" || payload.BodyEncoding != "" || len(payload.Multipart) != 0) {
		return HTTPPayload{}, errors.New("SSE requires a request without a body")
	}
	if len(payload.Multipart) == 0 {
		switch payload.BodyEncoding {
		case "", "text":
		case "base64":
			data, err := base64.StdEncoding.DecodeString(payload.Body)
			if err != nil {
				return HTTPPayload{}, errors.New("invalid Base64 HTTP body")
			}
			payload.Body = string(data)
		default:
			return HTTPPayload{}, errors.New("select text or Base64 HTTP body encoding")
		}
		payload.BodyEncoding = ""
		return payload, nil
	}
	if payload.Body != "" || payload.BodyEncoding == "base64" {
		return HTTPPayload{}, errors.New("multipart requests cannot include a separate HTTP body or Base64 body encoding")
	}
	if len(payload.Multipart) > 128 {
		return HTTPPayload{}, errors.New("multipart requests support up to 128 parts")
	}
	for _, header := range payload.Headers {
		if strings.EqualFold(header[0], "Content-Type") {
			return HTTPPayload{}, errors.New("multipart Content-Type is generated automatically")
		}
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range payload.Multipart {
		headers, err := part.headers()
		if err != nil {
			return HTTPPayload{}, err
		}
		data := []byte(part.Value)
		if part.Base64 {
			data, err = base64.StdEncoding.DecodeString(part.Value)
			if err != nil {
				return HTTPPayload{}, errors.New("invalid Base64 multipart part")
			}
		}
		if body.Len()+len(data) > 1<<20 {
			return HTTPPayload{}, errors.New("multipart body exceeds 1 MiB")
		}
		output, err := writer.CreatePart(headers)
		if err != nil {
			return HTTPPayload{}, err
		}
		if _, err := output.Write(data); err != nil {
			return HTTPPayload{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return HTTPPayload{}, err
	}
	if body.Len() > 1<<20 {
		return HTTPPayload{}, errors.New("multipart body exceeds 1 MiB")
	}
	payload.Headers = append(slices.Clone(payload.Headers), [2]string{"Content-Type", writer.FormDataContentType()})
	payload.Body, payload.BodyEncoding, payload.Multipart = body.String(), "", nil
	return payload, nil
}

func (part MultipartPart) headers() (textproto.MIMEHeader, error) {
	if strings.TrimSpace(part.Name) == "" || len(part.Name) > 1024 || strings.ContainsFunc(part.Name, unicode.IsControl) {
		return nil, errors.New("invalid multipart field name")
	}
	parameters := map[string]string{"name": part.Name}
	if part.FileName != nil {
		if len(*part.FileName) > 1024 || strings.ContainsFunc(*part.FileName, unicode.IsControl) {
			return nil, errors.New("invalid multipart filename")
		}
		parameters["filename"] = *part.FileName
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", parameters))
	if part.ContentType != nil {
		value := *part.ContentType
		if len(value) > 1024 || strings.ContainsFunc(value, unicode.IsControl) {
			return nil, errors.New("invalid multipart content type")
		}
		mediaType, params, err := mime.ParseMediaType(value)
		if err != nil || !strings.Contains(mediaType, "/") {
			return nil, errors.New("invalid multipart content type")
		}
		header.Set("Content-Type", mime.FormatMediaType(mediaType, params))
	}
	return header, nil
}
