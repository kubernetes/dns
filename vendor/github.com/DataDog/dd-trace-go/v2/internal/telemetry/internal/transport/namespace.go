// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type Namespace string

const (
	NamespaceGeneral      Namespace = "general"
	NamespaceTracers      Namespace = "tracers"
	NamespaceProfilers    Namespace = "profilers"
	NamespaceAppSec       Namespace = "appsec"
	NamespaceIAST         Namespace = "iast"
	NamespaceTelemetry    Namespace = "telemetry"
	NamespaceCIVisibility Namespace = "civisibility"
	NamespaceMLObs        Namespace = "mlobs"
	NamespaceRUM          Namespace = "rum"
)
