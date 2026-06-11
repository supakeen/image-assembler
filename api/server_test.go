package api

import (
	"context"
	"testing"
	"time"

	"github.com/supakeen/image-assembler/ipc"
)

func TestServerRoundTrip(t *testing.T) {
	var received map[string]interface{}
	done := make(chan struct{})

	handler := func(msg map[string]interface{}, fds *ipc.FDSet, sock *ipc.Socket) {
		if fds != nil {
			fds.Close()
		}
		received = msg
		sock.Send(map[string]interface{}{"status": "ok"}, nil)
		close(done)
	}

	srv := NewServer("test-endpoint", handler)
	srv.SocketAddress = t.TempDir() + "/test.sock"

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Cleanup()

	client, err := ipc.Dial(srv.SocketAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Send(map[string]interface{}{"method": "ping"}, nil); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	if received["method"] != "ping" {
		t.Errorf("expected method=ping, got %v", received["method"])
	}

	reply, _, err := client.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if reply["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", reply["status"])
	}
}

func TestOsbuildServerException(t *testing.T) {
	srv := NewOsbuildServer()
	srv.SocketAddress = t.TempDir() + "/osbuild.sock"

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Cleanup()

	client, err := ipc.Dial(srv.SocketAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	excMsg := map[string]interface{}{
		"method": "exception",
		"exception": map[string]interface{}{
			"type":  "RuntimeError",
			"value": "test error",
		},
	}

	if err := client.Send(excMsg, nil); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	if srv.Error == nil {
		t.Fatal("expected error to be captured")
	}
	if srv.Error["type"] != "exception" {
		t.Errorf("expected type=exception, got %v", srv.Error["type"])
	}

	data, ok := srv.Error["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be map")
	}
	if data["type"] != "RuntimeError" {
		t.Errorf("expected RuntimeError, got %v", data["type"])
	}
}
