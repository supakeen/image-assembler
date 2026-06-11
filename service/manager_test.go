package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestManagerStartStop(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	script := filepath.Join(t.TempDir(), "echo_service.py")
	os.WriteFile(script, []byte(echoServiceScript), 0755)

	mgr := NewManager("/usr/lib/osbuild")
	defer mgr.Close()

	client, err := mgr.Start(context.Background(), "test-echo", script, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, err := client.Call(context.Background(), "echo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T: %v", result, result)
	}
	if resultMap["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", resultMap["msg"])
	}

	if err := mgr.Stop("test-echo"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

const echoServiceScript = `#!/usr/bin/env python3
"""Minimal echo service for testing the Go service manager."""
import argparse
import json
import socket
import sys
import array
import os

def recv_msg(sock):
    data, ancdata, flags, addr = sock.recvmsg(65536, 4096)
    if not data:
        return None
    return json.loads(data)

def send_msg(sock, msg):
    data = json.dumps(msg).encode()
    sock.sendmsg([data])

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--service-id", type=str)
    parser.add_argument("--service-fd", type=int)
    # Accept but ignore extra positional args (the script path)
    parser.add_argument("extra", nargs="*")
    args = parser.parse_args()

    sock = socket.fromfd(args.service_fd, socket.AF_UNIX, socket.SOCK_SEQPACKET)
    os.close(args.service_fd)

    while True:
        msg = recv_msg(sock)
        if msg is None:
            break

        kind = msg.get("type")
        if kind != "method":
            break

        data = msg.get("data", {})
        method = data.get("name")
        method_args = data.get("args", [])

        if method == "echo":
            reply = {
                "type": "reply",
                "data": {"reply": method_args}
            }
        else:
            reply = {
                "type": "exception",
                "data": {
                    "name": "ValueError",
                    "value": f"unknown method: {method}",
                    "backtrace": ""
                }
            }

        send_msg(sock, reply)

    sock.close()

if __name__ == "__main__":
    main()
`
