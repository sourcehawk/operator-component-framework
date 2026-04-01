package app

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// RESTMapperEntry describes a single GVK and its scope for REST mapper setup.
type RESTMapperEntry struct {
	GVK   schema.GroupVersionKind
	Scope meta.RESTScope
}

// NewFakeClient builds a fake client with a properly configured REST mapper.
// The scheme must already have all required types registered.
func NewFakeClient(scheme *runtime.Scheme, entries []RESTMapperEntry) client.WithWatch {
	gvs := make(map[schema.GroupVersion]struct{})
	for _, e := range entries {
		gvs[e.GVK.GroupVersion()] = struct{}{}
	}

	gvSlice := make([]schema.GroupVersion, 0, len(gvs))
	for gv := range gvs {
		gvSlice = append(gvSlice, gv)
	}

	mapper := meta.NewDefaultRESTMapper(gvSlice)
	for _, e := range entries {
		mapper.Add(e.GVK, e.Scope)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithStatusSubresource(&ExampleApp{}).
		Build()
}
