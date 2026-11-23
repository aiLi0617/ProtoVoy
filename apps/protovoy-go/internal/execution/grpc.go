package execution

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

const GRPCMessagePreviewLimit = 2048
const GRPCMessageLimit = 4 << 20
const GRPCDescriptorLimit = 512 << 10
const GRPCBridgePath = "/api/grpc"

var ErrGRPCReflection = errors.New("gRPC schema lookup failed. Check server reflection or import a descriptor")

func DecodeGRPCDescriptor(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if len(encoded) > base64.StdEncoding.EncodedLen(GRPCDescriptorLimit) {
		return nil, errors.New("gRPC descriptor set exceeds 512 KiB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > GRPCDescriptorLimit {
		return nil, errors.New("expected a Base64 gRPC descriptor set within 512 KiB")
	}
	return data, nil
}

type GRPCBridgeEvent struct {
	Type        string `json:"type"`
	Status      int    `json:"status,omitempty"`
	Size        int    `json:"size,omitempty"`
	Preview     []byte `json:"preview,omitempty"`
	JSONPreview string `json:"jsonPreview,omitempty"`
	Error       string `json:"error,omitempty"`
	Code        string `json:"code,omitempty"`
}

type GRPCRequest struct {
	Target, Service, Method, Mode, MessageBase64 string
	MessageJSON                                  string `json:"messageJson,omitempty"`
	DescriptorSetBase64                          string `json:"descriptorSetBase64,omitempty"`
	Metadata                                     [][2]string
	TimeoutMS                                    int           `json:"timeoutMs,omitempty"`
	Timeout                                      time.Duration `json:"-"` // native callers can set a finer deadline
}

type GRPCMessage struct {
	Size        int
	Preview     []byte
	JSONPreview string
}

var grpcIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidateGRPCSelection(service, method, mode string) error {
	for segment := range strings.SplitSeq(service, ".") {
		if !grpcIdentifier.MatchString(segment) {
			return errors.New("enter a valid gRPC service name")
		}
	}
	if !grpcIdentifier.MatchString(method) {
		return errors.New("enter a valid gRPC method name")
	}
	switch mode {
	case "", "unary", "server_streaming", "client_streaming", "bidi_streaming":
		return nil
	default:
		return errors.New("invalid gRPC streaming mode")
	}
}

func (request GRPCRequest) ResolveVariables(scopes ...map[string]string) (GRPCRequest, error) {
	resolver := newVariableResolver(scopes)
	request.Target = resolver.text(request.Target)
	request.Service = resolver.text(request.Service)
	request.Method = resolver.text(request.Method)
	request.MessageBase64 = resolver.text(request.MessageBase64)
	request.MessageJSON = resolver.text(request.MessageJSON)
	request.Metadata = resolver.headers(request.Metadata)
	return request, resolver.err
}
