package http

import (
	"errors"
	"fmt"
	"github.com/spiral/roadrunner"
	"github.com/spiral/roadrunner/service"
	"net"
	"os"
	"strings"
)

// Config configures RoadRunner HTTP server.
type Config struct {
	// Port and port to handle as http server.
	Address string

	// SSL defines https server options.
	SSL SSLConfig

	FCGI FCGIConfig

	// MaxRequestSize specified max size for payload body in megabytes, set 0 to unlimited.
	MaxRequestSize int64

	// TrustedSubnets declare IP subnets which are allowed to set ip using X-Real-Ip and X-Forwarded-For
	TrustedSubnets []string
	cidrs          []*net.IPNet

	// Uploads configures uploads configuration.
	Uploads *UploadsConfig

	// HTTP2 configuration
	HTTP2 *HTTP2Config

	// Middlewares
	Middlewares *MiddlewaresConfig

	// Workers configures rr server and worker pool.
	Workers *roadrunner.ServerConfig
}

type MiddlewaresConfig struct {
	Headers *HeaderMiddlewareConfig
	CORS *CORSMiddlewareConfig
}

type CORSMiddlewareConfig struct {
	AllowedOrigin string
	AllowedMethods string
	AllowedHeaders string
	AllowCredentials *bool
	ExposedHeaders string
	MaxAge int
}

type HeaderMiddlewareConfig struct {
	CustomRequestHeaders map[string]string
	CustomResponseHeaders map[string]string
}

func (c *MiddlewaresConfig) EnableCORS() bool {
	return c.CORS != nil
}

func (c *MiddlewaresConfig) EnableHeaders() bool {
	return c.Headers.CustomRequestHeaders != nil || c.Headers.CustomResponseHeaders != nil
}

type FCGIConfig struct {
	// Port and port to handle as http server.
	Address string
}

type HTTP2Config struct {
	Enabled bool
	MaxConcurrentStreams uint32
}

func (cfg *HTTP2Config) InitDefaults() error {
	cfg.Enabled = true
	cfg.MaxConcurrentStreams = 128

	return nil
}

// SSLConfig defines https server configuration.
type SSLConfig struct {
	// Port to listen as HTTPS server, defaults to 443.
	Port int

	// Redirect when enabled forces all http connections to switch to https.
	Redirect bool

	// Key defined private server key.
	Key string

	// Cert is https certificate.
	Cert string
}

func (c *Config) EnableHTTP() bool {
	return c.Address != ""
}

func (c *Config) EnableMiddlewares() bool {
	return c.Middlewares != nil
}

// EnableTLS returns true if rr must listen TLS connections.
func (c *Config) EnableTLS() bool {
	return c.SSL.Key != "" || c.SSL.Cert != ""
}

func (c *Config) EnableHTTP2() bool {
	return c.HTTP2.Enabled
}

func (c *Config) EnableFCGI() bool {
	return c.FCGI.Address != ""
}

// Hydrate must populate Config values using given Config source. Must return error if Config is not valid.
func (c *Config) Hydrate(cfg service.Config) error {
	if c.Workers == nil {
		c.Workers = &roadrunner.ServerConfig{}
	}

	if c.HTTP2 == nil {
		c.HTTP2 = &HTTP2Config{}
	}

	if c.Uploads == nil {
		c.Uploads = &UploadsConfig{}
	}

	if c.SSL.Port == 0 {
		c.SSL.Port = 443
	}

	c.HTTP2.InitDefaults()
	c.Uploads.InitDefaults()
	c.Workers.InitDefaults()

	if err := cfg.Unmarshal(c); err != nil {
		return err
	}

	c.Workers.UpscaleDurations()

	if c.TrustedSubnets == nil {
		// @see https://en.wikipedia.org/wiki/Reserved_IP_addresses
		c.TrustedSubnets = []string{
			"10.0.0.0/8",
			"127.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"::1/128",
			"fc00::/7",
			"fe80::/10",
		}
	}

	if err := c.parseCIDRs(); err != nil {
		return err
	}

	return c.Valid()
}

func (c *Config) parseCIDRs() error {
	for _, cidr := range c.TrustedSubnets {
		_, cr, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}

		c.cidrs = append(c.cidrs, cr)
	}

	return nil
}

// IsTrusted if api can be trusted to use X-Real-Ip, X-Forwarded-For
func (c *Config) IsTrusted(ip string) bool {
	if c.cidrs == nil {
		return false
	}

	i := net.ParseIP(ip)
	if i == nil {
		return false
	}

	for _, cird := range c.cidrs {
		if cird.Contains(i) {
			return true
		}
	}

	return false
}

// Valid validates the configuration.
func (c *Config) Valid() error {
	if c.Uploads == nil {
		return errors.New("mailformed uploads config")
	}

	if c.HTTP2 == nil {
		return errors.New("mailformed http2 config")
	}

	if c.Workers == nil {
		return errors.New("mailformed workers config")
	}

	if c.Workers.Pool == nil {
		return errors.New("mailformed workers config (pool config is missing)")
	}

	if err := c.Workers.Pool.Valid(); err != nil {
		return err
	}

	if c.Address != "" && !strings.Contains(c.Address, ":") {
		return errors.New("mailformed http server address")
	}

	if c.EnableTLS() {
		if _, err := os.Stat(c.SSL.Key); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("key file '%s' does not exists", c.SSL.Key)
			}

			return err
		}

		if _, err := os.Stat(c.SSL.Cert); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("cert file '%s' does not exists", c.SSL.Cert)
			}

			return err
		}
	}

	return nil
}
