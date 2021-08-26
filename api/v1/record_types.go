package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretKeyRef describes a reference to a specific key in a Secret.
type SecretKeyRef struct {
	Secret string `json:"secret"` // Secret reference (either a name or namespace/name).
	Key    string `json:"key"`    // Key in the secret to read
}

// RecordSpec defines the desired state of Record
type RecordSpec struct {
	// Zone identifier.
	Zone string `json:"zone"`

	// Type of the DNS record (possible values: A, AAAA, CNAME, HTTPS, TXT, SRV, LOC, MX, NS, SPF, CERT, DNSKEY, DS, NAPTR, SMIMEA, SSHFP, SVCB, TLSA, URI).
	Type string `json:"type"`

	// Name of the DNS record, e.g. example.com (max length: 255).
	Name string `json:"name"`

	// Content of the DNS record (e.g. 127.0.0.1).
	Content string `json:"content"`

	// TTL (Time To Live) for DNS record (value of 1 is 'automatic').
	TTL int `json:"ttl"`

	// Priority of the record (only used by MX, SRV and URI records; unused by other record types). Records with lower priorities are preferred.
	Priority *uint16 `json:"priority,omitempty"`

	// Proxied status of the DNS record (proxied records are protected by Cloudflare).
	Proxied *bool `json:"proxied,omitempty"`

	// APIToken is a reference to the secret containing the API token.
	APIToken SecretKeyRef `json:"apiToken"`

	// SyncInterval specifies the interval to sync the record in Cloudflare.
	SyncInterval string `json:"syncInterval"`
}

// RecordStatus defines the observed state of Record
type RecordStatus struct {
	// Represents the observations of a Record current state.
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Record is the Schema for the records API
type Record struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecordSpec   `json:"spec,omitempty"`
	Status RecordStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RecordList contains a list of Record
type RecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Record `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Record{}, &RecordList{})
}
