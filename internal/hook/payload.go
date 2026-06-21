package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"PseudoClaude/internal/permission"
)

type Payload map[string]any

func NewPayload(event Event, sessionID, cwd string, mode permission.Mode) Payload {
	return Payload{
		"event":      string(event),
		"session_id": sessionID,
		"cwd":        cwd,
		"mode":       mode.String(),
	}
}

func (p Payload) With(key string, value any) Payload {
	if p == nil {
		p = Payload{}
	}
	p[key] = value
	return p
}

func (p Payload) JSON() ([]byte, error) {
	var b bytes.Buffer
	if err := writeStableJSON(&b, map[string]any(p)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func GetStringByPath(payload Payload, path string) string {
	var cur any = map[string]any(payload)
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return stringify(cur)
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			if p, ok := cur.(Payload); ok {
				obj = map[string]any(p)
			} else {
				return ""
			}
		}
		next, ok := obj[part]
		if !ok {
			return ""
		}
		cur = next
	}
	return stringify(cur)
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func writeStableJSON(b *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case Payload:
		return writeStableJSON(b, map[string]any(v))
	case map[string]any:
		b.WriteByte('{')
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			keyData, _ := json.Marshal(key)
			b.Write(keyData)
			b.WriteByte(':')
			if err := writeStableJSON(b, v[key]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeStableJSON(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(data)
	}
	return nil
}
