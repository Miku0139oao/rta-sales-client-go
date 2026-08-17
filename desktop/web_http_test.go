package desktop

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
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
