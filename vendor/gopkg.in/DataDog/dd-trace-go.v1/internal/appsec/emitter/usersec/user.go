// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package usersec

import (
	"context"

	"gopkg.in/DataDog/dd-trace-go.v1/appsec/events"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo"
)

const errorLog = `
appsec: user login monitoring ignored: could not find the http handler instrumentation metadata in the request context:
	the request handler is not being monitored by a middleware function or the provided context is not the expected request context
`

type (
	// UserLoginOperation type representing a call to appsec.SetUser(). It gets both created and destroyed in a single
	// call to ExecuteUserIDOperation
	UserLoginOperation struct {
		dyngo.Operation
	}
	// UserLoginOperationArgs is the user ID operation arguments.
	UserLoginOperationArgs struct{}

	// UserLoginOperationRes is the user ID operation results.
	UserLoginOperationRes struct {
		UserID    string
		SessionID string
		Success   bool
	}
)

func StartUserLoginOperation(ctx context.Context, args UserLoginOperationArgs) (*UserLoginOperation, *error) {
	parent, _ := dyngo.FromContext(ctx)
	op := &UserLoginOperation{Operation: dyngo.NewOperation(parent)}
	var err error
	dyngo.OnData(op, func(e *events.BlockingSecurityEvent) { err = e })
	dyngo.StartOperation(op, args)
	return op, &err
}

func (op *UserLoginOperation) Finish(args UserLoginOperationRes) {
	dyngo.FinishOperation(op, args)
}

func (UserLoginOperationArgs) IsArgOf(*UserLoginOperation)   {}
func (UserLoginOperationRes) IsResultOf(*UserLoginOperation) {}
