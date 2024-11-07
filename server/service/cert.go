package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/asn1"
	b64 "encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"nivlheim/utility"
	"os"
	"regexp"
	"software.sslmate.com/src/go-pkcs12"
	"strings"
	"time"
	"unicode"
)

type apiMethodReqCert struct {
	db *sql.DB
}

type apiMethodRenewCert struct {
	db *sql.DB
}

type authKeyId struct {
	KeyIdentifier             []byte       `asn1:"optional,tag:0"`
	AuthorityCertIssuer       generalNames `asn1:"optional,tag:1"`
	AuthorityCertSerialNumber *big.Int     `asn1:"optional,tag:2"`
}

type generalNames struct {
	Name []pkix.RDNSequence `asn1:"tag:4"`
}

func (vars *apiMethodReqCert) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    ipAddr := req.Header.Get("X-Forwarded-For")
	log.Printf("Request for new certificate from %s", ipAddr)

	trustedByCFE := false
	trustedByIP := false

	keyMD5 := req.FormValue("cfe_key_md5")

	// check if keyMD5 is a valid md5
	match, _ := regexp.MatchString("^[a-f0-9]{32}$", keyMD5)

	signature := req.FormValue("sign_b64")

	if keyMD5 != "" && match && signature != "" {
		sign := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, string(signature))

		keyDir := "/var/pubkeys"

		if config.CFEngineKeyDir != "" {
			keyDir = config.CFEngineKeyDir
		}

		pubKey, err := os.ReadFile(keyDir + "/root-MD5=" + keyMD5 + ".pub")
		if err != nil {
			log.Println("Failed to read cfengine key: " + err.Error())
		} else {
			pubKeySPKI, err := convertPKCS1ToSPKI(pubKey)
			if err != nil {
				log.Println("Failed to convert cfengine key: " + err.Error())
			} else {
				sDec, _ := b64.StdEncoding.DecodeString(sign)

				hash := crypto.SHA256
				h := hash.New()
				h.Write([]byte("nivlheim"))
				hashed := h.Sum(nil)

				err = rsa.VerifyPKCS1v15(&pubKeySPKI, crypto.SHA256, hashed, sDec)
				if err != nil {
					log.Println("Failed to verify signature: " + err.Error())
				} else {
					trustedByCFE = true
				}
			}
		}
	}

	if !trustedByCFE {
		var count int
		err := vars.db.QueryRow("SELECT COUNT(*) FROM ipranges WHERE ($1 <<= iprange)", ipAddr).Scan(&count)
		if err != nil {
			log.Println("Failed to query database: " + err.Error())
			http.Error(w, "Failed to query database", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			log.Printf("Client IP %s is in a trusted range", ipAddr)
			trustedByIP = true
		}
	}

	var osHostName string

	if trustedByCFE || trustedByIP {
		osHostName = req.FormValue("hostname")
		if osHostName == "" {
			log.Println("Missing required parameter: hostname")
			http.Error(w, "Missing required parameter: hostname", http.StatusBadRequest)
			return
		}
	}

	var approved sql.NullBool
	var hostName sql.NullString

	if !(trustedByCFE || trustedByIP) {
		err := vars.db.QueryRow("SELECT hostname, approved FROM waiting_for_approval WHERE ipaddr = $1", ipAddr).Scan(&hostName, &approved)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("The host %s has not been pre-approved", ipAddr)
				osHostName = req.FormValue("hostname")
				if osHostName == "" {
					log.Println("Missing required parameter: hostname")
					http.Error(w, "Missing required parameter: hostname", http.StatusBadRequest)
					return
				}
				log.Printf("%s says its hostname is %s", ipAddr, osHostName)
				dnsName := forwardConfirmReverseDNS(ipAddr)
				if dnsName != "" {
					osHostName = dnsName
					log.Printf("DNS lookup says %s is %s", ipAddr, osHostName)
				} else {
					log.Println("DNS lookup is inconclusive")
				}
				log.Printf("Adding %s to the waiting for approval list", osHostName)
				_, err = vars.db.Exec("INSERT INTO waiting_for_approval (ipaddr, hostname, received) VALUES ($1, $2, NOW())", ipAddr, osHostName)
				if err != nil {
					log.Printf("Failed to add %s to the waiting for approval list: %s", osHostName, err.Error())
					http.Error(w, "Failed to add to the waiting for approval list", http.StatusInternalServerError)
					return
				}
				fmt.Fprintln(w, "You have been added to the waiting list")
				return
			} else {
				log.Println("Failed to query database: " + err.Error())
				http.Error(w, "Failed to query database", http.StatusInternalServerError)
				return
			}
		}
		if !approved.Bool {
			log.Printf("The host %s / %s is already on the waiting list", osHostName, ipAddr)
			fmt.Fprint(w, "You are on the waiting list, be patient.")
			return

		}
		osHostName = hostName.String
	}
	// gen key
	clientKey, _ := rsa.GenerateKey(rand.Reader, 4096)

	keyBytes := x509.MarshalPKCS1PrivateKey(clientKey)
	keyString := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}))

	// gen the cert
	csr := genCSR(osHostName, clientKey)
	if csr == nil {
		log.Println("Failed to generate CSR")
		http.Error(w, "Failed to generate CSR", http.StatusInternalServerError)
		return
	}

	caCRT := getCACRT(config.ConfDir + "/" + config.CACertFile)
	caKey, err := getCAKey(config.ConfDir + "/" + config.CAKeyFile)
	if err != nil {
		log.Println("Failed to get CA key")
		http.Error(w, "Failed to get CA key", http.StatusInternalServerError)
		return
	}

	clientCRT, clientText := makeClientCert(csr, caCRT, caKey, osHostName, vars.db)
	if clientCRT == nil {
		log.Println("Failed to generate client certificate")
		http.Error(w, "Failed to generate client certificate", http.StatusInternalServerError)
		return
	}
	clientDER, err := x509.ParseCertificate(clientCRT)
	if err != nil {
		log.Println("Failed to parse client certificate")
		http.Error(w, "Failed to parse client certificate", http.StatusInternalServerError)
		return
	}
	clientFP := getCertFPString(clientDER)

	clientCRTText := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCRT}))

	pass := ""
	caCerts := []*x509.Certificate{caCRT}
	pfxBytes, err := pkcs12.Encode(rand.Reader, clientKey, clientDER, caCerts, pass)
	if err != nil {
		log.Println("Failed to generate pfx: " + err.Error())
		http.Error(w, "Failed to generate pfx", http.StatusInternalServerError)
		return
	}
	sEnc := b64.StdEncoding.EncodeToString(pfxBytes)

	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		_, err = tx.Exec("INSERT INTO certificates(issued,fingerprint,commonname,cert,trusted_by_cfengine) "+
			"VALUES (NOW(), $1, $2, $3, $4)", clientFP, osHostName, clientText+clientCRTText, trustedByCFE)
		if err != nil {
			log.Println("Failed to insert certificate into database: " + err.Error())
			return err
		}
		_, err = tx.Exec("UPDATE certificates SET first=(SELECT certid FROM certificates WHERE fingerprint=$1) "+
			"WHERE fingerprint=$1", clientFP)
		if err != nil {
			log.Println("Failed to update certificate in database: " + err.Error())
			return err
		}
		// everything ok
		return nil
	})
	if err != nil {
		log.Println("Failed to insert certificate into database: " + err.Error())
		http.Error(w, "Failed to insert certificate into database", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, clientCRTText)
	fmt.Fprint(w, keyString)
	fmt.Fprintf(w, "%s%s%s", "-----BEGIN P12-----\n", sEnc, "\n-----END P12-----\n")
}

func (vars *apiMethodRenewCert) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	pemContent := req.Header.Get("Cert-Client-Cert")

	// pem.Decode needs the header and footer to be on their own lines
	match := regexp.MustCompile("(?s)(-{5}BEGIN CERTIFICATE-{5})(.*)(-{5}END CERTIFICATE-{5})")
	pemContent2 := match.ReplaceAll([]byte(pemContent), []byte("$1\n$2\n$3"))

	cert := getCert(pemContent2)
	if cert == nil {
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}

	fingerprint := getCertFPString(cert)

	revoked, err := isRevoked(fingerprint, vars.db)
	if err != nil {
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}
	if revoked {
		http.Error(w, "Your certificate has been revoked.", http.StatusForbidden)
		return
	}

	var osHostName string
	var hostName sql.NullString

	err = vars.db.QueryRow("SELECT hostname from hostinfo WHERE certfp = $1", fingerprint).Scan(&hostName)
	if err != nil || !hostName.Valid || hostName.String == "" {
		if err == sql.ErrNoRows || !hostName.Valid || hostName.String == "" {
			log.Printf("Certificate with fingerprint %s not found in hostinfo table or empty hostname", fingerprint)
			dn := req.Header.Get("Cert-Client-S-DN")
			match = regexp.MustCompile("CN=(.*?),.*$")
			cn := match.ReplaceAll([]byte(dn), []byte("$1"))
			if cn == nil {
				log.Println("Failed to parse CN from certificate")
				http.Error(w, "Failed to parse CN from certificate", http.StatusInternalServerError)
				return
			}
			osHostName = string(cn)
			log.Printf("Hostname in certificate is %s", cn)
		} else {
			log.Println("Failed to query database: " + err.Error())
			http.Error(w, "Failed to query database", http.StatusInternalServerError)
			return
		}
	} else {
		osHostName = hostName.String
	}
	fmt.Fprintf(w, "Your hostname is: %s\n", osHostName)

	// gen key
	clientKey, _ := rsa.GenerateKey(rand.Reader, 4096)

	keyBytes := x509.MarshalPKCS1PrivateKey(clientKey)
	keyString := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}))

	// gen the cert
	csr := genCSR(osHostName, clientKey)
	if csr == nil {
		log.Println("Failed to generate CSR")
		http.Error(w, "Failed to generate CSR", http.StatusInternalServerError)
		return
	}

	caCRT := getCACRT(config.ConfDir + "/" + config.CACertFile)
	caKey, err := getCAKey(config.ConfDir + "/" + config.CAKeyFile)
	if err != nil {
		log.Println("Failed to get CA key")
		http.Error(w, "Failed to get CA key", http.StatusInternalServerError)
		return
	}
	clientCRT, clientText := makeClientCert(csr, caCRT, caKey, osHostName, vars.db)
	if clientCRT == nil {
		log.Println("Failed to generate client certificate")
		http.Error(w, "Failed to generate client certificate", http.StatusInternalServerError)
		return
	}
	clientDER, err := x509.ParseCertificate(clientCRT)
	if err != nil {
		log.Println("Failed to parse client certificate")
		http.Error(w, "Failed to parse client certificate", http.StatusInternalServerError)
		return
	}
	clientFP := getCertFPString(clientDER)

	clientCRTText := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCRT}))

	pass := ""
	caCerts := []*x509.Certificate{caCRT}
	pfxBytes, err := pkcs12.Encode(rand.Reader, clientKey, clientDER, caCerts, pass)
	if err != nil {
		log.Println("Failed to generate pfx: " + err.Error())
		http.Error(w, "Failed to generate pfx", http.StatusInternalServerError)
		return
	}
	sEnc := b64.StdEncoding.EncodeToString(pfxBytes)

	var previous sql.NullInt32
	var first sql.NullInt32
	var trustedByCFE sql.NullBool

	err = vars.db.QueryRow("SELECT certid, first, trusted_by_cfengine FROM certificates WHERE fingerprint = $1", fingerprint).Scan(&previous, &first, &trustedByCFE)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Certificate with fingerprint %s not found in certificates table", fingerprint)
			http.Error(w, "Certificate not found in database", http.StatusNotFound)
		} else {
			log.Println("Failed to query database: " + err.Error())
			http.Error(w, "Failed to query database", http.StatusInternalServerError)
			return
		}
	}

	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		_, err = tx.Exec("INSERT INTO certificates(issued,fingerprint,commonname,previous,first,cert,trusted_by_cfengine) "+
			"VALUES (NOW(), $1, $2, $3, $4, $5, $6)", clientFP, osHostName, previous.Int32, first.Int32, clientText+clientCRTText, trustedByCFE.Bool)
		if err != nil {
			log.Println("Failed to insert certificate into database: " + err.Error())
			return err
		}
		_, err = tx.Exec("UPDATE hostinfo SET certfp = $1 WHERE certfp = $2 ",
			clientFP, fingerprint)
		if err != nil {
			log.Println("Failed to update hostinfo table " + err.Error())
			return err
		}

		_, err = tx.Exec("UPDATE files SET certfp = $1 WHERE certfp = $2", clientFP, fingerprint)
		if err != nil {
			log.Println("Failed to update files table " + err.Error())
			return err
		}
		// everything ok
		return nil
	})

	if err != nil {
		log.Println("Failed to insert certificate into database: " + err.Error())
		http.Error(w, "Failed to insert certificate into database", http.StatusInternalServerError)
		return
	}

	requestURL := fmt.Sprintf("http://nivlheimapi:4040/api/internal/replaceCertificate?old=%s&new=%s", fingerprint, clientFP)
	_, _ = http.Get(requestURL)

	fmt.Fprint(w, clientCRTText)
	fmt.Fprint(w, keyString)
	fmt.Fprintf(w, "%s%s%s", "-----BEGIN P12-----\n", sEnc, "\n-----END P12-----\n")

	log.Printf("Created new certificate with id %s for hostname %s", fingerprint, osHostName)
}

func isRevoked(fingerprint string, db *sql.DB) (bool, error) {
	var revoked bool
	log.Printf("Checking revocation status for fingerprint: %s", fingerprint)
	err := db.QueryRow("SELECT revoked FROM certificates WHERE fingerprint=$1", fingerprint).Scan(&revoked)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Could not find certificate in database when checking revocation status. Fingerprint: %s", fingerprint)
			return false, err
		} else {
			log.Println(err.Error())
			return false, err
		}
	}
	return revoked, nil
}

func getCert(cert []byte) *x509.Certificate {
	block, _ := pem.Decode(cert)
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

func getCertFPString(cert *x509.Certificate) string {
	hash := sha1.Sum(cert.Raw)
	var buf bytes.Buffer
	for _, f := range hash {
		fmt.Fprintf(&buf, "%02X", f)
	}
	return buf.String()
}

func convertPKCS1ToSPKI(key []byte) (rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		log.Println("Failed to decode the private key PEM")
		return rsa.PublicKey{}, errors.New("failed to decode the private key PEM")
	}
	keyParsed, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		log.Println("Failed to parse private key: " + err.Error())
		return rsa.PublicKey{}, err
	}
	return *keyParsed, nil
}

func genCSR(hostname string, key *rsa.PrivateKey) []byte {
	san := []string{hostname}
	subj := pkix.Name{
		CommonName:         hostname,
		Country:            []string{"NO"},
		Province:           []string{"Norway"},
		Locality:           []string{"Oslo"},
		Organization:       []string{"UiO"},
		OrganizationalUnit: []string{"USIT"},
	}

	template := x509.CertificateRequest{
		Subject:            subj,
		DNSNames:           san,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, key)
	if err != nil {
		log.Println("Failed to create CSR: " + err.Error())
		return nil
	}
	return csrBytes
}

func getCACRT(filename string) *x509.Certificate {
	caCRTFile, err := os.ReadFile(filename)
	if err != nil {
		log.Println("Failed to read CA certificate: " + err.Error())
		return nil
	}

	caCRT := getCert(caCRTFile)
	if caCRT == nil {
		log.Println("Failed to parse CA certificate")
		return nil
	}
	return caCRT
}

func getCAKey(fileName string) (*rsa.PrivateKey, error) {
	caKeyFile, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Println("could not read key file")
		return nil, err
	}
	key, err := getKey(caKeyFile)
	if err != nil {
		fmt.Println("could not get ca key")
		return nil, err
	}
	return key, nil
}

func getKey(key []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		fmt.Println("failed to decode key")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fmt.Println("failed to parse key")
	}
	return parsedKey.(*rsa.PrivateKey), nil
}

func makeClientCert(csr []byte, caCRT *x509.Certificate, caKey *rsa.PrivateKey, hostname string, db *sql.DB) ([]byte, string) {
	c, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		fmt.Println("failed to parse csr")
		return nil, ""
	}

	subjectKeyId := pkix.Extension{}
	subjectKeyId.Id = asn1.ObjectIdentifier{2, 5, 29, 14}
	subjectKeyId.Critical = false
	keyBytes := x509.MarshalPKCS1PublicKey(c.PublicKey.(*rsa.PublicKey))
	hasher := sha1.New()
	hasher.Write(keyBytes)
	ski := hasher.Sum(nil)
	var skiFP bytes.Buffer
	for i, f := range ski {
		fmt.Fprintf(&skiFP, "%02X", f)
		if i < len(ski)-1 {
			skiFP.WriteByte(':')
		}
	}
	ski2, err := asn1.Marshal(ski)
	if err != nil {
		fmt.Println("failed to marshal subject key id")
		return nil, ""
	}
	subjectKeyId.Value = ski2

	authKeyId := pkix.Extension{}
	authKeyId.Id = asn1.ObjectIdentifier{2, 5, 29, 35}
	authKeyId.Critical = false
	aKIValue, err := gen(caCRT)
	if err != nil {
		fmt.Println("failed to generate auth key id")
		return nil, ""
	}
	authKeyId.Value = aKIValue

	san := []string{hostname}

	var serial sql.NullInt64
	err = db.QueryRow("SELECT nextval('cert_serial_seq')").Scan(&serial)
	if err != nil {
		fmt.Println("failed to get serial number")
		return nil, ""
	}

	clientCRTTemplate := x509.Certificate{
		Signature:          c.Signature,
		SignatureAlgorithm: c.SignatureAlgorithm,

		PublicKeyAlgorithm: c.PublicKeyAlgorithm,
		PublicKey:          c.PublicKey,

		Subject:      c.Subject,
		SerialNumber: big.NewInt(serial.Int64),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),

		DNSNames:        san,
		ExtraExtensions: []pkix.Extension{subjectKeyId, authKeyId},
	}

	// create client certificate from template and CA public key
	clientCRTRaw, err := x509.CreateCertificate(rand.Reader, &clientCRTTemplate, caCRT, c.PublicKey, caKey)
	if err != nil {
		fmt.Println("failed to create client certificate")
		return nil, ""
	}

	clientCRTDES, err := x509.ParseCertificate(clientCRTRaw)
	if err != nil {
		log.Println("failed to parse client certificate")
		log.Println(err.Error())
		return nil, ""
	}

	certText := printCertInfo(clientCRTDES, skiFP.String())
	return clientCRTRaw, certText
}

func printCertInfo(cert *x509.Certificate, ski string) string {
	var buf bytes.Buffer
	buf.WriteString("Certificate:\n")
	buf.WriteString(fmt.Sprintf("%4sData:\n", ""))
	version := cert.Version
	hexVersion := version - 1
	if hexVersion < 0 {
		hexVersion = 0
	}
	buf.WriteString(fmt.Sprintf("%8sVersion: %d (%#x)\n", "", version, hexVersion))
	buf.WriteString(fmt.Sprintf("%8sSerial Number: %d (%#x)\n", "", cert.SerialNumber, cert.SerialNumber))
	buf.WriteString(fmt.Sprintf("%8sSignature Algorithm: %s\n", "", cert.SignatureAlgorithm))
	buf.WriteString(fmt.Sprintf("%8sIssuer: %s\n", "", reverseAndPad(strings.Split(cert.Issuer.String(), ","))))
	buf.WriteString(fmt.Sprintf("%8sValidity\n", ""))
	buf.WriteString(fmt.Sprintf("%12sNot Before: %s\n", "", cert.NotBefore))
	buf.WriteString(fmt.Sprintf("%12sNot After : %s\n", "", cert.NotAfter))
	buf.WriteString(fmt.Sprintf("%8sSubject: %s\n", "", reverseAndPad(strings.Split(cert.Subject.String(), ","))))
	buf.WriteString(fmt.Sprintf("%8sSubject Public Key Info:\n", ""))
	buf.WriteString(fmt.Sprintf("%12sPublic Key Algorithm: %s\n", "", cert.PublicKeyAlgorithm.String()))
	buf.WriteString(fmt.Sprintf("%16sPublic-Key: (%d bit)\n", "", cert.PublicKey.(*rsa.PublicKey).Size()*8))
	buf.WriteString(fmt.Sprintf("%16sModulus:", ""))
	buf.WriteString(fmt.Sprintf("%16s%s\n", "", printKeyInfo(cert.PublicKey.(*rsa.PublicKey))))
	buf.WriteString(fmt.Sprintf("%16sExponent: %d (%#x)\n", "", cert.PublicKey.(*rsa.PublicKey).E, cert.PublicKey.(*rsa.PublicKey).E))
	buf.WriteString(fmt.Sprintf("%8sX509v3 extensions:\n", ""))
	buf.WriteString(fmt.Sprintf("%12sX509v3 Subject Alternative Name:\n", ""))
	buf.WriteString(fmt.Sprintf("%16sDNS:%s\n", "", cert.DNSNames[0]))
	buf.WriteString(fmt.Sprintf("%12sX509v3 Subject Key Identifier:\n", ""))
	buf.WriteString((fmt.Sprintf("%16s%s\n", "", ski)))

	return buf.String()
}

func printKeyInfo(publicKey *rsa.PublicKey) string {
	var buf bytes.Buffer
	for i, val := range publicKey.N.Bytes() {
		if (i % 15) == 0 {
			buf.WriteString(fmt.Sprintf("\n%20s", ""))
		}
		buf.WriteString(fmt.Sprintf("%02x", val))
		if i != len(publicKey.N.Bytes())-1 {
			buf.WriteString(":")
		}
	}
	return buf.String()
}

func reverseAndPad(s []string) string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		newI := strings.Join(strings.Split(s[i], "="), " = ")
		newJ := strings.Join(strings.Split(s[j], "="), " = ")
		s[i], s[j] = newJ, newI
	}
	return strings.Join(s, ", ")
}

func gen(issuer *x509.Certificate) ([]byte, error) {
	return asn1.Marshal(authKeyId{
		KeyIdentifier:             issuer.SubjectKeyId,
		AuthorityCertIssuer:       generalNames{Name: []pkix.RDNSequence{issuer.Issuer.ToRDNSequence()}},
		AuthorityCertSerialNumber: issuer.SerialNumber,
	})
}
