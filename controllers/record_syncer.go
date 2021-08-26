package controllers

import (
	"context"
	"errors"
	"fmt"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/arikkfir/cloudflare-operator/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// synchronizer holds an ongoing DNS synchronization controller.
type synchronizer struct {
	namespace        string
	name             string
	client           client.Client
	statusRecorder   statusRecorder
	eventRecorder    eventRecorder
	cloudflareClient internal.CloudflareClient
	interval         *time.Duration
	ticker           *time.Ticker
	stopRequestChan  chan struct{}
	stopResponseChan chan struct{}
}

func NewSyncer(namespace, name string, client client.Client, statusRecorder statusRecorder, eventRecorder eventRecorder, cloudflareClient internal.CloudflareClient) (*synchronizer, error) {
	return &synchronizer{
		namespace:        namespace,
		name:             name,
		client:           client,
		statusRecorder:   statusRecorder,
		eventRecorder:    eventRecorder,
		cloudflareClient: cloudflareClient,
		interval:         nil,
		ticker:           nil,
		stopRequestChan:  nil,
		stopResponseChan: nil,
	}, nil
}

func (s *synchronizer) Start(ctx context.Context, interval time.Duration) error {
	if currentInterval := s.interval; currentInterval != nil {
		if *currentInterval == interval {
			return nil
		}
		if err := s.Stop(ctx); err != nil {
			return err
		}
	}

	s.interval = &interval
	s.ticker = time.NewTicker(interval)
	s.stopRequestChan = make(chan struct{})
	s.stopResponseChan = make(chan struct{})
	go s.sync()
	return nil
}

func (s *synchronizer) Stop(ctx context.Context) error {
	if s.ticker == nil {
		return fmt.Errorf("cannot stop syncer - it was never started")
	} else if s.stopRequestChan == nil {
		return fmt.Errorf("cannot stop syncer - it was not started correctly (stop-request channel is nil)")
	} else if s.stopResponseChan == nil {
		return fmt.Errorf("cannot stop syncer - it was not started correctly (stop-response channel is nil)")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.ticker.Stop()
	s.stopRequestChan <- struct{}{}
	<-s.stopResponseChan

	s.interval = nil
	s.ticker = nil
	s.stopRequestChan = nil
	s.stopResponseChan = nil
	return nil
}

func (s *synchronizer) sync() {
	for {
		select {
		case _ = <-s.ticker.C:
			s.tick(context.Background())
		case <-s.stopRequestChan:
			s.stopResponseChan <- struct{}{}
			return
		}
	}
}

func (s *synchronizer) tick(ctx context.Context) {
	rec := &dnsv1.Record{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: s.name}, rec); err != nil {
		s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return
	}

	apiToken, err := internal.GetSecretValue(ctx, s.client, rec.Namespace, rec.Spec.APIToken.Secret, rec.Spec.APIToken.Key)
	if err != nil {
		s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return
	}

	dnsRec, err := s.cloudflareClient.GetDNSRecord(ctx, apiToken, rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name)
	if err != nil {
		if errors.Is(err, internal.DNSRecordNotFound) {
			if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionFalse, ReasonDNSRecordNotFound, ""); err != nil {
				s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return
			}

			err := s.cloudflareClient.CreateDNSRecord(ctx, apiToken, rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name, rec.Spec.Content, rec.Spec.Proxied, rec.Spec.TTL, rec.Spec.Priority)
			if err != nil {
				s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return
			}
			s.eventRecorder.recordEvent(ctx, rec, ReasonDNSRecordCreated, "")

			if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionTrue, ReasonDNSRecordCreated, ""); err != nil {
				s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return
			}
		} else {
			s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionUnknown, ReasonError, ""); err != nil {
				s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return
			}
		}
		return
	}

	diff := dnsRec.Content != rec.Spec.Content ||
		(dnsRec.Proxied == nil && rec.Spec.Proxied != nil) ||
		(dnsRec.Proxied != nil && rec.Spec.Proxied == nil) ||
		(dnsRec.Proxied != nil && rec.Spec.Proxied != nil && *dnsRec.Proxied != *rec.Spec.Proxied) ||
		(dnsRec.Proxied != nil && rec.Spec.Proxied != nil && !*dnsRec.Proxied && dnsRec.TTL != rec.Spec.TTL) ||
		(dnsRec.Priority == nil && rec.Spec.Priority != nil) ||
		(dnsRec.Priority != nil && rec.Spec.Priority == nil) ||
		(dnsRec.Priority != nil && rec.Spec.Priority != nil && *dnsRec.Priority != *rec.Spec.Priority)

	if diff {
		if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionFalse, ReasonDNSRecordStale, ""); err != nil {
			s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return
		}

		err := s.cloudflareClient.UpdateDNSRecord(ctx, apiToken, rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name, rec.Spec.Content, rec.Spec.Proxied, rec.Spec.TTL, rec.Spec.Priority)
		if err != nil {
			s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return
		}
		s.eventRecorder.recordEvent(ctx, rec, ReasonDNSRecordUpdated, "")
		if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionTrue, ReasonDNSRecordUpdated, ""); err != nil {
			s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return
		}
	} else {
		for _, condition := range rec.Status.Conditions {
			if condition.Type == ConditionSynced {
				if condition.Status == metav1.ConditionTrue {
					return
				}
				break
			}
		}
		if err := s.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionTrue, ReasonDNSRecordExists, ""); err != nil {
			s.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		}
	}
}
