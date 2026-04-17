package api

import (
	"encoding/json"
	"testing"
)

// TestIntOrFalse_UnmarshalNull reproduces the bug where the Rails API returns
// "processing_version": null for apps without versions. The Go json package
// treats JSON null into a non-pointer int as a no-op that returns a nil error,
// so IntOrFalse ends up with Set=true, Value=0 — incorrectly signaling that a
// version is being processed and causing AcquireForApp to poll forever.
func TestIntOrFalse_UnmarshalNull(t *testing.T) {
	var f IntOrFalse
	if err := json.Unmarshal([]byte("null"), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Set {
		t.Errorf("IntOrFalse decoded from null has Set=true (Value=%d); expected Set=false", f.Value)
	}
}

func TestIntOrFalse_UnmarshalFalse(t *testing.T) {
	var f IntOrFalse
	if err := json.Unmarshal([]byte("false"), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Set {
		t.Errorf("IntOrFalse decoded from false has Set=true; expected Set=false")
	}
}

func TestIntOrFalse_UnmarshalInt(t *testing.T) {
	var f IntOrFalse
	if err := json.Unmarshal([]byte("42"), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Set || f.Value != 42 {
		t.Errorf("IntOrFalse decoded from 42 = {Set:%v Value:%d}; expected {Set:true Value:42}", f.Set, f.Value)
	}
}

// TestApp_UnmarshalEmptyApp reproduces the end-to-end scenario: the Rails
// response for a channel app with zero versions puts null in both
// processing_version and publishing_version. AcquireForApp reads ProcessingVersion.Set
// and will wait forever if this is true.
func TestApp_UnmarshalEmptyApp(t *testing.T) {
	// Real response body captured from production for app 29004 (Helix Studio "test"),
	// trimmed to the fields that matter for this test.
	body := `{
		"id": 29004,
		"name": "test",
		"secret": "6753d95bcb1fc5c88181e7a84000e30c",
		"is_channel": true,
		"processing_version": null,
		"publishing_version": null,
		"allow_channel_direct_publish": true
	}`

	var app App
	if err := json.Unmarshal([]byte(body), &app); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.ProcessingVersion.Set {
		t.Errorf("ProcessingVersion.Set=true for null JSON — AcquireForApp would poll forever")
	}
	if app.PublishingVersion.Set {
		t.Errorf("PublishingVersion.Set=true for null JSON — AcquireForApp would poll forever")
	}
}
