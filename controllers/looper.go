package controllers

import (
	"context"
	"fmt"
	cfv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (l *looper) setCondition(ctx context.Context, status metav1.ConditionStatus, reason string, message string) {
	setCondition(ctx, l.client, l.dnsRecord, l.log, status, reason, message)
}

func (l *looper) sync(ctx context.Context) {
	apiToken, err := l.getAPIToken(ctx)
	if err != nil {
		l.setCondition(ctx, "Unknown", "APITokenMissing", fmt.Sprintf("Failed to get API token: %s", err))
		return
	}

	dnsrec := l.dnsRecord

	// Construct a new API object
	api, err := cloudflare.NewWithAPIToken(*apiToken)
	if err != nil {
		l.setCondition(ctx, "Unknown", "CloudflareAuthFailure", fmt.Sprintf("Failed to authenticate to Cloudflare: %s", err))
		return
	}

	zoneID, err := api.ZoneIDByName(dnsrec.Spec.Zone)
	if err != nil {
		l.setCondition(ctx, "Unknown", "ZoneNotFound", fmt.Sprintf("Failed to find Cloudflare zone: %s", err))
		return
	}

	records, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{
		Type: dnsrec.Spec.Type,
		Name: dnsrec.Spec.Name,
	})
	if err != nil {
		l.setCondition(ctx, "Unknown", "FailedListingRecords", fmt.Sprintf("Failed to list DNS records in zone: %s", err))
		return
	}

	if len(records) == 0 {
		l.setCondition(ctx, "False", "RecordNotFound", "DNS record not found")

		_, err = api.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecord{
			Type:     dnsrec.Spec.Type,
			Name:     dnsrec.Spec.Name,
			Content:  dnsrec.Spec.Content,
			Proxied:  dnsrec.Spec.Proxied,
			TTL:      dnsrec.Spec.TTL,
			Priority: dnsrec.Spec.Priority,
		})
		if err != nil {
			l.setCondition(ctx, "False", "RecordCreationFailed", fmt.Sprintf("Failed to create DNS record: %s", err))
		} else {
			l.setCondition(ctx, "True", "RecordCreated", "DNS record created")
		}
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
			l.setCondition(ctx, "False", "StaleRecord", "DNS record is out of sync")

			err = api.UpdateDNSRecord(ctx, zoneID, records[0].ID, cloudflare.DNSRecord{
				Type:     dnsrec.Spec.Type,
				Name:     dnsrec.Spec.Name,
				Content:  dnsrec.Spec.Content,
				Proxied:  dnsrec.Spec.Proxied,
				TTL:      dnsrec.Spec.TTL,
				Priority: dnsrec.Spec.Priority,
			})
			if err != nil {
				l.setCondition(ctx, "False", "RecordUpdateFailed", fmt.Sprintf("Failed to update DNS record: %s", err))
			} else {
				l.setCondition(ctx, "True", "RecordUpdated", "")
			}
		} else {
			l.setCondition(ctx, "True", "RecordUpToDate", "DNS record up to date")
		}
	} else {
		l.setCondition(ctx, "Unknown", "TooManyRecordsFound", "Too many DNS records matched this record spec (should not happen)")
	}
}

func (l *looper) start() error {
	if l.stopChannel != nil {
		return nil
	}

	l.setCondition(context.Background(), "Unknown", "StartingSyncer", "Starting reconciliation loop")

	interval, err := time.ParseDuration(SyncInterval)
	if err != nil {
		l.setCondition(context.Background(), "Unknown", "FailedToStartSyncer", fmt.Sprintf("Failed to start the reconciliation loop: %s", err))
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
				l.setCondition(context.Background(), "Unknown", "SyncerStopped", "Stopped reconciliation loop")
				return
			case _ = <-ticker.C:
				l.sync(context.TODO())
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
		} else {
			l.setCondition(ctx, "False", "RecordDeleted", "Deleted DNS record in Cloudflare zone")
		}
	} else {
		l.log.Error(fmt.Errorf("too many records found"), "Too many records", "type", dnsrec.Spec.Type, "name", dnsrec.Spec.Name, "records", records)
	}
}
