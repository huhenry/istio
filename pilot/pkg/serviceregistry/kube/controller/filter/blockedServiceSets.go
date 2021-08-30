package filter

import (
	corev1 "k8s.io/api/core/v1"
	"fmt"
)

// sets.String is a set of strings, implemented via map[string]struct{} for minimal memory consumption.
type ServiceSets map[string]*corev1.Service

// NewString creates a String from a list of values.
func NewServiceSets(svc *corev1.Service) ServiceSets {
	ss := ServiceSets{}
	ss.Insert(svc)
	return ss
}


func GenKey(svc *corev1.Service) string{
	key := fmt.Sprintf("%s/%s",svc.Namespace,svc.Name)
return key

}

// Insert adds items to the set.
func (s ServiceSets) Insert(svc *corev1.Service) ServiceSets {
		s[GenKey(svc)] = svc

	return s
}

// Delete removes all items from the set.
func (s ServiceSets) Delete(svc *corev1.Service) ServiceSets {
		delete(s, GenKey(svc))

	return s
}

// Has returns true if and only if item is contained in the set.
func (s ServiceSets) Has(svc *corev1.Service) bool {
	_, contained := s[GenKey(svc)]
	return contained
}



