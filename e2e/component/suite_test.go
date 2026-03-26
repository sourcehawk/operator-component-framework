//go:build e2e

package component

import (
	"context"
	"testing"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

var (
	ctx               context.Context
	cancel            context.CancelFunc
	k8sClient         client.Client
	clusterReconciler *framework.ClusterE2EReconciler
)

func TestE2EComponent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Component Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("getting cluster configuration")
	cfg := ctrl.GetConfigOrDie()

	By("installing CRDs")
	Expect(framework.InstallCRDs(cfg)).To(Succeed())

	By("setting up scheme")
	Expect(framework.AddToScheme(scheme.Scheme)).To(Succeed())

	By("creating manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("creating E2E reconciler")
	recorder := record.NewFakeRecorder(1000)
	metrics := &ocm.ConditionMetricRecorder{
		Controller:              "e2e-component",
		OperatorConditionsGauge: ocm.NewOperatorConditionsGauge("e2e_component"),
	}
	clusterReconciler = framework.NewClusterE2EReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		recorder,
		metrics,
	)

	By("registering cluster-scoped controller")
	err = ctrl.NewControllerManagedBy(mgr).
		For(&framework.ClusterTestApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(clusterReconciler)
	Expect(err).NotTo(HaveOccurred())

	By("starting manager")
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	By("waiting for cache sync")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	k8sClient = mgr.GetClient()
})

var _ = AfterSuite(func() {
	if cancel != nil {
		By("stopping manager")
		cancel()
	}
})
