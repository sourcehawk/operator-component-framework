//go:build e2e

package primitives

import (
	"context"
	"testing"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	reconciler        *framework.E2EReconciler
	clusterReconciler *framework.ClusterE2EReconciler
)

func TestE2EPrimitives(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Primitives Suite")
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

	By("creating E2E reconcilers")
	recorder := record.NewFakeRecorder(1000)
	metrics := &ocm.ConditionMetricRecorder{
		Controller:              "e2e-primitives",
		OperatorConditionsGauge: ocm.NewOperatorConditionsGauge("e2e_primitives"),
	}
	reconciler = framework.NewE2EReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		recorder,
		metrics,
	)
	clusterReconciler = framework.NewClusterE2EReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		recorder,
		metrics,
	)

	By("registering namespace-scoped controller")
	err = ctrl.NewControllerManagedBy(mgr).
		For(&framework.TestApp{}).
		// apps/v1
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.ReplicaSet{}).
		Owns(&appsv1.StatefulSet{}).
		// autoscaling/v2
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		// batch/v1
		Owns(&batchv1.CronJob{}).
		Owns(&batchv1.Job{}).
		// core/v1
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		// networking/v1
		Owns(&networkingv1.Ingress{}).
		Owns(&networkingv1.NetworkPolicy{}).
		// policy/v1
		Owns(&policyv1.PodDisruptionBudget{}).
		// rbac/v1
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(reconciler)
	Expect(err).NotTo(HaveOccurred())

	By("registering cluster-scoped controller")
	err = ctrl.NewControllerManagedBy(mgr).
		For(&framework.ClusterTestApp{}).
		Owns(&corev1.PersistentVolume{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
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
