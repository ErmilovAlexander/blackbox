package collector

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
	storepkg "github.com/ErmilovAlexander/blackbox/internal/store"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
)

type captureStore struct {
	records chan model.Record
}

func newCaptureStore() *captureStore {
	return &captureStore{records: make(chan model.Record, 16)}
}

func (s *captureStore) Append(ctx context.Context, record model.Record) error {
	select {
	case s.records <- record:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *captureStore) Query(context.Context, storepkg.Query) ([]model.Record, error) {
	return nil, nil
}

func (s *captureStore) Close() error { return nil }

func TestInformerDistinguishesSnapshotFromNewObject(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	initial := pod("initial", "uid-initial")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "PodList"},
		initial,
	)
	store := newCaptureStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector := New(client, store, "test", false, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	informer := collector.newInformer(ctx, watchedResource{GVR: gvr, Kind: "Pod"})
	go informer.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		t.Fatal("informer cache did not sync")
	}

	first := receiveRecord(t, store.records)
	if first.Name != "initial" || first.Action != model.ActionSnapshot {
		t.Fatalf("initial record = %s/%s, want initial/SNAPSHOT", first.Name, first.Action)
	}

	if _, err := client.Resource(gvr).Namespace("demo").Create(ctx, pod("created", "uid-created"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	second := receiveRecord(t, store.records)
	if second.Name != "created" || second.Action != model.ActionAdd {
		t.Fatalf("new record = %s/%s, want created/ADD", second.Name, second.Action)
	}
}

func pod(name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":            name,
			"namespace":       "demo",
			"uid":             uid,
			"resourceVersion": "1",
		},
	}}
}

func receiveRecord(t *testing.T, records <-chan model.Record) model.Record {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a record")
		return model.Record{}
	}
}

var _ storepkg.Store = (*captureStore)(nil)
