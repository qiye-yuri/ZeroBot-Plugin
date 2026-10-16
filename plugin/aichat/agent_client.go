package aichat

import (
	"errors"
	"net/http"
)

const agentResponseBytes = 2 << 20

var errAgentResponseLimit = errors.New("Agent response exceeds byte limit")

func boundedAgentClient(client *http.Client) *http.Client {
	return boundedModelResponseClient(client, agentResponseBytes, errAgentResponseLimit)
}
