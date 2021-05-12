package controllers

import (
	"context"
	"fmt"
	cfv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const SyncInterval = "1m"

type looper struct {
	log         logr.Logger
	dnsRecord   *cfv1.DNSRecord
	client      client.Client
	stopChannel chan struct{}
}

func (l *looper) getAPIKey(ctx context.Context) (*cfv1.APIKey, error) {
	var apiKeyNamespace, apiKeyName string
	apiKeyTokens := strings.SplitN(l.dnsRecord.Spec.APIKey, "/", 2)
	if len(apiKeyTokens) == 2 {
		apiKeyNamespace = apiKeyTokens[0]
		apiKeyName = apiKeyTokens[1]
	} else {
		apiKeyNamespace = l.dnsRecord.Namespace
		apiKeyName = apiKeyTokens[0]
	}

	api := &cfv1.APIKey{}
	err := l.client.Get(ctx, client.ObjectKey{Namespace: apiKeyNamespace, Name: apiKeyName}, api)
	if err != nil {
		return nil, fmt.Errorf("failed to find APIKey '%s': %w", l.dnsRecord.Spec.APIKey, err)
	}

	return api, nil
}

func (l *looper) sync(ctx context.Context) {
	apiKey, err := l.getAPIKey(ctx)
	if err != nil {
		l.log.V(1).Error(err, "Failed finding APIKey")
		return
	}

	dnsrec := l.dnsRecord

	// Construct a new API object
	api, err := cloudflare.New(apiKey.Spec.Key, apiKey.Spec.Email)
	if err != nil {
		l.log.V(1).Error(err, "Failed creating Cloudflare client")
		return
	}

	zoneID, err := api.ZoneIDByName(dnsrec.Spec.Zone)
	if err != nil {
		l.log.V(1).Error(err, "Failed looking up Cloudflare zone", "zone", dnsrec.Spec.Zone)
		return
	}

	records, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{
		Type: dnsrec.Spec.Type,
		Name: dnsrec.Spec.Name,
	})
	if err != nil {
		l.log.V(1).Error(err, "Failed listing DNS records")
		return
	}

	if len(records) == 0 {
		_, err := api.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecord{
			Type:     dnsrec.Spec.Type,
			Name:     dnsrec.Spec.Name,
			Content:  dnsrec.Spec.Content,
			Proxied:  dnsrec.Spec.Proxied,
			TTL:      dnsrec.Spec.TTL,
			Priority: dnsrec.Spec.Priority,
		})
		if err != nil {
			l.log.V(1).Error(err, "Failed creating DNS record")
			return
		}
		l.log.V(3).Info("Created record")
	} else if len(records) == 1 {
		cfRec := records[0]
		if cfRec.Content != dnsrec.Spec.Content || cfRec.Proxied != dnsrec.Spec.Proxied || cfRec.TTL != dnsrec.Spec.TTL || cfRec.Priority != dnsrec.Spec.Priority {
			err := api.UpdateDNSRecord(ctx, zoneID, records[0].ID, cloudflare.DNSRecord{
				Type:     dnsrec.Spec.Type,
				Name:     dnsrec.Spec.Name,
				Content:  dnsrec.Spec.Content,
				Proxied:  dnsrec.Spec.Proxied,
				TTL:      dnsrec.Spec.TTL,
				Priority: dnsrec.Spec.Priority,
			})
			if err != nil {
				l.log.V(1).Error(err, "Failed updating DNS record")
				return
			}
			l.log.V(3).Info("Updated record")
		}
	} else {
		l.log.Error(fmt.Errorf("too many records found"), "Too many records", "type", dnsrec.Spec.Type, "name", dnsrec.Spec.Name, "records", records)
	}
}

func (l *looper) start() error {
	if l.stopChannel != nil {
		return nil
	}

	interval, err := time.ParseDuration(SyncInterval)
	if err != nil {
		return fmt.Errorf("invalid duration '%s': %w", SyncInterval, err)
	}

	l.stopChannel = make(chan struct{})
	ticker := time.NewTicker(interval)
	go func(done chan struct{}, ticket *time.Ticker) {
		l.log.V(1).Info("Starting sync loop")
		defer ticker.Stop()
		for {
			select {
			case <-done:
				l.log.V(1).Info("Stopping sync loop")
				return
			case _ = <-ticker.C:
				ctx := context.TODO()
				l.sync(ctx)
			}
		}
	}(l.stopChannel, ticker)
	return nil
}

func (l *looper) stop() error {
	if l.stopChannel != nil {
		l.stopChannel <- struct{}{}
	}
	return nil
}
