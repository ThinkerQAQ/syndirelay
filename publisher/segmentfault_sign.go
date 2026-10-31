package publisher

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/url"
	"sort"
	"time"
)

const (
	segmentFaultKeyVersion   = "24.11.06"
	segmentFaultIVDSite      = "sf.gg"
	segmentFaultPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkof1rwl+U32URzw5lqqy
z4E+aKwu+f+A6/aSNvnSe62m6r/rjyb9WiRs7E1jfibgU196GNFX1+XxRaATFkTI
9Mzapr8qn+yR3b/xWAECU7PCR366ovhHSJlIozmmkkb1kwjdR6okUWKIHg7heq9v
z9cExuvN+whHjDjSKAQX9/1Sqv3py/Yo9+MkRC8Q5KhupYBBmgLAUtqL6ghU3HS6
Nnwx2CA13RyonLDB+Dh59l+j11Rf85ANL4XrD7dxCDsCFvjTBGIYm41F3qHne0fu
XPKTsamLHjkiMV3NPxVAlMQXUF71ZoSEO4cITaZZCpI5H9GSh16ZebBtSC6RSvek
pQIDAQAB
-----END PUBLIC KEY-----`
)

func segmentFaultPublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(segmentFaultPublicKeyPEM))
	if block == nil {
		return nil, errors.New("SegmentFault signing public key is invalid")
	}
	value, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := value.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("SegmentFault signing key is not RSA")
	}
	return key, nil
}

func segmentFaultSignedURL(rawURL string, now time.Time) (string, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := target.Query()
	payload := map[string]any{}
	if len(query) == 0 {
		payload["ivd_site"] = segmentFaultIVDSite
	} else {
		keys := make([]string, 0, len(query))
		for key := range query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			values := query[key]
			if len(values) == 1 {
				payload[key] = values[0]
			} else {
				payload[key] = append([]string{}, values...)
			}
		}
	}
	payload["timestamp"] = now.Unix()
	plain, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	publicKey, err := segmentFaultPublicKey()
	if err != nil {
		return "", err
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, plain)
	if err != nil {
		return "", err
	}
	query.Set("keyv", segmentFaultKeyVersion)
	query.Set("ivd", base64.StdEncoding.EncodeToString(ciphertext))
	target.RawQuery = query.Encode()
	return target.String(), nil
}
