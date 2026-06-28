package chatgpt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxAutoApproveRounds = 16

type streamRelay struct {
	approval       *pendingApproval
	done           bool
	conversationID string
}

func (c *Client) ConversationAutoApprove(ctx context.Context, req ChatRequest) (*http.Response, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		if err := c.runAutoApproveLoop(ctx, req, pw); err != nil {
			_, _ = pw.Write([]byte("data: " + fmt.Sprintf(`{"error":"%s"}`, err.Error()) + "\n\n"))
		}
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
	}, nil
}

func (c *Client) runAutoApproveLoop(ctx context.Context, req ChatRequest, out io.Writer) error {
	conversationID := req.ConversationID
	parentMessageID := req.ParentMessageID
	current := req

	for round := 0; round < maxAutoApproveRounds; round++ {
		current.ConversationID = conversationID
		current.ParentMessageID = parentMessageID

		resp, err := c.Conversation(ctx, current)
		if err != nil {
			return err
		}

		relay, err := relayConversationStream(resp.Body, out)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if relay.conversationID != "" {
			conversationID = relay.conversationID
		}

		if relay.approval != nil {
			parentMessageID = relay.approval.TargetMessageID
			current = ChatRequest{
				Model:          req.Model,
				GizmoID:        req.GizmoID,
				ConversationID: conversationID,
				ParentMessageID: parentMessageID,
				Messages:       nil,
				ApprovalOnly:   relay.approval,
			}
			continue
		}
		if relay.done {
			return nil
		}
		return nil
	}
	return fmt.Errorf("auto-approve exceeded %d rounds", maxAutoApproveRounds)
}

func relayConversationStream(body io.Reader, out io.Writer) (streamRelay, error) {
	var result streamRelay
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var conversationID string
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(line[6:])
		if payload == "[DONE]" {
			result.done = true
			break
		}

		raw, ok := parseSSEPayload(line)
		if ok {
			if cid, _ := raw["conversation_id"].(string); cid != "" {
				conversationID = cid
			}
			if approval := parsePendingApproval(raw); approval != nil {
				result.approval = approval
			}
		}

		if _, err := io.WriteString(out, line+"\n\n"); err != nil {
			return result, err
		}
	}
	result.conversationID = conversationID
	return result, scanner.Err()
}

func (c *Client) conversationHTTPTimeout() time.Duration {
	return 5 * time.Minute
}