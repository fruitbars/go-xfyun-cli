package auth

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignedURLUsesGMTAndRequestPath(t *testing.T) {
	now := time.Date(2025, 9, 18, 12, 34, 56, 0, time.FixedZone("CST", 8*60*60))
	signed, err := SignedURL("https://example.com/v1/private/test", "GET", "key", "secret", now)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := u.Query().Get("date"), "Thu, 18 Sep 2025 04:34:56 GMT"; got != want {
		t.Fatalf("date = %q, want %q", got, want)
	}
	authorization, err := base64.StdEncoding.DecodeString(u.Query().Get("authorization"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(authorization), `api_key="key"`) || !strings.Contains(string(authorization), `algorithm="hmac-sha256"`) {
		t.Fatalf("unexpected authorization: %s", authorization)
	}
	if u.Query().Get("host") != "example.com" {
		t.Fatalf("host = %q", u.Query().Get("host"))
	}
}

func TestHMACSHA1IsStableAndIgnoresEmptyValues(t *testing.T) {
	paramsA := map[string]string{"z": "last", "a": "first value", "empty": "", "signature": "old"}
	paramsB := map[string]string{"a": "first value", "z": "last"}
	gotA := HMACSHA1(paramsA, "secret")
	gotB := HMACSHA1(paramsB, "secret")
	if gotA != gotB {
		t.Fatalf("signatures differ: %q != %q", gotA, gotB)
	}
	if gotA == "" {
		t.Fatal("signature is empty")
	}
}

func TestStandardIFASRSignaMatchesOfficialExample(t *testing.T) {
	got := StandardIFASRSigna("595f23df", "1512041814", "d9f4aa7ea6d94faca62cd88a28fd5234")
	if want := "IrrzsJeOFk1NGfJHW6SkHUoN9CU="; got != want {
		t.Fatalf("signa = %q, want %q", got, want)
	}
}
