package ingest

import (
	"encoding/json"
	"strconv"
	"strings"
)

// jsonNumberAlias decodes a JSON number or numeric string.
type jsonNumberAlias float64

func (n *jsonNumberAlias) UnmarshalJSON(raw []byte) error {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = 0
		return nil
	}
	*n = jsonNumberAlias(f)
	return nil
}

func (n jsonNumberAlias) Float64() float64 { return float64(n) }

func (n jsonNumberAlias) Int64() (int64, bool) {
	if n == 0 {
		return 0, false
	}
	return int64(n), true
}

// jsonUnmarshal decodes JSON, tolerating a leading BOM.
func jsonUnmarshal(body []byte, v any) error {
	body = bytesTrimBOM(body)
	return json.Unmarshal(body, v)
}

func bytesTrimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
