package main

import (
	"fmt"
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"log"
)

// take a client certificate in string format and return the decoded and parsed certificate
func getCert(cert string) *x509.Certificate {
	block, _ := pem.Decode([]byte(cert))
	if block == nil {
        log.Println("Failed to decode the certificate PEM")
		return nil
	}
	certParsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Println("Failed to parse certificate: " + err.Error())
		return nil
	}
	return certParsed
}

func getCertFPHex(cert *x509.Certificate) string {
	hash := sha1.Sum(cert.Raw)
	return fmt.Sprintf("%x", hash)
}

func getCertFPString(cert *x509.Certificate) string {
	hash := sha1.Sum(cert.Raw)
	var buf bytes.Buffer
	for _, f := range hash {
		fmt.Fprintf(&buf, "%02X", f)
	}
	return buf.String()
}
