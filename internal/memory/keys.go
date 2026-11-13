package memory

import (
	"crypto/sha256"
	"encoding/hex"
)

func dupKey(userID, content string) string {
	sum := sha256.Sum256([]byte(content))
	return "memdup:" + userID + ":" + hex.EncodeToString(sum[:12])
}
