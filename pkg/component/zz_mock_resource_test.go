package component

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockResource struct {
	mock.Mock
}

func (m *MockResource) Mutate(current client.Object) error {
	args := m.Called(current)
	return args.Error(0)
}

func (m *MockResource) Object() (client.Object, error) {
	args := m.Called()
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(client.Object), args.Error(1)
}

func (m *MockResource) Identity() string {
	args := m.Called()
	return args.String(0)
}

type MockAliveResource struct {
	MockResource
}

func (m *MockAliveResource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.AliveStatusWithReason, error) {
	args := m.Called(op)
	return args.Get(0).(concepts.AliveStatusWithReason), args.Error(1)
}

func (m *MockAliveResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	args := m.Called()
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

type MockCompletableResource struct {
	MockResource
}

func (m *MockCompletableResource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.CompletionStatusWithReason, error) {
	args := m.Called(op)
	return args.Get(0).(concepts.CompletionStatusWithReason), args.Error(1)
}

func (m *MockCompletableResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	args := m.Called()
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

type MockOperationalResource struct {
	MockResource
}

func (m *MockOperationalResource) ConvergingStatus(op concepts.ConvergingOperation) (concepts.OperationalStatusWithReason, error) {
	args := m.Called(op)
	return args.Get(0).(concepts.OperationalStatusWithReason), args.Error(1)
}

func (m *MockOperationalResource) GraceStatus() (concepts.GraceStatusWithReason, error) {
	args := m.Called()
	return args.Get(0).(concepts.GraceStatusWithReason), args.Error(1)
}

// MockClient is a mock of the controller-runtime client.
type MockClient struct {
	mock.Mock
	client.Client
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

type MockSuspendableResource struct {
	MockResource
}

func (m *MockSuspendableResource) DeleteOnSuspend() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockSuspendableResource) Suspend() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockSuspendableResource) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	args := m.Called()
	return args.Get(0).(concepts.SuspensionStatusWithReason), args.Error(1)
}

type MockExtractableResource struct {
	MockResource
}

func (m *MockExtractableResource) ExtractData() error {
	args := m.Called()
	return args.Error(0)
}
