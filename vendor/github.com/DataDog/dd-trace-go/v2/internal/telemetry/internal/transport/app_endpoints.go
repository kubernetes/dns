// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

package transport

type AppEndpoints struct {
	// isFirst must be set to `true` for the first payload emitted in a given
	// service instance, causing the back-end to initiate a new set of API
	// definitions. When `false`, new messages are merged in with the other API
	// definitions accumulated so far for this instance.
	IsFirst   bool          `json:"is_first"`
	Endpoints []AppEndpoint `json:"endpoints"`
}

func (AppEndpoints) RequestType() RequestType {
	return RequestTypeAppEndpoints
}

type AppEndpoint struct {
	Kind             string                      `json:"type,omitempty"`
	Method           string                      `json:"method,omitempty"`
	Path             string                      `json:"path,omitempty"`
	OperationName    string                      `json:"operation_name"`
	ResourceName     string                      `json:"resource_name"`
	RequestBodyType  []string                    `json:"request_body_type,omitempty"`
	ResponseBodyType []string                    `json:"response_body_type,omitempty"`
	ResponseCode     []int                       `json:"response_code,omitempty"`
	Authentication   []AppEndpointAuthentication `json:"authentication,omitempty"`
	Metadata         map[string]any              `json:"metadata,omitempty"`
}

type AppEndpointAuthentication string

const (
	AppEndpointAuthenticationJWT     AppEndpointAuthentication = "JWT"
	AppEndpointAuthenticationBasic   AppEndpointAuthentication = "basic"
	AppEndpointAuthenticationOAuth   AppEndpointAuthentication = "oauth"
	AppEndpointAuthenticationOIDC    AppEndpointAuthentication = "OIDC"
	AppEndpointAuthenticationAPIKey  AppEndpointAuthentication = "api_key"
	AppEndpointAuthenticationSession AppEndpointAuthentication = "session"
	AppEndpointAuthenticationMTLS    AppEndpointAuthentication = "mTLS"
	AppEndpointAuthenticationSAML    AppEndpointAuthentication = "SAML"
	AppEndpointAuthenticationLDAP    AppEndpointAuthentication = "LDAP"
	AppEndpointAuthenticationForm    AppEndpointAuthentication = "Form"
	AppEndpointAuthenticationOther   AppEndpointAuthentication = "other"
)
