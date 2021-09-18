// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wasm

import (
	"sync"
	"time"

	udpa "github.com/cncf/udpa/go/udpa/type/v1"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	httpwasm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	networkwasm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/wasm/v3"
	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	apiTypePrefix         = "type.googleapis.com/"
	typedStructType       = apiTypePrefix + "udpa.type.v1.TypedStruct"
	wasmHTTPFilterType    = apiTypePrefix + "envoy.extensions.filters.http.wasm.v3.Wasm"
	wasmNetworkFilterType = apiTypePrefix + "envoy.extensions.filters.network.wasm.v3.Wasm"
)

// MaybeConvertWasmExtensionConfig converts any presence of module remote download to local file.
// It downloads the Wasm module and stores the module locally in the file system.
func MaybeConvertWasmExtensionConfig(resources []*any.Any, cache Cache) bool {
	var wg sync.WaitGroup
	numResources := len(resources)
	wg.Add(numResources)
	sendNack := atomic.NewBool(false)
	startTime := time.Now()
	defer func() {
		wasmConfigConversionDuration.Record(float64(time.Since(startTime).Milliseconds()))
	}()

	for i := 0; i < numResources; i++ {
		go func(i int) {
			defer wg.Done()

			newExtensionConfig, nack := convert(resources[i], cache)
			if nack {
				sendNack.Store(true)
				return
			}
			resources[i] = newExtensionConfig
		}(i)
	}

	wg.Wait()
	return sendNack.Load()
}

// MaybeConvertWasmExtensionConfigDelta converts any presence of module remote download to local file.
// It downloads the Wasm module and stores the module locally in the file system.
func MaybeConvertWasmExtensionConfigDelta(resources []*discovery.Resource, cache Cache) bool {
	var wg sync.WaitGroup
	numResources := len(resources)
	wg.Add(numResources)
	sendNack := atomic.NewBool(false)
	startTime := time.Now()
	defer func() {
		wasmConfigConversionDuration.Record(float64(time.Since(startTime).Milliseconds()))
	}()

	for i := 0; i < numResources; i++ {
		go func(i int) {
			defer wg.Done()

			newExtensionConfig, nack := convert(resources[i].Resource, cache)
			if nack {
				sendNack.Store(true)
				return
			}
			resources[i].Resource = newExtensionConfig
		}(i)
	}

	wg.Wait()
	return sendNack.Load()
}

func convert(resource *any.Any, cache Cache) (newExtensionConfig *any.Any, sendNack bool) {
	ec := &core.TypedExtensionConfig{}
	newExtensionConfig = resource
	sendNack = false
	status := noRemoteLoad
	defer func() {
		wasmConfigConversionCount.
			With(resultTag.Value(status)).
			Increment()
	}()
	if err := resource.UnmarshalTo(ec); err != nil {
		wasmLog.Debugf("failed to unmarshal extension config resource: %v", err)
		return
	}

	// Currently Wasm filter can only be configured using typed struct via EnvoyFilter.
	wasmLog.Debugf("original extension config resource %+v", ec)
	if ec.GetTypedConfig() == nil && ec.GetTypedConfig().TypeUrl != typedStructType {
		wasmLog.Debugf("cannot find typed struct in %+v", ec)
		return
	}
	wasmStruct := &udpa.TypedStruct{}
	wasmTypedConfig := ec.GetTypedConfig()
	// nolint: staticcheck
	if err := ptypes.UnmarshalAny(wasmTypedConfig, wasmStruct); err != nil {
		wasmLog.Debugf("failed to unmarshal typed config for wasm filter: %v", err)
		return
	}

	var wasmHTTPFilterConfig interface {
		proto.Message
		protoreflect.ProtoMessage

		GetConfig() *wasmv3.PluginConfig
	}

	switch wasmStruct.TypeUrl {
	case wasmHTTPFilterType:
		wasmHTTPFilterConfig = &httpwasm.Wasm{}
	case wasmNetworkFilterType:
		wasmHTTPFilterConfig = &networkwasm.Wasm{}
	default:
		wasmLog.Debugf("typed extension config %+v does not contain wasm filter", wasmStruct)
		return
	}

	if err := conversion.StructToMessage(wasmStruct.Value, wasmHTTPFilterConfig); err != nil {
		wasmLog.Debugf("failed to convert extension config struct %+v to Wasm HTTP filter", wasmStruct)
		return
	}

	if wasmHTTPFilterConfig.GetConfig().GetVmConfig().GetCode().GetRemote() == nil {
		wasmLog.Debugf("no remote load found in Wasm HTTP filter %+v", wasmHTTPFilterConfig)
		return
	}

	// Wasm plugin configuration has remote load. From this point, any failure should result as a Nack,
	// unless the plugin is marked as fail open.
	failOpen := wasmHTTPFilterConfig.GetConfig().GetFailOpen()
	sendNack = !failOpen
	status = conversionSuccess

	vm := wasmHTTPFilterConfig.GetConfig().GetVmConfig()
	remote := vm.GetCode().GetRemote()
	httpURI := remote.GetHttpUri()
	if httpURI == nil {
		status = missRemoteFetchHint
		wasmLog.Errorf("wasm remote fetch %+v does not have httpUri specified", remote)
		return
	}
	timeout := time.Duration(0)
	if remote.GetHttpUri().Timeout != nil {
		timeout = remote.GetHttpUri().Timeout.AsDuration()
	}
	f, err := cache.Get(httpURI.GetUri(), remote.GetSha256(), timeout)
	if err != nil {
		status = fetchFailure
		wasmLog.Errorf("cannot fetch Wasm module %v: %v", remote.GetHttpUri().GetUri(), err)
		return
	}

	// Rewrite remote fetch to local file.
	vm.Code = &core.AsyncDataSource{
		Specifier: &core.AsyncDataSource_Local{
			Local: &core.DataSource{
				Specifier: &core.DataSource_Filename{
					Filename: f,
				},
			},
		},
	}

	wasmTypedConfig, err = anypb.New(wasmHTTPFilterConfig)
	if err != nil {
		status = marshalFailure
		wasmLog.Errorf("failed to marshal new wasm HTTP filter %+v to protobuf Any: %v", wasmHTTPFilterConfig, err)
		return
	}
	ec.TypedConfig = wasmTypedConfig
	wasmLog.Debugf("new extension config resource %+v", ec)

	nec, err := anypb.New(ec)
	if err != nil {
		status = marshalFailure
		wasmLog.Errorf("failed to marshal new extension config resource: %v", err)
		return
	}

	// At this point, we are certain that wasm module has been downloaded and config is rewritten.
	// ECDS has been rewritten successfully and should not nack.
	newExtensionConfig = nec
	sendNack = false
	return
}
