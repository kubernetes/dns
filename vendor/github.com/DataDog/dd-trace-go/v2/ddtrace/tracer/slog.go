// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package tracer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// groupOrAttrs holds either a group name or a list of slog.Attrs.
type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

// slogHandler implements the slog.Handler interface to dispatch messages to our
// internal logger.
type slogHandler struct {
	goas []groupOrAttrs
}

func (h slogHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	if lvl <= slog.LevelDebug {
		return log.DebugEnabled()
	}
	// TODO(fg): Implement generic log level checking in the internal logger.
	// But we're we're not concerned with slog perf, so this is okay for now.
	return true
}

func (h slogHandler) Handle(_ context.Context, r slog.Record) error {
	goas := h.goas

	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list; they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != "" {
			goas = goas[:len(goas)-1]
		}
	}

	parts := make([]string, 0, len(goas)+r.NumAttrs())
	formatGroup := ""

	for _, goa := range goas {
		if goa.group != "" {
			formatGroup += goa.group + "."
		} else {
			for _, a := range goa.attrs {
				parts = append(parts, formatGroup+a.String())
			}
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		parts = append(parts, formatGroup+a.String())
		return true
	})

	extra := strings.Join(parts, " ")
	switch r.Level {
	case slog.LevelDebug:
		log.Debug("%s %s", r.Message, extra)
	case slog.LevelInfo:
		log.Info("%s %s", r.Message, extra)
	case slog.LevelWarn:
		log.Warn("%s %s", r.Message, extra)
	case slog.LevelError:
		log.Error("%s %s", r.Message, extra)
	}
	return nil
}

func (h slogHandler) withGroupOrAttrs(goa groupOrAttrs) slogHandler {
	h.goas = append(h.goas, goa)
	return h
}

// WithGroup returns a new Handler whose group consist of
// both the receiver's groups and the arguments.
func (h slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{group: name})
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
func (h slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}
