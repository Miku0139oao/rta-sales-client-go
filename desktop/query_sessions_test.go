package desktop

import (
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"testing"
	"time"
)

func TestQuerySessionsReuseAndInvalidate(t *testing.T) {
	factory := &sequenceClients{clients: []accountClient{
		&fakeAccountClient{}, &fakeAccountClient{}, &fakeAccountClient{}, &fakeAccountClient{}, &fakeAccountClient{},
	}}
	app := &App{clients: factory, cookies: &fakeCookies{}, querySessions: make(map[string]querySessionPool)}
	credential := securestore.Credential{Account: "account", Password: "secret"}
	first, err := app.reusableQuerySessions("profile", credential, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.reusableQuerySessions("profile", credential, 2)
	if err != nil {
		t.Fatal(err)
	}
	if factory.next != 2 || first[0].client != second[0].client || first[1].client != second[1].client {
		t.Fatal("repeated query rebuilt its authenticated clients")
	}
	credential.Password = "changed"
	changed, err := app.reusableQuerySessions("profile", credential, 1)
	if err != nil {
		t.Fatal(err)
	}
	if changed[0].client == first[0].client {
		t.Fatal("credential change reused old session")
	}
	pool := app.querySessions["profile"]
	pool.created = time.Now().Add(-11 * time.Minute)
	app.querySessions["profile"] = pool
	expired, err := app.reusableQuerySessions("profile", credential, 1)
	if err != nil {
		t.Fatal(err)
	}
	if expired[0].client == changed[0].client {
		t.Fatal("expired session was reused")
	}
	other := &App{clients: factory, cookies: &fakeCookies{}, querySessions: make(map[string]querySessionPool)}
	isolated, err := other.reusableQuerySessions("profile", credential, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isolated[0].client == expired[0].client {
		t.Fatal("browser sessions shared a client")
	}
}
