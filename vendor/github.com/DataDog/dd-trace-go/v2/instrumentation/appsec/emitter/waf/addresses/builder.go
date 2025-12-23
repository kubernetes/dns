// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package addresses

import (
	"net/netip"
	"strconv"

	"github.com/DataDog/go-libddwaf/v4"
)

const contextProcessKey = "waf.context.processor"

type RunAddressDataBuilder struct {
	libddwaf.RunAddressData
}

func NewAddressesBuilder() *RunAddressDataBuilder {
	return &RunAddressDataBuilder{
		RunAddressData: libddwaf.RunAddressData{
			Persistent: make(map[string]any, 1),
			Ephemeral:  make(map[string]any, 1),
			TimerKey:   WAFScope, // Default value for TimerKey
		},
	}
}

func (b *RunAddressDataBuilder) WithMethod(method string) *RunAddressDataBuilder {
	b.Persistent[ServerRequestMethodAddr] = method
	return b
}

func (b *RunAddressDataBuilder) WithRawURI(uri string) *RunAddressDataBuilder {
	b.Persistent[ServerRequestRawURIAddr] = uri
	return b
}

func (b *RunAddressDataBuilder) WithHeadersNoCookies(headers map[string][]string) *RunAddressDataBuilder {
	if len(headers) == 0 {
		headers = nil
	}
	b.Persistent[ServerRequestHeadersNoCookiesAddr] = headers
	return b
}

func (b *RunAddressDataBuilder) WithCookies(cookies map[string][]string) *RunAddressDataBuilder {
	if len(cookies) == 0 {
		return b
	}
	b.Persistent[ServerRequestCookiesAddr] = cookies
	return b
}

func (b *RunAddressDataBuilder) WithQuery(query map[string][]string) *RunAddressDataBuilder {
	if len(query) == 0 {
		query = nil
	}
	b.Persistent[ServerRequestQueryAddr] = query
	return b
}

func (b *RunAddressDataBuilder) WithPathParams(params map[string]string) *RunAddressDataBuilder {
	if len(params) == 0 {
		return b
	}
	b.Persistent[ServerRequestPathParamsAddr] = params
	return b
}

func (b *RunAddressDataBuilder) WithRequestBody(body any) *RunAddressDataBuilder {
	if body == nil {
		return b
	}
	b.Persistent[ServerRequestBodyAddr] = body
	return b
}

func (b *RunAddressDataBuilder) WithResponseBody(body any) *RunAddressDataBuilder {
	if body == nil {
		return b
	}
	b.Persistent[ServerResponseBodyAddr] = body
	return b
}

func (b *RunAddressDataBuilder) WithResponseStatus(status int) *RunAddressDataBuilder {
	if status == 0 {
		return b
	}
	b.Persistent[ServerResponseStatusAddr] = strconv.Itoa(status)
	return b
}

func (b *RunAddressDataBuilder) WithResponseHeadersNoCookies(headers map[string][]string) *RunAddressDataBuilder {
	if len(headers) == 0 {
		return b
	}
	b.Persistent[ServerResponseHeadersNoCookiesAddr] = headers
	return b
}

func (b *RunAddressDataBuilder) WithClientIP(ip netip.Addr) *RunAddressDataBuilder {
	if !ip.IsValid() {
		return b
	}
	b.Persistent[ClientIPAddr] = ip.String()
	return b
}

func (b *RunAddressDataBuilder) WithUserID(id string) *RunAddressDataBuilder {
	if id == "" {
		return b
	}
	b.Persistent[UserIDAddr] = id
	return b
}

func (b *RunAddressDataBuilder) WithUserLogin(login string) *RunAddressDataBuilder {
	if login == "" {
		return b
	}
	b.Persistent[UserLoginAddr] = login
	return b
}

func (b *RunAddressDataBuilder) WithUserOrg(org string) *RunAddressDataBuilder {
	if org == "" {
		return b
	}
	b.Persistent[UserOrgAddr] = org
	return b
}

func (b *RunAddressDataBuilder) WithUserSessionID(id string) *RunAddressDataBuilder {
	if id == "" {
		return b
	}
	b.Persistent[UserSessionIDAddr] = id
	return b

}

func (b *RunAddressDataBuilder) WithUserLoginSuccess() *RunAddressDataBuilder {
	b.Persistent[UserLoginSuccessAddr] = nil
	return b
}

func (b *RunAddressDataBuilder) WithUserLoginFailure() *RunAddressDataBuilder {
	b.Persistent[UserLoginFailureAddr] = nil
	return b
}

func (b *RunAddressDataBuilder) WithFilePath(file string) *RunAddressDataBuilder {
	if file == "" {
		return b
	}
	b.Ephemeral[ServerIOFSFileAddr] = file
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardMethod(method string) *RunAddressDataBuilder {
	if method == "" {
		return b
	}
	b.Ephemeral[ServerIONetRequestMethodAddr] = method
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardRequestHeaders(headers map[string][]string) *RunAddressDataBuilder {
	if len(headers) == 0 {
		return b
	}
	b.Ephemeral[ServerIONetRequestHeadersAddr] = headers
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardURL(url string) *RunAddressDataBuilder {
	if url == "" {
		return b
	}
	b.Ephemeral[ServerIONetURLAddr] = url
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardRequestBody(body any) *RunAddressDataBuilder {
	if body == nil {
		return b
	}
	b.Ephemeral[ServerIONetRequestBodyAddr] = body
	return b
}

func (b *RunAddressDataBuilder) WithDownwardResponseStatus(status int) *RunAddressDataBuilder {
	if status == 0 {
		return b
	}
	b.Ephemeral[ServerIONetResponseStatusAddr] = strconv.Itoa(status)
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardResponseHeaders(headers map[string][]string) *RunAddressDataBuilder {
	if len(headers) == 0 {
		return b
	}
	b.Ephemeral[ServerIONetResponseHeadersAddr] = headers
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDownwardResponseBody(body any) *RunAddressDataBuilder {
	if body == nil {
		return b
	}
	b.Ephemeral[ServerIONetResponseBodyAddr] = body
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDBStatement(statement string) *RunAddressDataBuilder {
	if statement == "" {
		return b
	}
	b.Ephemeral[ServerDBStatementAddr] = statement
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithDBType(driver string) *RunAddressDataBuilder {
	if driver == "" {
		return b
	}
	b.Ephemeral[ServerDBTypeAddr] = driver
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithSysExecCmd(cmd []string) *RunAddressDataBuilder {
	if len(cmd) == 0 {
		return b
	}
	b.Ephemeral[ServerSysExecCmd] = cmd
	b.TimerKey = RASPScope
	return b
}

func (b *RunAddressDataBuilder) WithGRPCMethod(method string) *RunAddressDataBuilder {
	if method == "" {
		return b
	}
	b.Persistent[GRPCServerMethodAddr] = method
	return b
}

func (b *RunAddressDataBuilder) WithGRPCRequestMessage(message any) *RunAddressDataBuilder {
	if message == nil {
		return b
	}
	b.Ephemeral[GRPCServerRequestMessageAddr] = message
	return b
}

func (b *RunAddressDataBuilder) WithGRPCRequestMetadata(metadata map[string][]string) *RunAddressDataBuilder {
	if len(metadata) == 0 {
		return b
	}
	b.Persistent[GRPCServerRequestMetadataAddr] = metadata
	return b
}

func (b *RunAddressDataBuilder) WithGRPCResponseMessage(message any) *RunAddressDataBuilder {
	if message == nil {
		return b
	}
	b.Ephemeral[GRPCServerResponseMessageAddr] = message
	return b
}

func (b *RunAddressDataBuilder) WithGRPCResponseMetadataHeaders(headers map[string][]string) *RunAddressDataBuilder {
	if len(headers) == 0 {
		return b
	}
	b.Persistent[GRPCServerResponseMetadataHeadersAddr] = headers
	return b
}

func (b *RunAddressDataBuilder) WithGRPCResponseMetadataTrailers(trailers map[string][]string) *RunAddressDataBuilder {
	if len(trailers) == 0 {
		return b
	}
	b.Persistent[GRPCServerResponseMetadataTrailersAddr] = trailers
	return b
}

func (b *RunAddressDataBuilder) WithGRPCResponseStatusCode(status int) *RunAddressDataBuilder {
	if status == 0 {
		return b
	}
	b.Persistent[GRPCServerResponseStatusCodeAddr] = strconv.Itoa(status)
	return b
}

func (b *RunAddressDataBuilder) WithGraphQLResolver(fieldName string, args map[string]any) *RunAddressDataBuilder {
	if _, ok := b.Ephemeral[GraphQLServerResolverAddr]; !ok {
		b.Ephemeral[GraphQLServerResolverAddr] = make(map[string]any, 1)
	}

	b.Ephemeral[GraphQLServerResolverAddr].(map[string]any)[fieldName] = args
	return b
}

func (b *RunAddressDataBuilder) ExtractSchema() *RunAddressDataBuilder {
	if _, ok := b.Persistent[contextProcessKey]; !ok {
		b.Persistent[contextProcessKey] = make(map[string]bool, 1)
	}

	b.Persistent[contextProcessKey].(map[string]bool)["extract-schema"] = true
	return b
}

func (b *RunAddressDataBuilder) NoExtractSchema() *RunAddressDataBuilder {
	if _, ok := b.Persistent[contextProcessKey]; !ok {
		b.Persistent[contextProcessKey] = make(map[string]bool, 1)
	}

	b.Persistent[contextProcessKey].(map[string]bool)["extract-schema"] = false
	return b
}

func (b *RunAddressDataBuilder) Build() libddwaf.RunAddressData {
	return b.RunAddressData
}
