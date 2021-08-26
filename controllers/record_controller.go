package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/arikkfir/cloudflare-operator/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
)

const (
	// ConditionSynced is a DNS record condition signalling whether the DNS record is synchronized with Cloudflare DNS.
	ConditionSynced = "Synced"

	// ConditionLive is a DNS record condition signalling whether the DNS record object is actively being monitored.
	ConditionLive = "Live"

	// Finalizer that signals whether the verifier has been stopped & removed
	stopSyncerFinalizerName = "syncer.finalizers.dns.cloudflare.k8s.kfirs.com"

	// Finalizer that signals whether the DNS record has been deleted
	deleteDNSRecordFinalizerName = "delete.finalizers.dns.cloudflare.k8s.kfirs.com"

	// Reasons:

	ReasonPending             = "ReasonPending"
	ReasonError               = "ReasonError"
	ReasonInvalidSyncInterval = "InvalidSyncInterval"
	ReasonDNSRecordNotFound   = "ReasonDNSRecordNotFound"
	ReasonDNSRecordStale      = "ReasonDNSRecordStale"
	ReasonDNSRecordExists     = "ReasonDNSRecordExists"
	ReasonDNSRecordDeleted    = "DNSRecordDeleted"
	ReasonDNSRecordCreated    = "DNSRecordCreated"
	ReasonDNSRecordUpdated    = "DNSRecordUpdated"
	ReasonResourceDeleted     = "ResourceDeleted"
)

// RecordReconciler reconciles a Record object
type RecordReconciler struct {
	log              logr.Logger
	scheme           *runtime.Scheme
	client           client.Client
	statusRecorder   statusRecorder
	eventRecorder    eventRecorder
	cloudflareClient internal.CloudflareClient
	synchronizers    map[string]*synchronizer
	lock             *sync.RWMutex
}

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=dns.cloudflare.k8s.kfirs.com,resources=records,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=dns.cloudflare.k8s.kfirs.com,resources=records/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.cloudflare.k8s.kfirs.com,resources=records/finalizers,verbs=update

// Reconcile will ensure that the given DNS record resource is actively being synchronized with Cloudflare.
func (r *RecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error

	// Obtain the write lock
	r.lock.Lock()
	defer r.lock.Unlock()

	// Fetch the resource
	rec := &dnsv1.Record{}
	if err := r.client.Get(ctx, req.NamespacedName, rec); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure status conditions are present on the status subresource
	if err := r.statusRecorder.addStatusConditionIfMissing(rec, ctx, ConditionSynced, metav1.ConditionUnknown, ReasonPending, ""); err != nil {
		r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.statusRecorder.addStatusConditionIfMissing(rec, ctx, ConditionLive, metav1.ConditionUnknown, ReasonPending, ""); err != nil {
		r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return ctrl.Result{Requeue: true}, nil
	}

	// If object is being deleted
	if !rec.ObjectMeta.DeletionTimestamp.IsZero() {

		// Stop syncer (if we haven't already)
		if internal.ContainsString(rec.ObjectMeta.Finalizers, stopSyncerFinalizerName) {
			key := rec.Namespace + "/" + rec.Name
			if syncer, syncerExists := r.synchronizers[key]; syncerExists {
				if err := syncer.Stop(ctx); err != nil {
					r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
					return ctrl.Result{Requeue: true}, nil
				}
				delete(r.synchronizers, key)
			}
			if err := r.statusRecorder.setStatusCondition(rec, ctx, ConditionLive, metav1.ConditionFalse, ReasonResourceDeleted, ""); err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}
			rec.ObjectMeta.Finalizers = internal.RemoveString(rec.ObjectMeta.Finalizers, stopSyncerFinalizerName)
			if err := r.client.Update(ctx, rec); err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}
		}

		// Delete DNS record (if we haven't already)
		if internal.ContainsString(rec.ObjectMeta.Finalizers, deleteDNSRecordFinalizerName) {
			apiToken, err := internal.GetSecretValue(ctx, r.client, rec.Namespace, rec.Spec.APIToken.Secret, rec.Spec.APIToken.Key)
			if err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}

			err = r.cloudflareClient.DeleteDNSRecord(ctx, apiToken, rec.Spec.Zone, rec.Spec.Type, rec.Spec.Name)
			if err != nil {
				if !errors.Is(err, internal.DNSRecordNotFound) {
					r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
					return ctrl.Result{Requeue: true}, nil
				}
			} else {
				r.eventRecorder.recordEvent(ctx, rec, ReasonDNSRecordDeleted, "")
				if err := r.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionFalse, ReasonDNSRecordDeleted, ""); err != nil {
					r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
					return ctrl.Result{Requeue: true}, nil
				}
			}

			rec.ObjectMeta.Finalizers = internal.RemoveString(rec.ObjectMeta.Finalizers, deleteDNSRecordFinalizerName)
			if err := r.client.Update(ctx, rec); err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}
		}

		return ctrl.Result{}, nil
	}

	// Object is not being deleted! but ensure our finalizers are listed
	for _, finalizer := range []string{stopSyncerFinalizerName, deleteDNSRecordFinalizerName} {
		if !internal.ContainsString(rec.ObjectMeta.Finalizers, finalizer) {
			rec.ObjectMeta.Finalizers = append(rec.ObjectMeta.Finalizers, finalizer)
			if err := r.client.Update(ctx, rec); err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}

	// Get syncer for this record, if any
	key := rec.Namespace + "/" + rec.Name
	syncer, syncerExists := r.synchronizers[key]

	// Validate sync interval
	syncInterval, err := time.ParseDuration(rec.Spec.SyncInterval)
	if err != nil {
		if syncerExists {
			if err := syncer.Stop(ctx); err != nil {
				r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
				return ctrl.Result{Requeue: true}, nil
			}
			delete(r.synchronizers, key)
		}
		if err := r.statusRecorder.setStatusCondition(rec, ctx, ConditionLive, metav1.ConditionFalse, ReasonInvalidSyncInterval, ""); err != nil {
			r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}
		if err := r.statusRecorder.setStatusCondition(rec, ctx, ConditionSynced, metav1.ConditionUnknown, ReasonInvalidSyncInterval, ""); err != nil {
			r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	}

	// Create syncer if haven't done so yet
	if !syncerExists {
		syncer, err = NewSyncer(rec.Namespace, rec.Name, r.client, r.statusRecorder, r.eventRecorder, r.cloudflareClient)
		if err != nil {
			r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}
		r.synchronizers[key] = syncer
	}
	if err := syncer.Start(ctx, syncInterval); err != nil {
		r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return ctrl.Result{Requeue: true}, nil
	}
	if err := r.statusRecorder.setStatusCondition(rec, ctx, ConditionLive, metav1.ConditionTrue, "", ""); err != nil {
		r.eventRecorder.recordWarning(ctx, rec, ReasonError, err.Error())
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.log = ctrl.Log.WithName("controllers").WithName("Record")
	r.lock = &sync.RWMutex{}
	r.statusRecorder = &statusRecorderImpl{client: r.client}
	r.eventRecorder = &eventRecorderImpl{k8sEventRecorder: mgr.GetEventRecorderFor("record-controller")}
	r.scheme = mgr.GetScheme()
	if r.cloudflareClient == nil {
		cfClient, err := internal.NewCloudflareClient()
		if err != nil {
			return fmt.Errorf("failed creating Cloudflare client: %w", err)
		}
		r.cloudflareClient = cfClient
	}
	r.synchronizers = make(map[string]*synchronizer)
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1.Record{}).
		Complete(r)
}
