package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIKeySpec defines the desired state of APIKeySpec
type APIKeySpec struct {
	Key   string `json:"key"`   // API Key
	Email string `json:"email"` // EMail address
}

// APIKeyStatus defines the observed state of APIKey
type APIKeyStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// APIKey is the Schema for the APIKey API
type APIKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIKeySpec   `json:"spec,omitempty"`
	Status APIKeyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIKeyList contains a list of APIKey
type APIKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIKey{}, &APIKeyList{})
}
