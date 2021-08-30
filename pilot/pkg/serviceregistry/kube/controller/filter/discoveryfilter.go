package filter

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DiscoveryFilter tracks the set of namespaces selected for discovery, which are updated by the discovery namespace controller.
// It exposes a filter function used for filtering out objects that don't reside in namespaces selected for discovery.
type DiscoveryFilter interface {
	// Filter returns true if the input object resides in a namespace selected for discovery
	Filter(obj interface{}) bool
	// SelectorsChanged is invoked when meshConfig's discoverySelectors change, returns any newly selected namespaces and deselected namespaces
	SelectorsChanged(discoverySelectors []*metav1.LabelSelector) (selectedNamespaces []string, deselectedNamespaces []string)
}
