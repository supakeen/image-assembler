package service

import "fmt"

// ProtocolError indicates a malformed message (missing type/data fields).
type ProtocolError struct {
	Msg string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error: %s", e.Msg)
}

// RemoteError wraps a Python exception received from a service process,
// preserving the exception name, message, and traceback for diagnostics.
type RemoteError struct {
	Name      string
	Value     string
	Backtrace string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("%s: %s", e.Name, e.Value)
}

func EncodeMethod(name string, args interface{}) map[string]interface{} {
	if args == nil {
		args = []interface{}{}
	}
	return map[string]interface{}{
		"type": "method",
		"data": map[string]interface{}{
			"name": name,
			"args": args,
		},
	}
}

func EncodeReply(reply interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "reply",
		"data": map[string]interface{}{
			"reply": reply,
		},
	}
}

func EncodeSignal(sig interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "signal",
		"data": map[string]interface{}{
			"reply": sig,
		},
	}
}

func EncodeException(name, value, backtrace string) map[string]interface{} {
	return map[string]interface{}{
		"type": "exception",
		"data": map[string]interface{}{
			"name":      name,
			"value":     value,
			"backtrace": backtrace,
		},
	}
}

func DecodeMessage(msg map[string]interface{}) (string, map[string]interface{}, error) {
	kind, ok := msg["type"].(string)
	if !ok {
		return "", nil, &ProtocolError{Msg: "missing or invalid 'type' field"}
	}
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return "", nil, &ProtocolError{Msg: "missing or invalid 'data' field"}
	}
	return kind, data, nil
}

func DecodeMethod(data map[string]interface{}) (string, interface{}, error) {
	name, ok := data["name"].(string)
	if !ok {
		return "", nil, &ProtocolError{Msg: "missing or invalid method 'name'"}
	}
	args := data["args"]
	if args == nil {
		args = []interface{}{}
	}
	return name, args, nil
}

func DecodeReply(data map[string]interface{}) (interface{}, error) {
	reply, ok := data["reply"]
	if !ok {
		return nil, &ProtocolError{Msg: "missing 'reply' field"}
	}
	return reply, nil
}

func DecodeException(data map[string]interface{}) (*RemoteError, error) {
	name, _ := data["name"].(string)
	value, _ := data["value"].(string)
	backtrace, _ := data["backtrace"].(string)
	return &RemoteError{
		Name:      name,
		Value:     value,
		Backtrace: backtrace,
	}, nil
}
