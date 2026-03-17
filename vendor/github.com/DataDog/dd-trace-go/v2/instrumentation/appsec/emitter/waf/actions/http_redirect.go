// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package actions

import (
	"net/http"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// redirectActionParams are the dynamic parameters to be provided to a "redirect_request"
// action type upon invocation
type redirectActionParams struct {
	Location           string
	StatusCode         int
	SecurityResponseID string
}

func (r *redirectActionParams) Decode(p map[string]any) error {
	for k := range p {
		switch k {
		case "location":
			v, err := decodeStr(p, k)
			if err != nil {
				return err
			}
			r.Location = v
		case "status_code":
			v, err := decodeInt(p, k)
			if err != nil {
				return err
			}
			r.StatusCode = v
		case "security_response_id":
			v, err := decodeStr(p, k)
			if err != nil {
				return err
			}
			r.SecurityResponseID = v
		}
	}
	return nil
}

func init() {
	registerActionHandler("redirect_request", NewRedirectAction)
}

func redirectParamsFromMap(params map[string]any) (redirectActionParams, error) {
	var p redirectActionParams
	err := p.Decode(params)
	return p, err
}

func newRedirectRequestAction(status int, loc string, securityResponseID string) *BlockHTTP {
	// Default to 303 if status is out of redirection codes bounds
	if status < http.StatusMultipleChoices || status >= http.StatusBadRequest {
		status = http.StatusSeeOther
	}

	// If location is not set we fall back on a default block action
	if loc == "" {
		return &BlockHTTP{Handler: newBlockHandler(http.StatusForbidden, "auto", securityResponseID)}
	}
	loc = strings.ReplaceAll(loc, securityResponsePlaceholder, securityResponseID)
	return &BlockHTTP{Handler: http.RedirectHandler(loc, status)}
}

// NewRedirectAction creates an action for the "redirect_request" action type
func NewRedirectAction(params map[string]any) []Action {
	p, err := redirectParamsFromMap(params)
	if err != nil {
		log.Debug("appsec: couldn't decode redirect action parameters")
		return nil
	}
	return []Action{newRedirectRequestAction(p.StatusCode, p.Location, p.SecurityResponseID)}
}
