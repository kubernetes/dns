// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type MessageBatch []Message

func (MessageBatch) RequestType() RequestType {
	return RequestTypeMessageBatch
}

type Message struct {
	RequestType RequestType `json:"request_type"`
	Payload     Payload     `json:"payload"`
}
