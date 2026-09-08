package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/collector"
	"github.com/ErmilovAlexander/blackbox/internal/config"
	storepkg "github.com/ErmilovAlexander/blackbox/internal/store"
	jsonlstore "github.com/ErmilovAlexander/blackbox/internal/store/jsonl"
	"github.com/ErmilovAlexander/blackbox/internal/timeline"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "recorder":
		runRecorder(os.Args[2:])
	case "timeline":
		runTimeline(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "kube-blackbox <recorder|timeline|version>")
	fmt.Fprintln(os.Stderr, "  recorder  watch Kubernetes objects and append canonical records")
	fmt.Fprintln(os.Stderr, "  timeline  query retained records without modifying the store")
	fmt.Fprintln(os.Stderr, "  version   print the build version")
}

func runRecorder(args []string) {
	cfg := config.Default()
	fs := flag.NewFlagSet("recorder", flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", "", "path to kubeconfig; empty uses in-cluster config")
	fs.StringVar(&cfg.ClusterName, "cluster", cfg.ClusterName, "logical cluster name")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "local data directory")
	fs.DurationVar(&cfg.Retention, "retention", cfg.Retention, "rolling retention window")
	fs.Int64Var(&cfg.MaxStoreBytes, "max-store-bytes", cfg.MaxStoreBytes, "hard local store limit")
	fs.Int64Var(&cfg.MaxSegmentBytes, "max-segment-bytes", cfg.MaxSegmentBytes, "segment rotation size")
	fs.BoolVar(&cfg.IncludeConfigMaps, "include-configmaps", cfg.IncludeConfigMaps, "store ConfigMap payloads (off by default)")
	_ = fs.Parse(args)
	if cfg.ClusterName == "" {
		fatalIf(fmt.Errorf("cluster must not be empty"))
	}
	if cfg.Retention < 0 {
		fatalIf(fmt.Errorf("retention must not be negative"))
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	restCfg, err := kubeConfig(*kubeconfig)
	fatalIf(err)
	client, err := dynamic.NewForConfig(restCfg)
	fatalIf(err)
	st, err := jsonlstore.Open(cfg.DataDir, cfg.Retention, cfg.MaxStoreBytes, cfg.MaxSegmentBytes)
	fatalIf(err)
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info("kube-blackbox recorder starting", "cluster", cfg.ClusterName, "dataDir", cfg.DataDir, "retention", cfg.Retention.String())
	c := collector.New(client, st, cfg.ClusterName, cfg.IncludeConfigMaps, log)
	if err := c.Run(ctx); err != nil && err != context.Canceled {
		fatalIf(err)
	}
}

func runTimeline(args []string) {
	fs := flag.NewFlagSet("timeline", flag.ExitOnError)
	dataDir := fs.String("data-dir", "/var/lib/kube-blackbox", "local data directory")
	fromText := fs.String("from", "", "RFC3339 start time")
	toText := fs.String("to", "", "RFC3339 end time")
	ns := fs.String("namespace", "", "namespace filter")
	kind := fs.String("kind", "", "kind filter")
	name := fs.String("name", "", "object name filter")
	uid := fs.String("uid", "", "object UID filter")
	limit := fs.Int("limit", 500, "max records")
	output := fs.String("output", "timeline", "output format: timeline or records")
	_ = fs.Parse(args)
	from, err := parseTime(*fromText)
	fatalIf(err)
	to, err := parseTime(*toText)
	fatalIf(err)
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		fatalIf(fmt.Errorf("from must be before or equal to to"))
	}
	if *limit < 0 {
		fatalIf(fmt.Errorf("limit must not be negative"))
	}
	st, err := jsonlstore.OpenReadOnly(*dataDir)
	fatalIf(err)
	defer st.Close()
	records, err := st.Query(context.Background(), storepkg.Query{From: from, To: to, Namespace: *ns, Kind: *kind, Name: *name, UID: *uid, Limit: *limit})
	fatalIf(err)
	var result any
	switch *output {
	case "timeline":
		result = timeline.Build(records)
	case "records":
		result = records
	default:
		fatalIf(fmt.Errorf("unsupported output %q: use timeline or records", *output))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	fatalIf(enc.Encode(result))
}

func kubeConfig(path string) (*rest.Config, error) {
	if path != "" {
		return clientcmd.BuildConfigFromFlags("", path)
	}
	return rest.InClusterConfig()
}

func parseTime(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, v)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
