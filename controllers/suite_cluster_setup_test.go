package controllers

import (
	"context"
	"fmt"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/arikkfir/cloudflare-operator/internal"
	testing2 "github.com/go-logr/logr/testing"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"math"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
	"testing"
)

type KubernetesTestingContext struct {
	t          *testing.T
	env        *envtest.Environment
	cfg        *rest.Config
	mgr        manager.Manager
	reconciler *RecordReconciler
	k8sClient  struct {
		client client.Client
		err    error
	}
	k8sClientFactory sync.Once
	k8sClientset     struct {
		clientset *kubernetes.Clientset
		err       error
	}
	k8sClientsetFactory sync.Once
}

func (t *KubernetesTestingContext) Client() client.Client {
	t.k8sClientFactory.Do(func() {
		k8sClient, err := client.New(t.cfg, client.Options{Scheme: scheme.Scheme})
		if err != nil {
			err = fmt.Errorf("failed to create Kubernetes client: %w", err)
		} else if k8sClient == nil {
			err = fmt.Errorf("failed to create Kubernetes client: nil Kubernetes client received")
		}
		t.k8sClient = struct {
			client client.Client
			err    error
		}{client: k8sClient, err: err}
	})
	if t.k8sClient.err != nil {
		t.t.Fatal(t.k8sClient.err)
	}
	return t.k8sClient.client
}

func (t *KubernetesTestingContext) Clientset() *kubernetes.Clientset {
	t.k8sClientsetFactory.Do(func() {
		clientset, err := kubernetes.NewForConfig(t.cfg)
		if err != nil {
			err = fmt.Errorf("failed to create Kubernetes client-set: %w", err)
		} else if clientset == nil {
			err = fmt.Errorf("failed to create Kubernetes client-set: nil Kubernetes client-set received")
		}
		t.k8sClientset = struct {
			clientset *kubernetes.Clientset
			err       error
		}{clientset: clientset, err: err}
	})
	if t.k8sClientset.err != nil {
		t.t.Fatal(t.k8sClientset.err)
	}
	return t.k8sClientset.clientset
}

func SetupK8s(t *testing.T) (*KubernetesTestingContext, error) {
	t.Helper()
	mockCtrl := gomock.NewController(t)

	k8sTestCtx := KubernetesTestingContext{
		t: t,
		env: &envtest.Environment{
			UseExistingCluster:    &[]bool{true}[0],
			CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		},
		k8sClientFactory:    sync.Once{},
		k8sClientsetFactory: sync.Once{},
	}

	cfg, err := k8sTestCtx.env.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start test environment: %w", err)
	} else if cfg == nil {
		return nil, fmt.Errorf("failed to start test environment: : nil test environment received")
	}
	k8sTestCtx.cfg = cfg
	t.Cleanup(func() {
		if err := k8sTestCtx.env.Stop(); err != nil {
			t.Errorf("Failed stopping Kubernetes manager: %v", err)
		}
	})

	if err := dnsv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register Kubernetes scheme: %w", err)
	}

	mgrOptions := ctrl.Options{
		Logger:                 testing2.TestLogger{T: t},
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     fmt.Sprintf("127.0.0.1:%d", rand.IntnRange(1024*20, math.MaxInt16)),
		HealthProbeBindAddress: fmt.Sprintf("127.0.0.1:%d", rand.IntnRange(1024*20, math.MaxInt16)),
	}
	mgr, err := ctrl.NewManager(k8sTestCtx.cfg, mgrOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes manager: %w", err)
	} else if mgr == nil {
		return nil, fmt.Errorf("failed to create Kubernetes manager: nil Kubernetes manager received")
	}
	k8sTestCtx.mgr = mgr

	k8sTestCtx.reconciler = &RecordReconciler{
		cloudflareClient: internal.NewMockCloudflareClient(mockCtrl),
	}
	if err := k8sTestCtx.reconciler.SetupWithManager(k8sTestCtx.mgr); err != nil {
		return nil, fmt.Errorf("failed to create Record reconciler: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := k8sTestCtx.mgr.Start(ctx); err != nil {
			t.Errorf("Failed starting Kubernetes manager: %v", err)
		}
	}()

	return &k8sTestCtx, nil
}
