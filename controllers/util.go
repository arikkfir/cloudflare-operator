package controllers

import (
	"context"
	"fmt"
	cfv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// setCondition applies the given condition in the resource's status subresource, adding it if it's missing.
func setCondition(ctx context.Context, client client.Client, dnsrec *cfv1.DNSRecord, logger logr.Logger, status metav1.ConditionStatus, reason string, message string) {
	err := updateCondition(ctx, client, dnsrec, metav1.Condition{
		Type:               ConditionTypeSynced,
		Status:             status,
		ObservedGeneration: dnsrec.Generation,
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Reason:             reason,
		Message:            message,
	})
	if err != nil {
		logger.V(1).Error(err, "Failed updating status for DNS record")
	}
}

// updateCondition updates the given condition in the resource's status subresource, adding it if it's missing.
func updateCondition(ctx context.Context, client client.Client, dnsrec *cfv1.DNSRecord, condition metav1.Condition) error {
	for i, v := range dnsrec.Status.Conditions {
		if v.Type == condition.Type {
			dnsrec.Status.Conditions[i] = condition
			err := client.Status().Update(ctx, dnsrec)
			if err != nil {
				return fmt.Errorf("failed updating status condition for '%s/%s': %w", dnsrec.Namespace, dnsrec.Name, err)
			}
			return nil
		}
	}

	dnsrec.Status.Conditions = append(dnsrec.Status.Conditions, condition)
	err := client.Status().Update(ctx, dnsrec)
	if err != nil {
		return fmt.Errorf("failed adding status condition for '%s/%s': %w", dnsrec.Namespace, dnsrec.Name, err)
	}
	return nil
}
