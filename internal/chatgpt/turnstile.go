package chatgpt

// Turnstile token solver — port of utils/turnstile.py from
// basketikun/chatgpt2api. Decodes the `dx` challenge carried in the
// /backend-api/sentinel/chat-requirements response, runs a small bytecode
// interpreter over the resulting token list, and emits a base64 token that
// satisfies Cloudflare Turnstile for the chatgpt.com endpoint.
//
// Each token in the list is [opcode, arg...]. Args are either numeric slot
// references (resolved through the `process` map) or literal values (assigned
// directly by opcode 2). The interpreter is a faithful Go port.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// orderedMap mirrors the Python OrderedMap used as a fake JS object.
type orderedMap struct {
	keys   []string
	values map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{values: map[string]any{}}
}

func (m *orderedMap) add(key string, val any) {
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = val
}

// turnstileStr is the Go equivalent of _turnstile_to_str.
func turnstileStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "undefined"
	case float64:
		// Match Python's str(float): integers print without decimals.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	case string:
		special := map[string]string{
			"window.Math":            "[object Math]",
			"window.Reflect":         "[object Reflect]",
			"window.performance":     "[object Performance]",
			"window.localStorage":    "[object Storage]",
			"window.Object":          "function Object() { [native code] }",
			"window.Reflect.set":     "function set() { [native code] }",
			"window.performance.now": "function () { [native code] }",
			"window.Object.create":   "function create() { [native code] }",
			"window.Object.keys":     "function keys() { [native code] }",
			"window.Math.random":     "function random() { [native code] }",
		}
		if s, ok := special[x]; ok {
			return s
		}
		return x
	case []string:
		return strings.Join(x, ",")
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, turnstileStr(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func xorString(text, key string) string {
	if key == "" {
		return text
	}
	tb := []byte(text)
	kb := []byte(key)
	out := make([]byte, len(tb))
	for i := range tb {
		out[i] = tb[i] ^ kb[i%len(kb)]
	}
	return string(out)
}

// slotArg resolves args[i] as a slot id and returns the stored value.
func slotArg(process map[float64]any, args []any, i int) any {
	if i >= len(args) {
		return nil
	}
	if id, ok := toFloat(args[i]); ok {
		return process[id]
	}
	return args[i]
}

// slotID treats args[i] as a numeric slot id.
func slotID(args []any, i int) float64 {
	if i >= len(args) {
		return 0
	}
	f, _ := toFloat(args[i])
	return f
}

// opcode is the closure type stored in the process map.
type opcode func(args []any, process map[float64]any, state *turnstileState)

type turnstileState struct {
	startTime time.Time
	result    string
}

// solveTurnstileToken runs the Turnstile bytecode challenge and returns the
// base64 token, or "" if the challenge could not be parsed/ solved.
func solveTurnstileToken(dx, p string) string {
	decoded, err := base64.StdEncoding.DecodeString(dx)
	if err != nil {
		return ""
	}
	xored := xorString(string(decoded), p)

	var tokenList []any
	if err := json.Unmarshal([]byte(xored), &tokenList); err != nil {
		return ""
	}

	state := &turnstileState{startTime: time.Now()}
	process := initTurnstileProcess(tokenList, p, state)
	executeTurnstileProgram(process, tokenList, state)
	return state.result
}

// toFloat coerces a JSON-decoded number to float64.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}
