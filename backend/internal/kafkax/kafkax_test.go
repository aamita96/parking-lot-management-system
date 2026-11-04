package kafkax

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"plms/internal/config"
)

// selfSigned makes a throwaway certificate + key in PEM form for the tests.
func selfSigned(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func TestConnectionOptions(t *testing.T) {
	cert, key := selfSigned(t)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "service.cert")
	if err := os.WriteFile(certFile, []byte(cert), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
		opts    int
	}{
		{"local plain Kafka", config.Config{KafkaBrokers: []string{"localhost:9094"}}, false, 1},
		{"SASL SCRAM over TLS", config.Config{KafkaTLS: true, KafkaSASLMechanism: "SCRAM-SHA-256", KafkaUsername: "u", KafkaPassword: "p", KafkaCACert: cert}, false, 3},
		{"client certificate, CA as a file path", config.Config{KafkaTLS: true, KafkaCACert: certFile, KafkaClientCert: cert, KafkaClientKey: key}, false, 2},
		{"unknown SASL mechanism", config.Config{KafkaSASLMechanism: "MAGIC"}, true, 0},
		{"broken CA", config.Config{KafkaTLS: true, KafkaCACert: "-----BEGIN CERTIFICATE-----\nnope\n-----END CERTIFICATE-----"}, true, 0},
		{"missing CA file", config.Config{KafkaTLS: true, KafkaCACert: filepath.Join(dir, "missing.pem")}, true, 0},
		{"key without certificate", config.Config{KafkaTLS: true, KafkaClientKey: key}, true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := ConnectionOptions(c.cfg)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if !c.wantErr && len(opts) != c.opts {
				t.Errorf("got %d options, want %d", len(opts), c.opts)
			}
		})
	}
}
