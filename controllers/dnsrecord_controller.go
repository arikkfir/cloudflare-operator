package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfv1 "github.com/arikkfir/cloudflare-operator/api/v1"
)

const (
	looperFinalizerName = "looper.finalizers." + cfv1.Group
)

// DNSRecordReconciler reconciles a DNSRecord object
type DNSRecordReconciler struct {
	client        client.Client
	dynamicClient dynamic.Interface
	log           logr.Logger
	loops         map[string]*looper
}

//+kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=cloudflare-operator.k8s.kfirs.com,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudflare-operator.k8s.kfirs.com,resources=dnsrecords/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudflare-operator.k8s.kfirs.com,resources=dnsrecords/finalizers,verbs=update

// Reconcile is the reconciliation loop implementation aiming to continuously
// move the current state of the cluster closer to the desired state, which in
// the DNSRecord controller's view means ensure the DNS record is synchronized in Cloudflare DNS.
// TODO: support status
func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.log.WithValues("dnsrecord", req.NamespacedName)

	dnsrec := &cfv1.DNSRecord{}
	if err := r.client.Get(ctx, req.NamespacedName, dnsrec); err != nil {
		logger.V(1).Info("Failed fetching DNSRecord resource")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If object is being deleted, perform finalization (if haven't already)
	if !dnsrec.ObjectMeta.DeletionTimestamp.IsZero() {
		if containsString(dnsrec.ObjectMeta.Finalizers, looperFinalizerName) {

			// Stop loop
			if err := r.removeLooperFor(dnsrec); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed deleting DNS record: %w", err)
			}

			// Remove our finalizer
			dnsrec.ObjectMeta.Finalizers = removeString(dnsrec.ObjectMeta.Finalizers, looperFinalizerName)
			if err := r.client.Update(context.Background(), dnsrec); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed removing finalizer: %w", err)
			}

		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Object is not being deleted! but ensure our finalizer is listed
	if !containsString(dnsrec.ObjectMeta.Finalizers, looperFinalizerName) {
		dnsrec.ObjectMeta.Finalizers = append(dnsrec.ObjectMeta.Finalizers, looperFinalizerName)
		if err := r.client.Update(context.Background(), dnsrec); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed adding finalizer: %w", err)
		}
	}

	// Ensure a loop exists for this DNS record
	if err := r.ensureLooperFor(dnsrec); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update loop: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.loops = make(map[string]*looper)
	r.client = mgr.GetClient()
	r.dynamicClient = dynamic.NewForConfigOrDie(mgr.GetConfig())
	r.log = ctrl.Log.WithName("controllers").WithName("DNSRecord")
	return ctrl.NewControllerManagedBy(mgr).
		For(&cfv1.DNSRecord{}).
		Complete(r)
}

// Register creates or updates the looper associated with the given binding.
// TODO: ensure thread-safety
func (r *DNSRecordReconciler) ensureLooperFor(dnsrec *cfv1.DNSRecord) error {
	key := dnsrec.Namespace + "/" + dnsrec.Name
	l, ok := r.loops[key]
	if ok {
		l.log.V(1).Info("Restarting sync loop")
		err := l.stop()
		if err != nil {
			return fmt.Errorf("failed stopping looper for '%s/%s': %w", dnsrec.GetNamespace(), dnsrec.GetName(), err)
		}
		l.dnsRecord = dnsrec
		err = l.start()
		if err != nil {
			return fmt.Errorf("failed updating looper for '%s/%s': %w", dnsrec.GetNamespace(), dnsrec.GetName(), err)
		}
		return nil
	} else {
		l = &looper{
			log:       r.log.WithValues("dnsrecord", dnsrec.Namespace+"/"+dnsrec.Name),
			dnsRecord: dnsrec,
			client:    r.client,
		}
		r.loops[key] = l
		err := l.start()
		if err != nil {
			return fmt.Errorf("failed creating loop: %w", err)
		}
		return nil
	}
}

// Unregister stops & removes the looper associated with the given binding.
// TODO: ensure thread-safety
func (r *DNSRecordReconciler) removeLooperFor(binding *cfv1.DNSRecord) error {
	key := binding.Namespace + "/" + binding.Name
	looper, ok := r.loops[key]
	if ok {
		r.log.V(1).Info("Stopping sync loop")
		err := looper.stop()
		if err != nil {
			return fmt.Errorf("failed stopping binding reconciliation loop: %w", err)
		}
	}
	return nil
}
