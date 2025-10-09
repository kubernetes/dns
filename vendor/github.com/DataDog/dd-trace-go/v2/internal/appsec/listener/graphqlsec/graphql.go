// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package graphqlsec

import (
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/graphqlsec"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/addresses"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/emitter/waf"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/listener"
)

type Feature struct{}

func (*Feature) String() string {
	return "GraphQL Security"
}

func (*Feature) Stop() {}

func (f *Feature) OnResolveField(op *graphqlsec.ResolveOperation, args graphqlsec.ResolveOperationArgs) {
	dyngo.EmitData(op, waf.RunEvent{
		Operation: op,
		RunAddressData: addresses.NewAddressesBuilder().
			WithGraphQLResolver(args.FieldName, args.Arguments).
			Build(),
	})
}

func NewGraphQLSecFeature(config *config.Config, rootOp dyngo.Operation) (listener.Feature, error) {
	if !config.SupportedAddresses.AnyOf(addresses.GraphQLServerResolverAddr) {
		return nil, nil
	}

	feature := &Feature{}
	dyngo.On(rootOp, feature.OnResolveField)

	return feature, nil
}
