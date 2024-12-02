// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package usersec

import (
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/config"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/emitter/usersec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/emitter/waf"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/emitter/waf/addresses"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/listener"
)

type Feature struct{}

func (*Feature) String() string {
	return "User Security"
}

func (*Feature) Stop() {}

func NewUserSecFeature(cfg *config.Config, rootOp dyngo.Operation) (listener.Feature, error) {
	if !cfg.SupportedAddresses.AnyOf(
		addresses.UserIDAddr,
		addresses.UserSessionIDAddr,
		addresses.UserLoginSuccessAddr,
		addresses.UserLoginFailureAddr) {
		return nil, nil
	}

	feature := &Feature{}
	dyngo.OnFinish(rootOp, feature.OnFinish)
	return feature, nil
}

func (*Feature) OnFinish(op *usersec.UserLoginOperation, res usersec.UserLoginOperationRes) {
	builder := addresses.NewAddressesBuilder().
		WithUserID(res.UserID).
		WithUserSessionID(res.SessionID)

	if res.Success {
		builder = builder.WithUserLoginSuccess()
	} else {
		builder = builder.WithUserLoginFailure()
	}

	dyngo.EmitData(op, waf.RunEvent{
		Operation:      op,
		RunAddressData: builder.Build(),
	})
}
