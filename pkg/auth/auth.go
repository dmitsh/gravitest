package auth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"google.golang.org/grpc/credentials"
)

func GetTLS(crt, key, caCrt string, isServer bool) (credentials.TransportCredentials, error) {
	certificate, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %v", err)
	}

	ca, err := ioutil.ReadFile(caCrt)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %v", err)
	}

	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to add ca certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    capool,
	}
	if isServer {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = capool
	} else {
		tlsConfig.RootCAs = capool
	}

	return credentials.NewTLS(tlsConfig), nil
}
