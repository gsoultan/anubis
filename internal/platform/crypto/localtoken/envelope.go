package localtoken

import "encoding/json"

type envelope struct {
	Purpose string          `json:"p"`
	Expires int64           `json:"exp"`
	TokenID string          `json:"jti"`
	Data    json.RawMessage `json:"d"`
}
