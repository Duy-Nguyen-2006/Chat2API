package httpclient

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
)

// BrowserFingerprint holds the per-account browser identity used to reach
// chatgpt.com. Fields mirror what the ChatGPT web client sends and are
// consumed by chatgpt.Client.buildHeaders.
type BrowserFingerprint struct {
	UserAgent      string
	DeviceID       string // oai-device-id
	SessionID      string // oai-session-id
	ClientVersion  string // OAI-Client-Version
	ClientBuild    string // OAI-Client-Build-Number
	SecChUA        string
	SecChUAMobile  string
	SecChUAPlatform string
}

// Default client version / build shipped by chatgpt.com prod (kept in sync
// with basketikun/chatgpt2api defaults; safe to override per account).
const (
	DefaultClientVersion = "prod-a194cd50d4416d3c0b47c740f206b12ce60f5887"
	DefaultClientBuild   = "6708908"

	defaultUA           = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0"
	defaultSecChUA      = `"Microsoft Edge";v="133", "Chromium";v="133", "Not A(Brand";v="24"`
	defaultSecChUAMob   = "?0"
	defaultSecChUAPlat  = `"Windows"`
)

// NewFingerprint returns a BrowserFingerprint with sensible defaults and
// freshly generated device/session IDs. Callers may override any field.
func NewFingerprint() BrowserFingerprint {
	return BrowserFingerprint{
		UserAgent:        defaultUA,
		DeviceID:         newUUID(),
		SessionID:        newUUID(),
		ClientVersion:    DefaultClientVersion,
		ClientBuild:      DefaultClientBuild,
		SecChUA:          defaultSecChUA,
		SecChUAMobile:    defaultSecChUAMob,
		SecChUAPlatform:  defaultSecChUAPlat,
	}
}

// Apply sets the static browser/client-hint headers onto h. Caller is
// responsible for Authorization, ChatGPT-Account-ID, Cookie and any
// sentinel tokens (added per-request).
func (f BrowserFingerprint) Apply(h http.Header) {
	if f.UserAgent != "" {
		h.Set("User-Agent", f.UserAgent)
	}
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Origin", "https://chatgpt.com")
	h.Set("Referer", "https://chatgpt.com/")
	h.Set("OAI-Language", "en-US")
	if f.DeviceID != "" {
		h.Set("Oai-Device-Id", f.DeviceID)
	}
	if f.SessionID != "" {
		h.Set("Oai-Session-Id", f.SessionID)
	}
	if f.ClientVersion != "" {
		h.Set("OAI-Client-Version", f.ClientVersion)
	}
	if f.ClientBuild != "" {
		h.Set("OAI-Client-Build-Number", f.ClientBuild)
	}
	if f.SecChUA != "" {
		h.Set("Sec-Ch-Ua", f.SecChUA)
	}
	if f.SecChUAMobile != "" {
		h.Set("Sec-Ch-Ua-Mobile", f.SecChUAMobile)
	}
	if f.SecChUAPlatform != "" {
		h.Set("Sec-Ch-Ua-Platform", f.SecChUAPlatform)
	}
	// Additional client hints the ChatGPT web client always sends.
	h.Set("Sec-Ch-Ua-Arch", `"x86"`)
	h.Set("Sec-Ch-Ua-Bitness", `"64"`)
	h.Set("Sec-Ch-Ua-Model", `""`)
	h.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Priority", "u=1, i")
}

// newUUID generates a v4 UUID using crypto/rand.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
