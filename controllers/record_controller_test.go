package controllers

import (
	"context"
	"fmt"
	"github.com/Pallinder/go-randomdata"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/arikkfir/cloudflare-operator/internal"
	"github.com/cloudflare/cloudflare-go"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
	"time"
)

func createSecret(g *GomegaWithT, k8sClient client.Client, namespace, name string) *v1.Secret {
	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Type:       v1.SecretTypeOpaque,
		Immutable:  &[]bool{true}[0],
		StringData: map[string]string{"token": "test"},
	}
	g.Expect(k8sClient.Create(context.Background(), &secret)).To(Succeed())
	return &secret
}

func createRecord(t *testing.T, g *GomegaWithT, k8sClient client.Client, namespace, name, content string, ttl int, secretRef, key, syncInterval string) *dnsv1.Record {
	rec := dnsv1.Record{
		TypeMeta:   metav1.TypeMeta{APIVersion: dnsv1.GroupVersion.String(), Kind: "Record"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: dnsv1.RecordSpec{
			Zone:         name + ".com",
			Type:         "A",
			Name:         "www." + name + ".com",
			Content:      content,
			TTL:          ttl,
			Priority:     nil,
			Proxied:      pointer.BoolPtr(true),
			APIToken:     dnsv1.SecretKeyRef{Secret: secretRef, Key: key},
			SyncInterval: syncInterval,
		},
	}
	g.Expect(func() error {
		err := k8sClient.Create(context.Background(), &rec)
		if err != nil {
			return err
		}
		t.Cleanup(deleteRecord(t, k8sClient, namespace, name))
		return nil
	}()).To(Succeed())
	return &rec
}

func deleteRecord(t *testing.T, k8sClient client.Client, namespace, name string) func() {
	return func() {
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			r := dnsv1.Record{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &r)
			if err != nil {
				return err
			}

			r.ObjectMeta.Finalizers = []string{}
			if err := k8sClient.Update(context.Background(), &r); err != nil {
				return err
			} else if err := k8sClient.Delete(context.Background(), &r); client.IgnoreNotFound(err) != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}
}

func TestFinalizersAreAddedToNewRecord(t *testing.T) {
	_, _, err := CreateManager(t)
	if err != nil {
		t.Fatalf("Failed creating Kubernetes environment: %+v", err)
	}

	var g = NewGomegaWithT(t)
	var name = strings.ToLower(randomdata.SillyName())
	rec := createRecord(t, g, k8sClient, "default", name, "1.2.3.4", 300, "default/"+name+"-secret", "token", "10s")
	g.Eventually(func() error {
		r := dnsv1.Record{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: rec.Name, Namespace: rec.Namespace}, &r)
		if err != nil {
			return err
		}

		// Verify finalizers are registered
		verifierFinalizerFound, deleteDNSRecordFinalizerFound := false, false
		for _, finalizer := range r.ObjectMeta.Finalizers {
			switch finalizer {
			case stopSyncerFinalizerName:
				verifierFinalizerFound = true
			case deleteDNSRecordFinalizerName:
				deleteDNSRecordFinalizerFound = true
			}
		}
		if !verifierFinalizerFound {
			return fmt.Errorf("verifier '%s' not found", stopSyncerFinalizerName)
		} else if !deleteDNSRecordFinalizerFound {
			return fmt.Errorf("verifier '%s' not found", deleteDNSRecordFinalizerName)
		}
		return nil
	}, 30*time.Second, 1000*time.Millisecond).Should(Succeed())
}

func TestInitialConditionsAreAddedToNewRecord(t *testing.T) {
	_, _, err := CreateManager(t)
	if err != nil {
		t.Fatalf("Failed creating Kubernetes environment: %+v", err)
	}

	var g = NewGomegaWithT(t)
	var name = strings.ToLower(randomdata.SillyName())

	rec := createRecord(t, g, k8sClient, "default", name, "1.2.3.4", 300, "default/"+name+"-secret", "token", "10s")
	g.Eventually(func() error {
		r := dnsv1.Record{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: rec.Name, Namespace: rec.Namespace}, &r)
		if err != nil {
			return err
		}
		for _, condition := range r.Status.Conditions {
			switch condition.Type {
			case ConditionLive:
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition '%s' should be '%s'", condition.Type, metav1.ConditionFalse)
				}
			case ConditionSynced:
				if condition.Status != metav1.ConditionUnknown {
					return fmt.Errorf("condition '%s' should be '%s'", condition.Type, metav1.ConditionUnknown)
				}
			}
		}
		return nil
	}, 30*time.Second, 1000*time.Millisecond).Should(Succeed())
}

func TestDNSRecordCreatedForNewRecord(t *testing.T) {
	_, reconciler, err := CreateManager(t)
	if err != nil {
		t.Fatalf("Failed creating Kubernetes environment: %+v", err)
	}

	var g = NewGomegaWithT(t)
	var name = strings.ToLower(randomdata.SillyName())

	rec := createRecord(t, g, k8sClient, "default", name, "1.2.3.4", 300, "default/"+name+"-secret", "token", "10s")
	secret := createSecret(g, k8sClient, rec.Namespace, rec.Name+"-secret")
	mock := reconciler.cloudflareClient.(*internal.MockCloudflareClient)
	mock.EXPECT().
		GetDNSRecord(gomock.AssignableToTypeOf(context.Background()), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name).
		Times(1).
		Return(nil, internal.DNSRecordNotFound)
	mock.EXPECT().
		CreateDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name, rec.Spec.Content, rec.Spec.Proxied, rec.Spec.TTL, rec.Spec.Priority).
		Times(1).
		Return(nil)
	mock.EXPECT().
		GetDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name).
		AnyTimes().
		Return(&cloudflare.DNSRecord{
			ID:         "999",
			Type:       rec.Spec.Type,
			Name:       rec.Spec.Name,
			Content:    rec.Spec.Content,
			Proxiable:  true,
			Proxied:    rec.Spec.Proxied,
			TTL:        rec.Spec.TTL,
			Locked:     false,
			ZoneID:     "9999",
			ZoneName:   rec.Spec.Zone,
			CreatedOn:  time.Time{},
			ModifiedOn: time.Time{},
			Data:       nil,
			Meta:       nil,
			Priority:   nil,
		}, nil)
	g.Eventually(func() error {
		r := dnsv1.Record{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: rec.Name, Namespace: rec.Namespace}, &r)
		if err != nil {
			return err
		}
		for _, condition := range r.Status.Conditions {
			switch condition.Type {
			case ConditionLive:
				if condition.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition '%s' should be '%s'", condition.Type, metav1.ConditionFalse)
				}
			case ConditionSynced:
				if condition.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition '%s' should be '%s'", condition.Type, metav1.ConditionTrue)
				}
			}
		}
		return nil
	}, 30*time.Second, 1000*time.Millisecond).Should(Succeed())
}

// func TestNewRecordCreation(t *testing.T) {
// 	t.SkipNow()
//
// 	env, err := SetupK8s(t)
// 	if err != nil {
// 		t.Fatalf("Failed creating Kubernetes environment: %+v", err)
// 	}
//
// 	var g = NewGomegaWithT(t)
// 	var k8sClient = env.Client()
// 	var name = strings.ToLower(randomdata.SillyName())
//
// 	// Generate record & secret
// 	rec := createRecord(nil, g, k8sClient, "default", name, "1.2.3.4", 300, "default/"+name+"-secret", "token", "10s")
// 	secret := createSecret(g, k8sClient, rec.Namespace, rec.Name+"-secret")
//
// 	// Prepare our mock
// 	mock := env.reconciler.cloudflareClient.(*internal.MockCloudflareClient)
// 	mock.EXPECT().
// 		GetDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name).
// 		Times(1).
// 		DoAndReturn(func(ctx context.Context, apiToken, zoneName, recordType, recordName string) (*cloudflare.DNSRecord, error) {
// 			time.Sleep(1 * time.Minute)
// 			t.FailNow()
// 			//goland:noinspection GoUnreachableCode
// 			panic("fail test")
// 			//	Return(nil, internal.DNSRecordNotFound)
// 		})
//
// 	mock.EXPECT().
// 		CreateDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name, rec.Spec.Content, rec.Spec.Proxied, rec.Spec.TTL, rec.Spec.Priority).
// 		Times(1).
// 		Return(nil)
// 	mock.EXPECT().
// 		GetDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name).
// 		AnyTimes().
// 		Return(&cloudflare.DNSRecord{
// 			ID:         "999",
// 			Type:       rec.Spec.Type,
// 			Name:       rec.Spec.Name,
// 			Content:    rec.Spec.Content,
// 			Proxiable:  true,
// 			Proxied:    rec.Spec.Proxied,
// 			TTL:        rec.Spec.TTL,
// 			Locked:     false,
// 			ZoneID:     "9999",
// 			ZoneName:   rec.Spec.Zone,
// 			CreatedOn:  time.Time{},
// 			ModifiedOn: time.Time{},
// 			Data:       nil,
// 			Meta:       nil,
// 			Priority:   nil,
// 		}, nil)
//
// 	// Verify over time that it progresses validly
// 	g.Eventually(func() error {
// 		r := dnsv1.Record{}
// 		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: rec.Name, Namespace: rec.Namespace}, &r)
// 		if err != nil {
// 			return err
// 		}
//
// 		events, err := env.Clientset().EventsV1().Events(rec.Namespace).List(context.Background(), metav1.ListOptions{})
// 		if err != nil {
// 			return fmt.Errorf("failed listing events: %w", err)
// 		}
// 		found := false
// 		for _, event := range events.Items {
// 			if event.Regarding.Name == name && event.Type == v1.EventTypeNormal && event.Reason == ReasonDNSRecordCreated {
// 				found = true
// 				break
// 			}
// 		}
// 		if !found {
// 			return fmt.Errorf("event '%s' not registered", ReasonDNSRecordCreated)
// 		}
//
// 		return nil
// 	}, 30*time.Second, 1000*time.Millisecond).Should(Succeed())
//
// 	// Expect delete call
// 	mock.EXPECT().
// 		DeleteDNSRecord(gomock.Any(), string(secret.Data["token"]), rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name).
// 		Times(1).
// 		Return(nil)
//
// 	// Delete record asynchronously
// 	go func() {
// 		r := rec
// 		err = k8sClient.Delete(context.Background(), r, &client.DeleteOptions{GracePeriodSeconds: &[]int64{300}[0]})
// 		if err != nil {
// 			t.Errorf("Failed deleting Record: %+v", err)
// 		}
// 	}()
//
// 	// Verify creation of record & its finalizers
// 	g.Eventually(func() error {
// 		r := dnsv1.Record{}
// 		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: rec.Name, Namespace: rec.Namespace}, &r)
// 		if err != nil {
// 			if apiErrors.IsNotFound(err) {
// 				return nil
// 			}
// 			return err
// 		}
// 		return fmt.Errorf("record still exists: %s/%s", r.Namespace, r.Name)
// 	}, 10*time.Second, 1000*time.Millisecond).Should(Succeed())
// }
