package service

import (
	"testing"
)

func TestEncodeDecodeMethod(t *testing.T) {
	msg := EncodeMethod("download", map[string]interface{}{"url": "http://example.com"})

	kind, data, err := DecodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "method" {
		t.Errorf("expected kind=method, got %s", kind)
	}

	name, args, err := DecodeMethod(data)
	if err != nil {
		t.Fatal(err)
	}
	if name != "download" {
		t.Errorf("expected name=download, got %s", name)
	}

	argsMap, ok := args.(map[string]interface{})
	if !ok {
		t.Fatal("expected args to be map")
	}
	if argsMap["url"] != "http://example.com" {
		t.Errorf("expected url, got %v", argsMap["url"])
	}
}

func TestEncodeDecodeMethodNilArgs(t *testing.T) {
	msg := EncodeMethod("ping", nil)
	_, data, _ := DecodeMessage(msg)
	_, args, _ := DecodeMethod(data)

	argsList, ok := args.([]interface{})
	if !ok || len(argsList) != 0 {
		t.Errorf("expected empty args list, got %v (%T)", args, args)
	}
}

func TestEncodeDecodeReply(t *testing.T) {
	msg := EncodeReply(map[string]interface{}{"status": "ok"})

	kind, data, err := DecodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "reply" {
		t.Errorf("expected kind=reply, got %s", kind)
	}

	reply, err := DecodeReply(data)
	if err != nil {
		t.Fatal(err)
	}
	replyMap, ok := reply.(map[string]interface{})
	if !ok {
		t.Fatal("expected reply to be map")
	}
	if replyMap["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", replyMap["status"])
	}
}

func TestEncodeDecodeSignal(t *testing.T) {
	msg := EncodeSignal(map[string]interface{}{"progress": float64(50)})

	kind, data, err := DecodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "signal" {
		t.Errorf("expected kind=signal, got %s", kind)
	}

	reply, err := DecodeReply(data)
	if err != nil {
		t.Fatal(err)
	}
	replyMap, ok := reply.(map[string]interface{})
	if !ok {
		t.Fatal("expected signal data to be map")
	}
	if replyMap["progress"] != float64(50) {
		t.Errorf("expected progress=50, got %v", replyMap["progress"])
	}
}

func TestEncodeDecodeException(t *testing.T) {
	msg := EncodeException("ValueError", "bad input", "Traceback...\n")

	kind, data, err := DecodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "exception" {
		t.Errorf("expected kind=exception, got %s", kind)
	}

	re, err := DecodeException(data)
	if err != nil {
		t.Fatal(err)
	}
	if re.Name != "ValueError" {
		t.Errorf("expected name=ValueError, got %s", re.Name)
	}
	if re.Value != "bad input" {
		t.Errorf("expected value='bad input', got %s", re.Value)
	}
	if re.Backtrace != "Traceback...\n" {
		t.Errorf("expected backtrace, got %q", re.Backtrace)
	}
}

func TestDecodeMessageMissingType(t *testing.T) {
	_, _, err := DecodeMessage(map[string]interface{}{"data": map[string]interface{}{}})
	if err == nil {
		t.Error("expected error for missing type")
	}
}

func TestDecodeMessageMissingData(t *testing.T) {
	_, _, err := DecodeMessage(map[string]interface{}{"type": "method"})
	if err == nil {
		t.Error("expected error for missing data")
	}
}

func TestRemoteErrorString(t *testing.T) {
	re := &RemoteError{Name: "RuntimeError", Value: "something broke"}
	got := re.Error()
	if got != "RuntimeError: something broke" {
		t.Errorf("unexpected error string: %s", got)
	}
}
