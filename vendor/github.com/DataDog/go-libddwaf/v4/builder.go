// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
	"github.com/DataDog/go-libddwaf/v4/internal/ruleset"
)

// Builder manages an evolving WAF configuration over time. Its lifecycle is
// typically tied to that of a remote configuration client, as its purpose is to
// keep an up-to-date view of the current coniguration with low overhead. This
// type is not safe for concurrent use, and users should protect it with a mutex
// or similar when sharing it across multiple goroutines. All methods of this
// type are safe to call with a nil receiver.
type Builder struct {
	handle        bindings.WAFBuilder
	defaultLoaded bool
}

// NewBuilder creates a new [Builder] instance. Its lifecycle is typically tied
// to that of a remote configuration client, as its purpose is to keep an
// up-to-date view of the current coniguration with low overhead. Returns nil if
// an error occurs when initializing the builder. The caller is responsible for
// calling [Builder.Close] when the builder is no longer needed.
func NewBuilder(keyObfuscatorRegex string, valueObfuscatorRegex string) (*Builder, error) {
	if ok, err := Load(); !ok {
		return nil, err
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	hdl := bindings.Lib.BuilderInit(newConfig(&pinner, keyObfuscatorRegex, valueObfuscatorRegex))

	if hdl == 0 {
		return nil, errors.New("failed to initialize the WAF builder")
	}

	return &Builder{handle: hdl}, nil
}

// Close releases all resources associated with this builder.
func (b *Builder) Close() {
	if b == nil || b.handle == 0 {
		return
	}
	bindings.Lib.BuilderDestroy(b.handle)
	b.handle = 0
}

var (
	errUpdateFailed  = errors.New("failed to update WAF Builder instance")
	errBuilderClosed = errors.New("builder has already been closed")
)

const defaultRecommendedRulesetPath = "::/go-libddwaf/default/recommended.json"

// AddDefaultRecommendedRuleset adds the default recommended ruleset to the
// receiving [Builder], and returns the [Diagnostics] produced in the process.
func (b *Builder) AddDefaultRecommendedRuleset() (Diagnostics, error) {
	defaultRuleset, err := ruleset.DefaultRuleset()
	defer bindings.Lib.ObjectFree(&defaultRuleset)
	if err != nil {
		return Diagnostics{}, fmt.Errorf("failed to load default recommended ruleset: %w", err)
	}

	diag, err := b.addOrUpdateConfig(defaultRecommendedRulesetPath, &defaultRuleset)
	if err == nil {
		b.defaultLoaded = true
	}
	return diag, err
}

// RemoveDefaultRecommendedRuleset removes the default recommended ruleset from
// the receiving [Builder]. Returns true if the removal occurred (meaning the
// default recommended ruleset was indeed present in the builder).
func (b *Builder) RemoveDefaultRecommendedRuleset() bool {
	if b.RemoveConfig(defaultRecommendedRulesetPath) {
		b.defaultLoaded = false
		return true
	}
	return false
}

// AddOrUpdateConfig adds or updates a configuration fragment to this [Builder].
// Returns the [Diagnostics] produced by adding or updating this configuration.
func (b *Builder) AddOrUpdateConfig(path string, fragment any) (Diagnostics, error) {
	if b == nil || b.handle == 0 {
		return Diagnostics{}, errBuilderClosed
	}

	if path == "" {
		return Diagnostics{}, errors.New("path cannot be blank")
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()

	encoder, err := newEncoder(newUnlimitedEncoderConfig(&pinner))
	if err != nil {
		return Diagnostics{}, fmt.Errorf("could not create encoder: %w", err)
	}

	frag, err := encoder.Encode(fragment)
	if err != nil {
		return Diagnostics{}, fmt.Errorf("could not encode the config fragment into a WAF object; %w", err)
	}

	return b.addOrUpdateConfig(path, frag)
}

// addOrUpdateConfig adds or updates a configuration fragment to this [Builder].
// Returns the [Diagnostics] produced by adding or updating this configuration.
func (b *Builder) addOrUpdateConfig(path string, cfg *bindings.WAFObject) (Diagnostics, error) {
	var diagnosticsWafObj bindings.WAFObject
	defer bindings.Lib.ObjectFree(&diagnosticsWafObj)

	res := bindings.Lib.BuilderAddOrUpdateConfig(b.handle, path, cfg, &diagnosticsWafObj)

	var diags Diagnostics
	if !diagnosticsWafObj.IsInvalid() {
		// The Diagnostics object will be invalid if the config was completely
		// rejected.
		var err error
		diags, err = decodeDiagnostics(&diagnosticsWafObj)
		if err != nil {
			return diags, fmt.Errorf("failed to decode WAF diagnostics: %w", err)
		}
	}

	if !res {
		return diags, errUpdateFailed
	}
	return diags, nil
}

// RemoveConfig removes the configuration associated with the given path from
// this [Builder]. Returns true if the removal was successful.
func (b *Builder) RemoveConfig(path string) bool {
	if b == nil || b.handle == 0 {
		return false
	}

	return bindings.Lib.BuilderRemoveConfig(b.handle, path)
}

// ConfigPaths returns the list of currently loaded configuration paths.
func (b *Builder) ConfigPaths(filter string) []string {
	if b == nil || b.handle == 0 {
		return nil
	}

	return bindings.Lib.BuilderGetConfigPaths(b.handle, filter)
}

// Build creates a new [Handle] instance that uses the current configuration.
// Returns nil if an error occurs when building the handle. The caller is
// responsible for calling [Handle.Close] when the handle is no longer needed.
// This function may return nil.
func (b *Builder) Build() *Handle {
	if b == nil || b.handle == 0 {
		return nil
	}

	hdl := bindings.Lib.BuilderBuildInstance(b.handle)
	if hdl == 0 {
		return nil
	}

	return wrapHandle(hdl)
}
