//go:build e2e

package component

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

// The orphan test proves the architectural payoff that unit tests cannot show:
// once OrphanWhen(true) removes the controller owner reference, the object is no
// longer garbage-collected with its former owner and survives the owner's
// deletion.
//
// It uses a namespace-scoped TestApp as the owner (the cluster-scoped
// ClusterTestApp the rest of this suite relies on cannot own a namespace-scoped
// ConfigMap, so the API server would never set the owner reference in the first
// place). The component is built directly and reconciled against a hand-rolled
// ReconcileContext, mirroring how the framework reconcilers construct it, so the
// recCtx.Owner is the namespace-scoped TestApp whose UID the orphan pass matches.
var _ = Describe("OrphanWhen Garbage Collection", func() {
	var (
		ns     string
		owner  *framework.TestApp
		recCtx component.ReconcileContext
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-orphan-")

		// A real namespace-scoped owner with a server-assigned UID. ConfigMaps in
		// the same namespace that carry its controller owner reference are eligible
		// for garbage collection when it is deleted.
		owner = framework.NewTestApp(ctx, k8sClient, ns, "orphan-owner")

		recCtx = component.ReconcileContext{
			Client:        k8sClient,
			Scheme:        scheme.Scheme,
			EventRecorder: events.NewFakeRecorder(100),
			Owner:         owner,
		}
	})

	AfterEach(func() {
		// Best-effort cleanup; the namespace teardown removes anything left behind.
		_ = k8sClient.Delete(ctx, &framework.TestApp{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-owner", Namespace: ns},
		})
	})

	It("should orphan a resource so it survives deletion of its owner", func() {
		// A control ConfigMap owned by the same owner but NOT orphaned. It proves
		// garbage collection is actually wired up in this cluster: when the owner is
		// deleted, the control object must disappear. Without it, a surviving orphan
		// could be a false positive (GC simply not running).
		control := newConfigMap(ns, "control-cm", map[string]string{"role": "control"})
		Expect(ctrl.SetControllerReference(owner, control, scheme.Scheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, control)).To(Succeed())

		// The orphan target: created already carrying the owner's controller owner
		// reference, exactly as the component would have set it while managing it.
		orphaned := newConfigMap(ns, "orphan-cm", map[string]string{"role": "orphan"})
		Expect(ctrl.SetControllerReference(owner, orphaned, scheme.Scheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, orphaned)).To(Succeed())

		By("verifying both ConfigMaps start with the owner reference")
		// k8sClient is the manager's cached client, so an object that was just created
		// can still be absent from the informer cache. Poll both reads rather than
		// asserting once, or a cold cache fails the spec before it tests anything.
		Eventually(func(g Gomega) []types.UID {
			return ownerReferenceUIDsG(g, ctx, ns, "control-cm")
		}, framework.DefaultTimeout, framework.DefaultPolling).
			Should(ContainElement(owner.UID))
		Eventually(func(g Gomega) []types.UID {
			return ownerReferenceUIDsG(g, ctx, ns, "orphan-cm")
		}, framework.DefaultTimeout, framework.DefaultPolling).
			Should(ContainElement(owner.UID))

		By("reconciling a component that orphans the target ConfigMap")
		orphanRes, err := configmap.NewBuilder(
			newConfigMap(ns, "orphan-cm", map[string]string{"role": "orphan"}),
		).Build()
		Expect(err).NotTo(HaveOccurred())

		comp, err := component.NewComponentBuilder().
			WithName("orphan-comp").
			WithConditionType("E2EReady").
			WithResource(orphanRes, component.OrphanWhen(true)).
			Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())

		By("verifying the orphan pass removed the owner reference")
		Eventually(func(g Gomega) []types.UID {
			return ownerReferenceUIDsG(g, ctx, ns, "orphan-cm")
		}, framework.DefaultTimeout, framework.DefaultPolling).
			ShouldNot(ContainElement(owner.UID))

		By("verifying the control ConfigMap still carries the owner reference")
		Expect(ownerReferenceUIDs(ctx, ns, "control-cm")).To(ContainElement(owner.UID))

		By("deleting the owner")
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())

		By("waiting for garbage collection to remove the still-owned control ConfigMap")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "control-cm", Namespace: ns}, &corev1.ConfigMap{})
		}, framework.DefaultTimeout, framework.DefaultPolling).
			ShouldNot(Succeed())

		By("verifying the orphaned ConfigMap survives garbage collection")
		Consistently(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-cm", Namespace: ns}, &corev1.ConfigMap{})
		}, "15s", framework.DefaultPolling).
			Should(Succeed())

		var survivor corev1.ConfigMap
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-cm", Namespace: ns}, &survivor)).To(Succeed())
		Expect(survivor.OwnerReferences).To(BeEmpty(), "surviving orphan must carry no owner reference")
		Expect(survivor.Data).To(HaveKeyWithValue("role", "orphan"))
	})
})

// ownerReferenceUIDs fetches a ConfigMap by name and returns the UIDs of its
// owner references. It asserts the fetch succeeds.
func ownerReferenceUIDs(c context.Context, namespace, name string) []types.UID {
	var cm corev1.ConfigMap
	ExpectWithOffset(1, k8sClient.Get(c, types.NamespacedName{Name: name, Namespace: namespace}, &cm)).To(Succeed())
	return uidsOf(cm.OwnerReferences)
}

// ownerReferenceUIDsG is the Eventually-friendly variant: it routes the fetch
// assertion through the supplied Gomega so a transient NotFound retries rather
// than failing the spec.
func ownerReferenceUIDsG(g Gomega, c context.Context, namespace, name string) []types.UID {
	var cm corev1.ConfigMap
	g.Expect(k8sClient.Get(c, types.NamespacedName{Name: name, Namespace: namespace}, &cm)).To(Succeed())
	return uidsOf(cm.OwnerReferences)
}

func uidsOf(refs []metav1.OwnerReference) []types.UID {
	uids := make([]types.UID, 0, len(refs))
	for _, ref := range refs {
		uids = append(uids, ref.UID)
	}
	return uids
}
