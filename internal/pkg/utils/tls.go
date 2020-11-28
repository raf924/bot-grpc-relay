package utils

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io/ioutil"
)

func loadTlsConfig(caFile string, certFile string, keyFile string) (*tls.Config, *x509.CertPool, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, err
	}
	caPool := x509.NewCertPool()
	pem, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, nil, err
	}
	if !caPool.AppendCertsFromPEM(pem) {
		return nil, nil, errors.New("couldn't use CA certificate")

	}
	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}, caPool, nil
}

func LoadTLSClientConfig(serverName string, caFile string, certFile string, keyFile string) (grpc.DialOption, error) {
	tlsConfig, caPool, err := loadTlsConfig(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.RootCAs = caPool
	tlsConfig.ServerName = serverName
	return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
}

func LoadTLSServerConfig(caFile string, certFile string, keyFile string) (grpc.ServerOption, error) {
	tlsConfig, caPool, err := loadTlsConfig(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.ClientCAs = caPool
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	serverCreds := credentials.NewTLS(tlsConfig)
	return grpc.Creds(serverCreds), nil
}
