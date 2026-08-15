package rtasales

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	persistentjar "github.com/juju/persistent-cookiejar"
)

const maximumStoredCookieBytes = 16 << 20

// CookieStore persists an opaque JSON representation of a client's cookies.
// Implementations must be safe for sequential Load and Save calls. Desktop
// callers should use an OS-protected implementation rather than a plaintext
// file.
type CookieStore interface {
	Load() ([]byte, error)
	Save([]byte) error
}

// storedCookieEntry mirrors the stable JSON representation exported by
// persistent-cookiejar. Keeping this adapter here lets secure stores encrypt
// bytes directly without ever creating a plaintext cookie file.
type storedCookieEntry struct {
	Name          string
	Value         string
	Domain        string
	Path          string
	Secure        bool
	HTTPOnly      bool `json:"HttpOnly"`
	Persistent    bool
	HostOnly      bool
	Expires       time.Time
	CanonicalHost string
}

func cookieJarFromStore(store CookieStore) (http.CookieJar, func() error, error) {
	if nilCookieStore(store) {
		return nil, nil, &InputError{Field: "CookieStore", Message: "must not be nil"}
	}
	jar, err := persistentjar.New(&persistentjar.Options{NoPersist: true})
	if err != nil {
		return nil, nil, fmt.Errorf("create secure cookie jar: %w", err)
	}
	encoded, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load secure cookie jar: %w", err)
	}
	if len(encoded) > maximumStoredCookieBytes {
		return nil, nil, fmt.Errorf("load secure cookie jar: data exceeds 16 MiB")
	}
	if len(encoded) > 0 {
		var entries []storedCookieEntry
		if err := json.Unmarshal(encoded, &entries); err != nil {
			return nil, nil, fmt.Errorf("load secure cookie jar: invalid data: %w", err)
		}
		for _, entry := range entries {
			host := strings.TrimSpace(entry.CanonicalHost)
			if host == "" || strings.ContainsAny(host, "/\\@") {
				return nil, nil, fmt.Errorf("load secure cookie jar: invalid canonical host")
			}
			path := entry.Path
			if path == "" || path[0] != '/' {
				path = "/"
			}
			domain := entry.Domain
			if entry.HostOnly {
				domain = ""
			}
			jar.SetCookies(&url.URL{Scheme: "https", Host: host, Path: path}, []*http.Cookie{{
				Name:     entry.Name,
				Value:    entry.Value,
				Domain:   domain,
				Path:     path,
				Expires:  entry.Expires,
				Secure:   entry.Secure,
				HttpOnly: entry.HTTPOnly,
			}})
		}
	}
	save := func() error {
		encoded, err := jar.MarshalJSON()
		if err != nil {
			return fmt.Errorf("encode secure cookie jar: %w", err)
		}
		if err := store.Save(encoded); err != nil {
			return fmt.Errorf("save secure cookie jar: %w", err)
		}
		return nil
	}
	return jar, save, nil
}

func nilCookieStore(store CookieStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
