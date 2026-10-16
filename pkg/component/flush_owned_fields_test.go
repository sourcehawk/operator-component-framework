package component

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These specs run against envtest because they depend on what the API server
// does with a merge patch: null removes a field, a resourceVersion in the body
// is a precondition, and a structural schema prunes unknown fields.
var _ = Describe("FlushStatus with WithOwnedStatusFields", func() {
	var (
		ctx       = context.Background()
		namespace string
	)

	infraComponent := func() *Component {
		GinkgoHelper()
		comp, err := NewComponentBuilder().WithName("infra").WithConditionType("InfraReady").Build()
		Expect(err).NotTo(HaveOccurred())
		return comp
	}

	// readThenRace creates the owner with the given status and reads it as a
	// controller does at the top of a reconcile. It then applies the second
	// writer's change on the server, so the returned owner is stale and its
	// first write conflicts.
	readThenRace := func(status MockStatus, secondWriter func(*MockOperatorCRD)) *MockOperatorCRD {
		GinkgoHelper()
		created := &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: namespace}}
		Expect(k8sClient.Create(ctx, created)).To(Succeed())
		created.Status = status
		Expect(k8sClient.Status().Update(ctx, created)).To(Succeed())

		owner := &MockOperatorCRD{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(created), owner)).To(Succeed())

		if secondWriter != nil {
			other := &MockOperatorCRD{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(created), other)).To(Succeed())
			secondWriter(other)
			Expect(k8sClient.Status().Update(ctx, other)).To(Succeed())
		}
		return owner
	}

	persisted := func(owner *MockOperatorCRD) *MockOperatorCRD {
		GinkgoHelper()
		live := &MockOperatorCRD{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), live)).To(Succeed())
		return live
	}

	condition := func(ctype, reason string) metav1.Condition {
		return metav1.Condition{
			Type: ctype, Status: metav1.ConditionTrue, Reason: reason, LastTransitionTime: metav1.Now(),
		}
	}

	BeforeEach(func() {
		namespace = createNamespace(ctx, "flush-owned-fields-")
	})

	It("keeps a second writer's observedGeneration across the conflict retry", func() {
		owner := readThenRace(MockStatus{ObservedGeneration: 1}, func(other *MockOperatorCRD) {
			other.Status.ObservedGeneration = 2
		})
		recording := &patchRecordingClient{Client: k8sClient}

		owner.Status.Phase = "Accepted"
		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: recording, Owner: owner}, []*Component{infraComponent()},
			WithOwnedStatusFields("phase"),
		)).To(Succeed())

		Expect(recording.bodies).To(HaveLen(2), "the first patch must conflict and the retry must succeed")
		live := persisted(owner)
		Expect(live.Status.ObservedGeneration).To(Equal(int64(2)))
		Expect(live.Status.Phase).To(Equal("Accepted"))
		Expect(meta.FindStatusCondition(live.Status.Conditions, "InfraReady")).NotTo(BeNil())
	})

	It("sends only the declared fields and the conditions, even without a conflict", func() {
		owner := readThenRace(MockStatus{ObservedGeneration: 1}, nil)
		recording := &patchRecordingClient{Client: k8sClient}

		owner.Status.ObservedGeneration = 99 // staged but not declared
		owner.Status.Phase = "Accepted"
		readVersion := owner.GetResourceVersion()

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: recording, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		)).To(Succeed())

		Expect(recording.bodies).To(HaveLen(1))
		var body map[string]any
		Expect(json.Unmarshal(recording.bodies[0], &body)).To(Succeed())
		Expect(body).To(HaveKey("metadata"))
		Expect(body["metadata"]).To(Equal(map[string]any{"resourceVersion": readVersion}))
		Expect(body["status"]).To(HaveKeyWithValue("phase", "Accepted"))
		Expect(body["status"]).NotTo(HaveKey("observedGeneration"))

		live := persisted(owner)
		Expect(live.Status.ObservedGeneration).To(Equal(int64(1)), "an undeclared field is never written")
		Expect(live.Status.Phase).To(Equal("Accepted"))
	})

	It("leaves the server's state in memory, not undeclared staged values", func() {
		owner := readThenRace(MockStatus{}, nil)

		owner.Status.ObservedGeneration = 99 // undeclared, and absent on the server
		owner.Status.Summary = &MockStatusSummary{Ready: 5}
		owner.Status.Phase = "Accepted"

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		)).To(Succeed())

		Expect(owner.Status.ObservedGeneration).To(BeZero())
		Expect(owner.Status.Summary).To(BeNil())
		Expect(owner.Status.Phase).To(Equal("Accepted"))
	})

	It("removes a child that the staged owner dropped from a wholly declared object", func() {
		owner := readThenRace(MockStatus{Summary: &MockStatusSummary{Ready: 1, Total: 3}}, nil)

		owner.Status.Summary = &MockStatusSummary{Ready: 2} // total cleared, so omitempty drops it

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary"),
		)).To(Succeed())

		Expect(persisted(owner).Status.Summary).To(Equal(&MockStatusSummary{Ready: 2}))
	})

	It("removes a dropped child against the server's version after a conflict", func() {
		owner := readThenRace(MockStatus{Summary: &MockStatusSummary{Ready: 1}}, nil)
		// The competing write lands after FlushStatus read the server's copy
		// and before its first patch, so that patch conflicts and the retry
		// must compute the removals against the newer copy.
		recording := &patchRecordingClient{Client: k8sClient, beforeFirstPatch: func() {
			other := &MockOperatorCRD{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), other)).To(Succeed())
			other.Status.Summary.Total = 4
			Expect(k8sClient.Status().Update(ctx, other)).To(Succeed())
		}}

		owner.Status.Summary = &MockStatusSummary{Ready: 2}

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: recording, Owner: owner}, nil, WithOwnedStatusFields("summary"),
		)).To(Succeed())

		Expect(recording.bodies).To(HaveLen(2), "the first patch must conflict and the retry must succeed")
		Expect(persisted(owner).Status.Summary).To(Equal(&MockStatusSummary{Ready: 2}),
			"the declared object is the staged one, so the other writer's total is removed")
	})

	It("keeps a staged unowned condition when a declared object needs the server's copy", func() {
		owner := readThenRace(MockStatus{Summary: &MockStatusSummary{Ready: 1}}, nil)

		owner.Status.Summary = &MockStatusSummary{Ready: 2}
		// An owner-level aggregate, owned by no component and not yet on the server.
		meta.SetStatusCondition(&owner.Status.Conditions, metav1.Condition{
			Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllReady",
		})

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary"),
		)).To(Succeed())

		Expect(meta.FindStatusCondition(persisted(owner).Status.Conditions, "Ready")).NotTo(BeNil(),
			"without a conflict the staged conditions are written as staged")
	})

	It("removes a declared field that the staged owner cleared", func() {
		owner := readThenRace(MockStatus{Phase: "Pending", ObservedGeneration: 1}, func(other *MockOperatorCRD) {
			other.Status.ObservedGeneration = 2
		})

		owner.Status.Phase = ""

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		)).To(Succeed())

		live := persisted(owner)
		Expect(live.Status.Phase).To(BeEmpty())
		Expect(live.Status.ObservedGeneration).To(Equal(int64(2)))
	})

	It("writes a nested declared field and leaves its siblings to the server", func() {
		owner := readThenRace(
			MockStatus{Summary: &MockStatusSummary{Ready: 1, Total: 3}},
			func(other *MockOperatorCRD) { other.Status.Summary.Total = 4 },
		)

		owner.Status.Summary.Ready = 2

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary.ready"),
		)).To(Succeed())

		Expect(persisted(owner).Status.Summary).To(Equal(&MockStatusSummary{Ready: 2, Total: 4}))
	})

	It("removes an explicitly stored zero child of a wholly declared object", func() {
		owner := readThenRace(MockStatus{Summary: &MockStatusSummary{Ready: 1}}, nil)
		// A typed write drops total: 0 through omitempty, so store it raw.
		Expect(k8sClient.Status().Patch(ctx, owner, client.RawPatch(
			types.MergePatchType, []byte(`{"status":{"summary":{"ready":1,"total":0}}}`),
		))).To(Succeed())
		Expect(rawStatus(ctx, owner)["summary"]).To(HaveKey("total"))

		owner.Status.Summary = &MockStatusSummary{Ready: 2}

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary"),
		)).To(Succeed())

		Expect(rawStatus(ctx, owner)["summary"]).To(Equal(map[string]any{"ready": int64(2)}))
	})

	It("leaves an absent parent absent when a declared child has nothing to remove", func() {
		owner := readThenRace(MockStatus{Phase: "Pending"}, nil)

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("summary.ready"),
		)).To(Succeed())

		Expect(rawStatus(ctx, owner)).NotTo(HaveKey("summary"))
	})

	It("does not write into a replacement created under the same name", func() {
		owner := readThenRace(MockStatus{Phase: "Pending"}, nil)

		Expect(k8sClient.Delete(ctx, owner.DeepCopy())).To(Succeed())
		replacement := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: owner.Name, Namespace: owner.Namespace},
		}
		Expect(k8sClient.Create(ctx, replacement)).To(Succeed())
		Expect(replacement.UID).NotTo(Equal(owner.UID))

		owner.Status.Phase = "Accepted"

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phase"),
		)).To(MatchError(ContainSubstring("was replaced")))

		Expect(persisted(owner).Status.Phase).To(BeEmpty(), "the replacement must stay as it was created")
	})

	It("lets owned conditions win and unowned conditions follow the server on a retry", func() {
		owner := readThenRace(
			MockStatus{Conditions: []metav1.Condition{
				condition("InfraReady", "StaleServerValue"),
				condition("ExternalReady", "BeforeOtherWrite"),
			}},
			func(other *MockOperatorCRD) {
				meta.SetStatusCondition(&other.Status.Conditions, metav1.Condition{
					Type: "ExternalReady", Status: metav1.ConditionFalse, Reason: "WrittenByOtherController",
				})
				meta.SetStatusCondition(&other.Status.Conditions, condition("AddedByOther", "BrandNew"))
			},
		)

		applyStatusCondition(ReconcileContext{Owner: owner}, Condition{
			Type: "InfraReady", Status: metav1.ConditionTrue, Reason: "Healthy",
		})

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, []*Component{infraComponent()},
			WithOwnedStatusFields(),
		)).To(Succeed())

		live := persisted(owner)
		Expect(meta.FindStatusCondition(live.Status.Conditions, "InfraReady").Reason).To(Equal("Healthy"))
		Expect(meta.FindStatusCondition(live.Status.Conditions, "ExternalReady").Reason).
			To(Equal("WrittenByOtherController"))
		Expect(meta.FindStatusCondition(live.Status.Conditions, "AddedByOther")).NotTo(BeNil())
	})

	It("does nothing for a misspelled path, because the schema prunes the unknown field", func() {
		owner := readThenRace(MockStatus{Phase: "Pending"}, nil)

		owner.Status.Phase = "Accepted"

		Expect(FlushStatus(
			ctx, ReconcileContext{Client: k8sClient, Owner: owner}, nil, WithOwnedStatusFields("phsae"),
		)).To(Succeed())

		Expect(persisted(owner).Status.Phase).To(Equal("Pending"), "the declared path names no field, so nothing is written")
	})

	It("fails without writing for a declared path below a scalar the staged owner holds", func() {
		owner := readThenRace(MockStatus{Phase: "Pending"}, nil)
		recording := &patchRecordingClient{Client: k8sClient}

		err := FlushStatus(
			ctx, ReconcileContext{Client: recording, Owner: owner}, nil, WithOwnedStatusFields("phase.value"),
		)

		Expect(err).To(MatchError(ContainSubstring(`"phase.value"`)))
		Expect(recording.bodies).To(BeEmpty())
		Expect(persisted(owner).Status.Phase).To(Equal("Pending"))
	})

	DescribeTable("rejects invalid path syntax before any write",
		func(path string) {
			owner := readThenRace(MockStatus{}, nil)
			recording := &patchRecordingClient{Client: k8sClient}

			err := FlushStatus(
				ctx, ReconcileContext{Client: recording, Owner: owner}, nil, WithOwnedStatusFields(path),
			)

			Expect(err).To(MatchError(ContainSubstring(`"` + path + `"`)))
			Expect(recording.bodies).To(BeEmpty())
		},
		Entry("empty", ""),
		Entry("empty segment", "summary..ready"),
		Entry("leading separator", ".phase"),
		Entry("trailing separator", "phase."),
		Entry("the conditions", "conditions"),
		Entry("below the conditions", "conditions.type"),
	)
})

func TestConditionsJSONPath(t *testing.T) {
	t.Run("finds the conditions directly under status", func(t *testing.T) {
		path, err := conditionsJSONPath(&MockOperatorCRD{})

		require.NoError(t, err)
		assert.Equal(t, []string{"conditions"}, path)
	})

	t.Run("finds conditions nested below status under any JSON name", func(t *testing.T) {
		path, err := conditionsJSONPath(&nestedConditionsOwner{})

		require.NoError(t, err)
		assert.Equal(t, []string{"health", "checks"}, path)
	})

	t.Run("leaves the staged conditions as they were", func(t *testing.T) {
		staged := []metav1.Condition{{Type: "InfraReady", Reason: "Healthy"}}
		owner := &MockOperatorCRD{Status: MockStatus{Conditions: staged}}

		_, err := conditionsJSONPath(owner)

		require.NoError(t, err)
		assert.Equal(t, staged, owner.Status.Conditions)
	})
}

func TestParseOwnedStatusPaths(t *testing.T) {
	conditions := []string{"health", "checks"}

	t.Run("splits a valid path into its segments", func(t *testing.T) {
		paths, err := parseOwnedStatusPaths([]string{"phase", "health.score"}, conditions)

		require.NoError(t, err)
		assert.Equal(t, [][]string{{"phase"}, {"health", "score"}}, paths)
	})

	for name, path := range map[string]string{
		"the conditions":            "health.checks",
		"a path that contains them": "health",
		"a path below them":         "health.checks.type",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := parseOwnedStatusPaths([]string{path}, conditions)

			require.Error(t, err)
			assert.Contains(t, err.Error(), `"`+path+`"`)
		})
	}
}

// rawStatus reads the owner's status as the server stores it, where a typed
// read cannot tell a stored zero from an absent field.
func rawStatus(ctx context.Context, owner *MockOperatorCRD) map[string]any {
	GinkgoHelper()
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(GroupVersion.WithKind("MockOperatorCRD"))
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(owner), live)).To(Succeed())
	status, _ := live.Object["status"].(map[string]any)
	return status
}

func TestOwnedStatusPatch(t *testing.T) {
	owner := &MockOperatorCRD{Status: MockStatus{Summary: &MockStatusSummary{Ready: 2}}}
	conditionsPath := []string{"conditions"}

	statusOf := func(t *testing.T, body []byte) map[string]any {
		t.Helper()
		var patch map[string]any
		require.NoError(t, json.Unmarshal(body, &patch))
		status, _ := patch["status"].(map[string]any)
		return status
	}

	t.Run("does not depend on the order of overlapping declarations", func(t *testing.T) {
		server := map[string]any{"summary": map[string]any{"ready": int64(1)}}
		forward, err := parseOwnedStatusPaths([]string{"summary", "summary.details.ready"}, conditionsPath)
		require.NoError(t, err)
		backward, err := parseOwnedStatusPaths([]string{"summary.details.ready", "summary"}, conditionsPath)
		require.NoError(t, err)

		forwardBody, err := ownedStatusPatch(owner, server, forward, conditionsPath)
		require.NoError(t, err)
		backwardBody, err := ownedStatusPatch(owner, server, backward, conditionsPath)
		require.NoError(t, err)

		assert.Equal(t, statusOf(t, forwardBody), statusOf(t, backwardBody))
		assert.Equal(t, map[string]any{"ready": float64(2)}, statusOf(t, forwardBody)["summary"])
	})

	for name, server := range map[string]map[string]any{
		"absent": {},
		"null":   {"summary": nil},
	} {
		t.Run("sends nothing to remove below a parent the server holds as "+name, func(t *testing.T) {
			stagedWithout := &MockOperatorCRD{}
			paths, err := parseOwnedStatusPaths([]string{"summary.ready"}, conditionsPath)
			require.NoError(t, err)

			body, err := ownedStatusPatch(stagedWithout, server, paths, conditionsPath)
			require.NoError(t, err)

			assert.NotContains(t, statusOf(t, body), "summary")
		})
	}
}

// patchRecordingClient records the body of every status patch it forwards.
type patchRecordingClient struct {
	client.Client
	bodies [][]byte
	// beforeFirstPatch, when set, runs once before the first patch is forwarded.
	beforeFirstPatch func()
}

func (c *patchRecordingClient) Status() client.SubResourceWriter {
	return &patchRecordingStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type patchRecordingStatusWriter struct {
	client.SubResourceWriter
	parent *patchRecordingClient
}

func (w *patchRecordingStatusWriter) Patch(
	ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption,
) error {
	body, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if w.parent.beforeFirstPatch != nil && len(w.parent.bodies) == 0 {
		w.parent.beforeFirstPatch()
	}
	w.parent.bodies = append(w.parent.bodies, body)
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}

// nestedConditionsOwner keeps its conditions below status.health.checks, so
// the conditions JSON path has more than one segment.
type nestedConditionsOwner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status nestedConditionsStatus `json:"status,omitempty"`
}

type nestedConditionsStatus struct {
	Phase  string       `json:"phase,omitempty"`
	Health healthStatus `json:"health,omitzero"`
}

type healthStatus struct {
	Checks []metav1.Condition `json:"checks,omitempty"`
}

func (o *nestedConditionsOwner) GetStatusConditions() *[]metav1.Condition {
	return &o.Status.Health.Checks
}

func (o *nestedConditionsOwner) GetKind() string {
	return "nestedConditionsOwner"
}

func (o *nestedConditionsOwner) DeepCopyObject() runtime.Object {
	out := *o
	o.ObjectMeta.DeepCopyInto(&out.ObjectMeta) //nolint:staticcheck // naming ObjectMeta keeps the copy readable
	out.Status.Health.Checks = append([]metav1.Condition(nil), o.Status.Health.Checks...)
	return &out
}
