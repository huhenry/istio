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

package filter

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"istio.io/pkg/log"
)

// DiscoveryNamespacesFilter tracks the set of namespaces selected for discovery, which are updated by the discovery namespace controller.
// It exposes a filter function used for filtering out objects that don't reside in namespaces selected for discovery.
type DiscoveryServicesFilter interface {
	// Filter returns true if the input object resides in a namespace selected for discovery
	Filter(obj interface{}) bool
	// SelectorsChanged is invoked when meshConfig's discoverySelectors change, returns any newly selected namespaces and deselected namespaces
	SelectorsChanged(discoverySelectors *metav1.LabelSelector) labels.Selector

	// GetSelector returns the selector  for discovery
	GetSelector() labels.Selector
}

type discoveryServicesFilter struct {
	lock              sync.RWMutex
	discoverySelector labels.Selector // nil if discovery selectors are not specified, permits all namespaces for discovery
}

func NewDiscoveryServicesFilter(
	discoverySelectors *metav1.LabelSelector,
) DiscoveryServicesFilter {
	discoveryServicesFilter := &discoveryServicesFilter{}

	// initialize discovery service filter
	discoveryServicesFilter.SelectorsChanged(discoverySelectors)

	return discoveryServicesFilter
}

func (d *discoveryServicesFilter) Filter(obj interface{}) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()
	// permit all objects if discovery selectors are not specified
	if d.discoverySelector == nil {
		return true
	}
	lbs := labels.Set(obj.(metav1.Object).GetLabels())
	if !d.discoverySelector.Matches(lbs) {
		return false
	}
	// permit if object resides in a namespace labeled for discovery
	return true
}

// SelectorsChanged initializes the discovery filter state with the discovery selectors and selected namespaces
func (d *discoveryServicesFilter) SelectorsChanged(
	discoverySelectors *metav1.LabelSelector,
) (selector labels.Selector) {
	d.lock.Lock()
	defer d.lock.Unlock()

	// convert LabelSelectors to Selectors
	selector, err := metav1.LabelSelectorAsSelector(discoverySelectors)
	if err != nil {
		log.Errorf("error initializing discovery namespaces filter, invalid discovery selector: %v", err)
		return
	}

	// update filter state
	d.discoverySelector = selector

	return
}
// GetSelector returns the selector  for discovery
func (d *discoveryServicesFilter) GetSelector() labels.Selector {
	d.lock.RLock()
	defer d.lock.RUnlock()
	return d.discoverySelector
}

func (d *discoveryServicesFilter) isSelected(labels labels.Set) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()
	// permit all objects if discovery selectors are not specified
	if d.discoverySelector == nil {
		return true
	}

	if d.discoverySelector.Matches(labels) {
		return true
	}

	return false
}
