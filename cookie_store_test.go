package rtasales

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type testCookieStore struct {
	data []byte
}

func (s *testCookieStore) Load() ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}

func (s *testCookieStore) Save(data []byte) error {
	s.data = append(s.data[:0], data...)
	return nil
}

func TestCookieStoreRoundTrip(t *testing.T) {
	store := new(testCookieStore)
	jar, save, err := cookieJarFromStore(store)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://example.test/report")
	jar.SetCookies(target, []*http.Cookie{{
		Name:     "session",
		Value:    "secret-cookie-value",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		Expires:  time.Now().Add(time.Hour),
	}})
	if err := save(); err != nil {
		t.Fatal(err)
	}

	reloaded, _, err := cookieJarFromStore(store)
	if err != nil {
		t.Fatal(err)
	}
	cookies := reloaded.Cookies(target)
	if len(cookies) != 1 || cookies[0].Name != "session" || cookies[0].Value != "secret-cookie-value" {
		t.Fatalf("unexpected reloaded cookies: %#v", cookies)
	}
}

func TestCookieStoreAndCookieFileAreMutuallyExclusive(t *testing.T) {
	_, _, err := clientCookieJar(Config{CookieStore: new(testCookieStore), CookieFile: "cookies.json"})
	if err == nil {
		t.Fatal("expected mutually exclusive cookie storage error")
	}
}

func TestTypedNilCookieStoreIsRejected(t *testing.T) {
	var store *testCookieStore
	_, _, err := clientCookieJar(Config{CookieStore: store})
	var inputError *InputError
	if !errors.As(err, &inputError) || inputError.Field != "CookieStore" {
		t.Fatalf("error=%T %v, want CookieStore InputError", err, err)
	}
}
