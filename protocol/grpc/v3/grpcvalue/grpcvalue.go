// Package grpcvalue 负责把业务 handler 的返回值转换成 gRPC 可编码的 proto.Message。
//
// 背景：
// - gRPC 只能编码 proto.Message
// - 框架允许 handler 返回任意 JSON 友好的 Go 值（map/struct/slice/标量）
//
// 因此这里统一做一次"JSON -> google.protobuf.Struct/Value"的桥接。
package grpcvalue

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// ToStruct 把任意 JSON 友好的值转换成 *structpb.Struct。
//
// 与 structpb.NewStruct 的区别：后者要求输入已经是 map[string]any 且值类型受限，
// 这里允许传入包含 Go struct 的嵌套 map（例如 map[string]any{"account": accountView}）。
func ToStruct(v any) (*structpb.Struct, error) {
	msg, err := ToMessage(v)
	if err != nil {
		return nil, err
	}
	s, ok := msg.(*structpb.Struct)
	if !ok {
		return nil, fmt.Errorf("grpc value is not a JSON object: %T", v)
	}
	return s, nil
}

// ToMessage 把任意返回值转换成 proto.Message。
//
// 规则：
// - nil -> google.protobuf.Empty
// - proto.Message -> 原样返回
// - 其他值 -> 先 JSON 序列化，再按 JSON 形态映射到 Struct / ListValue / Value
func ToMessage(v any) (proto.Message, error) {
	if v == nil {
		return &emptypb.Empty{}, nil
	}
	if msg, ok := v.(proto.Message); ok {
		return msg, nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("grpc response is not JSON-serializable: %w", err)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return &emptypb.Empty{}, nil
	}

	switch data[0] {
	case '{':
		out := &structpb.Struct{}
		if err := protojson.Unmarshal(data, out); err != nil {
			return nil, fmt.Errorf("grpc response struct conversion failed: %w", err)
		}
		return out, nil
	case '[':
		out := &structpb.ListValue{}
		if err := protojson.Unmarshal(data, out); err != nil {
			return nil, fmt.Errorf("grpc response list conversion failed: %w", err)
		}
		return out, nil
	default:
		out := &structpb.Value{}
		if err := protojson.Unmarshal(data, out); err != nil {
			return nil, fmt.Errorf("grpc response value conversion failed: %w", err)
		}
		return out, nil
	}
}
