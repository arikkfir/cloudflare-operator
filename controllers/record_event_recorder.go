package controllers

import (
	"context"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

type eventRecorder interface {
	recordEvent(_ context.Context, rec *dnsv1.Record, reason string, message string)
	recordEventf(_ context.Context, rec *dnsv1.Record, reason string, message string, args ...interface{})
	recordWarning(_ context.Context, rec *dnsv1.Record, reason string, message string)
	recordWarningf(_ context.Context, rec *dnsv1.Record, reason string, message string, args ...interface{})
	recordError(_ context.Context, rec *dnsv1.Record, reason string, message string, err error)
}

type eventRecorderImpl struct {
	k8sEventRecorder record.EventRecorder
}

// Record an event for the given Record resource.
func (r *eventRecorderImpl) recordEvent(_ context.Context, rec *dnsv1.Record, reason string, message string) {
	r.k8sEventRecorder.Event(rec, v1.EventTypeNormal, reason, message)
}

// Record an event for the given Record resource.
func (r *eventRecorderImpl) recordEventf(_ context.Context, rec *dnsv1.Record, reason string, message string, args ...interface{}) {
	r.k8sEventRecorder.Eventf(rec, v1.EventTypeNormal, reason, message, args...)
}

// Record a warning event for the given Record resource.
func (r *eventRecorderImpl) recordWarning(_ context.Context, rec *dnsv1.Record, reason string, message string) {
	r.k8sEventRecorder.Event(rec, v1.EventTypeWarning, reason, message)
}

// Record a warning event for the given Record resource.
func (r *eventRecorderImpl) recordWarningf(_ context.Context, rec *dnsv1.Record, reason string, message string, args ...interface{}) {
	r.k8sEventRecorder.Eventf(rec, v1.EventTypeWarning, reason, message, args...)
}

// Record an error event for the given Record resource.
func (r *eventRecorderImpl) recordError(_ context.Context, rec *dnsv1.Record, reason string, message string, err error) {
	r.k8sEventRecorder.Eventf(rec, v1.EventTypeWarning, reason, message+": %v", err)
}
