package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

func TestWebHTTPSyncAndLiveStoreList(t *testing.T) {
	profileID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	server := NewWebServer()
	server.Clients = fakeClients{byAccount: map[string]accountClient{
		"sa01": &fakeAccountClient{stores: []rtasales.Store{
			{BusinessID: "107", Label: "107 - Central"},
		}},
	}}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	base, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	syncBody, _ := json.Marshal(webSyncRequest{
		Profiles: []webSyncProfile{{
			ID: profileID, DisplayName: "店長", Enabled: true, Priority: 0,
		}},
		Secrets: map[string]webSyncSecret{
			profileID: {Account: "sa01", Password: "secret"},
		},
	})
	resp, err := client.Post(base.JoinPath("/api/session").String(), "application/json", bytes.NewReader(syncBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("sync status %d: %s", resp.StatusCode, body)
	}

	rpcBody, _ := json.Marshal(webRPCRequest{
		Method: "ListSalesAnalysisStores",
		Args:   []json.RawMessage{json.RawMessage(`{"profileId":"` + profileID + `"}`)},
	})
	resp, err = client.Post(base.JoinPath("/api/rpc").String(), "application/json", bytes.NewReader(rpcBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("rpc status %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Result []SalesAnalysisStore `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Result) != 1 || payload.Result[0].BusinessID != "107" {
		t.Fatalf("stores = %#v", payload.Result)
	}

	testBody, _ := json.Marshal(webRPCRequest{
		Method: "TestProfile",
		Args:   []json.RawMessage{json.RawMessage(`{"profileId":"` + profileID + `"}`)},
	})
	resp, err = client.Post(base.JoinPath("/api/rpc").String(), "application/json", bytes.NewReader(testBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("test status %d: %s", resp.StatusCode, body)
	}
}

func TestWebHTTPSyncDropsRemovedProfileSecrets(t *testing.T) {
	keptID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	droppedID := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	server := NewWebServer()
	server.Clients = fakeClients{byAccount: map[string]accountClient{
		"sa01": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "107", Label: "107 - Central"}}},
		"sa02": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "108", Label: "108 - South"}}},
	}}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	base, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	syncProfiles := func(profiles []webSyncProfile, secrets map[string]webSyncSecret) {
		t.Helper()
		body, _ := json.Marshal(webSyncRequest{Profiles: profiles, Secrets: secrets})
		resp, err := client.Post(base.JoinPath("/api/session").String(), "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(resp.Body)
			t.Fatalf("sync status %d: %s", resp.StatusCode, payload)
		}
	}
	syncProfiles([]webSyncProfile{
		{ID: keptID, DisplayName: "Keep", Enabled: true, Priority: 0},
		{ID: droppedID, DisplayName: "Drop", Enabled: true, Priority: 1},
	}, map[string]webSyncSecret{
		keptID:    {Account: "sa01", Password: "secret"},
		droppedID: {Account: "sa02", Password: "secret"},
	})
	syncProfiles([]webSyncProfile{
		{ID: keptID, DisplayName: "Keep", Enabled: true, Priority: 0},
	}, map[string]webSyncSecret{
		keptID: {Account: "sa01", Password: "secret"},
	})

	server.sessions.Lock()
	var session *webSession
	for _, item := range server.byID {
		session = item
	}
	server.sessions.Unlock()
	if session == nil {
		t.Fatal("missing web session")
	}
	store, ok := session.app.credentials.(*securestore.MemoryCredentialStore)
	if !ok {
		t.Fatal("web session credential store is unavailable")
	}
	if _, err := store.Get(droppedID); !errors.Is(err, securestore.ErrNotFound) {
		t.Fatal("removed profile secret remained in the web session")
	}

	rpc := func(profileID string) int {
		t.Helper()
		body, _ := json.Marshal(webRPCRequest{
			Method: "TestProfile",
			Args:   []json.RawMessage{json.RawMessage(`{"profileId":"` + profileID + `"}`)},
		})
		resp, err := client.Post(base.JoinPath("/api/rpc").String(), "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if status := rpc(keptID); status != http.StatusOK {
		t.Fatalf("kept profile test status %d", status)
	}
	if status := rpc(droppedID); status == http.StatusOK {
		t.Fatal("removed profile still had usable credentials")
	}
}

func TestWebHTTPSyncInvalidatesCookieOnCredentialChange(t *testing.T) {
	profileID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	server := NewWebServer()
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	base, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	syncSecrets := func(account, password string) {
		t.Helper()
		body, _ := json.Marshal(webSyncRequest{
			Profiles: []webSyncProfile{{ID: profileID, DisplayName: "Keep", Enabled: true}},
			Secrets:  map[string]webSyncSecret{profileID: {Account: account, Password: password}},
		})
		resp, err := client.Post(base.JoinPath("/api/session").String(), "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(resp.Body)
			t.Fatalf("sync status %d: %s", resp.StatusCode, payload)
		}
	}
	syncSecrets("sa01", "secret-one")
	server.sessions.Lock()
	var session *webSession
	for _, item := range server.byID {
		session = item
	}
	server.sessions.Unlock()
	if session == nil {
		t.Fatal("missing web session")
	}
	store, err := session.app.cookies.CookieStore(profileID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save([]byte(`{"cookies":[]}`)); err != nil {
		t.Fatal(err)
	}
	syncSecrets("sa01", "secret-one")
	kept, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) == 0 {
		t.Fatal("unchanged credentials dropped the cookie session")
	}
	syncSecrets("sa02", "secret-two")
	cleared, err := session.app.cookies.CookieStore(profileID)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := cleared.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("changed credentials kept previous cookie session: %q", loaded)
	}
}

func TestSSEHubUnsubscribeAfterCloseIsIdempotent(t *testing.T) {
	hub := newSSEHub()
	ch := hub.Subscribe()
	hub.Close()
	hub.Unsubscribe(ch)
}

func TestWebHTTPRejectsPathEscapeAndOversizedUpload(t *testing.T) {
	server := NewWebServer()
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", `quote".xlsx`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not-really-xlsx")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(httpServer.URL+"/api/upload", writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status %d: %s", resp.StatusCode, payload)
	}
	var uploaded struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}

	resp, err = client.Get(httpServer.URL + "/api/download?path=" + url.QueryEscape(uploaded.Path))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status %d", resp.StatusCode)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if disposition == "" || strings.ContainsAny(disposition, "\r\n") || strings.Contains(disposition, `quote"`) {
		t.Fatalf("Content-Disposition is not quoted safely: %q", disposition)
	}

	outside := filepath.Join(os.TempDir(), "rta-web-escape.xlsx")
	resp, err = client.Get(httpServer.URL + "/api/download?path=" + url.QueryEscape(outside))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("download escaped the session directory")
	}

	var oversized bytes.Buffer
	oversizedWriter := multipart.NewWriter(&oversized)
	oversizedPart, err := oversizedWriter.CreateFormFile("file", "huge.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oversizedPart.Write(bytes.Repeat([]byte("a"), maximumWebUpload+8)); err != nil {
		t.Fatal(err)
	}
	if err := oversizedWriter.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err = client.Post(httpServer.URL+"/api/upload", oversizedWriter.FormDataContentType(), &oversized)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("oversize upload status %d: %s", resp.StatusCode, payload)
	}
}

func TestWebHTTPStaticStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := NewWebServer()
	server.Static = root
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp, err := http.Get(httpServer.URL + "/../web_http.go")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if bytes.Contains(body, []byte("package desktop")) {
		t.Fatal("static handler served a file outside the static root")
	}
}
