package redact

import (
	"encoding/json"
	"strings"
)

// Object sanitizes Kubernetes objects before they are written to disk.
// Rules are deliberately conservative: the recorder must be useful for
// forensics without becoming a credential archive.
func Object(in []byte, includeConfigMaps bool) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	root, ok := v.(map[string]any)
	if !ok {
		return in, nil
	}

	kind, _ := root["kind"].(string)
	if meta, ok := root["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
		if annotations, ok := meta["annotations"].(map[string]any); ok {
			for k := range annotations {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "last-applied-configuration") || strings.Contains(lk, "token") || strings.Contains(lk, "password") || strings.Contains(lk, "secret") {
					annotations[k] = "<redacted>"
				}
			}
		}
	}

	if strings.EqualFold(kind, "Secret") {
		delete(root, "data")
		delete(root, "stringData")
		root["redaction"] = "secret payload removed"
	}
	if strings.EqualFold(kind, "ConfigMap") && !includeConfigMaps {
		delete(root, "data")
		delete(root, "binaryData")
		root["redaction"] = "configmap payload removed"
	}
	redactEnv(root)
	return json.Marshal(root)
}

func redactEnv(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == "env" {
				if arr, ok := child.([]any); ok {
					for _, item := range arr {
						if env, ok := item.(map[string]any); ok {
							if _, exists := env["value"]; exists {
								env["value"] = "<redacted>"
							}
						}
					}
				}
			}
			redactEnv(child)
		}
	case []any:
		for _, child := range x {
			redactEnv(child)
		}
	}
}
