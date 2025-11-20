/*
Copyright 2021 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	serverCertFile = "test_data/server.crt"
	serverKeyFile  = "test_data/server.key"
	clientCertFile = "test_data/client_selfsigned.crt"
	clientKeyFile  = "test_data/client_selfsigned.key"
	tlsCaChainFile = "test_data/tls-ca-chain.pem"
)

func createTestCertFiles(t *testing.T) error {
	// Generate CA certificate
	caCert, caKey, err := generateCA()
	if err != nil {
		t.Fatalf("Failed to generate CA certificate: %v", err)
		return err
	}

	// Generate server certificate signed by CA
	cert, key, err := generateCert(caCert, caKey)
	if err != nil {
		t.Fatalf("Failed to generate server certificate: %v", err)
		return err
	}

	// Generate client CA certificate
	clientCaCert, clientCaKey, err := generateCA()
	if err != nil {
		t.Fatalf("Failed to generate client CA certificate: %v", err)
		return err
	}

	// Generate client certificate signed by CA
	clientCert, clientKey, err := generateCert(clientCaCert, clientCaKey)
	if err != nil {
		t.Fatalf("Failed to generate client certificate: %v", err)
		return err
	}

	// Create ca chain file
	caChain := append(caCert, clientCaCert...)

	// Write certificates to temporary files
	err = writeFile(t, string(cert), serverCertFile)
	if err != nil {
		t.Fatalf("Failed to write server certificate: %v", err)
		return err
	}
	err = writeFile(t, string(key), serverKeyFile)
	if err != nil {
		t.Fatalf("Failed to write server key: %v", err)
		return err
	}
	err = writeFile(t, string(clientCert), clientCertFile)
	if err != nil {
		t.Fatalf("Failed to write client certificate: %v", err)
		return err
	}
	err = writeFile(t, string(clientKey), clientKeyFile)
	if err != nil {
		t.Fatalf("Failed to write client key: %v", err)
		return err
	}
	err = writeFile(t, string(caChain), tlsCaChainFile)
	if err != nil {
		t.Fatalf("Failed to write CA certificate: %v", err)
		return err
	}

	return nil
}

func generateCA() ([]byte, []byte, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	return caPEM, caPrivKeyPEM, nil
}

func generateCert(caCertPEM, caKeyPEM []byte) ([]byte, []byte, error) {
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			Organization: []string{"Test Server"},
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	return certPEM, certPrivKeyPEM, nil
}

func cleanupTestCertFiles() {
	os.Remove(serverCertFile)
	os.Remove(serverKeyFile)
	os.Remove(clientCertFile)
	os.Remove(clientKeyFile)
	os.Remove(tlsCaChainFile)
}

func writeFile(t *testing.T, content, path string) error {
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return err
	}
	return nil
}

func getTLSClient(clientCertName bool) *http.Client {
	cert, err := os.ReadFile(tlsCaChainFile)
	if err != nil {
		panic("Unable to start TLS client. Check cert path")
	}

	var clientCertficate tls.Certificate
	if clientCertName {
		clientCertficate, err = tls.LoadX509KeyPair(
			clientCertFile,
			clientKeyFile,
		)
		if err != nil {
			panic(fmt.Sprintf("failed to load client certificate: %v", err))
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: func() *x509.CertPool {
					caCertPool := x509.NewCertPool()
					caCertPool.AppendCertsFromPEM(cert)
					return caCertPool
				}(),
				GetClientCertificate: func(req *tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return &clientCertficate, nil
				},
			},
		},
	}
	return client
}

func TestMetricsTLS(t *testing.T) {
	err := createTestCertFiles(t)
	if err != nil {
		t.Fatalf("Failed to create test certificate files: %v", err)
	}
	defer cleanupTestCertFiles()

	tests := []struct {
		name               string
		tlsConfigPath      string
		UseTLSClient       bool
		clientCertificate  bool
		caFile             string
		expectStartupError bool
		expectRequestError bool
	}{
		{
			name:          "No TLS config: starts a HTTP server, connect successfully with default client",
			tlsConfigPath: "",
		},
		{
			name:               "No TLS config: starts HTTP server, connection fails with TLS client",
			tlsConfigPath:      "",
			UseTLSClient:       true,
			expectRequestError: true,
		},
		{
			name:          "Empty TLS config: starts a HTTP server",
			tlsConfigPath: "test_data/configs/empty.yml",
		},
		{
			name:          "Valid TLS config, no client cert, successful connection with TLS client",
			tlsConfigPath: "test_data/configs/valid_verifyclientcertifgiven.yml",
			UseTLSClient:  true,
		},
		{
			name:               `Valid TLS config, connection fails with default client`,
			tlsConfigPath:      "test_data/configs/valid_verifyclientcertifgiven.yml",
			expectRequestError: true,
		},
		{
			name:              `Valid TLS config with RequireAnyClientCert, connection succeeds with TLS client presenting (valid) certificate`,
			tlsConfigPath:     "test_data/configs/valid_requireanyclientcert.yml",
			UseTLSClient:      true,
			clientCertificate: true,
		},
		{
			name:               "Wrong path to TLS config file fails to start server",
			tlsConfigPath:      "test_data/configs/this-does-not-exist.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
		{
			name:               `TLS config hasinvalid structure, fails to start server`,
			tlsConfigPath:      "test_data/configs/junk.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
		{
			name:               "Missing key file, fails to start server",
			tlsConfigPath:      "test_data/configs/keyPath_empty.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
		{
			name:               "Missing cert file, fails to start server",
			tlsConfigPath:      "test_data/configs/certPath_empty.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
		{
			name:               "Wrong key file path, fails to start server",
			tlsConfigPath:      "test_data/configs/keyPath_invalid.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
		{
			name:               "Wrong cert file path, fails to start server",
			tlsConfigPath:      "test_data/configs/certPath_invalid.yml",
			UseTLSClient:       true,
			expectStartupError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			met := New("localhost:0")
			met.tlsConfigPath = tt.tlsConfigPath

			// Start server
			err := met.OnStartup()
			if tt.expectStartupError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Failed to start metrics handler: %s", err)
			}
			defer met.OnFinalShutdown()

			// Wait for server to be ready
			select {
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for server to start")
			case <-func() chan struct{} {
				ch := make(chan struct{})
				go func() {
					for {
						conn, err := net.DialTimeout("tcp", ListenAddr, 100*time.Millisecond)
						if err == nil {
							conn.Close()
							close(ch)
							return
						}
						time.Sleep(100 * time.Millisecond)
					}
				}()
				return ch
			}():
			}

			// Create appropriate client and protocol
			var client *http.Client
			var protocol string
			if tt.UseTLSClient {
				client = getTLSClient(tt.clientCertificate)
				protocol = "https"
			} else {
				client = http.DefaultClient
				protocol = "http"
			}

			// Try multiple times to account for server startup time
			var resp *http.Response
			var err2 error
			for i := range 10 {
				url := fmt.Sprintf("%s://%s/metrics", protocol, ListenAddr)
				t.Logf("Attempt %d: Connecting to %s", i+1, url)
				resp, err2 = client.Get(url)
				if err2 == nil {
					t.Logf("Successfully connected to metrics server")
					break
				}
				t.Logf("Connection attempt failed: %v", err2)
				time.Sleep(200 * time.Millisecond)
			}
			if err2 != nil {
				if tt.expectRequestError {
					return
				}
				t.Fatalf("Failed to connect to metrics server: %v", err2)
			}
			if resp != nil {
				defer resp.Body.Close()
			}

			if tt.expectRequestError {
				// If we expect a request error but got a response, check if it's a bad status code
				// which indicates the connection succeeded but the request was invalid (e.g., HTTP to HTTPS server)
				if resp.StatusCode == http.StatusBadRequest {
					// Got expected error response
					return
				}
				// Got unexpected response status
				t.Fatalf("Expected request error with status %d but got response with status %d", http.StatusBadRequest, resp.StatusCode)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestMetricsHTTPTimeout(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	// Use context with timeout to prevent test from hanging indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		conn, err := net.Dial("tcp", ListenAddr)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		// Send partial HTTP request and then stop sending data
		// This will cause the server to wait for more data and hit ReadTimeout
		partialRequest := "GET /metrics HTTP/1.1\r\nHost: " + ListenAddr + "\r\nContent-Length: 100\r\n\r\n"
		_, err = conn.Write([]byte(partialRequest))
		if err != nil {
			done <- err
			return
		}

		// Now just wait - server should timeout trying to read the remaining data
		// If server has no ReadTimeout, this will hang indefinitely
		buffer := make([]byte, 1024)
		_, err = conn.Read(buffer)
		done <- err
	}()

	select {
	case <-done:
		t.Log("HTTP request timed out by server")
	case <-ctx.Done():
		t.Error("HTTP request did not time out")
	}
}
