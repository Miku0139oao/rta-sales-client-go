package desktop

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

// Keep independent clients within one App so subsequent queries
// reuse authenticated sessions and authorized-store caches. Never share them
// between App instances; refresh after ten minutes or a credential change.
type querySessionPool struct {
	identity [32]byte
	created  time.Time
	sessions []accountSession
}

func (a *App) discardQuerySessions(profileID string) {
	a.querySessionMu.Lock()
	defer a.querySessionMu.Unlock()
	delete(a.querySessions, profileID)
}

func (a *App) reusableQuerySessions(profileID string, credential securestore.Credential, count int) ([]accountSession, error) {
	a.querySessionMu.Lock()
	defer a.querySessionMu.Unlock()
	now := time.Now()
	for id, pool := range a.querySessions {
		if now.Sub(pool.created) >= 10*time.Minute {
			delete(a.querySessions, id)
		}
	}
	identity := sha256.Sum256([]byte(credential.Account + "\x00" + credential.Password))
	pool := a.querySessions[profileID]
	if pool.identity != identity {
		pool = querySessionPool{identity: identity, created: now}
	}
	for len(pool.sessions) < count {
		index := len(pool.sessions)
		var cookies = new(securestore.MemoryCookieStore)
		if index == 0 {
			primaryCookies, err := a.cookies.CookieStore(profileID)
			if err != nil {
				return nil, err
			}
			client, err := a.clients.New(credential, primaryCookies)
			if err != nil {
				return nil, err
			}
			pool.sessions = append(pool.sessions, accountSession{client: client, lane: accountLane(credential.Account)})
		} else {
			client, err := a.clients.New(credential, cookies)
			if err != nil {
				return nil, err
			}
			pool.sessions = append(pool.sessions, accountSession{client: client, lane: fmt.Sprintf("%s:%d", accountLane(credential.Account), index)})
		}
	}
	a.querySessions[profileID] = pool
	return append([]accountSession(nil), pool.sessions[:count]...), nil
}
