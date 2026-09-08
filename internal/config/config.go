package config

import "time"

type Config struct {
	ClusterName       string
	DataDir           string
	Retention         time.Duration
	MaxStoreBytes     int64
	MaxSegmentBytes   int64
	IncludeConfigMaps bool
}

func Default() Config {
	return Config{
		ClusterName:       "default",
		DataDir:           "/var/lib/kube-blackbox",
		Retention:         24 * time.Hour,
		MaxStoreBytes:     2 << 30,  // 2 GiB
		MaxSegmentBytes:   64 << 20, // 64 MiB
		IncludeConfigMaps: false,
	}
}
