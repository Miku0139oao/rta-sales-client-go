package desktop

import (
	"context"
	"sync"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

type memoryProfileRepository struct {
	mu       sync.Mutex
	profiles []profileRecord
}

func (r *memoryProfileRepository) List() ([]profileRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]profileRecord, len(r.profiles))
	copy(out, r.profiles)
	return out, nil
}

func (r *memoryProfileRepository) Replace(profiles []profileRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	profiles = append([]profileRecord(nil), profiles...)
	sortProfiles(profiles)
	if err := validateProfiles(profiles); err != nil {
		return err
	}
	for index := range profiles {
		profiles[index].Priority = index
	}
	r.profiles = profiles
	return nil
}

type memoryManCodeRepository struct {
	mu     sync.Mutex
	groups []ManCodeGroup
}

func (r *memoryManCodeRepository) List() ([]ManCodeGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ManCodeGroup, len(r.groups))
	copy(out, r.groups)
	return out, nil
}

func (r *memoryManCodeRepository) Replace(groups []ManCodeGroup) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups = append([]ManCodeGroup(nil), groups...)
	return nil
}

type memoryProfileCookies struct {
	mu   sync.Mutex
	byID map[string]*securestore.MemoryCookieStore
}

func newMemoryProfileCookies() *memoryProfileCookies {
	return &memoryProfileCookies{byID: make(map[string]*securestore.MemoryCookieStore)}
}

func (s *memoryProfileCookies) CookieStore(profileID string) (rtasales.CookieStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	store := s.byID[profileID]
	if store == nil {
		store = &securestore.MemoryCookieStore{}
		s.byID[profileID] = store
	}
	return store, nil
}

func (s *memoryProfileCookies) DeleteCookie(profileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, profileID)
	return nil
}

func (s *memoryProfileCookies) dropMissing(keep map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for profileID := range s.byID {
		if _, ok := keep[profileID]; !ok {
			delete(s.byID, profileID)
		}
	}
}

type sseEvent struct {
	Name    string
	Payload any
}

type sseHub struct {
	mu   sync.Mutex
	subs map[chan sseEvent]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{subs: make(map[chan sseEvent]struct{})}
}

func (h *sseHub) Subscribe() chan sseEvent {
	ch := make(chan sseEvent, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) Unsubscribe(ch chan sseEvent) {
	h.mu.Lock()
	_, ok := h.subs[ch]
	if ok {
		delete(h.subs, ch)
	}
	h.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (h *sseHub) Emit(_ context.Context, name string, payload any) {
	h.EmitEvent(name, payload)
}

func (h *sseHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *sseHub) EmitEvent(name string, payload any) {
	event := sseEvent{Name: name, Payload: payload}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- event:
		default:
		}
	}
}
