package services

import (
	"context"
	"fmt"

	contract "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/session"
	"github.com/crystaldolphin/crystaldolphin/internal/workers"
)

// Compile-time check that AgentService satisfies the domain interface.
var _ contract.AgentService = (*AgentService)(nil)

// AgentService orchestrates a user-facing conversation turn.
// It loads the session, resolves memory, builds the worker request, calls the
// ChatWorker, saves the session, and returns the response. The caller is
// responsible for routing the ServiceResponse to the appropriate channel bus.
type AgentService struct {
	worker       workers.Worker
	sessionStore session.Store
	compactor    MemoryCompactor
	memoryStore  MemoryReader
	memoryWindow int
}

// NewAgentService constructs an AgentService.
func NewAgentService(
	worker workers.Worker,
	sessions session.Store,
	compactor MemoryCompactor,
	memoryReader MemoryReader,
	memoryWindow int,
) *AgentService {
	return &AgentService{
		worker:       worker,
		sessionStore: sessions,
		compactor:    compactor,
		memoryStore:  memoryReader,
		memoryWindow: memoryWindow,
	}
}

// Handle implements contractagent.AgentService.
func (s *AgentService) Handle(ctx context.Context, req contract.ServiceRequest) (contract.ServiceResponse, error) {
	key := s.sessionKey(req)
	sess := s.sessionStore.GetOrCreate(key)

	if s.compactor != nil {
		s.compactor.Schedule(key, sess)
	}

	longTermMemory := ""
	if s.memoryStore != nil {
		if mem, err := s.memoryStore.ReadLongTermMemory(); err == nil {
			longTermMemory = mem
		}
	}

	workerReq := workers.WorkerRequest{
		Channel:        req.Channel(),
		ChatID:         req.ChatID(),
		SenderID:       req.SenderID(),
		Content:        req.Content(),
		Media:          req.Media(),
		Metadata:       req.Metadata(),
		History:        sess.History(s.memoryWindow),
		LongTermMemory: longTermMemory,
	}

	result, err := s.worker.Process(ctx, workerReq)
	if err != nil {
		return contract.ServiceResponse{}, fmt.Errorf("worker: %w", err)
	}

	sess.
		Record(session.NewUserEntry(req.Content())).
		Record(session.NewAssistantEntry(result.Content, result.ToolsUsed))
	if err := s.sessionStore.Save(sess); err != nil {
		return contract.ServiceResponse{}, fmt.Errorf("save session: %w", err)
	}

	return contract.NewServiceResponse(result.Content, req.Channel(), req.ChatID(), req.Metadata()), nil
}

func (s *AgentService) sessionKey(req contract.ServiceRequest) string {
	if key := req.RoutingKey(); key != "" {
		return key
	}
	return req.Channel() + ":" + req.ChatID()
}
