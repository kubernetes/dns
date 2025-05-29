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

func createTestCertFiles(t *testing.T) (certFile, keyFile, caFile string, cleanup func()) {
	// Generate CA certificate
	caCert, caKey, err := generateCA()
	if err != nil {
		t.Fatalf("Failed to generate CA certificate: %v", err)
	}

	// Generate server certificate signed by CA
	cert, key, err := generateCert(caCert, caKey)
	if err != nil {
		t.Fatalf("Failed to generate server certificate: %v", err)
	}

	// Write certificates to temporary files
	certFile = writeTempFile(t, string(cert))
	keyFile = writeTempFile(t, string(key))
	caFile = writeTempFile(t, string(caCert))

	// Return cleanup function to remove files
	cleanup = func() {
		os.Remove(certFile)
		os.Remove(keyFile)
		os.Remove(caFile)
	}
	return certFile, keyFile, caFile, cleanup
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

func loadTestClientCert(t *testing.T, certFile, keyFile string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load client certificate: %v", err)
	}
	return cert
}

func writeTempFile(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "testcert")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	return tmpFile.Name()
}

func TestMetricsTLS(t *testing.T) {
	certFile, keyFile, caFile, cleanup := createTestCertFiles(t)
	defer cleanup()

	tests := []struct {
		name        string
		tlsConfig   *tlsConfig
		expectError bool
		expectHTTPS bool
	}{
		{
			name:        "No TLS",
			tlsConfig:   nil,
			expectError: false,
			expectHTTPS: false,
		},
		{
			name: "Valid TLS",
			tlsConfig: &tlsConfig{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				MinVersion: tls.VersionTLS13,
			},
			expectError: false,
			expectHTTPS: true,
		},
		{
			name: "TLS with client auth",
			tlsConfig: &tlsConfig{
				Enabled:        true,
				CertFile:       certFile,
				KeyFile:        keyFile,
				ClientCAFile:   caFile,
				ClientAuthType: "RequireAndVerifyClientCert",
				MinVersion:     tls.VersionTLS13,
			},
			expectError: false,
			expectHTTPS: true,
		},
		{
			name: "Missing cert file",
			tlsConfig: &tlsConfig{
				Enabled:    true,
				CertFile:   "missing.pem",
				KeyFile:    keyFile,
				MinVersion: tls.VersionTLS13,
			},
			expectError: true,
			expectHTTPS: false,
		},
		{
			name: "Missing key file",
			tlsConfig: &tlsConfig{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    "missing-key.pem",
				MinVersion: tls.VersionTLS13,
			},
			expectError: true,
			expectHTTPS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			met := New("localhost:0", tt.tlsConfig)
			// met.tlsConfig = tt.tlsConfig

			// Start server
			err := met.OnStartup()
			if tt.expectError {
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

			// Configure client
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
			}
			if tt.tlsConfig != nil && tt.tlsConfig.ClientCAFile != "" {
				// Load CA cert for client auth
				caCert, err := os.ReadFile(tt.tlsConfig.ClientCAFile)
				if err != nil {
					t.Fatalf("Failed to read CA cert: %v", err)
				}
				caCertPool := x509.NewCertPool()
				if !caCertPool.AppendCertsFromPEM(caCert) {
					t.Fatal("Failed to parse CA cert")
				}
				tlsConfig.RootCAs = caCertPool
				tlsConfig.Certificates = []tls.Certificate{loadTestClientCert(t, certFile, keyFile)}
			}

			client := &http.Client{
				Timeout: 10 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: tlsConfig,
					// Allow connection reuse
					MaxIdleConns:        10,
					IdleConnTimeout:     30 * time.Second,
					DisableCompression:  true,
					TLSHandshakeTimeout: 10 * time.Second,
				},
			}

			// Determine protocol
			protocol := "http"
			if tt.expectHTTPS {
				protocol = "https"
			}

			// Try multiple times to account for server startup time
			var resp *http.Response
			var err2 error
			for i := range 10 { // Increased retry count
				url := fmt.Sprintf("%s://%s/metrics", protocol, ListenAddr)
				t.Logf("Attempt %d: Connecting to %s", i+1, url)
				resp, err2 = client.Get(url)
				if err2 == nil {
					t.Logf("Successfully connected to metrics server")
					break
				}
				t.Logf("Connection attempt failed: %v", err2)
				time.Sleep(500 * time.Millisecond) // Increased delay between attempts
			}
			if err2 != nil {
				t.Fatalf("Failed to connect to metrics server: %v", err2)
			}
			if resp != nil {
				defer resp.Body.Close()
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}
