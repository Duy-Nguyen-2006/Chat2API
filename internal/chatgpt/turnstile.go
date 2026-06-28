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
	"math/rand"
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

	process := map[float64]any{}
	state := &turnstileState{startTime: time.Now()}

	// --- opcodes ---
	op1 := func(args []any, process map[float64]any, _ *turnstileState) {
		// process[e] = xor(str(process[e]), str(process[t]))
		e, t := slotID(args, 0), slotID(args, 1)
		process[e] = xorString(turnstileStr(process[e]), turnstileStr(process[t]))
	}
	op2 := func(args []any, process map[float64]any, _ *turnstileState) {
		// process[e] = literal(args[1])  — args[1] is the value, not a slot ref
		e := slotID(args, 0)
		if len(args) >= 2 {
			process[e] = args[1]
		}
	}
	op3 := func(args []any, _ map[float64]any, st *turnstileState) {
		// result = base64(str(process[e]))
		e := slotID(args, 0)
		st.result = base64.StdEncoding.EncodeToString([]byte(turnstileStr(process[e])))
	}
	op5 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t := slotID(args, 0), slotID(args, 1)
		current := process[e]
		incoming := process[t]
		switch c := current.(type) {
		case []any:
			process[e] = append(c, incoming)
			return
		case []string:
			if s, ok := incoming.(string); ok {
				process[e] = append(c, s)
				return
			}
			process[e] = append(c, turnstileStr(incoming))
			return
		}
		switch current.(type) {
		case string, float64:
			process[e] = turnstileStr(current) + turnstileStr(incoming)
			return
		}
		switch incoming.(type) {
		case string, float64:
			process[e] = turnstileStr(current) + turnstileStr(incoming)
			return
		}
		process[e] = "NaN"
	}
	op6 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t, n := slotID(args, 0), slotID(args, 1), slotID(args, 2)
		tv, _ := process[t].(string)
		nv, _ := process[n].(string)
		value := tv + "." + nv
		if value == "window.document.location" {
			process[e] = "https://chatgpt.com/"
		} else {
			process[e] = value
		}
	}
	op7 := func(args []any, process map[float64]any, _ *turnstileState) {
		// invoke Reflect.set or a stored callable
		e := slotID(args, 0)
		target := process[e]
		values := make([]any, 0, len(args)-1)
		for i := 1; i < len(args); i++ {
			values = append(values, slotArg(process, args, i))
		}
		switch tp := target.(type) {
		case string:
			if tp == "window.Reflect.set" && len(values) >= 3 {
				if obj, ok := values[0].(*orderedMap); ok {
					obj.add(turnstileStr(values[1]), values[2])
				}
			}
		case opcode:
			tp(args[1:], process, &turnstileState{})
		}
	}
	op8 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t := slotID(args, 0), slotID(args, 1)
		process[e] = process[t]
	}
	op14 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t := slotID(args, 0), slotID(args, 1)
		var v any
		if err := json.Unmarshal([]byte(turnstileStr(process[t])), &v); err == nil {
			process[e] = v
		}
	}
	op15 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t := slotID(args, 0), slotID(args, 1)
		b, _ := json.Marshal(process[t])
		process[e] = string(b)
	}
	op17 := func(args []any, process map[float64]any, st *turnstileState) {
		e := slotID(args, 0)
		t := slotID(args, 1)
		target, _ := process[t].(string)
		// callArgs: slot refs after t
		callFirst := func(i int) any { return slotArg(process, args, i) }
		switch target {
		case "window.performance.now":
			elapsed := float64(time.Now().UnixNano() - st.startTime.UnixNano())
			process[e] = (elapsed + rand.Float64()) / 1e6
		case "window.Object.create":
			process[e] = newOrderedMap()
		case "window.Object.keys":
			if len(args) >= 3 {
				if ks, _ := callFirst(2).(string); ks == "window.localStorage" {
					process[e] = []string{
						"STATSIG_LOCAL_STORAGE_INTERNAL_STORE_V4",
						"STATSIG_LOCAL_STORAGE_STABLE_ID",
						"client-correlated-secret",
						"oai/apps/capExpiresAt",
						"oai-did",
						"STATSIG_LOCAL_STORAGE_LOGGING_REQUEST",
						"UiState.isNavigationCollapsed.1",
					}
				}
			}
		case "window.Math.random":
			process[e] = rand.Float64()
		}
	}
	op18 := func(args []any, process map[float64]any, _ *turnstileState) {
		e := slotID(args, 0)
		b, err := base64.StdEncoding.DecodeString(turnstileStr(process[e]))
		if err == nil {
			process[e] = string(b)
		}
	}
	op19 := func(args []any, process map[float64]any, _ *turnstileState) {
		e := slotID(args, 0)
		process[e] = base64.StdEncoding.EncodeToString([]byte(turnstileStr(process[e])))
	}
	op20 := func(args []any, process map[float64]any, _ *turnstileState) {
		// if process[e]==process[t]: call process[n](rest...)
		if len(args) < 3 {
			return
		}
		e, t, n := slotID(args, 0), slotID(args, 1), slotID(args, 2)
		if turnstileStr(process[e]) == turnstileStr(process[t]) {
			if tp, ok := process[n].(opcode); ok {
				tp(args[3:], process, &turnstileState{})
			}
		}
	}
	op21 := func(args []any, _ map[float64]any, _ *turnstileState) {}
	op23 := func(args []any, process map[float64]any, _ *turnstileState) {
		// if process[e] != nil and callable(process[t]): process[t](rest after t)
		if len(args) < 2 {
			return
		}
		e := slotID(args, 0)
		t := slotID(args, 1)
		if process[e] == nil {
			return
		}
		if tp, ok := process[t].(opcode); ok {
			tp(args[2:], process, &turnstileState{})
		}
	}
	op24 := func(args []any, process map[float64]any, _ *turnstileState) {
		e, t, n := slotID(args, 0), slotID(args, 1), slotID(args, 2)
		tv, _ := process[t].(string)
		nv, _ := process[n].(string)
		process[e] = tv + "." + nv
	}

	process[9] = tokenList
	process[10] = "window"
	process[16] = p
	process[1] = op1
	process[2] = op2
	process[3] = op3
	process[5] = op5
	process[6] = op6
	process[7] = op7
	process[8] = op8
	process[14] = op14
	process[15] = op15
	process[17] = op17
	process[18] = op18
	process[19] = op19
	process[20] = op20
	process[21] = op21
	process[23] = op23
	process[24] = op24

	for _, tok := range tokenList {
		instr, ok := tok.([]any)
		if !ok || len(instr) == 0 {
			continue
		}
		opcodeID, ok := toFloat(instr[0])
		if !ok {
			continue
		}
		fn, ok := process[opcodeID].(opcode)
		if !ok {
			continue
		}
		args := instr[1:]
		(func() {
			defer func() { _ = recover() }()
			fn(args, process, state)
		})()
	}

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
