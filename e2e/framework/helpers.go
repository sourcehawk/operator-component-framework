package framework

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
)

// DefaultTimeout is the default timeout for Eventually assertions in E2E tests.
const DefaultTimeout = 120 * time.Second

// DefaultPolling is the default polling interval for Eventually assertions.
const DefaultPolling = 2 * time.Second

// CreateTestNamespace creates a namespace with GenerateName and returns its name.
func CreateTestNamespace(ctx context.Context, c client.Client, prefix string) string {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix,
		},
	}
	gomega.ExpectWithOffset(1, c.Create(ctx, ns)).To(gomega.Succeed())
	gomega.ExpectWithOffset(1, ns.Name).NotTo(gomega.BeEmpty())
	return ns.Name
}

// NewTestApp creates a TestApp CR in the given namespace and returns it.
func NewTestApp(ctx context.Context, c client.Client, namespace, name string) *TestApp {
	app := &TestApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	app.SetGroupVersionKind(GroupVersion.WithKind("TestApp"))
	gomega.ExpectWithOffset(1, c.Create(ctx, app)).To(gomega.Succeed())
	return app
}

// NewClusterTestApp creates a cluster-scoped ClusterTestApp CR and returns it.
func NewClusterTestApp(ctx context.Context, c client.Client, name string) *ClusterTestApp {
	app := &ClusterTestApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	app.SetGroupVersionKind(GroupVersion.WithKind("ClusterTestApp"))
	gomega.ExpectWithOffset(1, c.Create(ctx, app)).To(gomega.Succeed())
	return app
}

// DeleteClusterTestApp deletes the named ClusterTestApp from the cluster.
// It is intended for AfterEach cleanup since cluster-scoped resources are not
// garbage-collected by namespace deletion.
func DeleteClusterTestApp(ctx context.Context, c client.Client, name string) {
	app := &ClusterTestApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	gomega.ExpectWithOffset(1, client.IgnoreNotFound(c.Delete(ctx, app))).To(gomega.Succeed())
}

// GetClusterCondition returns a closure that fetches a fresh ClusterTestApp from the
// cluster and returns the named condition. Suitable for use with gomega.Eventually.
func GetClusterCondition(
	ctx context.Context, c client.Client,
	name string, conditionType string,
) func(gomega.Gomega) *metav1.Condition {
	return func(g gomega.Gomega) *metav1.Condition {
		app := &ClusterTestApp{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: name}, app)).To(gomega.Succeed())
		return meta.FindStatusCondition(app.Status.Conditions, conditionType)
	}
}

// InstallCRDs applies both the TestApp and ClusterTestApp CRD definitions and
// waits until they are established.
func InstallCRDs(cfg *rest.Config) error {
	cs, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating apiextensions client: %w", err)
	}

	for _, crd := range []*apiextensionsv1.CustomResourceDefinition{TestAppCRD, ClusterTestAppCRD} {
		if err := installCRD(cs, crd); err != nil {
			return err
		}
	}
	return nil
}

// InstallCRD applies the TestApp CRD definition and waits until it is established.
func InstallCRD(cfg *rest.Config) error {
	cs, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating apiextensions client: %w", err)
	}
	return installCRD(cs, TestAppCRD)
}

func installCRD(cs apiextensionsclient.Interface, crd *apiextensionsv1.CustomResourceDefinition) error {
	ctx := context.Background()
	existing, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(
		ctx, crd.Name, metav1.GetOptions{},
	)
	switch {
	case apierrors.IsNotFound(err):
		_, err = cs.ApiextensionsV1().CustomResourceDefinitions().Create(
			ctx, crd, metav1.CreateOptions{},
		)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating CRD %s: %w", crd.Name, err)
		}
	case err != nil:
		return fmt.Errorf("getting CRD %s: %w", crd.Name, err)
	default:
		crd.ResourceVersion = existing.ResourceVersion
		_, err = cs.ApiextensionsV1().CustomResourceDefinitions().Update(
			ctx, crd, metav1.UpdateOptions{},
		)
		if err != nil {
			return fmt.Errorf("updating CRD %s: %w", crd.Name, err)
		}
	}

	// Wait for the CRD to be established.
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			fetched, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(
				ctx, crd.Name, metav1.GetOptions{},
			)
			if err != nil {
				return false, err
			}
			for _, c := range fetched.Status.Conditions {
				if c.Type == apiextensionsv1.Established && c.Status == apiextensionsv1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		},
	)
}

// GetCondition returns a closure that fetches a fresh TestApp from the cluster
// and returns the named condition. Suitable for use with gomega.Eventually.
//
// Usage:
//
//	Eventually(framework.GetCondition(ctx, c, key, "E2EReady")).
//	    Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
func GetCondition(
	ctx context.Context, c client.Client,
	key types.NamespacedName, conditionType string,
) func(gomega.Gomega) *metav1.Condition {
	return func(g gomega.Gomega) *metav1.Condition {
		app := &TestApp{}
		g.Expect(c.Get(ctx, key, app)).To(gomega.Succeed())
		return meta.FindStatusCondition(app.Status.Conditions, conditionType)
	}
}

// HaveConditionStatus returns a GomegaMatcher that checks a *metav1.Condition
// has the expected Status and Reason fields.
func HaveConditionStatus(status metav1.ConditionStatus, reason string) gomegatypes.GomegaMatcher {
	return &conditionStatusMatcher{
		expectedStatus: status,
		expectedReason: reason,
	}
}

type conditionStatusMatcher struct {
	expectedStatus metav1.ConditionStatus
	expectedReason string
}

func (m *conditionStatusMatcher) Match(actual interface{}) (bool, error) {
	cond, ok := actual.(*metav1.Condition)
	if !ok {
		return false, fmt.Errorf("HaveConditionStatus expects a *metav1.Condition, got %T", actual)
	}
	if cond == nil {
		return false, nil
	}
	return cond.Status == m.expectedStatus && cond.Reason == m.expectedReason, nil
}

func (m *conditionStatusMatcher) FailureMessage(actual interface{}) string {
	cond, ok := actual.(*metav1.Condition)
	if !ok || cond == nil {
		return fmt.Sprintf("expected condition with Status=%q Reason=%q, but got %+v",
			m.expectedStatus, m.expectedReason, actual)
	}
	return fmt.Sprintf("expected condition Status=%q Reason=%q, but got Status=%q Reason=%q Message=%q",
		m.expectedStatus, m.expectedReason, cond.Status, cond.Reason, cond.Message)
}

func (m *conditionStatusMatcher) NegatedFailureMessage(_ interface{}) string {
	return fmt.Sprintf("expected condition NOT to have Status=%q Reason=%q, but it did",
		m.expectedStatus, m.expectedReason)
}
