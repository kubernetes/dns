// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package tracer

import (
	"golang.org/x/exp/constraints"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

//go:generate go run github.com/tinylib/msgp -unexported -marshal=false -o=span_event_msgp.go -tests=false

// SpanEvent represent an event at an instant in time related to this span, but not necessarily during the span.
type spanEvent struct {
	// Name is the name of event.
	Name string `msg:"name" json:"name"`

	// TimeUnixNano is the number of nanoseconds between the Unix epoch and this event.
	TimeUnixNano uint64 `msg:"time_unix_nano" json:"time_unix_nano"`

	// Attributes is a map of string to attribute.
	Attributes map[string]*spanEventAttribute `msg:"attributes" json:"-"`

	// RawAttributes is used when native span event serialization is not supported by the agent.
	RawAttributes map[string]any `msg:"-" json:"attributes,omitempty"`
}

type spanEventAttribute struct {
	Type        spanEventAttributeType   `msg:"type" json:"type"`
	StringValue string                   `msg:"string_value,omitempty"`
	BoolValue   bool                     `msg:"bool_value,omitempty" `
	IntValue    int64                    `msg:"int_value,omitempty" `
	DoubleValue float64                  `msg:"double_value,omitempty"`
	ArrayValue  *spanEventArrayAttribute `msg:"array_value,omitempty"`
}

type spanEventAttributeType int32

const (
	spanEventAttributeTypeString spanEventAttributeType = 0
	spanEventAttributeTypeBool   spanEventAttributeType = 1
	spanEventAttributeTypeInt    spanEventAttributeType = 2
	spanEventAttributeTypeDouble spanEventAttributeType = 3
	spanEventAttributeTypeArray  spanEventAttributeType = 4
)

type spanEventArrayAttribute struct {
	Values []*spanEventArrayAttributeValue `msg:"values" json:"values"`
}

type spanEventArrayAttributeValue struct {
	Type        spanEventArrayAttributeValueType `msg:"type"`
	StringValue string                           `msg:"string_value,omitempty"`
	BoolValue   bool                             `msg:"bool_value,omitempty"`
	IntValue    int64                            `msg:"int_value,omitempty"`
	DoubleValue float64                          `msg:"double_value,omitempty"`
}

type spanEventArrayAttributeValueType int32

const (
	spanEventArrayAttributeValueTypeString spanEventArrayAttributeValueType = 0
	spanEventArrayAttributeValueTypeBool   spanEventArrayAttributeValueType = 1
	spanEventArrayAttributeValueTypeInt    spanEventArrayAttributeValueType = 2
	spanEventArrayAttributeValueTypeDouble spanEventArrayAttributeValueType = 3
)

func toSpanEventAttributeMsg(attrs map[string]any) map[string]*spanEventAttribute {
	if attrs == nil {
		return nil
	}
	res := make(map[string]*spanEventAttribute, len(attrs))
	for key, val := range attrs {
		if msgVal := toSpanEventAttributeValueMsg(val); msgVal != nil {
			res[key] = msgVal
		} else {
			log.Warn("dropped unsupported span event attribute %s (unsupported type: %T)", key, val)
		}
	}
	return res
}

func toSpanEventAttributeValueMsg(v any) *spanEventAttribute {
	switch v := v.(type) {
	// string
	case string:
		return &spanEventAttribute{
			Type:        spanEventAttributeTypeString,
			StringValue: v,
		}
	// bool
	case bool:
		return &spanEventAttribute{
			Type:      spanEventAttributeTypeBool,
			BoolValue: v,
		}
	// int types
	case int:
		return intValue(v)
	case uint:
		return intValue(v)
	case int64:
		return intValue(v)
	case uint64:
		return intValue(v)
	case uint8:
		return intValue(v)
	case uint16:
		return intValue(v)
	case uint32:
		return intValue(v)
	case uintptr:
		return intValue(v)
	case int8:
		return intValue(v)
	case int16:
		return intValue(v)
	case int32:
		return intValue(v)
	// float types
	case float64:
		return floatValue(v)
	case float32:
		return floatValue(v)
	// string slice
	case []string:
		return stringSliceValue(v)
	// bool slice
	case []bool:
		return boolSliceValue(v)
	// int slice
	case []int:
		return intSliceValue(v)
	case []uint:
		return intSliceValue(v)
	case []int64:
		return intSliceValue(v)
	case []uint64:
		return intSliceValue(v)
	case []uint8:
		return intSliceValue(v)
	case []uint16:
		return intSliceValue(v)
	case []uint32:
		return intSliceValue(v)
	case []uintptr:
		return intSliceValue(v)
	case []int8:
		return intSliceValue(v)
	case []int16:
		return intSliceValue(v)
	case []int32:
		return intSliceValue(v)
	// float slice
	case []float64:
		return floatSliceValue(v)
	case []float32:
		return floatSliceValue(v)
	default:
		return nil
	}
}

func intValue[T constraints.Integer](v T) *spanEventAttribute {
	return &spanEventAttribute{
		Type:     spanEventAttributeTypeInt,
		IntValue: int64(v),
	}
}

func floatValue[T constraints.Float](v T) *spanEventAttribute {
	return &spanEventAttribute{
		Type:        spanEventAttributeTypeDouble,
		DoubleValue: float64(v),
	}
}

func stringSliceValue(values []string) *spanEventAttribute {
	arrayVal := make([]*spanEventArrayAttributeValue, 0, len(values))
	for _, v := range values {
		arrayVal = append(arrayVal, &spanEventArrayAttributeValue{
			Type:        spanEventArrayAttributeValueTypeString,
			StringValue: v,
		})
	}
	return &spanEventAttribute{
		Type: spanEventAttributeTypeArray,
		ArrayValue: &spanEventArrayAttribute{
			Values: arrayVal,
		},
	}
}

func boolSliceValue(values []bool) *spanEventAttribute {
	arrayVal := make([]*spanEventArrayAttributeValue, 0, len(values))
	for _, v := range values {
		arrayVal = append(arrayVal, &spanEventArrayAttributeValue{
			Type:      spanEventArrayAttributeValueTypeBool,
			BoolValue: v,
		})
	}
	return &spanEventAttribute{
		Type: spanEventAttributeTypeArray,
		ArrayValue: &spanEventArrayAttribute{
			Values: arrayVal,
		},
	}
}

func intSliceValue[T constraints.Integer](values []T) *spanEventAttribute {
	arrayVal := make([]*spanEventArrayAttributeValue, 0, len(values))
	for _, v := range values {
		arrayVal = append(arrayVal, &spanEventArrayAttributeValue{
			Type:     spanEventArrayAttributeValueTypeInt,
			IntValue: int64(v),
		})
	}
	return &spanEventAttribute{
		Type: spanEventAttributeTypeArray,
		ArrayValue: &spanEventArrayAttribute{
			Values: arrayVal,
		},
	}
}

func floatSliceValue[T constraints.Float](values []T) *spanEventAttribute {
	arrayVal := make([]*spanEventArrayAttributeValue, 0, len(values))
	for _, v := range values {
		arrayVal = append(arrayVal, &spanEventArrayAttributeValue{
			Type:        spanEventArrayAttributeValueTypeDouble,
			DoubleValue: float64(v),
		})
	}
	return &spanEventAttribute{
		Type: spanEventAttributeTypeArray,
		ArrayValue: &spanEventArrayAttribute{
			Values: arrayVal,
		},
	}
}
