package taskexec

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func writeJSONFile(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func readJSONFile[T any](path string) (T, error) {
	var out T
	body, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(trimUTF8BOM(body), &out)
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= len(utf8BOM) && body[0] == utf8BOM[0] && body[1] == utf8BOM[1] && body[2] == utf8BOM[2] {
		return body[len(utf8BOM):]
	}
	return body
}
