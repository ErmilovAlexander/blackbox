package redact

import (
	"encoding/json"
	"testing"
)

func TestSecretAndEnvAreRedacted(t *testing.T) {
	in := []byte(`{
		"kind":"Secret",
		"metadata":{"name":"s","managedFields":[{"x":1}],"annotations":{"password-note":"abc","safe":"ok"}},
		"data":{"password":"c2VjcmV0"},
		"spec":{"containers":[{"env":[{"name":"TOKEN","value":"plain"},{"name":"FROM_SECRET","valueFrom":{"secretKeyRef":{"name":"s","key":"token"}}}]}]}
	}`)
	out, err := Object(in, false)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["data"]; ok {
		t.Fatal("secret data must not be stored")
	}
	meta := got["metadata"].(map[string]any)
	if _, ok := meta["managedFields"]; ok {
		t.Fatal("managedFields must be removed")
	}
	ann := meta["annotations"].(map[string]any)
	if ann["password-note"] != "<redacted>" {
		t.Fatalf("annotation not redacted: %#v", ann)
	}
	containers := got["spec"].(map[string]any)["containers"].([]any)
	env := containers[0].(map[string]any)["env"].([]any)
	if env[0].(map[string]any)["value"] != "<redacted>" {
		t.Fatal("env value must be redacted")
	}
	if _, ok := env[1].(map[string]any)["valueFrom"]; !ok {
		t.Fatal("secret reference metadata should remain")
	}
}

func TestConfigMapPayloadIsOptIn(t *testing.T) {
	in := []byte(`{"kind":"ConfigMap","metadata":{"name":"cfg"},"data":{"a":"b"}}`)
	out, err := Object(in, false)
	if err != nil {
		t.Fatal(err)
	}
	var redacted map[string]any
	_ = json.Unmarshal(out, &redacted)
	if _, ok := redacted["data"]; ok {
		t.Fatal("configmap data should be removed by default")
	}

	out, err = Object(in, true)
	if err != nil {
		t.Fatal(err)
	}
	var included map[string]any
	_ = json.Unmarshal(out, &included)
	if _, ok := included["data"]; !ok {
		t.Fatal("configmap data should be kept when explicitly enabled")
	}
}
