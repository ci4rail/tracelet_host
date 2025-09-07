package io4edgecore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	defaultUser = "io4edge"
	defaultPass = "core_io4edge" // as documented
	apiBase     = "/api/v1"
)

// --- In-memory device model --------------------------------------------------

type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ParameterSetGet struct {
	Version    string            `json:"version"`
	Parameters map[string]string `json:"parameters"`
}

type ParameterSetPut struct {
	Version    string            `json:"version"`
	Parameters map[string]string `json:"parameters"`
}

type RouteRegistrar func(api chi.Router)

type CoreServer struct {
	dev *Device
	// security
	username string
	password string

	// crypto material
	certificatePEM string
	privateKeyPEM  string
}

func NewCoreServer(dev *Device, addr string, additionalRoutes []RouteRegistrar) (*CoreServer, error) {
	cs := &CoreServer{
		dev:      dev,
		username: defaultUser,
		password: defaultPass,
	}
	if err := cs.start(addr, additionalRoutes); err != nil {
		return nil, fmt.Errorf("failed to start core server: %w", err)
	}
	return cs, nil
}

// --- Basic auth middleware ---------------------------------------------------

func basicAuth(cs *CoreServer) func(http.Handler) http.Handler {
	realm := "io4edge-sio06"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != cs.username || p != cs.password {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				httpError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Helpers ----------------------------------------------------------------

func httpError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorInfo{Code: status, Message: msg})
}

func readBodyText(r *http.Request, max int64) (string, error) {
	defer r.Body.Close()
	if r.ContentLength > 0 && r.ContentLength > max {
		return "", fmt.Errorf("payload too large")
	}
	data := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	var total int64
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > max {
				return "", fmt.Errorf("payload too large")
			}
			data = append(data, buf[:n]...)
		}
		if errors.Is(err, os.ErrClosed) {
			break
		}
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				break
			}
			if err.Error() == "EOF" {
				break
			}
			if n == 0 && err.Error() != "EOF" {
				return "", err
			}
			break
		}
	}
	return string(data), nil
}

// Validate PEM quickly (not cryptographically strict)
func isPEM(s string) bool {
	block, _ := pem.Decode([]byte(s))
	return block != nil
}

// --- Handlers (implementing the spec paths) ----------------------------------

// PUT /users/{user}/basic_auth  — change password for the user
type changePasswordRequest struct {
	Password string `json:"password"`
}

func changePassword(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := chi.URLParam(r, "user")
		if user == "" {
			httpError(w, http.StatusBadRequest, "missing user")
			return
		}
		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Password) == "" {
			httpError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if user != cs.username {
			// In a real device you might 404 or forbid — here we restrict to the single account.
			httpError(w, http.StatusForbidden, "only io4edge user can be modified")
			return
		}
		cs.password = req.Password
		w.WriteHeader(http.StatusNoContent)
	}
}

// PUT /certificate — upload PEM certificate
func putCertificate(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := readBodyText(r, 1<<20) // 1 MiB
		if err != nil {
			httpError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if !isPEM(text) {
			httpError(w, http.StatusBadRequest, "expected PEM certificate in request body")
			return
		}
		cs.certificatePEM = text
		w.WriteHeader(http.StatusNoContent)
	}
}

// PUT /key — upload PEM private key
func putKey(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := readBodyText(r, 1<<20)
		if err != nil {
			httpError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if !isPEM(text) {
			httpError(w, http.StatusBadRequest, "expected PEM private key in request body")
			return
		}
		cs.privateKeyPEM = text
		w.WriteHeader(http.StatusNoContent)
	}
}

// GET /firmware — current version; POST /firmware — upload new (mock)
func getFirmware(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cs.dev.fw)
	}
}

func postFirmware(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept arbitrary bytes (a .fwpkg), pretend to verify & install.
		// Optional header X-Firmware-Name: and X-Firmware-Version:
		name := r.Header.Get("X-Firmware-Name")
		ver := r.Header.Get("X-Firmware-Version")
		if name == "" {
			name = "fw_sio06_default"
		}
		if ver == "" {
			ver = "1.0.1"
		}
		// drain body but ignore content
		_, _ = readBodyText(r, 20<<20) // 20 MiB
		cs.dev.fw = FirmwareVersion{Name: name, Version: ver}
		// Spec suggests device restarts automatically; we simulate 202 Accepted.
		w.WriteHeader(http.StatusAccepted)
	}
}

// GET /hardware
func getHardware(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cs.dev.hw)
	}
}

// POST /restart
func postRestart(_ *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		os.Exit(0) // in real device, restart the system; here we just exit
	}
}

// POST /factoryreset
func postFactoryReset(_ *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ParameterSetForceFactoryDefaults()
		w.WriteHeader(http.StatusAccepted)
		os.Exit(0) // in real device, restart the system; here we just exit
	}
}

// POST /repl — execute a single-line command and return output (mock)
type replRequest struct {
	Cmd string `json:"cmd"`
}
type replResponse struct {
	Output string `json:"output"`
}

func postRepl(cs *CoreServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req replRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Cmd) == "" {
			httpError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out := fmt.Sprintf("OK: %s", req.Cmd)
		cs.dev.repl = append(cs.dev.repl, out)
		writeJSON(w, replResponse{Output: out})
	}
}

// GET /parameter — list all core parameters
func getParameters(cs *CoreServer) http.HandlerFunc {
	return cs.dev.globalParams.ListParameterSetHandlerFunc()
}

type putParamRequest struct {
	Value string `json:"value"`
}

type putParamSetRequest struct {
	Version    string            `json:"version"`
	Parameters map[string]string `json:"parameters"`
}

type putParamSetResponse struct {
	Missing        []string `json:"missing_parameters"`
	Unsupported    []string `json:"unsupported_parameters"`
	RebootRequired bool     `json:"reboot_required"`
}

type getParamResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type getParamSetResponse struct {
	Version        string            `json:"version"`
	Parameters     map[string]string `json:"parameters"`
	Unsupported    []string          `json:"unsupported_parameters"`
	Missing        []string          `json:"missing_parameters"`
	Invalid        []string          `json:"invalid_parameters"`
	RebootRequired bool              `json:"reboot_required"`
}

type listParamsEntry struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Default       string `json:"default"`
	ReadProtected bool   `json:"read_protected"`
	Persistence   string `json:"persistence"`
}

type listParamsResponse []*listParamsEntry

func (ps *ParameterSet) PutParameterSetHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req putParamSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "invalid body")
			return
		}
		rv, err := ps.ParameterSetSet(req.Version, req.Parameters)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "failed to set parameters")
			return
		}
		writeJSON(w, putParamSetResponse{
			Missing:        rv.Missing,
			Unsupported:    rv.Unsupported,
			RebootRequired: rv.RebootRequired,
		})
	}
}

func (ps *ParameterSet) PutParameterHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "parameter")
		if name == "" {
			httpError(w, http.StatusBadRequest, "missing parameter name")
			return
		}
		var req putParamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := ps.ParamSetSingle(name, req.Value); err != nil {
			httpError(w, http.StatusInternalServerError, "failed to set parameter")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func putParameter(cs *CoreServer) http.HandlerFunc {
	return cs.dev.globalParams.PutParameterHandlerFunc()
}

func (ps *ParameterSet) GetParameterSetHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rv, err := ps.ParameterSetGet()
		if err != nil {
			httpError(w, http.StatusInternalServerError, "failed to get parameters")
			return
		}
		writeJSON(w, getParamSetResponse{
			Version:        rv.Version,
			Parameters:     rv.Params,
			Missing:        rv.Missing,
			Unsupported:    rv.Unsupported,
			Invalid:        rv.Invalid,
			RebootRequired: rv.RebootRequired,
		})
	}
}

func (ps *ParameterSet) GetParameterHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "parameter")
		if name == "" {
			httpError(w, http.StatusBadRequest, "missing parameter name")
			return
		}
		val, err := ps.ParamGetSingle(name)
		if err != nil {
			if errors.Is(err, ErrParameterNotFound) {
				httpError(w, http.StatusNotFound, "parameter not found")
				return
			}
			httpError(w, http.StatusInternalServerError, "failed to get parameter")
			return
		}
		writeJSON(w, getParamResponse{Name: name, Value: val})
	}
}

// GET /parameter/{parameter} — get one (add convenience not always in spec)
func getParameter(cs *CoreServer) http.HandlerFunc {
	return cs.dev.globalParams.GetParameterHandlerFunc()
}

func (ps *ParameterSet) ListParameterSetHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rv, err := ps.ListParams()
		if err != nil {
			httpError(w, http.StatusInternalServerError, "failed to list parameters")
			return
		}
		resp := make(listParamsResponse, 0, len(rv))
		for _, e := range rv {
			resp = append(resp, &listParamsEntry{
				Name:          e.Name,
				Description:   e.Description,
				Default:       e.Default,
				ReadProtected: e.ReadProtected,
				Persistence:   e.Persistence,
			})
		}
		writeJSON(w, resp)
	}
}

// --- Router setup ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func routes(cs *CoreServer, additionalRoutes []RouteRegistrar) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(basicAuth(cs))

	r.Route(apiBase, func(api chi.Router) {
		api.Put("/users/{user}/basic_auth", changePassword(cs))

		api.Put("/certificate", putCertificate(cs))
		api.Put("/key", putKey(cs))

		api.Get("/firmware", getFirmware(cs))
		api.Post("/firmware", postFirmware(cs))

		api.Get("/hardware", getHardware(cs))

		api.Post("/restart", postRestart(cs))
		api.Post("/factoryreset", postFactoryReset(cs))

		api.Post("/repl", postRepl(cs))

		api.Get("/parameter", getParameters(cs))
		api.Get("/parameter/{parameter}", getParameter(cs))
		api.Put("/parameter/{parameter}", putParameter(cs))

		// additional routes (e.g. for tracelet)
		for _, ar := range additionalRoutes {
			ar(api)
		}
	})

	// health
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return r
}

// --- HTTPS bootstrapping -----------------------------------------------------

func (cs *CoreServer) start(addr string, additionalRoutes []RouteRegistrar) error {

	srv := &http.Server{
		Addr:         addr,
		Handler:      routes(cs, additionalRoutes),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		// Configure TLS handshake/session behavior to encourage reuse.
		// Clients that support resumption/PSK will skip the full handshake on subsequent connections.
		TLSConfig: &tls.Config{
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
			SessionTicketsDisabled:   false,
			ClientSessionCache:       tls.NewLRUClientSessionCache(1024),
			// HTTP/2 is on by default with Go's server when TLSConfig is present.
			// No client certs required (Basic Auth is used).
		},
	}

	// Generate ephemeral self-signed cert for local/dev usage.
	log.Printf("TLS_CERT_FILE/TLS_KEY_FILE not set — generating self-signed certificate for %s", addr)
	certFile, keyFile, err := generateSelfSigned()
	if err != nil {
		return fmt.Errorf("failed to generate self-signed cert: %w", err)
	}

	log.Printf("Listening on https://%s%s", prettyAddr(addr), apiBase)

	go func() {
		if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()
	return nil
}

func prettyAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return "localhost" + a
	}
	return a
}

// generateSelfSigned writes cert+key (PEM) into $TMPDIR and returns paths.
func generateSelfSigned() (certPath, keyPath string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	notBefore := time.Now().Add(-time.Hour)
	notAfter := time.Now().Add(365 * 24 * time.Hour)

	serial, err := rand.Int(rand.Reader, big.NewInt(0).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	hostname, _ := os.Hostname()

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "sio06-dev",
			Organization: []string{"io4edge"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", hostname},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	dir := os.TempDir()
	certPath = filepath.Join(dir, "sio06-cert.pem")
	keyPath = filepath.Join(dir, "sio06-key.pem")
	if err := os.WriteFile(certPath, certBuf, 0644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, keyBuf, 0600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}
