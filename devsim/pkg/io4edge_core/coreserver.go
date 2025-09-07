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

type FirmwareVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HardwareInventory struct {
	Name   string `json:"name"`
	Rev    int    `json:"rev"`
	Serial string `json:"serial"`
}

type ParameterSetGet struct {
	Parameters map[string]string `json:"parameters"`
}

type ParameterSetPut struct {
	Parameters map[string]string `json:"parameters"`
}

type Device struct {
	// security
	username string
	password string

	// crypto material
	certificatePEM string
	privateKeyPEM  string

	// fw/hw
	fw   FirmwareVersion
	hw   HardwareInventory
	repl []string

	// parameters (core + vwu share same store in this mock)
	params map[string]string
}

func newDevice() *Device {
	return &Device{
		username: defaultUser,
		password: defaultPass,
		fw: FirmwareVersion{
			Name:    "fw_sio06_default",
			Version: "1.0.0",
		},
		hw: HardwareInventory{
			Name:   "S103-LTR01",
			Rev:    0,
			Serial: "00000000-0000-0000-0000-000000000000",
		},
		params: map[string]string{
			"device-id": "SIO06-DEV-1",
		},
	}
}

// --- Basic auth middleware ---------------------------------------------------

func basicAuth(dev *Device) func(http.Handler) http.Handler {
	realm := "io4edge-sio06"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != dev.username || p != dev.password {
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
			if n == 0 && err != nil && err.Error() != "EOF" {
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

func changePassword(dev *Device) http.HandlerFunc {
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
		if user != dev.username {
			// In a real device you might 404 or forbid — here we restrict to the single account.
			httpError(w, http.StatusForbidden, "only io4edge user can be modified")
			return
		}
		dev.password = req.Password
		w.WriteHeader(http.StatusNoContent)
	}
}

// PUT /certificate — upload PEM certificate
func putCertificate(dev *Device) http.HandlerFunc {
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
		dev.certificatePEM = text
		w.WriteHeader(http.StatusNoContent)
	}
}

// PUT /key — upload PEM private key
func putKey(dev *Device) http.HandlerFunc {
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
		dev.privateKeyPEM = text
		w.WriteHeader(http.StatusNoContent)
	}
}

// GET /firmware — current version; POST /firmware — upload new (mock)
func getFirmware(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, dev.fw)
	}
}

func postFirmware(dev *Device) http.HandlerFunc {
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
		dev.fw = FirmwareVersion{Name: name, Version: ver}
		// Spec suggests device restarts automatically; we simulate 202 Accepted.
		w.WriteHeader(http.StatusAccepted)
	}
}

// GET /hardware
func getHardware(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, dev.hw)
	}
}

// POST /restart
func postRestart(_ *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}
}

// POST /factoryreset
func postFactoryReset(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// wipe parameters to defaults
		dev.params = map[string]string{"device-id": "SIO06-DEV-1"}
		w.WriteHeader(http.StatusAccepted)
	}
}

// POST /repl — execute a single-line command and return output (mock)
type replRequest struct {
	Cmd string `json:"cmd"`
}
type replResponse struct {
	Output string `json:"output"`
}

func postRepl(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req replRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Cmd) == "" {
			httpError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out := fmt.Sprintf("OK: %s", req.Cmd)
		dev.repl = append(dev.repl, out)
		writeJSON(w, replResponse{Output: out})
	}
}

// GET /parameter — list all core parameters
func getParameters(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ParameterSetGet{Parameters: dev.params})
	}
}

// PUT /parameter/{parameter} — set one
type setParamRequest struct {
	Value string `json:"value"`
}

func putParameter(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "parameter")
		if name == "" {
			httpError(w, http.StatusBadRequest, "missing parameter name")
			return
		}
		var req setParamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "invalid body")
			return
		}
		dev.params[name] = req.Value
		w.WriteHeader(http.StatusNoContent)
	}
}

// GET /parameter/{parameter} — get one (add convenience not always in spec)
func getParameter(dev *Device) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "parameter")
		val, ok := dev.params[name]
		if !ok {
			httpError(w, http.StatusNotFound, "parameter not found")
			return
		}
		writeJSON(w, map[string]string{"name": name, "value": val})
	}
}

// VWU aliases (vehicle-wakeup-unit specific)
func getVWUParameters(dev *Device) http.HandlerFunc   { return getParameters(dev) }
func putVWUParam(dev *Device) http.HandlerFunc        { return putParameter(dev) }
func getVWUParam(dev *Device) http.HandlerFunc        { return getParameter(dev) }
func getVWUParameterSet(dev *Device) http.HandlerFunc { return getParameters(dev) }

// --- Router setup ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func routes(dev *Device) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(basicAuth(dev))

	r.Route(apiBase, func(api chi.Router) {
		api.Put("/users/{user}/basic_auth", changePassword(dev))

		api.Put("/certificate", putCertificate(dev))
		api.Put("/key", putKey(dev))

		api.Get("/firmware", getFirmware(dev))
		api.Post("/firmware", postFirmware(dev))

		api.Get("/hardware", getHardware(dev))

		api.Post("/restart", postRestart(dev))
		api.Post("/factoryreset", postFactoryReset(dev))

		api.Post("/repl", postRepl(dev))

		api.Get("/parameter", getParameters(dev))
		api.Get("/parameter/{parameter}", getParameter(dev))
		api.Put("/parameter/{parameter}", putParameter(dev))

		// VWU-specific mirrors
		api.Get("/vwu/parameter", getVWUParameters(dev))
		api.Get("/vwu/parameter/{parameter}", getVWUParam(dev))
		api.Put("/vwu/parameter/{parameter}", putVWUParam(dev))
		api.Get("/vwu/parameterset", getVWUParameterSet(dev))
	})

	// health
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return r
}

// --- HTTPS bootstrapping -----------------------------------------------------

func main() {
	addr := env("ADDR", ":8443")

	dev := newDevice()
	// Allow overriding the default credentials via env.
	if u := os.Getenv("BASIC_USER"); u != "" {
		dev.username = u
	}
	if p := os.Getenv("BASIC_PASS"); p != "" {
		dev.password = p
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      routes(dev),
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

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	var err error
	if certFile == "" || keyFile == "" {
		// Generate ephemeral self-signed cert for local/dev usage.
		log.Printf("TLS_CERT_FILE/TLS_KEY_FILE not set — generating self-signed certificate for %s", addr)
		certFile, keyFile, err = generateSelfSigned()
		if err != nil {
			log.Fatalf("failed to generate self-signed cert: %v", err)
		}
	}

	log.Printf("Listening on https://%s%s", prettyAddr(addr), apiBase)
	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
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
