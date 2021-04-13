package utils

import (
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io/ioutil"
	"path"
)

func loadCaPool(caFile string) (*x509.CertPool, error) {
	caPool := x509.NewCertPool()
	pem, err := ioutil.ReadFile(path.Clean(caFile))
	if err != nil {
		return nil, err
	}
	if !caPool.AppendCertsFromPEM(pem) {
		return nil, errors.New("couldn't use CA certificate")
	}
	return caPool, nil
}

func loadTlsConfig(caFile string, certFile string, keyFile string) (*tls.Config, *x509.CertPool, error) {
	certificate, err := tls.LoadX509KeyPair(path.Clean(certFile), path.Clean(keyFile))
	if err != nil {
		return nil, nil, err
	}
	caPool, err := loadCaPool(caFile)
	if err != nil {
		return nil, nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}, caPool, nil
}

func LoadMutualTLSClientConfig(serverName string, caFile string, certFile string, keyFile string) (grpc.DialOption, error) {
	tlsConfig, caPool, err := loadTlsConfig(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.RootCAs = caPool
	tlsConfig.ServerName = serverName
	return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
}

func LoadTLSClientConfig(serverName string, caFile string) (grpc.DialOption, error) {
	caPool, err := loadCaPool(caFile)
	if err != nil {
		return nil, err
	}
	return grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(caPool, serverName)), nil
}

func LoadTLSServerConfig(caFile string, certFile string, keyFile string) (grpc.ServerOption, error) {
	tlsConfig, caPool, err := loadTlsConfig(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.ClientCAs = caPool
	tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	serverCreds := credentials.NewTLS(tlsConfig)
	return grpc.Creds(serverCreds), nil
}

func CheckHash(s string, hash string) bool {
	shaHash := sha512.New()
	shaHash.Write([]byte(s))
	b := shaHash.Sum(nil)
	return hash == hex.EncodeToString(b)
}

func Hash(username string, password string) string {
	shaHash := sha512.New()
	shaHash.Write([]byte(fmt.Sprintf("%s:%s", username, password)))
	b := shaHash.Sum(nil)
	return hex.EncodeToString(b)
}
