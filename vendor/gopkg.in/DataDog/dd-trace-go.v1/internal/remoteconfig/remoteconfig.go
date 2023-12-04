// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package remoteconfig

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"reflect"
	"strings"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"

	rc "github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Callback represents a function that can process a remote config update.
// A Callback function can be registered to a remote config client to automatically
// react upon receiving updates. This function returns the configuration processing status
// for each config file received through the update.
type Callback func(updates map[string]ProductUpdate) map[string]rc.ApplyStatus

// Capability represents a bit index to be set in clientData.Capabilites in order to register a client
// for a specific capability
type Capability uint

const (
	_ Capability = iota
	// ASMActivation represents the capability to activate ASM through remote configuration
	ASMActivation
	// ASMIPBlocking represents the capability for ASM to block requests based on user IP
	ASMIPBlocking
	// ASMDDRules represents the capability to update the rules used by the ASM WAF for threat detection
	ASMDDRules
	// ASMExclusions represents the capability for ASM to exclude traffic from its protections
	ASMExclusions
	// ASMRequestBlocking represents the capability for ASM to block requests based on the HTTP request related WAF addresses
	ASMRequestBlocking
	// ASMResponseBlocking represents the capability for ASM to block requests based on the HTTP response related WAF addresses
	ASMResponseBlocking
	// ASMUserBlocking represents the capability for ASM to block requests based on user ID
	ASMUserBlocking
	// ASMCustomRules represents the capability for ASM to receive and use user-defined security rules
	ASMCustomRules
	// ASMCustomRules represents the capability for ASM to receive and use user-defined blocking responses
	ASMCustomBlockingResponse
)

// ProductUpdate represents an update for a specific product.
// It is a map of file path to raw file content
type ProductUpdate map[string][]byte

// A Client interacts with an Agent to update and track the state of remote
// configuration
type Client struct {
	ClientConfig

	clientID   string
	endpoint   string
	repository *rc.Repository
	stop       chan struct{}

	callbacks []Callback

	lastError error
}

// NewClient creates a new remoteconfig Client
func NewClient(config ClientConfig) (*Client, error) {
	repo, err := rc.NewUnverifiedRepository()
	if err != nil {
		return nil, err
	}
	if config.HTTP == nil {
		config.HTTP = DefaultClientConfig().HTTP
	}

	return &Client{
		ClientConfig: config,
		clientID:     generateID(),
		endpoint:     fmt.Sprintf("%s/v0.7/config", config.AgentURL),
		repository:   repo,
		stop:         make(chan struct{}),
		lastError:    nil,
		callbacks:    []Callback{},
	}, nil
}

// Start starts the client's update poll loop in a fresh goroutine
func (c *Client) Start() {
	go func() {
		ticker := time.NewTicker(c.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-c.stop:
				close(c.stop)
				return
			case <-ticker.C:
				c.updateState()
			}
		}
	}()
}

// Stop stops the client's update poll loop
func (c *Client) Stop() {
	log.Debug("remoteconfig: gracefully stopping the client")
	c.stop <- struct{}{}
	select {
	case <-c.stop:
		log.Debug("remoteconfig: client stopped successfully")
	case <-time.After(time.Second):
		log.Debug("remoteconfig: client stopping timeout")
	}
}

func (c *Client) updateState() {
	data, err := c.newUpdateRequest()
	if err != nil {
		log.Error("remoteconfig: unexpected error while creating a new update request payload: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodGet, c.endpoint, &data)
	if err != nil {
		log.Error("remoteconfig: unexpected error while creating a new http request: %v", err)
		return
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		log.Debug("remoteconfig: http request error: %v", err)
		return
	}
	// Flush and close the response body when returning (cf. https://pkg.go.dev/net/http#Client.Do)
	defer func() {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}()

	if sc := resp.StatusCode; sc != http.StatusOK {
		log.Debug("remoteconfig: http request error: response status code is not 200 (OK) but %s", http.StatusText(sc))
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("remoteconfig: http request error: could not read the response body: %v", err)
		return
	}

	if body := string(respBody); body == `{}` || body == `null` {
		return
	}

	var update clientGetConfigsResponse
	if err := json.Unmarshal(respBody, &update); err != nil {
		log.Error("remoteconfig: http request error: could not parse the json response body: %v", err)
		return
	}

	c.lastError = c.applyUpdate(&update)
}

// RegisterCallback allows registering a callback that will be invoked when the client
// receives configuration updates. It is up to that callback to then decide what to do
// depending on the product related to the configuration update.
func (c *Client) RegisterCallback(f Callback) {
	c.callbacks = append(c.callbacks, f)
}

// UnregisterCallback removes a previously registered callback from the active callbacks list
// This remove operation preserves ordering
func (c *Client) UnregisterCallback(f Callback) {
	fValue := reflect.ValueOf(f)
	for i, callback := range c.callbacks {
		if reflect.ValueOf(callback) == fValue {
			c.callbacks = append(c.callbacks[:i], c.callbacks[i+1:]...)
		}
	}
}

// RegisterProduct adds a product to the list of products listened by the client
func (c *Client) RegisterProduct(p string) {
	c.Products[p] = struct{}{}
}

// UnregisterProduct removes a product from the list of products listened by the client
func (c *Client) UnregisterProduct(p string) {
	delete(c.Products, p)
}

// RegisterCapability adds a capability to the list of capabilities exposed by the client when requesting
// configuration updates
func (c *Client) RegisterCapability(cap Capability) {
	c.Capabilities[cap] = struct{}{}
}

// UnregisterCapability removes a capability from the list of capabilities exposed by the client when requesting
// configuration updates
func (c *Client) UnregisterCapability(cap Capability) {
	delete(c.Capabilities, cap)
}

func (c *Client) applyUpdate(pbUpdate *clientGetConfigsResponse) error {
	fileMap := make(map[string][]byte, len(pbUpdate.TargetFiles))
	productUpdates := make(map[string]ProductUpdate, len(c.Products))
	for p := range c.Products {
		productUpdates[p] = make(ProductUpdate)
	}
	for _, f := range pbUpdate.TargetFiles {
		fileMap[f.Path] = f.Raw
		for p := range c.Products {
			// Check the config file path to make sure it belongs to the right product
			if strings.Contains(f.Path, "/"+p+"/") {
				productUpdates[p][f.Path] = f.Raw
			}
		}
	}

	mapify := func(s *rc.RepositoryState) map[string]string {
		m := make(map[string]string)
		for i := range s.Configs {
			path := s.CachedFiles[i].Path
			product := s.Configs[i].Product
			m[path] = product
		}
		return m
	}

	// Check the repository state before and after the update to detect which configs are not being sent anymore.
	// This is needed because some products can stop sending configurations, and we want to make sure that the subscribers
	// are provided with this information in this case
	stateBefore, err := c.repository.CurrentState()
	if err != nil {
		return fmt.Errorf("repository current state error: %v", err)
	}
	products, err := c.repository.Update(rc.Update{
		TUFRoots:      pbUpdate.Roots,
		TUFTargets:    pbUpdate.Targets,
		TargetFiles:   fileMap,
		ClientConfigs: pbUpdate.ClientConfigs,
	})
	if err != nil {
		return fmt.Errorf("repository update error: %v", err)
	}
	stateAfter, err := c.repository.CurrentState()
	if err != nil {
		return fmt.Errorf("repository current state error after update: %v", err)
	}

	// Create a config files diff between before/after the update to see which config files are missing
	mBefore := mapify(&stateBefore)
	for k := range mapify(&stateAfter) {
		delete(mBefore, k)
	}

	// Set the payload data to nil for missing config files. The callbacks then can handle the nil config case to detect
	// that this config will not be updated anymore.
	updatedProducts := make(map[string]struct{})
	for path, product := range mBefore {
		if productUpdates[product] == nil {
			productUpdates[product] = make(ProductUpdate)
		}
		productUpdates[product][path] = nil
		updatedProducts[product] = struct{}{}
	}
	// Aggregate updated products and missing products so that callbacks get called for both
	for _, p := range products {
		updatedProducts[p] = struct{}{}
	}

	if len(updatedProducts) == 0 {
		return nil
	}
	// Performs the callbacks registered and update the application status in the repository (RCTE2)
	// In case of several callbacks handling the same config, statuses take precedence in this order:
	// 1 - ApplyStateError
	// 2 - ApplyStateUnacknowledged
	// 3 - ApplyStateAcknowledged
	// This makes sure that any product that would need to re-receive the config in a subsequent update will be allowed to
	statuses := make(map[string]rc.ApplyStatus)
	for _, fn := range c.callbacks {
		for path, status := range fn(productUpdates) {
			if s, ok := statuses[path]; !ok || status.State == rc.ApplyStateError ||
				s.State == rc.ApplyStateAcknowledged && status.State == rc.ApplyStateUnacknowledged {
				statuses[path] = status
			}
		}
	}
	for p, s := range statuses {
		c.repository.UpdateApplyStatus(p, s)
	}

	return nil
}

func (c *Client) newUpdateRequest() (bytes.Buffer, error) {
	state, err := c.repository.CurrentState()
	if err != nil {
		return bytes.Buffer{}, err
	}
	// Temporary check while using untrusted repo, for which no initial root file is provided
	if state.RootsVersion < 1 {
		state.RootsVersion = 1
	}

	pbCachedFiles := make([]*targetFileMeta, 0, len(state.CachedFiles))
	for _, f := range state.CachedFiles {
		pbHashes := make([]*targetFileHash, 0, len(f.Hashes))
		for alg, hash := range f.Hashes {
			pbHashes = append(pbHashes, &targetFileHash{
				Algorithm: alg,
				Hash:      hex.EncodeToString(hash),
			})
		}
		pbCachedFiles = append(pbCachedFiles, &targetFileMeta{
			Path:   f.Path,
			Length: int64(f.Length),
			Hashes: pbHashes,
		})
	}

	hasError := c.lastError != nil
	errMsg := ""
	if hasError {
		errMsg = c.lastError.Error()
	}

	var pbConfigState []*configState
	if !hasError {
		pbConfigState = make([]*configState, 0, len(state.Configs))
		for _, f := range state.Configs {
			pbConfigState = append(pbConfigState, &configState{
				ID:         f.ID,
				Version:    f.Version,
				Product:    f.Product,
				ApplyState: f.ApplyStatus.State,
				ApplyError: f.ApplyStatus.Error,
			})
		}
	}

	capa := big.NewInt(0)
	for i := range c.Capabilities {
		capa.SetBit(capa, int(i), 1)
	}
	products := make([]string, 0, len(c.Products))
	for p := range c.Products {
		products = append(products, p)
	}
	req := clientGetConfigsRequest{
		Client: &clientData{
			State: &clientState{
				RootVersion:    uint64(state.RootsVersion),
				TargetsVersion: uint64(state.TargetsVersion),
				ConfigStates:   pbConfigState,
				HasError:       hasError,
				Error:          errMsg,
			},
			ID:       c.clientID,
			Products: products,
			IsTracer: true,
			ClientTracer: &clientTracer{
				RuntimeID:     c.RuntimeID,
				Language:      "go",
				TracerVersion: c.TracerVersion,
				Service:       c.ServiceName,
				Env:           c.Env,
				AppVersion:    c.AppVersion,
			},
			Capabilities: capa.Bytes(),
		},
		CachedTargetFiles: pbCachedFiles,
	}

	var b bytes.Buffer

	err = json.NewEncoder(&b).Encode(&req)
	if err != nil {
		return bytes.Buffer{}, err
	}

	return b, nil
}

var (
	idSize     = 21
	idAlphabet = []rune("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

func generateID() string {
	bytes := make([]byte, idSize)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	id := make([]rune, idSize)
	for i := 0; i < idSize; i++ {
		id[i] = idAlphabet[bytes[i]&63]
	}
	return string(id[:idSize])
}
