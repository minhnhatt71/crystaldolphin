package agent

import "context"

// ServiceRequest carries a complete inbound event for the service layer.
// It is a pure value type with no dependency on the bus or transport packages.
// The gateway/CLI entry points translate bus messages into ServiceRequest.
type ServiceRequest struct {
	channel    string
	chatID     string
	senderID   string
	routingKey string // explicit session key; empty = derive from channel+":"+chatID
	content    string
	media      []string
	metadata   map[string]any
}

// NewServiceRequest constructs a ServiceRequest.
func NewServiceRequest(channel, chatID, senderID, content string, media []string, metadata map[string]any) ServiceRequest {
	return ServiceRequest{
		channel:  channel,
		chatID:   chatID,
		senderID: senderID,
		content:  content,
		media:    media,
		metadata: metadata,
	}
}

// WithRoutingKey returns a copy of the request with an explicit session routing key.
func (r ServiceRequest) WithRoutingKey(key string) ServiceRequest {
	r.routingKey = key
	return r
}

func (r ServiceRequest) Channel() string        { return r.channel }
func (r ServiceRequest) ChatID() string         { return r.chatID }
func (r ServiceRequest) SenderID() string       { return r.senderID }
func (r ServiceRequest) RoutingKey() string     { return r.routingKey }
func (r ServiceRequest) Content() string        { return r.content }
func (r ServiceRequest) Media() []string        { return r.media }
func (r ServiceRequest) Metadata() map[string]any { return r.metadata }

// ServiceResponse is the result returned by AgentService.Handle.
// The caller routes it to the appropriate channel bus.
type ServiceResponse struct {
	content  string
	channel  string
	chatID   string
	metadata map[string]any
}

// NewServiceResponse constructs a ServiceResponse.
func NewServiceResponse(content, channel, chatID string, metadata map[string]any) ServiceResponse {
	return ServiceResponse{content: content, channel: channel, chatID: chatID, metadata: metadata}
}

func (r ServiceResponse) Content() string        { return r.content }
func (r ServiceResponse) Channel() string        { return r.channel }
func (r ServiceResponse) ChatID() string         { return r.chatID }
func (r ServiceResponse) Metadata() map[string]any { return r.metadata }

// AgentService is the domain interface for the orchestration service.
// It owns the full request lifecycle: session load → worker → session save → response.
// Implementations live in internal/services/.
type AgentService interface {
	Handle(ctx context.Context, req ServiceRequest) (ServiceResponse, error)
}
