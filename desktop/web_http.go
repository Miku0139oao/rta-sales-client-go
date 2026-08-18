package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

const (
	webSessionCookie  = "rta_web_session"
	webSessionTTL     = 2 * time.Hour
	maximumWebRPCBody = 8 << 20
	maximumWebUpload  = 32 << 20
)

type webSession struct {
	id       string
	app      *App
	hub      *sseHub
	dir      string
	lastUsed time.Time
}

type WebServer struct {
	Static   string
	Clients  clientFactory
	sessions sync.Mutex
	byID     map[string]*webSession
}

func NewWebServer() *WebServer {
	return &WebServer{
		Clients: rtaClientFactory{},
		byID:    make(map[string]*webSession),
	}
}

func (s *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/session", s.handleSync)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/download", s.handleDownload)
	mux.HandleFunc("POST /api/rpc", s.handleRPC)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("/", s.handleStatic)
	go s.reapLoop()
	return mux
}

func (s *WebServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type webSyncRequest struct {
	Profiles []webSyncProfile         `json:"profiles"`
	Secrets  map[string]webSyncSecret `json:"secrets"`
	Groups   []ManCodeGroup           `json:"groups"`
}

type webSyncProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority"`
}

type webSyncSecret struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

func (s *WebServer) handleSync(w http.ResponseWriter, r *http.Request) {
	session, err := s.ensureSession(w, r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	var request webSyncRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	records := make([]profileRecord, 0, len(request.Profiles))
	for _, profile := range request.Profiles {
		if !validProfileID(profile.ID) {
			writeAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid profile identifier %q", profile.ID))
			return
		}
		name := strings.TrimSpace(profile.DisplayName)
		if name == "" {
			writeAPIError(w, http.StatusBadRequest, errors.New("profile displayName is required"))
			return
		}
		records = append(records, profileRecord{
			ID: profile.ID, DisplayName: name, Enabled: profile.Enabled, Priority: profile.Priority,
		})
	}
	if err := session.app.profiles.Replace(records); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	store, ok := session.app.credentials.(*securestore.MemoryCredentialStore)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, errors.New("web session credential store is unavailable"))
		return
	}
	for _, record := range records {
		secret := request.Secrets[record.ID]
		if strings.TrimSpace(secret.Account) == "" || secret.Password == "" {
			_ = store.Delete(record.ID)
			continue
		}
		if err := store.Put(record.ID, securestore.Credential{Account: strings.TrimSpace(secret.Account), Password: secret.Password}); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := session.app.mancodes.Replace(request.Groups); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *WebServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	session, err := s.ensureSession(w, r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if err := r.ParseMultipartForm(maximumWebUpload); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, errors.New("file is required"))
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		writeAPIError(w, http.StatusBadRequest, errors.New("file name is required"))
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".xlsx" && ext != ".json" && ext != ".csv" {
		writeAPIError(w, http.StatusBadRequest, errors.New("only .xlsx, .json, or .csv uploads are allowed"))
		return
	}
	dest := filepath.Join(session.dir, name)
	if err := session.ensureInside(dest); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(file, maximumWebUpload+1)); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": dest, "fileName": name})
}

func (s *WebServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	session, err := s.ensureSession(w, r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	target := r.URL.Query().Get("path")
	if err := session.confinePath(&target); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		writeAPIError(w, http.StatusNotFound, errors.New("file is not available"))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(target)))
	http.ServeFile(w, r, target)
}

type webRPCRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

func (s *WebServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	session, err := s.ensureSession(w, r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	var request webRPCRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	result, err := dispatchWebRPC(session, request.Method, request.Args)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func dispatchWebRPC(session *webSession, method string, args []json.RawMessage) (any, error) {
	app := session.app
	arg := func(index int, dest any) error {
		if index >= len(args) {
			return fmt.Errorf("%s is missing argument %d", method, index)
		}
		if err := json.Unmarshal(args[index], dest); err != nil {
			return fmt.Errorf("%s argument %d: %w", method, index, err)
		}
		return nil
	}
	switch method {
	case "TestProfile":
		var request TestProfileRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.TestProfile(request)
	case "ListSalesAnalysisStores":
		var request ProfileIDRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.ListSalesAnalysisStores(request)
	case "RunSalesAnalysis":
		var request SalesAnalysisRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.RunSalesAnalysis(request)
	case "GetSalesAnalysisItems":
		var request SalesAnalysisItemsRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.GetSalesAnalysisItems(request)
	case "GetSalesAnalysisReportMemo":
		var request SalesAnalysisReportMemoRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.GetSalesAnalysisReportMemo(request)
	case "GetSalesAnalysisReportGlyphs":
		var request OperationRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.GetSalesAnalysisReportGlyphs(request)
	case "ClearSalesAnalysis":
		var request OperationRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return nil, app.ClearSalesAnalysis(request)
	case "CancelSalesAnalysis":
		var request OperationRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return nil, app.CancelSalesAnalysis(request)
	case "ListProfiles":
		return app.ListProfiles()
	case "ScanWorkbook":
		var request ScanWorkbookRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		if err := session.confinePath(&request.InputPath); err != nil {
			return nil, err
		}
		if request.MappingPath != "" {
			if err := session.confinePath(&request.MappingPath); err != nil {
				return nil, err
			}
		}
		return app.ScanWorkbook(request)
	case "Analyze":
		var request AnalyzeRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		if err := session.confinePath(&request.InputPath); err != nil {
			return nil, err
		}
		if request.MappingPath != "" {
			if err := session.confinePath(&request.MappingPath); err != nil {
				return nil, err
			}
		}
		return app.Analyze(request)
	case "RetryFailed":
		var request OperationRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return app.RetryFailed(request)
	case "Apply":
		var request ApplyRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		if err := session.confinePath(&request.InputPath); err != nil {
			return nil, err
		}
		if err := session.confinePath(&request.OutputPath); err != nil {
			return nil, err
		}
		return app.Apply(request)
	case "Cancel":
		var request OperationRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return nil, app.Cancel(request)
	case "SaveWorkbook":
		var request SaveWorkbookRequest
		if err := arg(0, &request); err != nil {
			return nil, err
		}
		return session.suggestOutputPath(request)
	default:
		return nil, fmt.Errorf("unsupported web method %s", method)
	}
}

func (s *WebServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	session, err := s.ensureSession(w, r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, errors.New("streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := session.hub.Subscribe()
	defer session.hub.Unsubscribe(ch)
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(event.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, payload)
			flusher.Flush()
		}
	}
}

func (s *WebServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.Static == "" {
		http.NotFound(w, r)
		return
	}
	cleaned := path.Clean("/" + r.URL.Path)
	if cleaned == "/" {
		cleaned = "/index.html"
	}
	full := path.Join(s.Static, cleaned)
	if !strings.HasPrefix(full, path.Clean(s.Static)+"/") && full != path.Clean(s.Static)+"/index.html" && full != path.Join(s.Static, "index.html") {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.ServeFile(w, r, path.Join(s.Static, "index.html"))
		return
	}
	http.ServeFile(w, r, full)
}

func (s *WebServer) ensureSession(w http.ResponseWriter, r *http.Request) (*webSession, error) {
	s.touchReap()
	if cookie, err := r.Cookie(webSessionCookie); err == nil {
		s.sessions.Lock()
		session := s.byID[cookie.Value]
		if session != nil {
			session.lastUsed = time.Now()
			s.sessions.Unlock()
			return session, nil
		}
		s.sessions.Unlock()
	}
	session, err := s.newSession()
	if err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookie,
		Value:    session.id,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(webSessionTTL.Seconds()),
	})
	return session, nil
}

func (s *WebServer) newSession() (*webSession, error) {
	id, err := randomSessionID()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "rta-web-")
	if err != nil {
		return nil, err
	}
	hub := newSSEHub()
	app, err := newWebApp(s.Clients, hub)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	session := &webSession{id: id, app: app, hub: hub, dir: dir, lastUsed: time.Now()}
	s.sessions.Lock()
	s.byID[id] = session
	s.sessions.Unlock()
	return session, nil
}

func (session *webSession) confinePath(value *string) error {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	absolute, err := filepath.Abs(strings.TrimSpace(*value))
	if err != nil {
		return err
	}
	if err := session.ensureInside(absolute); err != nil {
		return err
	}
	*value = absolute
	return nil
}

func (session *webSession) ensureInside(target string) error {
	root, err := filepath.Abs(session.dir)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil || strings.HasPrefix(rel, "..") {
		return errors.New("path is outside the web session")
	}
	return nil
}

func (session *webSession) suggestOutputPath(request SaveWorkbookRequest) (string, error) {
	inputPath, err := existingWorkbookPath(request.InputPath)
	if err != nil {
		return "", err
	}
	if err := session.ensureInside(inputPath); err != nil {
		return "", err
	}
	fromText, toText := strings.TrimSpace(request.From), strings.TrimSpace(request.To)
	if fromText == "" {
		fromText = strings.TrimSpace(request.Date)
	}
	if fromText == "" {
		fromText = time.Now().Format("2006-01-02")
	}
	if toText == "" {
		toText = fromText
	}
	name := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath)) + "_filled_" + strings.ReplaceAll(fromText, "-", "")
	if toText != fromText {
		name += "-" + strings.ReplaceAll(toText, "-", "")
	}
	return filepath.Join(session.dir, name+".xlsx"), nil
}

func newWebApp(clients clientFactory, events eventSink) (*App, error) {
	if clients == nil {
		clients = rtaClientFactory{}
	}
	app, err := newApp(appDependencies{
		profiles:    &memoryProfileRepository{},
		mancodes:    &memoryManCodeRepository{},
		credentials: securestore.NewMemoryCredentialStore(),
		cookies:     newMemoryProfileCookies(),
		clients:     clients,
		engine:      newXLSXEngine(),
		dialogs:     wailsDialogService{},
		events:      events,
		runtime:     nativeRuntimeChecker{},
	})
	if err != nil {
		return nil, err
	}
	Start(app, context.Background())
	return app, nil
}

func (session *webSession) discard() {
	if session == nil {
		return
	}
	if session.hub != nil {
		session.hub.Close()
		session.hub = nil
	}
	if session.app != nil {
		if store, ok := session.app.credentials.(*securestore.MemoryCredentialStore); ok {
			store.Clear()
		}
		session.app = nil
	}
	if session.dir != "" {
		_ = os.RemoveAll(session.dir)
		session.dir = ""
	}
}

func (s *WebServer) reapLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.touchReap()
	}
}

func (s *WebServer) touchReap() {
	cutoff := time.Now().Add(-webSessionTTL)
	s.sessions.Lock()
	defer s.sessions.Unlock()
	for id, session := range s.byID {
		if session.lastUsed.Before(cutoff) {
			delete(s.byID, id)
			session.discard()
		}
	}
}

func randomSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func decodeJSONBody(r *http.Request, dest any) error {
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, maximumWebRPCBody+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maximumWebRPCBody {
		return errors.New("request body is too large")
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    webErrorCode(err),
			"message": err.Error(),
		},
	})
}

func webErrorCode(err error) string {
	if err == nil {
		return "backend_error"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "cancelled"):
		return "cancelled"
	case strings.Contains(message, "credential"):
		return "credentials_required"
	case strings.Contains(message, "does not exist"), strings.Contains(message, "not found"):
		return "profile_not_found"
	default:
		return "backend_error"
	}
}
