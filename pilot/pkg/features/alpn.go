package features

import (
	"strings"
	"sync"

	"istio.io/istio/pilot/pkg/util/sets"
	"istio.io/pkg/env"
)

var (
	ALPNProtocols = RegisterALPNProtocolsVar(
		"ALPN_PROTOCOLS",
		strings.Join(defaultALPNProtocols, ","),
		"Supported ALPN protocols.",
	)

	defaultALPNProtocols = []string{"h2", "http/1.1"}
	allowedALPNProtocols = sets.NewSet("h2", "http/1.1")
)

type ALPNProtocolsVar struct {
	lock sync.RWMutex
	env.StringVar
	protocols []string
}

func RegisterALPNProtocolsVar(name string, defaultValue string, description string) *ALPNProtocolsVar {
	v := env.RegisterStringVar(name, defaultValue, description)
	return &ALPNProtocolsVar{
		StringVar: v,
	}
}

func (v *ALPNProtocolsVar) init() []string {
	v.lock.Lock()
	defer v.lock.Unlock()

	s, _ := v.Lookup()
	var protocols []string
	protocolParams := strings.Split(s, ",")
	for _, proto := range protocolParams {
		proto = strings.TrimSpace(proto)
		if allowedALPNProtocols.Contains(proto) {
			protocols = append(protocols, proto)
		}
	}

	if len(protocols) == 0 {
		protocols = defaultALPNProtocols
	}

	v.protocols = protocols
	return protocols
}

func (v *ALPNProtocolsVar) Get() []string {
	v.lock.RLock()
	protocols := v.protocols
	v.lock.RUnlock()

	if len(protocols) > 0 {
		return protocols
	}

	return v.init()
}

func (v *ALPNProtocolsVar) Reset() {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.protocols = nil
}
