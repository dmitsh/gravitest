package auth

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

func GetTLS(crt, key, caCrt string, isServer bool) (credentials.TransportCredentials, error) {
	certificate, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %v", err)
	}

	ca, err := os.ReadFile(caCrt)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %v", err)
	}

	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(ca) {
		return nil, errors.New("failed to add ca certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    capool,

		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		PreferServerCipherSuites: true,
	}
	if isServer {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = capool
	} else {
		tlsConfig.RootCAs = capool
	}

	return credentials.NewTLS(tlsConfig), nil
}
