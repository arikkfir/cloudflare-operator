package internal

//go:generate mockgen -destination cloudflare_client_mock.go -package internal . CloudflareClient

import (
	"context"
	"fmt"
	"github.com/cloudflare/cloudflare-go"
)

var DNSRecordNotFound = fmt.Errorf("record not found")

type CloudflareClient interface {
	GetDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName string) (*cloudflare.DNSRecord, error)
	CreateDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName, content string, proxied *bool, ttl int, priority *uint16) error
	UpdateDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName, content string, proxied *bool, ttl int, priority *uint16) error
	DeleteDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName string) error
}

type cloudflareClientImpl struct {
}

// NewCloudflareClient creates a new Cloudflare client that will use the given API token.
func NewCloudflareClient() (CloudflareClient, error) {
	return &cloudflareClientImpl{}, nil
}

// GetDNSRecord will fetch the DNS record matching the given type & name in the specified zone.
//
// An error is returned if the client fails to connect, authenticate or fetch the data from Cloudflare, as well as
// when the record or zone do not exist. In such cases when the record/zone cannot be found, the returned error will
// satisfy a call to:
//		errors.Is(err, DNSRecordNotFound)
func (c *cloudflareClientImpl) GetDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName string) (*cloudflare.DNSRecord, error) {
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed creating Cloudflare client: %w", err)
	}

	zones, err := api.ListZones(ctx, zoneName)
	if err != nil {
		return nil, fmt.Errorf("failed listing DNS zones: %w", err)
	} else if len(zones) == 0 {
		return nil, DNSRecordNotFound
	} else if len(zones) > 1 {
		return nil, fmt.Errorf("too many zones matched")
	}
	zoneID := zones[0].ID

	records, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{Type: recordType, Name: recordName})
	if err != nil {
		return nil, fmt.Errorf("failed listing DNS reords: %w", err)
	}

	if len(records) == 0 {
		return nil, DNSRecordNotFound
	} else if len(records) > 1 {
		return nil, fmt.Errorf("too many DNS records match")
	} else {
		return &records[0], nil
	}
}

// CreateDNSRecord creates a DNS record with the given type, name, content & other properties in the specified zone.
// If such record already exists, an error is returned.
func (c *cloudflareClientImpl) CreateDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName, content string, proxied *bool, ttl int, priority *uint16) error {
	err := c.CreateDNSRecord(ctx, apiToken, zoneName, recordType, recordName, content, proxied, ttl, priority)
	if err != nil {
		return fmt.Errorf("failed to create DNS record: %w", err)
	}
	return nil
}

// UpdateDNSRecord updates the DNS record that matches the given type & name in the specified zone.
//
// If no such record can be found, an error is returned which satisfies a call to:
//		errors.Is(err, DNSRecordNotFound)
func (c *cloudflareClientImpl) UpdateDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName, content string, proxied *bool, ttl int, priority *uint16) error {
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return fmt.Errorf("failed creating Cloudflare client: %w", err)
	}

	record, err := c.GetDNSRecord(ctx, apiToken, zoneName, recordType, recordName)
	if err != nil {
		return fmt.Errorf("failed to update DNS record: %w", err)
	}

	err = api.UpdateDNSRecord(ctx, record.ZoneID, record.ID, cloudflare.DNSRecord{
		Type:     recordType,
		Name:     recordName,
		Content:  content,
		Proxied:  proxied,
		TTL:      ttl,
		Priority: priority,
	})
	if err != nil {
		return fmt.Errorf("failed to update DNS record: %w", err)
	}

	return nil
}

// DeleteDNSRecord deletes the DNS record that matches the given type & name in the specified zone.
//
// If no such record can be found, an error is returned which satisfies a call to:
//		errors.Is(err, DNSRecordNotFound)
func (c *cloudflareClientImpl) DeleteDNSRecord(ctx context.Context, apiToken, zoneName, recordType, recordName string) error {
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return fmt.Errorf("failed creating Cloudflare client: %w", err)
	}

	record, err := c.GetDNSRecord(ctx, apiToken, zoneName, recordType, recordName)
	if err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	err = api.DeleteDNSRecord(ctx, record.ZoneID, record.ID)
	if err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	return nil
}
