// Package util provides utility functions for the CLI Proxy API server.
// This file provides uTLS fingerprinting capabilities to mimic real browser TLS handshakes.
package util

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	utls "github.com/refraction-networking/utls"
	log "github.com/sirupsen/logrus"
)

// TLSFingerprint represents the type of TLS fingerprint to use
type TLSFingerprint string

const (
	// Chrome fingerprints
	FingerprintChrome120    TLSFingerprint = "chrome_120"
	FingerprintChrome131    TLSFingerprint = "chrome_131"
	FingerprintChrome133    TLSFingerprint = "chrome_133"
	FingerprintChromeLatest TLSFingerprint = "chrome_latest"

	// Firefox fingerprints
	FingerprintFirefox102    TLSFingerprint = "firefox_102"
	FingerprintFirefox105    TLSFingerprint = "firefox_105"
	FingerprintFirefox120    TLSFingerprint = "firefox_120"
	FingerprintFirefoxLatest TLSFingerprint = "firefox_latest"

	// Safari fingerprints
	FingerprintSafari16     TLSFingerprint = "safari_16"
	FingerprintSafariLatest TLSFingerprint = "safari_latest"

	// Edge fingerprints
	FingerprintEdge85     TLSFingerprint = "edge_85"
	FingerprintEdgeLatest TLSFingerprint = "edge_latest"

	// iOS fingerprints
	FingerprintIOS11     TLSFingerprint = "ios_11"
	FingerprintIOS12     TLSFingerprint = "ios_12"
	FingerprintIOS13     TLSFingerprint = "ios_13"
	FingerprintIOS14     TLSFingerprint = "ios_14"
	FingerprintiOSLatest TLSFingerprint = "ios_latest"

	// Android fingerprints
	FingerprintAndroid11     TLSFingerprint = "android_11"
	FingerprintAndroidLatest TLSFingerprint = "android_latest"

	// Default (no fingerprinting)
	FingerprintNone    TLSFingerprint = ""
	FingerprintDefault TLSFingerprint = "default"
)

// GetClientHelloID converts a TLSFingerprint string to a utls.ClientHelloID
func GetClientHelloID(fingerprint TLSFingerprint) utls.ClientHelloID {
	switch fingerprint {
	// Chrome
	case FingerprintChrome120:
		return utls.HelloChrome_120
	case FingerprintChrome131:
		return utls.HelloChrome_131
	case FingerprintChrome133:
		return utls.HelloChrome_133
	case FingerprintChromeLatest, FingerprintDefault:
		return utls.HelloChrome_Auto

	// Firefox
	case FingerprintFirefox102:
		return utls.HelloFirefox_102
	case FingerprintFirefox105:
		return utls.HelloFirefox_105
	case FingerprintFirefox120:
		return utls.HelloFirefox_120
	case FingerprintFirefoxLatest:
		return utls.HelloFirefox_Auto

	// Safari
	case FingerprintSafari16, FingerprintSafariLatest:
		return utls.HelloSafari_16_0

	// Edge
	case FingerprintEdge85, FingerprintEdgeLatest:
		return utls.HelloEdge_85

	// iOS
	case FingerprintIOS11:
		return utls.HelloIOS_11_1
	case FingerprintIOS12:
		return utls.HelloIOS_12_1
	case FingerprintIOS13:
		return utls.HelloIOS_13
	case FingerprintIOS14, FingerprintiOSLatest:
		return utls.HelloIOS_14

	// Android
	case FingerprintAndroid11, FingerprintAndroidLatest:
		return utls.HelloAndroid_11_OkHttp

	// None/Default
	case FingerprintNone:
		return utls.HelloGolang
	default:
		log.Warnf("Unknown TLS fingerprint: %s, using Chrome Auto", fingerprint)
		return utls.HelloChrome_Auto
	}
}

// uTLSDialer wraps a dialer and applies uTLS fingerprinting
type uTLSDialer struct {
	dialer      *net.Dialer
	config      *tls.Config
	fingerprint utls.ClientHelloID
}

// newUTLSDialer creates a new uTLS dialer with the specified fingerprint
func newUTLSDialer(fingerprint TLSFingerprint, tlsConfig *tls.Config) *uTLSDialer {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: false,
		}
	}

	return &uTLSDialer{
		dialer:      &net.Dialer{},
		config:      tlsConfig,
		fingerprint: GetClientHelloID(fingerprint),
	}
}

// DialContext performs a TLS handshake using the specified fingerprint
func (d *uTLSDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Establish TCP connection
	conn, err := d.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	// Extract hostname for SNI
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	// Create uTLS config based on standard tls.Config
	uConfig := &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: d.config.InsecureSkipVerify,
		MinVersion:         d.config.MinVersion,
		MaxVersion:         d.config.MaxVersion,
		CipherSuites:       d.config.CipherSuites,
		RootCAs:            d.config.RootCAs,
	}

	// Create uTLS connection with fingerprint
	uConn := utls.UClient(conn, uConfig, d.fingerprint)

	// Perform TLS handshake
	if err := uConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}

	return uConn, nil
}

// DialTLSContext is a convenience wrapper for DialContext
func (d *uTLSDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.DialContext(ctx, network, addr)
}

// CreateUTLSTransport creates an HTTP transport with uTLS fingerprinting
func CreateUTLSTransport(fingerprint TLSFingerprint, baseTransport *http.Transport) *http.Transport {
	if fingerprint == FingerprintNone || fingerprint == "" {
		log.Debug("uTLS fingerprinting disabled")
		if baseTransport != nil {
			return baseTransport
		}
		return http.DefaultTransport.(*http.Transport).Clone()
	}

	// Clone base transport or create new one
	var transport *http.Transport
	if baseTransport != nil {
		transport = baseTransport.Clone()
	} else {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}

	// Create uTLS dialer
	dialer := newUTLSDialer(fingerprint, transport.TLSClientConfig)

	// Replace the DialTLS function with our uTLS implementation
	transport.DialTLSContext = dialer.DialTLSContext

	log.Infof("uTLS fingerprinting enabled with profile: %s", fingerprint)
	return transport
}

// ApplyUTLSToClient applies uTLS fingerprinting to an existing HTTP client
func ApplyUTLSToClient(client *http.Client, fingerprint TLSFingerprint) *http.Client {
	if client == nil {
		client = &http.Client{}
	}

	if fingerprint == FingerprintNone || fingerprint == "" {
		log.Debug("Skipping uTLS for client (fingerprint disabled)")
		return client
	}

	// Get or create base transport
	var baseTransport *http.Transport
	if client.Transport != nil {
		if t, ok := client.Transport.(*http.Transport); ok {
			baseTransport = t
		} else {
			log.Warn("Client transport is not *http.Transport, creating new one")
			baseTransport = nil
		}
	}

	// Apply uTLS fingerprinting
	client.Transport = CreateUTLSTransport(fingerprint, baseTransport)

	return client
}
