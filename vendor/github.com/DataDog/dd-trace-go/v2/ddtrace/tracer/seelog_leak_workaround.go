// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package tracer

import (
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/cihub/seelog"
)

// This workaround fixes goroutine leaks caused by seelog.
// See https://github.com/DataDog/dd-trace-go/issues/2987.
//
// TODO(felixge): Remove this once a proper fix lands in the agent or after we
// drop the agent dependency that causes this [1].
//
// [1] github.com/DataDog/datadog-agent/pkg/util/log
func init() {
	if env.Get("DD_TRACE_DEBUG_SEELOG_WORKAROUND") == "false" {
		return
	}

	// Close the seelog loggers to fix the goroutine leaks.
	seelog.Default.Close()
	seelog.Disabled.Close()

	// Setup a new seelog logger that doesn't leak goroutines.
	constraints, err := seelog.NewMinMaxConstraints(seelog.TraceLvl, seelog.CriticalLvl)
	if err != nil {
		log.Error("failed to create seelog constraints: %v", err.Error())
		return
	}
	console, err := seelog.NewConsoleWriter()
	if err != nil {
		log.Error("failed to create seelog console writer: %v", err.Error())
		return
	}
	dispatcher, err := seelog.NewSplitDispatcher(seelog.DefaultFormatter, []any{console})
	if err != nil {
		log.Error("failed to create seelog dispatcher: %v", err.Error())
		return
	}
	seelog.Default = seelog.NewSyncLogger(
		seelog.NewLoggerConfig(
			constraints,
			[]*seelog.LogLevelException{},
			dispatcher,
		),
	)
	seelog.Current = seelog.Default
}
