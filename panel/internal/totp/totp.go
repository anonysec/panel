package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

func GenerateCode(secret string, t time.Time) (string, error) {
	secret = strings.ToUpper(strings.TrimRight(secret, "="))
	for len(secret)%8 != 0 {
		secret += "="
	}
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(math.Floor(float64(t.Unix()) / 30))
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000), nil
}

func Verify(secret, code string) bool {
	now := time.Now()
	for _, offset := range []int{-1, 0, 1} {
		t := now.Add(time.Duration(offset*30) * time.Second)
		expected, err := GenerateCode(secret, t)
		if err != nil {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

func QRCodeURL(issuer, username, secret string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30",
		issuer, username, secret, issuer)
}
