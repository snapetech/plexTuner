package tuner

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// BrowserCookiesForHost extracts cookies for the given hostname (and parent domains)
// from all Chrome and Firefox profiles found on the local machine.
// Returns a best-effort result — any error is silently skipped.
func BrowserCookiesForHost(host string) []*http.Cookie {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return nil
	}
	var cookies []*http.Cookie
	for _, dbPath := range findChromeCookieDBs() {
		cc, err := readChromeCookies(dbPath, host)
		if err != nil {
			log.Printf("cf-bootstrap: chrome cookies %q: %v", dbPath, err)
			continue
		}
		cookies = append(cookies, cc...)
	}
	for _, dbPath := range findFirefoxCookieDBs() {
		fc, err := readFirefoxCookies(dbPath, host)
		if err != nil {
			log.Printf("cf-bootstrap: firefox cookies %q: %v", dbPath, err)
			continue
		}
		cookies = append(cookies, fc...)
	}
	return cookies
}

// findChromeCookieDBs returns file paths to Chrome/Chromium cookie databases on the local machine.
func findChromeCookieDBs() []string {
	home, _ := os.UserHomeDir()
	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{
			filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"),
			filepath.Join(home, ".config", "google-chrome-beta", "Default", "Cookies"),
			filepath.Join(home, ".config", "chromium", "Default", "Cookies"),
			filepath.Join(home, ".config", "chromium-browser", "Default", "Cookies"),
			filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "Default", "Cookies"),
		}
	case "darwin":
		candidates = []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"),
			filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Cookies"),
			filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "Cookies"),
		}
	}
	var found []string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	return found
}

// findFirefoxCookieDBs returns file paths to Firefox cookie databases on the local machine.
func findFirefoxCookieDBs() []string {
	home, _ := os.UserHomeDir()
	var profileBase string
	switch runtime.GOOS {
	case "linux":
		profileBase = filepath.Join(home, ".mozilla", "firefox")
	case "darwin":
		profileBase = filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")
	default:
		return nil
	}
	entries, err := os.ReadDir(profileBase)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(profileBase, e.Name(), "cookies.sqlite")
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	return found
}

// readChromeCookies reads cookies for host from a Chrome/Chromium SQLite cookie database.
// It tries both the plaintext value and the v10 (Linux PBKDF2/AES) encrypted value.
func readChromeCookies(dbPath, host string) ([]*http.Cookie, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_journal=wal")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// Match host_key: exact match or parent domain match (Chrome stores as .example.com)
	rows, err := db.Query(
		`SELECT host_key, path, name, value, encrypted_value, is_secure, expires_utc
		 FROM cookies
		 WHERE host_key = ? OR host_key = ? OR host_key = ?`,
		host, "."+host, strings.Join(hostLabels(host), ""),
	)
	if err != nil {
		// Older Chrome schemas may differ — try broader query.
		rows, err = db.Query(
			`SELECT host_key, path, name, value, encrypted_value, is_secure, expires_utc
			 FROM cookies WHERE instr(?, host_key) > 0 OR host_key = ?`,
			"."+host, host,
		)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	now := time.Now().Unix()
	var cookies []*http.Cookie
	for rows.Next() {
		var hostKey, path, name, value string
		var encValue []byte
		var isSecure, expiresUTC int64
		if err := rows.Scan(&hostKey, &path, &name, &value, &encValue, &isSecure, &expiresUTC); err != nil {
			continue
		}
		if value == "" && len(encValue) > 0 {
			if dec, err := decryptChromeCookieLinux(encValue); err == nil {
				value = dec
			}
		}
		if value == "" {
			continue
		}
		// Chrome expires_utc is microseconds since 1601-01-01.
		var expiresUnix int64
		if expiresUTC > 0 {
			expiresUnix = (expiresUTC / 1000000) - 11644473600
		}
		if expiresUnix > 0 && expiresUnix < now {
			continue // expired
		}
		if name == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:     name,
			Value:    value,
			Domain:   strings.TrimPrefix(hostKey, "."),
			Path:     path,
			Secure:   isSecure == 1,
			HttpOnly: false,
			Expires:  time.Unix(expiresUnix, 0),
		})
	}
	return cookies, rows.Err()
}

// readFirefoxCookies reads cookies from a Firefox SQLite cookie database (no encryption on Linux/macOS).
func readFirefoxCookies(dbPath, host string) ([]*http.Cookie, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_journal=wal")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT host, path, name, value, isSecure, expiry FROM moz_cookies
		 WHERE host = ? OR host = ?`,
		host, "."+host,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().Unix()
	var cookies []*http.Cookie
	for rows.Next() {
		var host2, path, name, value string
		var isSecure, expiry int64
		if err := rows.Scan(&host2, &path, &name, &value, &isSecure, &expiry); err != nil {
			continue
		}
		if expiry > 0 && expiry < now {
			continue
		}
		if name == "" || value == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:    name,
			Value:   value,
			Domain:  strings.TrimPrefix(host2, "."),
			Path:    path,
			Secure:  isSecure == 1,
			Expires: time.Unix(expiry, 0),
		})
	}
	return cookies, rows.Err()
}

// decryptChromeCookieLinux decrypts a Chrome v10 cookie value using AES-128-CBC with a key
// derived from the static passphrase "peanuts" and salt "saltysalt" via a single PBKDF2 iteration.
// This is the standard Linux Chrome cookie encryption when no Gnome Keyring / KDE Wallet is used.
// Returns the plaintext string, or an error if the value is not v10-encrypted.
func decryptChromeCookieLinux(enc []byte) (string, error) {
	if !bytes.HasPrefix(enc, []byte("v10")) && !bytes.HasPrefix(enc, []byte("v11")) {
		// Not v10/v11 — may be plaintext or an unsupported scheme; return as-is.
		if len(enc) > 0 {
			return string(enc), nil
		}
		return "", nil
	}
	ciphertext := enc[3:] // strip "v10" or "v11" prefix
	key := pbkdf2OneIter([]byte("peanuts"), []byte("saltysalt"), 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", nil
	}
	iv := bytes.Repeat([]byte(" "), aes.BlockSize) // Chrome uses 0x20 repeated
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)
	// Remove PKCS7 padding.
	if len(plaintext) == 0 {
		return "", nil
	}
	pad := int(plaintext[len(plaintext)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plaintext) {
		return string(plaintext), nil // decryption may have failed silently; return raw
	}
	return string(plaintext[:len(plaintext)-pad]), nil
}

// pbkdf2OneIter is a minimal PBKDF2-SHA1 implementation for a single iteration (Chrome's case).
// Avoids pulling in golang.org/x/crypto: one iteration means T_1 = HMAC-SHA1(password, salt || 0x00000001).
func pbkdf2OneIter(password, salt []byte, keyLen int) []byte {
	mac := hmac.New(sha1.New, password)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1}) // INT(1) big-endian
	t := mac.Sum(nil)
	if keyLen > len(t) {
		keyLen = len(t)
	}
	return t[:keyLen]
}

// hostLabels builds parent domain patterns for SQL matching, e.g. "a.b.c" → [".b.c", ".c"].
// Used to match cookies stored at parent domains.
func hostLabels(host string) []string {
	parts := strings.Split(host, ".")
	var out []string
	for i := 1; i < len(parts)-1; i++ {
		out = append(out, "."+strings.Join(parts[i:], "."))
	}
	return out
}
