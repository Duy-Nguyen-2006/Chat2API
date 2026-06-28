package chatgpt

import (
	"crypto/rand"
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	mrand "math/rand"
	"time"
)

const maxPowIterations = 500000

var (
	powCores         = []int{8, 16, 24, 32}
	powCachedScripts = []string{
		"https://cdn.oaistatic.com/_next/static/cXh69klOLzS0Gy2joLDRS/_ssgManifest.js?dpl=453ebaec0d44c2decab71692e1bfe39be35a24b3",
	}
	powCachedDPL = "prod-f501fe933b3edf57aea882da888e1a544df99840"
	powNavigator = []string{
		"webdriver−false",
		"hardwareConcurrency−32",
		"language−en-US",
	}
	powDocument = []string{"location"}
	powWindow   = []string{"window", "document", "navigator"}
)

type powConfig struct {
	values []any
}

func newPowConfig(userAgent string) powConfig {
	now := time.Now().In(time.FixedZone("EST", -5*3600))
	parseTime := now.Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
	perf := float64(time.Now().UnixNano()) / 1e6

	return powConfig{values: []any{
		powCores[mrand.Intn(len(powCores))] + 1920 + 1080,
		parseTime,
		4294705152,
		0,
		userAgent,
		powCachedScripts[mrand.Intn(len(powCachedScripts))],
		powCachedDPL,
		"en-US",
		"en-US,es-US,en,es",
		0,
		powNavigator[mrand.Intn(len(powNavigator))],
		powDocument[mrand.Intn(len(powDocument))],
		powWindow[mrand.Intn(len(powWindow))],
		perf,
		newUUID(),
		"",
		powCores[mrand.Intn(len(powCores))],
		float64(time.Now().UnixMilli()) - perf,
	}}
}

func (c powConfig) staticParts() ([]byte, []byte, []byte, error) {
	part1, err := json.Marshal(c.values[:3])
	if err != nil {
		return nil, nil, nil, err
	}
	part1Bytes := append(part1[:len(part1)-1], ',')

	mid, err := json.Marshal(c.values[4:9])
	if err != nil {
		return nil, nil, nil, err
	}
	part2Bytes := append([]byte{','}, mid[1:len(mid)-1]...)
	part2Bytes = append(part2Bytes, ',')

	tail, err := json.Marshal(c.values[10:])
	if err != nil {
		return nil, nil, nil, err
	}
	part3Bytes := append([]byte{','}, tail[1:]...)

	return part1Bytes, part2Bytes, part3Bytes, nil
}

func generatePowAnswer(seed, diff string, cfg powConfig) (string, bool) {
	diffLen := len(diff)
	seedEncoded := []byte(seed)

	part1, part2, part3, err := cfg.staticParts()
	if err != nil {
		return "", false
	}

	target, err := hex.DecodeString(diff)
	if err != nil {
		return "", false
	}

	for i := 0; i < maxPowIterations; i++ {
		dynamicI := []byte(fmt.Sprintf("%d", i))
		dynamicJ := []byte(fmt.Sprintf("%d", i>>1))

		payload := make([]byte, 0, len(part1)+len(dynamicI)+len(part2)+len(dynamicJ)+len(part3))
		payload = append(payload, part1...)
		payload = append(payload, dynamicI...)
		payload = append(payload, part2...)
		payload = append(payload, dynamicJ...)
		payload = append(payload, part3...)

		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(payload)))
		base64.StdEncoding.Encode(encoded, payload)

		hash := sha3.Sum512(append(seedEncoded, encoded...))
		if compareHashPrefix(hash[:], diffLen, target) {
			return string(encoded), true
		}
	}

	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func compareHashPrefix(hash []byte, diffLen int, target []byte) bool {
	if diffLen > len(hash) {
		diffLen = len(hash)
	}
	return string(hash[:diffLen]) <= string(target)
}

func randomSeed() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	f := math.Float64frombits(uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]))
	return fmt.Sprintf("%v", f)
}

func requirementsToken(userAgent string) string {
	cfg := newPowConfig(userAgent)
	answer, _ := generatePowAnswer(randomSeed(), "0fffff", cfg)
	return "gAAAAAC" + answer
}

func answerToken(seed, diff, userAgent string) (string, bool) {
	cfg := newPowConfig(userAgent)
	answer, solved := generatePowAnswer(seed, diff, cfg)
	if !solved {
		return "", false
	}
	return "gAAAAAB" + answer, true
}