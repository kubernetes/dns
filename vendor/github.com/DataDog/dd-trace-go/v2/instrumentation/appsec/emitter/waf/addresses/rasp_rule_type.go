// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package addresses

import (
	"math"

	"github.com/DataDog/go-libddwaf/v4"
)

type RASPRuleType uint8

const (
	RASPRuleTypeLFI RASPRuleType = iota
	RASPRuleTypeSSRFRequest
	RASPRuleTypeSSRFResponse
	RASPRuleTypeSQLI
	RASPRuleTypeCMDI
)

var RASPRuleTypes = [...]RASPRuleType{
	RASPRuleTypeLFI,
	RASPRuleTypeSSRFRequest,
	RASPRuleTypeSSRFResponse,
	RASPRuleTypeSQLI,
	RASPRuleTypeCMDI,
}

func (r RASPRuleType) String() string {
	switch r {
	case RASPRuleTypeLFI:
		return "lfi"
	case RASPRuleTypeSSRFRequest, RASPRuleTypeSSRFResponse:
		return "ssrf"
	case RASPRuleTypeSQLI:
		return "sql_injection"
	case RASPRuleTypeCMDI:
		return "command_injection"
	}
	return "unknown()"
}

// RASPRuleTypeFromAddressSet returns the RASPRuleType for the given address set if it has a RASP address.
func RASPRuleTypeFromAddressSet(addressSet libddwaf.RunAddressData) (RASPRuleType, bool) {
	if addressSet.TimerKey != RASPScope {
		return math.MaxUint8, false
	}

	for address := range addressSet.Ephemeral {
		switch address {
		case ServerIOFSFileAddr:
			return RASPRuleTypeLFI, true
		case ServerIONetURLAddr:
			return RASPRuleTypeSSRFRequest, true
		case ServerIONetResponseStatusAddr:
			return RASPRuleTypeSSRFResponse, true
		case ServerDBStatementAddr, ServerDBTypeAddr:
			return RASPRuleTypeSQLI, true
		case ServerSysExecCmd:
			return RASPRuleTypeCMDI, true
		}
	}

	return math.MaxUint8, false
}
