package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

func mainGizmoKeys() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gizmoID := "g-6a407590545c8191af136331cdcc4844"
	body := fetchGET(creds, "/backend-api/gizmos/"+gizmoID)
	var root map[string]any
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		fmt.Println(body)
		os.Exit(1)
	}

	gizmo, _ := root["gizmo"].(map[string]any)
	if gizmo == nil {
		fmt.Println("no gizmo object")
		os.Exit(1)
	}

	fmt.Println("gizmo top-level keys:")
	for k := range gizmo {
		if k == "instructions" || k == "display" || k == "author" {
			continue
		}
		v := gizmo[k]
		switch x := v.(type) {
		case string:
			preview := x
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			fmt.Printf("  %s: %q\n", k, preview)
		default:
			b, _ := json.Marshal(x)
			if len(b) > 500 {
				b = append(b[:500], []byte("...")...)
			}
			fmt.Printf("  %s: %s\n", k, string(b))
		}
	}

	needle := []string{"tool", "action", "connector", "mcp", "plugin", "schema", "openapi"}
	fmt.Println("\nrecursive hits:")
	walkHits(gizmo, "", needle)
}

func walkHits(v any, prefix string, needles []string) {
	switch x := v.(type) {
	case map[string]any:
		walkMapHits(x, prefix, needles)
	case []any:
		for i, child := range x {
			walkHits(child, fmt.Sprintf("%s[%d].", prefix, i), needles)
		}
	}
}

func walkMapHits(m map[string]any, prefix string, needles []string) {
	for k, child := range m {
		path := prefix + k
		if hit := needleHit(k, child, needles); hit {
			printHit(path, child)
		}
		if len(path) < 80 {
			walkHits(child, path+".", needles)
		}
	}
}

func needleHit(key string, child any, needles []string) bool {
	kl := strings.ToLower(key)
	childText := strings.ToLower(fmt.Sprint(child))
	for _, n := range needles {
		if strings.Contains(kl, n) || strings.Contains(childText, n) {
			return true
		}
	}
	return false
}

func printHit(path string, child any) {
	b, _ := json.Marshal(child)
	if len(b) > 400 {
		b = append(b[:400], []byte("...")...)
	}
	fmt.Printf("  %s = %s\n", path, string(b))
}