package chatgpt

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"time"
)

func initTurnstileProcess(tokenList []any, p string, st *turnstileState) map[float64]any {
	process := map[float64]any{}
	registerTurnstileOpcodes(process, st)
	process[9] = tokenList
	process[10] = "window"
	process[16] = p
	return process
}

func registerTurnstileOpcodes(process map[float64]any, st *turnstileState) {
	process[1] = turnstileOp1
	process[2] = turnstileOp2
	process[3] = turnstileOp3
	process[5] = turnstileOp5
	process[6] = turnstileOp6
	process[7] = turnstileOp7
	process[8] = turnstileOp8
	process[14] = turnstileOp14
	process[15] = turnstileOp15
	process[17] = turnstileOp17(st)
	process[18] = turnstileOp18
	process[19] = turnstileOp19
	process[20] = turnstileOp20
	process[21] = turnstileOp21
	process[23] = turnstileOp23
	process[24] = turnstileOp24
}

func executeTurnstileProgram(process map[float64]any, tokenList []any, st *turnstileState) {
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
			fn(args, process, st)
		})()
	}
}

func turnstileOp1(args []any, process map[float64]any, _ *turnstileState) {
	e, t := slotID(args, 0), slotID(args, 1)
	process[e] = xorString(turnstileStr(process[e]), turnstileStr(process[t]))
}

func turnstileOp2(args []any, process map[float64]any, _ *turnstileState) {
	e := slotID(args, 0)
	if len(args) >= 2 {
		process[e] = args[1]
	}
}

func turnstileOp3(args []any, process map[float64]any, st *turnstileState) {
	e := slotID(args, 0)
	st.result = base64.StdEncoding.EncodeToString([]byte(turnstileStr(process[e])))
}

func turnstileOp5(args []any, process map[float64]any, _ *turnstileState) {
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

func turnstileOp6(args []any, process map[float64]any, _ *turnstileState) {
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

func turnstileOp7(args []any, process map[float64]any, _ *turnstileState) {
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

func turnstileOp8(args []any, process map[float64]any, _ *turnstileState) {
	e, t := slotID(args, 0), slotID(args, 1)
	process[e] = process[t]
}

func turnstileOp14(args []any, process map[float64]any, _ *turnstileState) {
	e, t := slotID(args, 0), slotID(args, 1)
	var v any
	if err := json.Unmarshal([]byte(turnstileStr(process[t])), &v); err == nil {
		process[e] = v
	}
}

func turnstileOp15(args []any, process map[float64]any, _ *turnstileState) {
	e, t := slotID(args, 0), slotID(args, 1)
	b, _ := json.Marshal(process[t])
	process[e] = string(b)
}

func turnstileOp17(st *turnstileState) opcode {
	return func(args []any, process map[float64]any, state *turnstileState) {
		e := slotID(args, 0)
		t := slotID(args, 1)
		target, _ := process[t].(string)
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
}

func turnstileOp18(args []any, process map[float64]any, _ *turnstileState) {
	e := slotID(args, 0)
	b, err := base64.StdEncoding.DecodeString(turnstileStr(process[e]))
	if err == nil {
		process[e] = string(b)
	}
}

func turnstileOp19(args []any, process map[float64]any, _ *turnstileState) {
	e := slotID(args, 0)
	process[e] = base64.StdEncoding.EncodeToString([]byte(turnstileStr(process[e])))
}

func turnstileOp20(args []any, process map[float64]any, _ *turnstileState) {
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

func turnstileOp21(_ []any, _ map[float64]any, _ *turnstileState) {
	// No-op placeholder opcode in the Turnstile VM bytecode.
}

func turnstileOp23(args []any, process map[float64]any, _ *turnstileState) {
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

func turnstileOp24(args []any, process map[float64]any, _ *turnstileState) {
	e, t, n := slotID(args, 0), slotID(args, 1), slotID(args, 2)
	tv, _ := process[t].(string)
	nv, _ := process[n].(string)
	process[e] = tv + "." + nv
}