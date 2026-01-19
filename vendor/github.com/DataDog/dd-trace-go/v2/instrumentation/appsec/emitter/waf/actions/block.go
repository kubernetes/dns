// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package actions

import (
	"bytes"
	_ "embed" // embed is used to embed the blocked-template.json and blocked-template.html files
	"net/http"
	"os"
	"strings"
	"unsafe"

	"github.com/DataDog/dd-trace-go/v2/appsec/events"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// blockedTemplateJSON is the default JSON template used to write responses for blocked requests
//
//go:embed blocked-template.json
var blockedTemplateJSON []byte

// blockedTemplateHTML is the default HTML template used to write responses for blocked requests
//
//go:embed blocked-template.html
var blockedTemplateHTML []byte

const (
	envBlockedTemplateHTML      = "DD_APPSEC_HTTP_BLOCKED_TEMPLATE_HTML"
	envBlockedTemplateJSON      = "DD_APPSEC_HTTP_BLOCKED_TEMPLATE_JSON"
	securityResponsePlaceholder = "[security_response_id]"
)

func init() {
	for key, template := range map[string]*[]byte{envBlockedTemplateJSON: &blockedTemplateJSON, envBlockedTemplateHTML: &blockedTemplateHTML} {
		if path, ok := env.Lookup(key); ok {
			if t, err := os.ReadFile(path); err != nil {
				log.Error("Could not read template at %q: %v", path, err.Error())
			} else {
				*template = t
			}
		}
	}

	registerActionHandler("block_request", NewBlockAction)
}

type (
	// blockActionParams are the dynamic parameters to be provided to a "block_request"
	// action type upon invocation
	blockActionParams struct {
		// GRPCStatusCode is the gRPC status code to be returned. Since 0 is the OK status, the value defaults to Abort (10)
		// if not set to OK (0).
		GRPCStatusCode     int
		StatusCode         int
		Type               string
		SecurityResponseID string
	}
	// GRPCWrapper is an opaque prototype abstraction for a gRPC handler (to avoid importing grpc)
	// that returns a status code and an error
	GRPCWrapper func() (uint32, error)

	// BlockGRPC are actions that interact with a GRPC request flow
	BlockGRPC struct {
		GRPCWrapper
	}

	// BlockHTTP are actions that interact with an HTTP request flow
	BlockHTTP struct {
		http.Handler
	}
)

func (b *blockActionParams) Decode(p map[string]any) error {
	for k := range p {
		switch k {
		case "grpc_status_code":
			v, err := decodeInt(p, k)
			if err != nil {
				return err
			}
			b.GRPCStatusCode = v
		case "status_code":
			v, err := decodeInt(p, k)
			if err != nil {
				return err
			}
			b.StatusCode = v
		case "security_response_id":
			v, err := decodeStr(p, k)
			if err != nil {
				return err
			}
			b.SecurityResponseID = v
		case "type":
			v, err := decodeStr(p, k)
			if err != nil {
				return err
			}
			b.Type = v
		default:
			// We ignore any other field.
		}
	}
	return nil
}

func (a *BlockGRPC) EmitData(op dyngo.Operation) {
	dyngo.EmitData(op, a)
	dyngo.EmitData(op, &events.BlockingSecurityEvent{})
}

func (a *BlockHTTP) EmitData(op dyngo.Operation) {
	dyngo.EmitData(op, a)
	dyngo.EmitData(op, &events.BlockingSecurityEvent{})
}

func newGRPCBlockRequestAction(status int) *BlockGRPC {
	return &BlockGRPC{GRPCWrapper: newGRPCBlockHandler(status)}
}

func newGRPCBlockHandler(status int) GRPCWrapper {
	return func() (uint32, error) {
		return uint32(status), &events.BlockingSecurityEvent{}
	}
}

func blockParamsFromMap(params map[string]any) (blockActionParams, error) {
	p := blockActionParams{
		Type:           "auto",
		StatusCode:     403,
		GRPCStatusCode: 10,
	}
	if err := p.Decode(params); err != nil {
		return p, err
	}
	return p, nil
}

// NewBlockAction creates an action for the "block_request" action type
func NewBlockAction(params map[string]any) []Action {
	p, err := blockParamsFromMap(params)
	if err != nil {
		log.Debug("appsec: couldn't decode redirect action parameters")
		return nil
	}
	return []Action{
		newHTTPBlockRequestAction(p.StatusCode, p.Type, p.SecurityResponseID),
		newGRPCBlockRequestAction(p.GRPCStatusCode),
	}
}

func newHTTPBlockRequestAction(status int, template string, securityResponseID string) *BlockHTTP {
	return &BlockHTTP{Handler: newBlockHandler(status, template, securityResponseID)}
}

// newBlockHandler creates, initializes and returns a new BlockRequestAction
func newBlockHandler(status int, template string, securityResponseID string) http.Handler {
	htmlHandler := newBlockRequestHandler(status, "text/html", blockedTemplateHTML, securityResponseID)
	jsonHandler := newBlockRequestHandler(status, "application/json", blockedTemplateJSON, securityResponseID)
	switch template {
	case "json":
		return jsonHandler
	case "html":
		return htmlHandler
	default:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := jsonHandler
			hdr := r.Header.Get("Accept")
			htmlIdx := strings.Index(hdr, "text/html")
			jsonIdx := strings.Index(hdr, "application/json")
			// Switch to html handler if text/html comes before application/json in the Accept header
			if htmlIdx != -1 && (jsonIdx == -1 || htmlIdx < jsonIdx) {
				h = htmlHandler
			}
			h.ServeHTTP(w, r)
		})
	}
}

func newBlockRequestHandler(status int, ct string, payload []byte, securityResponseID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		w.Write(renderSecurityResponsePayload(payload, securityResponseID))
	})
}

func renderSecurityResponsePayload(payload []byte, securityResponseID string) []byte {
	securityResponseBytes := []byte(securityResponseID)
	placeholderBytes := unsafe.Slice(unsafe.StringData(securityResponsePlaceholder), len(securityResponsePlaceholder))
	return bytes.ReplaceAll(payload, placeholderBytes, securityResponseBytes)
}
