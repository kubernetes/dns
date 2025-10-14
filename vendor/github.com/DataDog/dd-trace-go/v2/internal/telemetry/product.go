// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

type products struct {
	mu       sync.Mutex
	products map[Namespace]transport.Product
}

func (p *products) Add(namespace Namespace, enabled bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.products == nil {
		p.products = make(map[Namespace]transport.Product)
	}

	product := transport.Product{
		Enabled: enabled,
	}

	if err != nil {
		product.Error = transport.Error{
			Message: err.Error(),
		}
	}

	p.products[namespace] = product
}

func (p *products) Payload() transport.Payload {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.products) == 0 {
		return nil
	}

	res := transport.AppProductChange{
		Products: p.products,
	}
	p.products = nil
	return res
}
