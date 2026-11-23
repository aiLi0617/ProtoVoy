package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

func CollectHTTPStatus(ctx context.Context, client HTTPClient, input HTTPRequest, protocol, bodyContains string) (int, error) {
	response, err := client.Execute(ctx, input)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	checkProtocol := (protocol == "graphql" || protocol == "jsonrpc" || protocol == "soap") &&
		response.StatusCode >= 200 && response.StatusCode < 300
	if checkProtocol {
		const maxResultBytes = 8 << 20
		body, err := io.ReadAll(io.LimitReader(response.Body, maxResultBytes+1))
		if err != nil {
			return response.StatusCode, errors.New("could not read the complete HTTP response")
		}
		if len(body) > maxResultBytes {
			return response.StatusCode, fmt.Errorf("%s response exceeds 8 MiB", protocol)
		}
		switch protocol {
		case "jsonrpc":
			err = checkCollectionJSONRPCResult(body)
		case "soap":
			err = checkCollectionSOAPResult(body)
		case "graphql":
			err = CheckCollectionGraphQLResult(body)
		}
		if err != nil {
			return response.StatusCode, err
		}
		if bodyContains != "" && !bytes.Contains(body, []byte(bodyContains)) {
			return response.StatusCode, errors.New("response body does not contain the expected text")
		}
		return response.StatusCode, nil
	}
	if bodyContains != "" {
		found, err := StreamContains(response.Body, []byte(bodyContains))
		if err != nil {
			return response.StatusCode, errors.New("could not read the complete HTTP response")
		}
		if !found {
			return response.StatusCode, errors.New("response body does not contain the expected text")
		}
		return response.StatusCode, nil
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return response.StatusCode, errors.New("could not read the complete HTTP response")
	}
	return response.StatusCode, nil
}

// StreamContains retains only the suffix that could begin a match in the next chunk.
func StreamContains(reader io.Reader, needle []byte) (bool, error) {
	if len(needle) == 0 {
		_, err := io.Copy(io.Discard, reader)
		return true, err
	}
	const chunkSize = 32 << 10
	buffer := make([]byte, chunkSize+len(needle)-1)
	overlap := 0
	for {
		n, err := reader.Read(buffer[overlap:])
		size := overlap + n
		if bytes.Contains(buffer[:size], needle) {
			if err == nil {
				_, err = io.Copy(io.Discard, reader)
			}
			if err == io.EOF {
				err = nil
			}
			return true, err
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return false, err
		}
		overlap = min(size, len(needle)-1)
		copy(buffer, buffer[size-overlap:size])
	}
}

func checkCollectionJSONRPCResult(body []byte) error {
	invalid := errors.New("invalid JSON-RPC 2.0 response")
	var response struct {
		Version string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
		ID      json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Version != "2.0" ||
		len(response.ID) == 0 || (response.Result == nil) == (response.Error == nil) {
		return invalid
	}
	if response.Error == nil {
		return nil
	}
	var failure struct {
		Code    *int64
		Message *string
	}
	if err := json.Unmarshal(response.Error, &failure); err != nil || failure.Code == nil || failure.Message == nil {
		return invalid
	}
	return fmt.Errorf("JSON-RPC error %d", *failure.Code)
}

func CheckCollectionGraphQLResult(body []byte) error {
	var result struct {
		Data   json.RawMessage
		Errors []json.RawMessage
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return errors.New("invalid GraphQL response")
	}
	if len(result.Errors) != 0 {
		return fmt.Errorf("GraphQL returned %d error(s)", len(result.Errors))
	}
	if len(result.Data) == 0 {
		return errors.New("GraphQL response is missing data")
	}
	return nil
}

func checkCollectionSOAPResult(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil // One-way operations can return no envelope.
	}
	const soap11 = "http://schemas.xmlsoap.org/soap/envelope/"
	const soap12 = "http://www.w3.org/2003/05/soap-envelope"
	invalid := errors.New("invalid SOAP response envelope")
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var namespace string
	var hasBody, inBody, fault bool
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return invalid
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch depth {
			case 0:
				if namespace != "" || value.Name.Local != "Envelope" ||
					(value.Name.Space != soap11 && value.Name.Space != soap12) {
					return invalid
				}
				namespace = value.Name.Space
			case 1:
				inBody = value.Name == (xml.Name{Space: namespace, Local: "Body"})
				if inBody && hasBody {
					return invalid
				}
				hasBody = hasBody || inBody
			case 2:
				fault = fault || inBody && value.Name == (xml.Name{Space: namespace, Local: "Fault"})
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(value)) != 0 {
				return invalid
			}
		case xml.Directive:
			return invalid
		}
	}
	if !hasBody {
		return invalid
	}
	if fault {
		return errors.New("SOAP Fault")
	}
	return nil
}
