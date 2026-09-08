package collector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
	"github.com/ErmilovAlexander/blackbox/internal/redact"
	storepkg "github.com/ErmilovAlexander/blackbox/internal/store"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

type Collector struct {
	client            dynamic.Interface
	store             storepkg.Store
	cluster           string
	includeConfigMaps bool
	log               *slog.Logger
}

func New(client dynamic.Interface, store storepkg.Store, cluster string, includeConfigMaps bool, log *slog.Logger) *Collector {
	return &Collector{client: client, store: store, cluster: cluster, includeConfigMaps: includeConfigMaps, log: log}
}

type watchedResource struct {
	GVR  schema.GroupVersionResource
	Kind string
}

var defaultResources = []watchedResource{
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, "Pod"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, "Service"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, "ConfigMap"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, "Node"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, "PersistentVolumeClaim"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, "PersistentVolume"},
	{schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}, "Event"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "Deployment"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, "ReplicaSet"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, "StatefulSet"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, "DaemonSet"},
	{schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}, "EndpointSlice"},
	{schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, "NetworkPolicy"},
}

func (c *Collector) Run(ctx context.Context) error {
	informers := make([]cache.SharedInformer, 0, len(defaultResources))
	synced := make([]cache.InformerSynced, 0, len(defaultResources))
	for _, wr := range defaultResources {
		informer := c.newInformer(ctx, wr)
		informers = append(informers, informer)
		synced = append(synced, informer.HasSynced)
		go informer.Run(ctx.Done())
		c.log.Info("watch started", "resource", wr.GVR.String())
	}
	if !cache.WaitForCacheSync(ctx.Done(), synced...) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("one or more Kubernetes watches failed to complete their initial list")
	}
	c.log.Info("initial Kubernetes snapshots persisted", "resources", len(informers))
	<-ctx.Done()
	return ctx.Err()
}

func (c *Collector) newInformer(ctx context.Context, wr watchedResource) cache.SharedInformer {
	resource := c.client.Resource(wr.GVR)
	lw := &cache.ListWatch{
		ListWithContextFunc: func(listCtx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return resource.Namespace(metav1.NamespaceAll).List(listCtx, opts)
		},
		WatchFuncWithContext: func(watchCtx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			return resource.Namespace(metav1.NamespaceAll).Watch(watchCtx, opts)
		},
	}
	// The wrapper lets client-go disable streaming-list semantics for clients
	// that explicitly do not support them (notably the dynamic fake in tests).
	// Real dynamic clients keep the more efficient WatchList path enabled.
	listerWatcher := cache.ToListWatcherWithWatchListSemantics(lw, c.client)
	informer := cache.NewSharedInformer(listerWatcher, &unstructured.Unstructured{}, 0)
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, isInInitialList bool) {
			action := model.ActionAdd
			if isInInitialList {
				action = model.ActionSnapshot
			}
			c.persist(ctx, wr, action, nil, unwrap(obj))
		},
		UpdateFunc: func(oldObj, newObj any) { c.persist(ctx, wr, model.ActionUpdate, unwrap(oldObj), unwrap(newObj)) },
		DeleteFunc: func(obj any) { c.persist(ctx, wr, model.ActionDelete, unwrap(obj), nil) },
	})
	return informer
}

func unwrap(obj any) *unstructured.Unstructured {
	switch x := obj.(type) {
	case *unstructured.Unstructured:
		return x
	case cache.DeletedFinalStateUnknown:
		if u, ok := x.Obj.(*unstructured.Unstructured); ok {
			return u
		}
	}
	return nil
}

func (c *Collector) persist(ctx context.Context, wr watchedResource, action model.Action, oldObj, newObj *unstructured.Unstructured) {
	obj := newObj
	if obj == nil {
		obj = oldObj
	}
	if obj == nil {
		return
	}

	current, err := c.sanitize(newObj)
	if err != nil {
		c.log.Error("sanitize current", "error", err)
		return
	}
	previous, err := c.sanitize(oldObj)
	if err != nil {
		c.log.Error("sanitize previous", "error", err)
		return
	}

	now := time.Now().UTC()
	r := model.Record{
		SchemaVersion: model.SchemaVersionV1Alpha1,
		ID:            makeID(now, obj.GetUID(), obj.GetResourceVersion(), action),
		ObservedAt:    now,
		Source:        model.SourceKubernetes,
		Action:        action,
		Cluster:       c.cluster,
		APIVersion:    obj.GetAPIVersion(),
		Kind:          wr.Kind,
		Namespace:     obj.GetNamespace(),
		Name:          obj.GetName(),
		UID:           string(obj.GetUID()),
		ResourceVer:   obj.GetResourceVersion(),
		Object:        current,
		Previous:      previous,
		Labels:        obj.GetLabels(),
	}
	if wr.Kind == "Event" {
		r.Reason, _, _ = unstructured.NestedString(obj.Object, "reason")
		r.Summary, _, _ = unstructured.NestedString(obj.Object, "message")
	}
	if err := c.store.Append(ctx, r); err != nil {
		c.log.Error("append record", "error", err, "kind", wr.Kind, "name", obj.GetName())
	}
}

func (c *Collector) sanitize(obj *unstructured.Unstructured) (json.RawMessage, error) {
	if obj == nil {
		return nil, nil
	}
	b, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, err
	}
	b, err = redact.Object(b, c.includeConfigMaps)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func makeID(t time.Time, uid any, rv string, action model.Action) string {
	s := fmt.Sprintf("%d:%v:%s:%s", t.UnixNano(), uid, rv, action)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:16])
}
