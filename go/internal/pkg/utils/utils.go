package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// GenerateUUID generates a UUID v4 string.
func GenerateUUID() string {
	return uuid.New().String()
}

// GenerateOrderID generates a unique order ID for payments.
func GenerateOrderID() string {
	return fmt.Sprintf("ORD-%d-%s", time.Now().UnixMilli(), RandomHex(4))
}

// RandomHex generates a random hex string of n bytes.
func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RandomCode generates a random alphanumeric code of given length.
func RandomCode(length int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// GenerateUsername creates a username for VPN configs.
// Matches PHP's generateUsername() behavior.
func GenerateUsername(prefix string) string {
	code := RandomCode(6)
	if prefix != "" {
		return prefix + "_" + code
	}
	return "user_" + code
}

// FormatBytes converts bytes to human-readable format.
// Matches PHP's formatBytes() function.
func FormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(bytes)
	i := 0
	for size >= 1024 && i < len(units)-1 {
		size /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", size, units[i])
}

// ConvertPersianToEnglish converts Persian/Arabic numerals to English.
// Matches PHP's convertPersianNumbersToEnglish().
func ConvertPersianToEnglish(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹':
			result.WriteRune(r - '۰' + '0')
		case r >= '٠' && r <= '٩':
			result.WriteRune(r - '٠' + '0')
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ParseInt safely converts string to int with a default value.
func ParseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(ConvertPersianToEnglish(s))
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// ParseInt64 safely converts string to int64.
func ParseInt64(s string, defaultVal int64) int64 {
	s = strings.TrimSpace(ConvertPersianToEnglish(s))
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

// FormatNumber adds comma separators to a number.
func FormatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	if neg {
		return "-" + result.String()
	}
	return result.String()
}

// IsNumeric checks if a string is numeric.
func IsNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// SanitizeUsername sanitizes a username for VPN usage.
func SanitizeUsername(username string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return re.ReplaceAllString(username, "")
}

// NowUnix returns current Unix timestamp as string.
func NowUnix() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// TimeAgo returns human-readable time difference.
func TimeAgo(timestamp int64) string {
	diff := time.Now().Unix() - timestamp
	switch {
	case diff < 60:
		return fmt.Sprintf("%d seconds ago", diff)
	case diff < 3600:
		return fmt.Sprintf("%d minutes ago", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%d hours ago", diff/3600)
	default:
		return fmt.Sprintf("%d days ago", diff/86400)
	}
}

// GBToBytes converts gigabytes to bytes.
func GBToBytes(gb float64) int64 {
	return int64(gb * 1024 * 1024 * 1024)
}

// BytesToGB converts bytes to gigabytes.
func BytesToGB(bytes int64) float64 {
	return float64(bytes) / (1024 * 1024 * 1024)
}
