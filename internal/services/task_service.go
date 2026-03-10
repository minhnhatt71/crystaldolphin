package services

import (
	"context"
	"fmt"
	"strings"

	contractagent "github.com/crystaldolphin/crystaldolphin/internal/modeling/agent"
	"github.com/crystaldolphin/crystaldolphin/internal/modeling/session"
	"github.com/crystaldolphin/crystaldolphin/internal/workers"
)

// Compile-time check that TaskService satisfies the domain interface.
var _ contractagent.AgentService = (*TaskService)(nil)

// TaskService orchestrates a background task turn (subagent result delivery).
// When a subagent completes, it encodes the original chat location as the routing
// key ("channel:chatID"). TaskService parses that, loads the original session,
// runs the TaskWorker to summarise the result, and returns a response targeted
// at the originating chat.
type TaskService struct {
	worker       workers.Worker
	sessionStore session.Store
	memoryWindow int
}

// NewTaskService constructs a TaskService.
func NewTaskService(worker workers.Worker, sessions session.Store, memoryWindow int) *TaskService {
	return &TaskService{
		worker:       worker,
		sessionStore: sessions,
		memoryWindow: memoryWindow,
	}
}

// Handle implements contractagent.AgentService.
func (s *TaskService) Handle(ctx context.Context, req contractagent.ServiceRequest) (contractagent.ServiceResponse, error) {
	// The routing key encodes the original chat location as "channel:chatID".
	originChannel, originChatID := parseOrigin(req.RoutingKey(), req.Channel(), req.ChatID())
	key := originChannel + ":" + originChatID

	sess := s.sessionStore.GetOrCreate(key)

	workerReq := workers.WorkerRequest{
		Channel:  originChannel,
		ChatID:   originChatID,
		SenderID: req.SenderID(),
		Content:  req.Content(),
		History:  sess.History(s.memoryWindow),
	}

	result, err := s.worker.Process(ctx, workerReq)
	if err != nil {
		return contractagent.ServiceResponse{}, fmt.Errorf("task worker: %w", err)
	}

	// Record the raw task result as a system entry, then the LLM summary.
	sess.
		Record(session.NewSystemEntry(fmt.Sprintf("[Task result] %s", req.Content()))).
		Record(session.NewAssistantEntry(result.Content, result.ToolsUsed))

	if err := s.sessionStore.Save(sess); err != nil {
		return contractagent.ServiceResponse{}, fmt.Errorf("save session: %w", err)
	}

	return contractagent.NewServiceResponse(result.Content, originChannel, originChatID, nil), nil
}

// parseOrigin splits "channel:chatID" from the routing key.
// Falls back to req.Channel() and req.ChatID() if the key is absent or unparseable.
func parseOrigin(routingKey, fallbackChannel, fallbackChatID string) (channel, chatID string) {
	if routingKey != "" {
		if ch, id, ok := strings.Cut(routingKey, ":"); ok {
			return ch, id
		}
	}
	return fallbackChannel, fallbackChatID
}
