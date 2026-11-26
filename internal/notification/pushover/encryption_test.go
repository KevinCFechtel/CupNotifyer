package pushover

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// Independently verify the wire format, authentication, padding and compression.
func decryptTestField(encoded string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < 2*aes.BlockSize+sha256.Size {
		return "", fmt.Errorf("invalid encrypted field")
	}
	authenticated, tag := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(authenticated)
	if !hmac.Equal(tag, mac.Sum(nil)) {
		return "", fmt.Errorf("invalid HMAC")
	}
	iv, ciphertext := authenticated[:aes.BlockSize], authenticated[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid ciphertext length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padded := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ciphertext)
	padding := int(padded[len(padded)-1])
	if padding == 0 || padding > aes.BlockSize ||
		!bytes.Equal(padded[len(padded)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return "", fmt.Errorf("invalid PKCS7 padding")
	}
	reader, err := gzip.NewReader(bytes.NewReader(padded[:len(padded)-padding]))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	return string(plain), err
}

func TestNewEncryptionKey(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "disabled", valid: true},
		{name: "lowercase", value: testEncryptionKey, valid: true},
		{name: "uppercase", value: strings.ToUpper(testEncryptionKey), valid: true},
		{name: "short", value: testEncryptionKey[:32]},
		{name: "long", value: testEncryptionKey + "ab"},
		{name: "odd length", value: testEncryptionKey[:63]},
		{name: "non hex", value: strings.Repeat("z", 64)},
		{name: "whitespace", value: " "},
		{name: "surrounding whitespace", value: " " + testEncryptionKey + " "},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, err := New("token", "user", test.value, http.DefaultClient)
			if !test.valid {
				if err == nil || client != nil {
					t.Fatalf("New() = (%v, %v), want nil notifier and error", client, err)
				}
				if strings.TrimSpace(test.value) != "" && strings.Contains(err.Error(), test.value) {
					t.Error("error exposes supplied key")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.value == "" {
				if client.encryptionKey != nil {
					t.Error("empty key does not disable encryption")
				}
			} else {
				want, _ := hex.DecodeString(testEncryptionKey)
				if !bytes.Equal(client.encryptionKey, want) {
					t.Error("decoded key differs from configured key")
				}
			}
			plain, err := New("token", "user", "", http.DefaultClient)
			if err != nil || client.ID() != plain.ID() {
				t.Fatal("encryption changed the notifier ID and would resend old updates")
			}
		})
	}
}

func TestEncryptFieldRoundTrip(t *testing.T) {
	t.Parallel()
	key, _ := hex.DecodeString(testEncryptionKey)
	for _, plaintext := range []string{"", "Secret message", "Grüße 🚨\nServer: home", strings.Repeat("long line ", 100)} {
		t.Run(fmt.Sprintf("length %d", len(plaintext)), func(t *testing.T) {
			t.Parallel()
			first, err := encryptField(plaintext, key, maxMessageRunes)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decryptTestField(first, key)
			if err != nil || got != plaintext {
				t.Fatalf("decrypted = %q, error = %v, want %q", got, err, plaintext)
			}
			second, err := encryptField(plaintext, key, maxMessageRunes)
			if err != nil {
				t.Fatal(err)
			}
			firstBytes, _ := base64.StdEncoding.DecodeString(first)
			secondBytes, _ := base64.StdEncoding.DecodeString(second)
			if bytes.Equal(firstBytes[:aes.BlockSize], secondBytes[:aes.BlockSize]) {
				t.Error("repeated encryption reused an IV")
			}
			for _, offset := range []int{0, aes.BlockSize, len(firstBytes) - 1} {
				tampered := bytes.Clone(firstBytes)
				tampered[offset] ^= 1
				if _, err := decryptTestField(base64.StdEncoding.EncodeToString(tampered), key); err == nil {
					t.Errorf("tampering at offset %d was not detected", offset)
				}
			}
		})
	}
}

func TestEncryptFieldFitsWireLimits(t *testing.T) {
	t.Parallel()
	key, _ := hex.DecodeString(testEncryptionKey)
	for _, limit := range []int{maxTitleRunes, maxMessageRunes} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			var text strings.Builder
			for i := range limit {
				text.WriteRune(rune(0x10000 + i))
			}
			plaintext := text.String()
			encrypted, err := encryptField(plaintext, key, limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(encrypted) > limit {
				t.Errorf("encrypted length = %d, want <= %d", len(encrypted), limit)
			}
			got, err := decryptTestField(encrypted, key)
			if err != nil {
				t.Fatal(err)
			}
			if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") ||
				!strings.HasPrefix(plaintext, strings.TrimSuffix(got, "…")) {
				t.Errorf("not a Unicode-safe truncated prefix: %q", got)
			}
			if utf8.RuneCountInString(got) >= limit {
				t.Error("input did not exercise additional truncation for encryption overhead")
			}
		})
	}
}

func TestEncryptFieldFailsClosed(t *testing.T) {
	t.Parallel()
	key, _ := hex.DecodeString(testEncryptionKey)
	for _, invalidKey := range [][]byte{nil, make([]byte, 16), make([]byte, 24), make([]byte, 33)} {
		if got, err := encryptField("secret", invalidKey, maxMessageRunes); err == nil || got != "" {
			t.Errorf("with %d-byte key = (%q, %v), want empty result and error", len(invalidKey), got, err)
		}
	}
	if got, err := encryptField("secret", key, 1); err == nil || got != "" {
		t.Errorf("with impossible limit = (%q, %v), want empty result and error", got, err)
	}
}

func TestEncryptFieldOpenSSLCompatibility(t *testing.T) {
	t.Parallel()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL is not installed; Go wire-format tests still run")
	}
	key, _ := hex.DecodeString(testEncryptionKey)
	const plaintext = "Pushover interoperability: Grüße 🚨"
	encoded, err := encryptField(plaintext, key, maxMessageRunes)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, tag := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	mac := exec.Command(openssl, "dgst", "-sha256", "-mac", "HMAC", "-macopt", "hexkey:"+testEncryptionKey, "-binary")
	mac.Stdin = bytes.NewReader(authenticated)
	expectedTag, err := mac.Output()
	if err != nil {
		t.Fatalf("OpenSSL HMAC: %v", err)
	}
	if !bytes.Equal(tag, expectedTag) {
		t.Fatal("HMAC differs from OpenSSL")
	}
	decrypt := exec.Command(openssl, "enc", "-d", "-aes-256-cbc", "-K", testEncryptionKey,
		"-iv", hex.EncodeToString(authenticated[:aes.BlockSize]))
	decrypt.Stdin = bytes.NewReader(authenticated[aes.BlockSize:])
	compressed, err := decrypt.Output()
	if err != nil {
		t.Fatalf("OpenSSL AES-256-CBC decryption: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil || string(got) != plaintext {
		t.Fatalf("OpenSSL plaintext = %q, error = %v, want %q", got, err, plaintext)
	}
}

func TestSendEncrypted(t *testing.T) {
	t.Parallel()
	key, _ := hex.DecodeString(testEncryptionKey)
	for _, projectURL := range []string{"https://example.com/private/project", "", "javascript:alert(1)"} {
		t.Run(projectURL, func(t *testing.T) {
			t.Parallel()
			available := update.Available{
				Reference:    "ghcr.io/private/app:latest",
				Server:       "home",
				Kind:         update.KindDigest,
				RemoteDigest: "sha256:new",
				URL:          projectURL,
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Form.Get("encrypted") != "1" || r.Form.Get("token") != "token" || r.Form.Get("user") != "user" {
					t.Error("encrypted flag or routing credentials missing")
				}
				if r.Form.Has("url_title") {
					t.Error("optional URL title must be omitted in E2EE mode")
				}
				wantFields := map[string]string{"title": updateTitle, "message": formatMessage(available)}
				if safeProjectURL(projectURL) != "" {
					wantFields["url"] = projectURL
				} else if r.Form.Has("url") {
					t.Error("missing or unsafe URL was included")
				}
				ivs := map[string]bool{}
				for field, want := range wantFields {
					value := r.Form.Get(field)
					limit := map[string]int{"title": maxTitleRunes, "message": maxMessageRunes, "url": maxURLRunes}[field]
					if len(value) > limit {
						t.Errorf("%s wire length = %d, want <= %d", field, len(value), limit)
					}
					got, err := decryptTestField(value, key)
					if err != nil || got != want {
						t.Errorf("%s decrypted = %q, error = %v, want %q", field, got, err, want)
					}
					data, err := base64.StdEncoding.DecodeString(value)
					if err == nil && len(data) >= aes.BlockSize {
						iv := string(data[:aes.BlockSize])
						if ivs[iv] {
							t.Error("fields share an IV")
						}
						ivs[iv] = true
					}
				}
				if strings.Contains(r.PostForm.Encode(), testEncryptionKey) {
					t.Error("request exposes encryption key")
				}
				_, _ = w.Write([]byte(`{"status":1}`))
			}))
			defer server.Close()
			notifier, err := New("token", "user", testEncryptionKey, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			notifier.apiURL = server.URL
			if err := notifier.Send(context.Background(), available); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSendEncryptionFailureDoesNotSend(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		badKey  bool
		longURL bool
	}{
		{name: "invalid internal key", badKey: true},
		{name: "URL cannot fit without truncation", longURL: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A tiny URL limit is not configurable; use incompressible data to
			// exercise the actual 512-character limit without breaking the URL.
			var projectURL string
			if test.longURL {
				var path strings.Builder
				for i := range 11 {
					hash := sha256.Sum256([]byte(fmt.Sprint(i)))
					path.WriteString(base64.RawURLEncoding.EncodeToString(hash[:]))
				}
				projectURL = "https://example.com/" + path.String()
				if len(projectURL) > maxURLRunes {
					t.Fatal("test URL already exceeds the plaintext limit")
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("encryption failure issued an HTTP request")
				_, _ = w.Write([]byte(`{"status":1}`))
			}))
			defer server.Close()
			notifier, err := New("token", "user", testEncryptionKey, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			notifier.apiURL = server.URL
			if test.badKey {
				notifier.encryptionKey = []byte("invalid")
			}
			err = notifier.send(context.Background(), updateTitle, "Secret message", projectURL)
			if err == nil || !strings.Contains(err.Error(), "encrypt Pushover") {
				t.Fatalf("send() = %v, want encryption error", err)
			}
			if strings.Contains(err.Error(), "Secret message") || strings.Contains(err.Error(), projectURL) && projectURL != "" {
				t.Fatal("error exposes notification content")
			}
		})
	}
}
