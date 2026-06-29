//go:build probe

package chatgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func TestProbeChatRequirementsTransports(t *testing.T) {
	token := os.Getenv("CHATGPT_ACCESS_TOKEN")
	if token == "" {
		t.Skip("CHATGPT_ACCESS_TOKEN not set")
	}
	accountID := os.Getenv("CHATGPT_ACCOUNT_ID")
	cookie := OptionalCookieHeader(envOrProbe("COOKIES_FILE", "cookies_1.json"))

	fp := httpclient.NewFingerprint()
	doer, err := httpclient.New(httpclient.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClientWith(token, accountID, cookie, fp, doer)

	ua := fp.UserAgent
	body, _ := json.Marshal(map[string]string{"p": requirementsToken(ua)})

	// TLS impersonating client (current path)
	tlsReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(body))
	tlsReq.Header = client.buildHeaders(nil)
	tlsResp, err := doer.Do(tlsReq)
	if err != nil {
		t.Log("tls err:", err)
	} else {
		defer tlsResp.Body.Close()
		tlsB, _ := io.ReadAll(io.LimitReader(tlsResp.Body, 500))
		t.Logf("tls status=%d body=%q", tlsResp.StatusCode, preview(string(tlsB)))
	}

	// sessionDo minimal headers
	sessReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(body))
	sessReq.Header = client.buildAccountListHeaders()
	sessResp, err := http.DefaultClient.Do(sessReq)
	if err != nil {
		t.Log("session err:", err)
	} else {
		defer sessResp.Body.Close()
		sessB, _ := io.ReadAll(io.LimitReader(sessResp.Body, 500))
		t.Logf("session status=%d body=%q", sessResp.StatusCode, preview(string(sessB)))
	}

	tokens, err := client.chatRequirements(context.Background())
	if err != nil {
		t.Log("client.chatRequirements:", err)
		return
	}
	t.Log("client.chatRequirements: ok")

	req := ChatRequest{
		Model:    "auto",
		Messages: []Message{{Role: "user", Content: "say hi"}},
	}
	convBody, _ := json.Marshal(client.buildConversationBody(req))

	// TLS stream client (current Conversation path — new client per request)
	tlsConvReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(convBody))
	tlsConvReq.Header = client.conversationHeaders(tokens, "")
	tlsStream, _ := httpclient.New(httpclient.DefaultOptions())
	tlsConv, err := tlsStream.Do(tlsConvReq)
	if err != nil {
		t.Log("tls-new conversation err:", err)
	} else {
		defer tlsConv.Body.Close()
		tlsB, _ := io.ReadAll(io.LimitReader(tlsConv.Body, 500))
		t.Logf("tls-new conversation status=%d body=%q", tlsConv.StatusCode, preview(string(tlsB)))
	}

	// Shared pool TLS client (same doer as chat-requirements first attempt)
	poolConvReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(convBody))
	poolConvReq.Header = client.conversationHeaders(tokens, "")
	poolConv, err := doer.Do(poolConvReq)
	if err != nil {
		t.Log("tls-pool conversation err:", err)
	} else {
		defer poolConv.Body.Close()
		poolB, _ := io.ReadAll(io.LimitReader(poolConv.Body, 500))
		t.Logf("tls-pool conversation status=%d body=%q", poolConv.StatusCode, preview(string(poolB)))
	}

	// TLS pool + minimal sentinel headers (hybrid)
	hybridReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(convBody))
	hybridReq.Header = conversationSessionHeadersProbe(client, tokens, "")
	hybridConv, err := doer.Do(hybridReq)
	if err != nil {
		t.Log("tls-hybrid conversation err:", err)
	} else {
		defer hybridConv.Body.Close()
		hybridB, _ := io.ReadAll(io.LimitReader(hybridConv.Body, 500))
		t.Logf("tls-hybrid conversation status=%d body=%q", hybridConv.StatusCode, preview(string(hybridB)))
	}

	// sessionDo minimal + sentinel headers
	sessConvReq, _ := http.NewRequest(http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(convBody))
	sessConvReq.Header = conversationSessionHeadersProbe(client, tokens, "")
	sessConv, err := http.DefaultClient.Do(sessConvReq)
	if err != nil {
		t.Log("session conversation err:", err)
	} else {
		defer sessConv.Body.Close()
		sessB, _ := io.ReadAll(io.LimitReader(sessConv.Body, 500))
		t.Logf("session conversation status=%d body=%q", sessConv.StatusCode, preview(string(sessB)))
	}
}

func envOrProbe(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func conversationSessionHeadersProbe(c *Client, tokens *sentinelTokens, gizmoID string) http.Header {
	h := c.buildAccountListHeaders()
	h.Set("Accept", "text/event-stream")
	h.Set(headerContentType, mimeApplicationJSON)
	if gizmoID != "" {
		h.Set("Referer", baseURL+"/g/"+gizmoID+"/chat")
	}
	if tokens.chat != "" {
		h.Set("OpenAI-Sentinel-Chat-Requirements-Token", tokens.chat)
	}
	if tokens.proof != "" {
		h.Set("OpenAI-Sentinel-Proof-Token", tokens.proof)
	}
	if tokens.turnstile != "" {
		h.Set("OpenAI-Sentinel-Turnstile-Token", tokens.turnstile)
	}
	if tokens.so != "" {
		h.Set("OpenAI-Sentinel-SO-Token", tokens.so)
	}
	return h
}

func preview(s string) string {
	if strings.Contains(s, "<html") {
		return "HTML challenge"
	}
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}