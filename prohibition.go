package main

import (
	"encoding/json"
	"slices"
	"sync"
	"uuid"

	"github.com/aerth/mostly/mllm"
)

type Approval struct {
	uuid.UUID
	Approved bool
	From     string
}
type ApprovalRequest struct {
	uuid.UUID
	Reason string
}

func createRequest(reason string) ApprovalRequest {
	x := ApprovalRequest{UUID: uuid.New(), Reason: reason}
	approvals.mu.Lock()
	defer approvals.mu.Unlock()
	approvals.pending[x.UUID] = reason
	return x
}

func isResolved(id uuid.UUID) bool {
	approvals.mu.Lock()
	defer approvals.mu.Unlock()
	_, ok := approvals.approved[id]
	return ok
}

func getResolved(id uuid.UUID) Approval {
	approvals.mu.Lock()
	defer approvals.mu.Unlock()
	approval, _ := approvals.approved[id]
	return approval
}

func doApproval(from string, id uuid.UUID, approve bool) bool {
	approvals.mu.Lock()
	defer approvals.mu.Unlock()
	_, ok := approvals.pending[id]
	if ok {
		approvals.approved[id] = Approval{From: from, UUID: id, Approved: approve}
	}
	return ok
}

// for now lets just let it grow
var approvals = struct {
	mu               sync.Mutex
	pending          map[uuid.UUID]string   // reason
	approved         map[uuid.UUID]Approval // from, yes/no
	allowedToolNames []string               // in [Flags]. empty = all
	needsApprovals   []string               // tool names, in [startGen]. empty = none
}{
	pending:  map[uuid.UUID]string{},
	approved: map[uuid.UUID]Approval{},
}

func FilterTools(v []mllm.Tool) mllm.Tools {
	return slices.DeleteFunc(slices.Clone(v), IsNotAllowedTool)
}

func IsNotAllowedToolSpec(v json.RawMessage) bool {
	t := mllm.Tool{Spec: v}
	return IsNotAllowedTool(t)
}

func IsNotAllowedTool(t mllm.Tool) bool {
	a := approvals.allowedToolNames
	if len(a) == 0 {
		return false // no restrictions
	}
	return !slices.Contains(a, t.GetName())
}
