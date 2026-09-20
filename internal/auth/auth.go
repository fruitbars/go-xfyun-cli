package auth

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// StandardIFASRSigna creates the signa used by the legacy recording
// transcription API: HMAC-SHA1(MD5(appID + timestamp), secretKey).
func StandardIFASRSigna(appID, timestamp, secretKey string) string {
	digest := md5.Sum([]byte(appID + timestamp))
	baseString := hex.EncodeToString(digest[:])
	mac := hmac.New(sha1.New, []byte(secretKey))
	_, _ = mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// SignedURL creates the HMAC-SHA256 URL used by XFYun WebAPI endpoints.
func SignedURL(rawURL, method, apiKey, apiSecret string, now time.Time) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	// XFYun requires the RFC1123 timestamp to use the literal GMT zone.
	date := now.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	requestLine := strings.ToUpper(method) + " " + u.EscapedPath() + " HTTP/1.1"
	signatureOrigin := "host: " + u.Host + "\ndate: " + date + "\n" + requestLine
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signatureOrigin))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authorizationOrigin := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`, apiKey, signature)

	q := u.Query()
	q.Set("host", u.Host)
	q.Set("date", date)
	q.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authorizationOrigin)))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// HMACSHA1 signs sorted, URL-encoded query parameters for the ASR APIs.
func HMACSHA1(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key != "signature" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(params[key]))
	}
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join(parts, "&")))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Query builds a query string using the same encoding rules as HMACSHA1.
func Query(params map[string]string) string {
	values := make(url.Values, len(params))
	for key, value := range params {
		if value != "" {
			values.Set(key, value)
		}
	}
	return values.Encode()
}
