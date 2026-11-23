//go:build !js

package execution

import (
	"encoding/json"
	"errors"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type grpcSchema struct {
	method protoreflect.MethodDescriptor
	types  *dynamicpb.Types
}

func grpcSchemaFor(input GRPCRequest) (*grpcSchema, error) {
	data, err := DecodeGRPCDescriptor(input.DescriptorSetBase64)
	if err != nil || len(data) == 0 {
		return nil, err
	}
	return decodeGRPCSchema(data, input)
}

func decodeGRPCSchema(data []byte, input GRPCRequest) (*grpcSchema, error) {
	var set descriptorpb.FileDescriptorSet
	if err := (proto.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(data, &set); err != nil ||
		len(set.File) == 0 || len(set.File) > 256 {
		return nil, errors.New("invalid gRPC descriptor set or file count")
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, errors.New("invalid gRPC descriptor set; include imported .proto files")
	}
	descriptor, err := files.FindDescriptorByName(protoreflect.FullName(input.Service))
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if err != nil || !ok {
		return nil, errors.New("gRPC service was not found in the descriptor set")
	}
	method := service.Methods().ByName(protoreflect.Name(input.Method))
	if method == nil {
		return nil, errors.New("gRPC method was not found in the descriptor set")
	}
	return &grpcSchema{method: method, types: dynamicpb.NewTypes(files)}, nil
}

func encodeGRPCRequest(input GRPCRequest, schema *grpcSchema, multiple bool) ([]byte, error) {
	payload := strings.TrimSpace(input.MessageJSON)
	if payload == "" {
		return encodeGRPCMessages(input.MessageBase64, multiple)
	}
	if schema == nil {
		return nil, errors.New("gRPC JSON requires a descriptor set")
	}
	if len(input.MessageJSON) > grpcRequestLimit {
		return nil, errors.New("gRPC request JSON exceeds 1 MiB")
	}
	var body []byte
	encode := func(value []byte) error {
		message := dynamicpb.NewMessage(schema.method.Input())
		if err := (protojson.UnmarshalOptions{Resolver: schema.types, RecursionLimit: 64}).Unmarshal(value, message); err != nil {
			return errors.New("gRPC request JSON does not match the method input")
		}
		data, err := proto.Marshal(message)
		if err != nil {
			return errors.New("could not encode the gRPC request message")
		}
		body, err = appendGRPCFrame(body, data)
		return err
	}
	if !multiple || payload[0] != '[' {
		err := encode([]byte(payload))
		return body, err
	}
	if !json.Valid([]byte(payload)) {
		return nil, errors.New("invalid gRPC request JSON array")
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	_, _ = decoder.Token() // The opening bracket was checked above.
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid gRPC request JSON array")
		}
		if err := encode(value); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func (schema *grpcSchema) preview(data []byte) string {
	if schema == nil || len(data) > GRPCMessagePreviewLimit {
		return ""
	}
	message := dynamicpb.NewMessage(schema.method.Output())
	if err := (proto.UnmarshalOptions{Resolver: schema.types, RecursionLimit: 64}).Unmarshal(data, message); err != nil {
		return ""
	}
	output, err := (protojson.MarshalOptions{Resolver: schema.types}).Marshal(message)
	if err != nil || len(output) > GRPCMessagePreviewLimit {
		return ""
	}
	return string(output)
}
