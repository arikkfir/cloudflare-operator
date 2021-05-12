package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type APIToken struct {
	Secret string `json:"secret"` // Secret reference (either a name or namespace/name).
	Key    string `json:"key"`    // Key in the secret to read
}

// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	Zone     string   `json:"zone"`               // Zone identifier.
	Type     string   `json:"type"`               // Type of the DNS record (possible values: A, AAAA, CNAME, HTTPS, TXT, SRV, LOC, MX, NS, SPF, CERT, DNSKEY, DS, NAPTR, SMIMEA, SSHFP, SVCB, TLSA, URI).
	Name     string   `json:"name"`               // Name of the DNS record, e.g. example.com (max length: 255).
	Content  string   `json:"content"`            // Content of the DNS record (e.g. 127.0.0.1).
	TTL      int      `json:"ttl"`                // TTL (Time To Live) for DNS record (value of 1 is 'automatic').
	Priority *uint16  `json:"priority,omitempty"` // Priority of the record (only used by MX, SRV and URI records; unused by other record types). Records with lower priorities are preferred.
	Proxied  *bool    `json:"proxied,omitempty"`  // Proxied status of the DNS record (proxied records are protected by Cloudflare).
	APIToken APIToken `json:"apiToken"`           // Reference to the API token.
}

// DNSRecordStatus defines the observed state of DNSRecord
type DNSRecordStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DNSRecord is the Schema for the DNSRecord API
type DNSRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSRecordSpec   `json:"spec,omitempty"`
	Status DNSRecordStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DNSRecordList contains a list of DNSRecord
type DNSRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSRecord{}, &DNSRecordList{})
}
