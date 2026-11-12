package component

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// betweenReadAndApplyClient runs between once, right after the next read of a
// Deployment returns. That is the window between the read of the component and
// its apply, where a write by another controller lands.
type betweenReadAndApplyClient struct {
	client.Client
	between func(ctx context.Context) error
}

func (c *betweenReadAndApplyClient) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*appsv1.Deployment); !ok || c.between == nil {
		return nil
	}
	write := c.between
	c.between = nil
	return write(ctx)
}

// The Updated<Kind> event of a component reports a change to the fields its
// field manager owns. A write by another controller that lands between the read
// and the apply of the component is not one
// (sourcehawk/operator-component-framework#248).
var _ = Describe("Apply classification with a concurrent writer", func() {
	var (
		ctx       = context.Background()
		namespace string
		owner     *MockOperatorCRD
	)

	const deploymentName = "web"

	buildDeployment := func(image string, port int32) func() client.Object {
		return func() client.Object {
			labels := map[string]string{"app": deploymentName}
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](1),
					Selector: &metav1.LabelSelector{MatchLabels: labels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "app",
								Image: image,
								Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: port}},
							}},
						},
					},
				},
			}
		}
	}

	addFinalizer := func(ctx context.Context) error {
		deployment := &appsv1.Deployment{}
		key := client.ObjectKey{Name: deploymentName, Namespace: namespace}
		if err := k8sClient.Get(ctx, key, deployment); err != nil {
			return err
		}
		deployment.Finalizers = append(deployment.Finalizers, "other.example.com/cleanup")
		return k8sClient.Update(ctx, deployment, client.FieldOwner("other-controller"))
	}

	BeforeEach(func() {
		namespace = createNamespace(ctx, "concurrent-writer-test-")
		owner = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: namespace}}
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
	})

	It("records no Updated event for a finalizer another controller adds, and one for each owned change", func() {
		res := &objectOperationRecordingResource{build: buildDeployment("app:1", 8080)}
		comp := &Component{
			name:               "web",
			conditionType:      "WebReady",
			reconcileResources: []reconcileEntry{{Resource: res}},
		}
		racing := &betweenReadAndApplyClient{Client: k8sClient}
		rec := newTestReconcileContext(owner)
		rec.Client = racing

		Expect(comp.Reconcile(ctx, rec)).To(Succeed())

		racing.between = addFinalizer
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(racing.between).To(BeNil(), "the concurrent write must have run")

		res.build = buildDeployment("app:2", 8080)
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())

		res.build = buildDeployment("app:2", 9090)
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())

		Expect(res.operations).To(Equal([]concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationNone,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}))
		Expect(rec.EventRecorder.(*spyRecorder).recordedWithReason("UpdatedDeployment")).To(HaveLen(2))

		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, live)).To(Succeed())
		Expect(live.Finalizers).To(ContainElement("other.example.com/cleanup"))
	})
})
