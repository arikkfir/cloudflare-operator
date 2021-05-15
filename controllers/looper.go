package controllers

import (
	"context"
	"fmt"
	cfv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
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

func (l *looper) getAPIToken(ctx context.Context) (*string, error) {
	var secretNamespace, secretName string
	secretTokens := strings.SplitN(l.dnsRecord.Spec.APIToken.Secret, "/", 2)
	if len(secretTokens) == 2 {
		secretNamespace = secretTokens[0]
		secretName = secretTokens[1]
	} else {
		secretNamespace = l.dnsRecord.Namespace
		secretName = secretTokens[0]
	}

	secret := &v1.Secret{}
	err := l.client.Get(ctx, client.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to find secret '%s': %w", l.dnsRecord.Spec.APIToken.Secret, err)
	}

	token, ok := secret.Data[l.dnsRecord.Spec.APIToken.Key]
	if !ok {
		return nil, fmt.Errorf("failed to find key '%s' in secret '%s': %w", l.dnsRecord.Spec.APIToken.Key, l.dnsRecord.Spec.APIToken.Secret, err)
	}

	tokenString := string(token)
	return &tokenString, nil
}

func (l *looper) sync(ctx context.Context) {
	apiToken, err := l.getAPIToken(ctx)
	if err != nil {
		l.log.V(1).Error(err, "Failed finding APIKey")
		return
	}

	dnsrec := l.dnsRecord

	// Construct a new API object
	api, err := cloudflare.NewWithAPIToken(*apiToken)
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
		l.log.V(1).Info("DNS record not found, creating")
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
		diff := false
		if cfRec.Content != dnsrec.Spec.Content {
			diff = true
		}
		if cfRec.Proxied == nil && dnsrec.Spec.Proxied != nil || cfRec.Proxied != nil && dnsrec.Spec.Proxied == nil {
			diff = true
		}
		if cfRec.Proxied != nil && dnsrec.Spec.Proxied != nil && *cfRec.Proxied != *dnsrec.Spec.Proxied {
			diff = true
		}
		if cfRec.Proxied != nil && dnsrec.Spec.Proxied != nil && !*cfRec.Proxied && cfRec.TTL != dnsrec.Spec.TTL {
			diff = true
		}
		if cfRec.Priority == nil && dnsrec.Spec.Priority != nil || cfRec.Priority != nil && dnsrec.Spec.Priority == nil {
			diff = true
		}
		if cfRec.Priority != nil && dnsrec.Spec.Priority != nil && *cfRec.Priority != *dnsrec.Spec.Priority {
			diff = true
		}
		if diff {
			l.log.V(1).Info("DNS record not up to date, syncing")
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

func (l *looper) delete(ctx context.Context) {
	apiToken, err := l.getAPIToken(ctx)
	if err != nil {
		l.log.V(1).Error(err, "Failed finding APIKey")
		return
	}

	dnsrec := l.dnsRecord

	// Construct a new API object
	api, err := cloudflare.NewWithAPIToken(*apiToken)
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

	if len(records) == 1 {
		l.log.V(1).Info("Deleting DNS record")
		err := api.DeleteDNSRecord(ctx, zoneID, records[0].ID)
		if err != nil {
			l.log.V(1).Error(err, "Failed deleting DNS record")
			return
		}
	} else {
		l.log.Error(fmt.Errorf("too many records found"), "Too many records", "type", dnsrec.Spec.Type, "name", dnsrec.Spec.Name, "records", records)
	}
}
