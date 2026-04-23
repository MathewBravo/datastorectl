package mysql

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/go-sql-driver/mysql"
)

// Client wraps the *sql.DB used to talk to a MySQL server. The pool is
// sized at one connection since datastorectl is a short-lived CLI.
type Client struct {
	db *sql.DB
}

// Close releases the underlying connection pool.
func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB exposes the raw *sql.DB for handler use.
func (c *Client) DB() *sql.DB {
	return c.db
}

// tlsConfigSeq generates unique names for custom TLS configs registered
// with the go-sql-driver at runtime, so repeated Configure calls in a
// single process don't collide on a fixed name.
var tlsConfigSeq atomic.Uint64

// ClientConfig captures everything NewPasswordClient needs from the
// resolved provider block.
type ClientConfig struct {
	Endpoint string
	Username string
	Password string
	TLS      string // "required", "skip-verify", "disabled", or "" (default: required)
	TLSCA    string
	TLSCert  string
	TLSKey   string
}

// NewPasswordClient opens a *sql.DB configured for username/password
// authentication. TLS mode and optional CA/client-cert paths are
// respected. The returned client has an open connection pool capped at
// one connection.
func NewPasswordClient(cfg ClientConfig) (*Client, error) {
	driverCfg := mysql.NewConfig()
	driverCfg.User = cfg.Username
	driverCfg.Passwd = cfg.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = cfg.Endpoint
	driverCfg.AllowNativePasswords = true

	tlsMode := cfg.TLS
	if tlsMode == "" {
		tlsMode = "required"
	}
	if err := applyTLSConfig(driverCfg, tlsMode, cfg); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", driverCfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("opening mysql connection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return &Client{db: db}, nil
}

// applyTLSConfig translates the TLS enum (required | skip-verify |
// disabled) plus optional CA/cert paths into a go-sql-driver TLS
// setting. Custom CAs and client certs are registered via
// mysql.RegisterTLSConfig under a unique name per call.
func applyTLSConfig(driverCfg *mysql.Config, mode string, cfg ClientConfig) error {
	switch mode {
	case "disabled":
		driverCfg.TLSConfig = "false"
		return nil
	case "skip-verify":
		driverCfg.TLSConfig = "skip-verify"
		return nil
	case "required":
		// fall through
	default:
		return fmt.Errorf(`internal: unexpected tls mode %q`, mode)
	}

	hasCustom := cfg.TLSCA != "" || (cfg.TLSCert != "" || cfg.TLSKey != "")
	if !hasCustom {
		driverCfg.TLSConfig = "true"
		return nil
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.TLSCA != "" {
		pem, err := os.ReadFile(cfg.TLSCA)
		if err != nil {
			return fmt.Errorf("reading tls_ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("tls_ca %s contains no valid PEM certificates", cfg.TLSCA)
		}
		tlsCfg.RootCAs = pool
	}

	if (cfg.TLSCert != "") != (cfg.TLSKey != "") {
		return fmt.Errorf("tls_cert and tls_key must be set together")
	}
	if cfg.TLSCert != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return fmt.Errorf("loading client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	name := fmt.Sprintf("datastorectl-%d", tlsConfigSeq.Add(1))
	if err := mysql.RegisterTLSConfig(name, tlsCfg); err != nil {
		return fmt.Errorf("registering TLS config: %w", err)
	}
	driverCfg.TLSConfig = name
	return nil
}

// HostFromEndpoint returns the host portion of a host:port endpoint,
// or the full string if no port is present. Used when configuring
// SigV4 token generation in later phases.
func HostFromEndpoint(endpoint string) string {
	if i := strings.LastIndex(endpoint, ":"); i >= 0 {
		return endpoint[:i]
	}
	return endpoint
}
