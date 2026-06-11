package ipc

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPairSendRecv(t *testing.T) {
	a, b, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()

	msg := map[string]interface{}{
		"type": "method",
		"data": map[string]interface{}{
			"name": "test",
			"args": []interface{}{"hello", float64(42)},
		},
	}

	if err := a.Send(msg, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got, fds, err := b.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if fds != nil {
		defer fds.Close()
	}

	kind, ok := got["type"].(string)
	if !ok || kind != "method" {
		t.Errorf("expected type=method, got %v", got["type"])
	}

	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be map")
	}
	if data["name"] != "test" {
		t.Errorf("expected name=test, got %v", data["name"])
	}
}

func TestPairFDPassing(t *testing.T) {
	a, b, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	testData := "hello from fd"
	w.WriteString(testData)
	w.Close()

	msg := map[string]interface{}{"type": "fd-test"}
	if err := a.Send(msg, []int{int(r.Fd())}); err != nil {
		t.Fatalf("Send with fd: %v", err)
	}

	got, fds, err := b.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if got["type"] != "fd-test" {
		t.Errorf("expected type=fd-test, got %v", got["type"])
	}
	if fds == nil || fds.Len() == 0 {
		t.Fatal("expected fds")
	}
	defer fds.Close()

	fd, err := fds.Steal(0)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 100)
	n, _ := unix.Read(fd, buf)
	unix.Close(fd)

	if string(buf[:n]) != testData {
		t.Errorf("expected %q from passed fd, got %q", testData, buf[:n])
	}
}

func TestPairLargePayload(t *testing.T) {
	a, b, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()

	largeValue := strings.Repeat("x", wmemMax()+1000)
	msg := map[string]interface{}{
		"type":  "large",
		"value": largeValue,
	}

	if err := a.Send(msg, nil); err != nil {
		t.Fatalf("Send large: %v", err)
	}

	got, fds, err := b.Recv()
	if err != nil {
		t.Fatalf("Recv large: %v", err)
	}
	if fds != nil {
		defer fds.Close()
	}

	if got["type"] != "large" {
		t.Errorf("expected type=large, got %v", got["type"])
	}
	gotValue, ok := got["value"].(string)
	if !ok || gotValue != largeValue {
		t.Errorf("large value mismatch: got len %d, want len %d", len(gotValue), len(largeValue))
	}
}

func TestListenAndDial(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.sock"

	server, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if err := server.SetListenBacklog(5); err != nil {
		t.Fatal(err)
	}
	if err := server.SetBlocking(true); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		client, err := Dial(path)
		if err != nil {
			done <- err
			return
		}
		defer client.Close()
		done <- client.Send(map[string]interface{}{"hello": "world"}, nil)
	}()

	conn, err := server.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	msg, fds, err := conn.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if fds != nil {
		defer fds.Close()
	}

	if msg["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", msg)
	}
}
