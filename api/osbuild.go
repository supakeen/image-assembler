package api

import (
	"github.com/supakeen/image-assembler/ipc"
)

// OsbuildServer receives exception reports from stages inside the sandbox.
// When a stage encounters a Python exception, it sends it here so the build
// system can include the traceback in the result output.
type OsbuildServer struct {
	*Server
	Error map[string]interface{}
}

func NewOsbuildServer() *OsbuildServer {
	s := &OsbuildServer{}
	s.Server = NewServer("osbuild", s.handle)
	return s
}

func (s *OsbuildServer) handle(msg map[string]interface{}, fds *ipc.FDSet, _ *ipc.Socket) {
	if fds != nil {
		fds.Close()
	}

	method, _ := msg["method"].(string)
	if method != "exception" {
		return
	}

	exc, _ := msg["exception"].(map[string]interface{})
	if exc == nil {
		return
	}

	s.Error = map[string]interface{}{
		"type": "exception",
		"data": exc,
	}
}
