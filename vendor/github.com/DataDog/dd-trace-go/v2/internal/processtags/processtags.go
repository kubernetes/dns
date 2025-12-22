// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package processtags

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

const envProcessTagsEnabled = "DD_EXPERIMENTAL_PROPAGATE_PROCESS_TAGS_ENABLED"

const (
	tagEntrypointName    = "entrypoint.name"
	tagEntrypointBasedir = "entrypoint.basedir"
	tagEntrypointWorkdir = "entrypoint.workdir"
	tagEntrypointType    = "entrypoint.type"
)

const (
	entrypointTypeExecutable = "executable"
)

var (
	enabled bool
	pTags   *ProcessTags
)

func init() {
	Reload()
}

type ProcessTags struct {
	mu    sync.RWMutex
	tags  map[string]string
	str   string
	slice []string
}

// String returns the string representation of the process tags.
func (p *ProcessTags) String() string {
	if p == nil {
		return ""
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.str
}

// Slice returns the string slice representation of the process tags.
func (p *ProcessTags) Slice() []string {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.slice
}

func (p *ProcessTags) merge(newTags map[string]string) {
	if len(newTags) == 0 {
		return
	}
	pTags.mu.Lock()
	defer pTags.mu.Unlock()

	if p.tags == nil {
		p.tags = make(map[string]string)
	}
	for k, v := range newTags {
		p.tags[k] = v
	}

	// loop over the sorted map keys so the resulting string and slice versions are created consistently.
	keys := make([]string, 0, len(p.tags))
	for k := range p.tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	tagsSlice := make([]string, 0, len(p.tags))
	var b strings.Builder
	first := true
	for _, k := range keys {
		val := p.tags[k]
		if !first {
			b.WriteByte(',')
		}
		first = false
		keyVal := normalize.NormalizeTag(k + ":" + val)
		b.WriteString(keyVal)
		tagsSlice = append(tagsSlice, keyVal)
	}
	p.slice = tagsSlice
	p.str = b.String()
}

// Reload initializes the configuration and process tags collection. This is useful for tests.
func Reload() {
	enabled = internal.BoolEnv(envProcessTagsEnabled, true)
	if !enabled {
		return
	}
	pTags = &ProcessTags{}
	tags := collect()
	if len(tags) > 0 {
		Add(tags)
	}
}

func collect() map[string]string {
	tags := make(map[string]string)
	execPath, err := os.Executable()
	if err != nil {
		log.Debug("failed to get binary path: %s", err.Error())
	} else {
		baseDirName := filepath.Base(filepath.Dir(execPath))
		tags[tagEntrypointName] = filepath.Base(execPath)
		tags[tagEntrypointBasedir] = baseDirName
		tags[tagEntrypointType] = entrypointTypeExecutable
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Debug("failed to get working directory: %s", err.Error())
	} else {
		tags[tagEntrypointWorkdir] = filepath.Base(wd)
	}
	return tags
}

// GlobalTags returns the global process tags.
func GlobalTags() *ProcessTags {
	if !enabled {
		return nil
	}
	return pTags
}

// Add merges the given tags into the global processTags map.
func Add(tags map[string]string) {
	if !enabled {
		return
	}
	pTags.merge(tags)
}
