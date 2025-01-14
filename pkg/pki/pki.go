/* -------------------------------------------------------------------------- *\
 *             Apache 2.0 License Copyright © 2022 The Aurae Authors          *
 *                                                                            *
 *                +--------------------------------------------+              *
 *                |   █████╗ ██╗   ██╗██████╗  █████╗ ███████╗ |              *
 *                |  ██╔══██╗██║   ██║██╔══██╗██╔══██╗██╔════╝ |              *
 *                |  ███████║██║   ██║██████╔╝███████║█████╗   |              *
 *                |  ██╔══██║██║   ██║██╔══██╗██╔══██║██╔══╝   |              *
 *                |  ██║  ██║╚██████╔╝██║  ██║██║  ██║███████╗ |              *
 *                |  ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝ |              *
 *                +--------------------------------------------+              *
 *                                                                            *
 *                         Distributed Systems Runtime                        *
 *                                                                            *
 * -------------------------------------------------------------------------- *
 *                                                                            *
 *   Licensed under the Apache License, Version 2.0 (the "License");          *
 *   you may not use this file except in compliance with the License.         *
 *   You may obtain a copy of the License at                                  *
 *                                                                            *
 *       http://www.apache.org/licenses/LICENSE-2.0                           *
 *                                                                            *
 *   Unless required by applicable law or agreed to in writing, software      *
 *   distributed under the License is distributed on an "AS IS" BASIS,        *
 *   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. *
 *   See the License for the specific language governing permissions and      *
 *   limitations under the License.                                           *
 *                                                                            *
\* -------------------------------------------------------------------------- */

package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type AuraeCA struct {
	Certificate string `json:"cert" yaml:"cert"`
	Key         string `json:"key" yaml:"key"`
}

func CreateAuraeRootCA(path string, domainName string) (*AuraeCA, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return &AuraeCA{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	template := x509.Certificate{}
	template.Subject = pkix.Name{
		Organization:       []string{"Aurae"},
		OrganizationalUnit: []string{"Runtime"},
		StreetAddress:      []string{"aurae"},
		Locality:           []string{"aurae"},
		Country:            []string{"IS"},
		CommonName:         domainName,
	}
	template.NotBefore = time.Now()
	template.NotAfter = template.NotBefore.Add(24 * time.Hour * 9999)
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.DNSNames = []string{domainName}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	template.SerialNumber, err = rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return &AuraeCA{}, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// To get an AuthorityKeyId which is equal to SubjectKeyId, we are manually
	// setting it according to the rules of x509.go.
	//
	// From X509 docs:
	// > The AuthorityKeyId will be taken from the SubjectKeyId of parent, if any,
	// > unless the resulting certificate is self-signed. Otherwise the value from
	// > template will be used.
	//
	// > If SubjectKeyId from template is empty and the template is a CA, SubjectKeyId
	// > will be generated from the hash of the public key.
	//
	// We need a hash of the publickey, so hopefully this link is right
	// https://stackoverflow.com/questions/52502511/how-to-generate-bytes-array-from-publickey#comment92269419_52502639
	pubHash := sha1.Sum(priv.PublicKey.N.Bytes())
	template.SubjectKeyId = pubHash[:]
	template.AuthorityKeyId = template.SubjectKeyId

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return &AuraeCA{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	certBuf, err := getCertBuf(derBytes)
	if err != nil {
		return &AuraeCA{}, err
	}
	keyBuf, err := getKeyBuf(priv)
	if err != nil {
		return &AuraeCA{}, err
	}

	ca := &AuraeCA{
		Certificate: certBuf.String(),
		Key:         keyBuf.String(),
	}

	if path != "" {
		err = createCAFiles(path, ca)
		if err != nil {
			return ca, err
		}
	}

	return ca, nil
}

func createCAFiles(path string, ca *AuraeCA) error {
	path = filepath.Clean(path)
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	crtPath := filepath.Join(path, "ca.crt")
	keyPath := filepath.Join(path, "ca.key")

	err = writeStringToFile(crtPath, ca.Certificate)
	if err != nil {
		return err
	}

	err = writeStringToFile(keyPath, ca.Key)
	if err != nil {
		return err
	}
	return nil
}

func writeStringToFile(p string, s string) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", p, err)
	}
	defer f.Close()

	_, err = f.WriteString(s)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", p, err)
	}

	return nil
}

func getCertBuf(derBytes []byte) (*bytes.Buffer, error) {
	var certBytes []byte
	certBuf := bytes.NewBuffer(certBytes)
	err := pem.Encode(certBuf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
	if err != nil {
		return &bytes.Buffer{}, fmt.Errorf("failed to write certificate buffer: %w", err)
	}

	return certBuf, nil
}

func getKeyBuf(priv *rsa.PrivateKey) (*bytes.Buffer, error) {
	var keyBytes []byte
	keyBuf := bytes.NewBuffer(keyBytes)
	err := pem.Encode(keyBuf, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	if err != nil {
		return &bytes.Buffer{}, fmt.Errorf("failed to write private key buffer: %w", err)
	}

	return keyBuf, nil
}
