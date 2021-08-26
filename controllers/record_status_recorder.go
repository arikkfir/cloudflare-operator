package controllers

import (
	"context"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type statusRecorder interface {
	addStatusConditionIfMissing(recp *dnsv1.Record, ctx context.Context, conditionType string, status metav1.ConditionStatus, reason, message string) error
	setStatusCondition(recp *dnsv1.Record, ctx context.Context, conditionType string, status metav1.ConditionStatus, reason, message string) error
}

type statusRecorderImpl struct {
	client client.Client
}

func (r *statusRecorderImpl) addStatusConditionIfMissing(recp *dnsv1.Record, ctx context.Context, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	for _, v := range recp.Status.Conditions {
		if v.Type == conditionType {
			return nil
		}
	}
	recp.Status.Conditions = append(recp.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: recp.Generation,
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             reason,
		Message:            message,
	})
	return r.client.Status().Update(ctx, recp)
}

func (r *statusRecorderImpl) setStatusCondition(recp *dnsv1.Record, ctx context.Context, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	for _, v := range recp.Status.Conditions {
		if v.Type == conditionType {
			if status != v.Status || reason != v.Reason || message != v.Message {
				v.Status = status
				v.Reason = reason
				v.Message = message
				v.LastTransitionTime = metav1.Time{Time: time.Now()}
				return r.client.Status().Update(ctx, recp)
			} else {
				return nil
			}
		}
	}
	recp.Status.Conditions = append(recp.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: recp.Generation,
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             reason,
		Message:            message,
	})
	return r.client.Status().Update(ctx, recp)
}
