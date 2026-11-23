//go:build !js

package execution

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"google.golang.org/protobuf/encoding/protowire"
)

func (client GRPCClient) reflectGRPCSchema(ctx context.Context, target url.URL, input GRPCRequest) (*grpcSchema, error) {
	// FileContainingSymbol is field 4 in both reflection versions.
	request := protowire.AppendString(protowire.AppendTag(nil, 4, protowire.BytesType), input.Service)
	body, err := appendGRPCFrame(nil, request)
	if err != nil {
		return nil, err
	}
	lookup := func(version string) (*grpcSchema, error) {
		target.Path = "/grpc.reflection." + version + ".ServerReflection/ServerReflectionInfo"
		var response []byte
		err := client.call(ctx, target.String(), body, input.Metadata, false, GRPCDescriptorLimit+1024, nil, func(data []byte) error {
			response = data
			return nil
		})
		if err != nil {
			return nil, err
		}
		set, err := reflectionDescriptorSet(response)
		if err != nil {
			return nil, err
		}
		return decodeGRPCSchema(set, input)
	}
	schema, v1Err := lookup("v1")
	if v1Err == nil {
		return schema, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	schema, alphaErr := lookup("v1alpha")
	if alphaErr == nil {
		return schema, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("%w (v1: %v, v1alpha: %v)", ErrGRPCReflection, v1Err, alphaErr)
}

func reflectionDescriptorSet(data []byte) ([]byte, error) {
	var set []byte
	for len(data) > 0 {
		field, kind, tagSize := protowire.ConsumeTag(data)
		if tagSize < 0 || kind == protowire.StartGroupType || kind == protowire.EndGroupType {
			return nil, errors.New("invalid gRPC reflection response")
		}
		data = data[tagSize:]
		size := protowire.ConsumeFieldValue(field, kind, data)
		if size < 0 {
			return nil, errors.New("invalid gRPC reflection response")
		}
		switch field {
		case 4:
			if kind != protowire.BytesType || set != nil {
				return nil, errors.New("invalid gRPC reflection descriptor field")
			}
			// FileDescriptorResponse and FileDescriptorSet both encode the
			// repeated FileDescriptorProto bytes as field 1.
			set, _ = protowire.ConsumeBytes(data)
		case 5, 6, 7:
			return nil, errors.New("gRPC reflection did not return file descriptors")
		}
		data = data[size:]
	}
	if len(set) == 0 || len(set) > GRPCDescriptorLimit {
		return nil, errors.New("expected reflected descriptors within 512 KiB")
	}
	return set, nil
}
