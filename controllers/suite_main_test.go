package controllers

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/Pallinder/go-randomdata"
	dnsv1 "github.com/arikkfir/cloudflare-operator/api/v1"
	"github.com/arikkfir/cloudflare-operator/internal"
	testing2 "github.com/go-logr/logr/testing"
	"github.com/golang/mock/gomock"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"
	"testing"
	"time"
)

//go:embed kind-cluster-config.yaml
var kindClusterConfig string

var restConfig *rest.Config
var k8sClient client.Client
var clientset *kubernetes.Clientset

// TestMain bootstraps the test suite by launching a temporary "kind" Kubernetes cluster before running the tests, and
// deleting it after tests have completed.
func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true, Level: zapcore.DebugLevel})))

	var exitCode int

	skipCreateCluster := strings.ToLower(os.Getenv("SKIP_CREATE_CLUSTER"))
	if skipCreateCluster == "1" || skipCreateCluster == "yes" || skipCreateCluster == "true" {

		ctrl.Log.Info("Skipping 'kind' cluster creation")
		exitCode = run(m)

	} else {

		ctrl.Log.Info("Creating 'kind' cluster")
		clusterName := startCluster()
		exitCode = run(m)
		deleteCluster(clusterName)

	}

	os.Exit(exitCode)
}

// startCluster creates & starts a temporary "kind" cluster. It will extracts the embedded configuration file into a
// temporary file and provide the path for that file to "kind". It will also ask "kind" to generate a kubeconfig file
// and set the "KUBECONFIG" environment variable to point to it.
func startCluster() string {
	kindClusterConfigFile, err := ioutil.TempFile("", "kind-cluster-config-*.yaml")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(kindClusterConfigFile.Name(), []byte(kindClusterConfig), 0644); err != nil {
		panic(err)
	}

	kindConfigFile, err := ioutil.TempFile("", "kubeconfig-*.yaml")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("KUBECONFIG", kindConfigFile.Name()); err != nil {
		panic(err)
	}

	clusterName := "cf-operator-" + strings.ToLower(randomdata.SillyName())
	cmd := exec.Command("../bin/kind", "create", "cluster", "--name="+clusterName, "--wait=60s",
		"--config="+kindClusterConfigFile.Name(), "--kubeconfig="+kindConfigFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	return clusterName
}

func deleteCluster(clusterName string) {
	cmd := exec.Command("kind", "delete", "cluster", "--name="+clusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		ctrl.Log.Error(err, "Failed to delete 'kind' cluster")
	}
}

func run(m *testing.M) int {
	env := &envtest.Environment{
		UseExistingCluster:    &[]bool{true}[0],
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		panic(fmt.Errorf("failed to start test environment: %w", err))
	}
	if cfg == nil {
		panic(fmt.Errorf("failed to start test environment: : nil test environment received"))
	}
	restConfig = cfg

	k8sClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(fmt.Errorf("failed to create Kubernetes client: %w", err))
	} else if k8sClient == nil {
		panic(fmt.Errorf("failed to create Kubernetes client: nil Kubernetes client received"))
	}

	clientset, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create Kubernetes client-set: %w", err))
	} else if clientset == nil {
		panic(fmt.Errorf("failed to create Kubernetes client-set: nil Kubernetes client-set received"))
	}

	if err := dnsv1.AddToScheme(scheme.Scheme); err != nil {
		panic(fmt.Errorf("failed to register Kubernetes scheme: %w", err))
	}

	code := m.Run()

	if err := env.Stop(); err != nil {
		panic(fmt.Errorf("failed to stop Kubernetes manager: %w", err))
	}

	return code
}

func CreateManager(t *testing.T) (manager.Manager, *RecordReconciler, error) {
	t.Helper()

	mgrOptions := ctrl.Options{
		Logger:                 testing2.TestLogger{T: t},
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     fmt.Sprintf("127.0.0.1:%d", rand.IntnRange(1024*20, math.MaxInt16)),
		HealthProbeBindAddress: fmt.Sprintf("127.0.0.1:%d", rand.IntnRange(1024*20, math.MaxInt16)),
	}
	mgr, err := ctrl.NewManager(restConfig, mgrOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes manager: %w", err)
	} else if mgr == nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes manager: nil Kubernetes manager received")
	}

	mockCtrl := gomock.NewController(t)
	reconciler := &RecordReconciler{
		cloudflareClient: internal.NewMockCloudflareClient(mockCtrl),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return nil, nil, fmt.Errorf("failed to create Record reconciler: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		time.Sleep(5 * time.Second)
		cancel()
	})
	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("Failed starting Kubernetes manager: %v", err)
		}
	}()

	return mgr, reconciler, nil
}
