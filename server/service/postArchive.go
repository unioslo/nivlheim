package main

import (
	"archive/zip"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type apiMethodPostArchive struct {
	db *sql.DB
}

const MAX_UPLOAD_SIZE = 1024 * 1024 * 10

func (vars *apiMethodPostArchive) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ipAddr := req.Header.Get("X-Forwarded-For")
	log.Printf("post from %s\n", ipAddr)

	contentType := req.Header.Get("Content-Type")

	req.Body = http.MaxBytesReader(w, req.Body, MAX_UPLOAD_SIZE)

	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if err := req.ParseForm(); err != nil {
			log.Printf("Could not parse form or file too big: %s", err.Error())
			http.Error(w, "Error parsing form or file is too big. Please choose a file that's less than 10MB in size", http.StatusBadRequest)
			return
		}
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := req.ParseMultipartForm(MAX_UPLOAD_SIZE); err != nil {
			log.Printf("Could not parse multipart form or file too big: %s", err.Error())
			http.Error(w, "Error parsing form or file is too big. Please choose a file that's less than 10MB in size", http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Unsupported content type", http.StatusUnsupportedMediaType)
		return
	}

	pemContent := req.Header.Get("Cert-Client-Cert")

	match := regexp.MustCompile("(?s)(-{5}BEGIN CERTIFICATE-{5})(.*)(-{5}END CERTIFICATE-{5})")
	pemContent2 := match.ReplaceAll([]byte(pemContent), []byte("$1\n$2\n$3"))

	cert := getCert(pemContent2)
	if cert == nil {
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}

	fingerprint := getCertFPString(cert)

	// Check revoked status
	var revoked bool
	var nonce sql.NullInt32
	err := vars.db.QueryRow("SELECT revoked, nonce FROM certificates WHERE fingerprint=$1", fingerprint).Scan(&revoked, &nonce)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Could not find certificate in database when checking revocation status. Fingerprint: %s", fingerprint)
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		} else {
			log.Println(err.Error())
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		}
	}
	if revoked {
		http.Error(w, "Your certificate has been revoked.", http.StatusForbidden)
		return
	}

	reqNonce := req.FormValue("nonce")
	if reqNonce == "" {
		log.Println("Nonce missing")
		http.Error(w, "Missing parameters.", http.StatusUnprocessableEntity)
		return
	}
	reqNonceInt, err := strconv.Atoi(reqNonce)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}

	if nonce.Valid && int(nonce.Int32) != reqNonceInt {
		log.Printf("Nonce mismatch. Expected %d, got %d", nonce.Int32, reqNonceInt)
		_, err = vars.db.Exec("UPDATE certificates SET revoked=true WHERE fingerprint=$1", fingerprint)
		if err != nil {
			log.Printf("Could not revoke certificate: %s", err.Error())
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		}
		http.Error(w, "Nonce mismatch", http.StatusForbidden)
		return
	}

	osHostName := req.FormValue("hostname")
	if osHostName == "" {
		http.Error(w, "Missing parameters.", http.StatusUnprocessableEntity)
	}
	log.Printf("client says its hostname is %s", osHostName)

	osHostName = strings.ToLower(osHostName)
	shortHost := osHostName
	match = regexp.MustCompile(`^(\S+?)\..*$`)
	shortHost2 := match.ReplaceAll([]byte(shortHost), []byte("$1"))

	clientVersion := req.FormValue("version")

	loadAvg, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		log.Printf("throwing away a post from %s (%s) (v%s) (%s)", ipAddr, shortHost2, clientVersion, fingerprint)
		http.Error(w, "", http.StatusServiceUnavailable)
	}
	oneMinAvg := strings.Split(string(loadAvg), " ")[0]
	var load float64
	load, _ = strconv.ParseFloat(oneMinAvg, 32)
	if load > 200 {
		log.Printf("throwing away a post from %s (%s) (v%s) (%s)", ipAddr, shortHost2, clientVersion, fingerprint)
		http.Error(w, "", http.StatusServiceUnavailable)
	}

	log.Printf("post from %s (%s) (v%s) (%s)", ipAddr, shortHost2, clientVersion, fingerprint)

	var archiveFile = fmt.Sprintf("%s/%s.tgz", config.UploadDir, fingerprint)
	var signatureFile string
	var metaFile string

	dst, err := os.Create(archiveFile)
	if err != nil {
		log.Printf("Could not create archive file: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer dst.Close()

	if file := req.FormValue("archive_base64"); file != "" {
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(file))
		_, err = io.Copy(dst, decoder)
		if err != nil {
			log.Printf("Could not write archive file: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if _, ok := req.MultipartForm.File["archive"]; ok {
		rFile := "archive"
		file, _, err := req.FormFile(rFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		_, err = io.Copy(dst, file)
		if err != nil {
			log.Printf("Could not write archive file: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("missing file upload parameter (%s)", fingerprint)
		http.Error(w, "File missing", http.StatusBadRequest)
		return
	}

	defer func() {
		if fileExists(archiveFile) {
			err := os.Remove(archiveFile)
			if err != nil {
				log.Printf("Could not remove archive file: %s", err.Error())
			}
		}
		if fileExists(signatureFile) {
			err = os.Remove(signatureFile)
			if err != nil {
				log.Printf("Could not remove signature file: %s", err.Error())
			}
		}
		if fileExists(metaFile) {
			err = os.Remove(metaFile)
			if err != nil {
				log.Printf("Could not remove meta file: %s", err.Error())
			}
		}
	}()

	dstInfo, err := dst.Stat()
	if err != nil {
		log.Printf("Could not stat archive file: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[%s] received archive file (%d bytes)", shortHost2, dstInfo.Size())

	file, err := os.Open(archiveFile)
	if err != nil {
		log.Printf("Could not open archive file: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buff := make([]byte, 512)
	_, err = file.Read(buff)

	if err != nil {
		log.Printf("Could not read first 512 bytes of archive file: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filetype := http.DetectContentType(buff)

	var size uint64 = 0

	if filetype == "application/zip" {
		log.Println("The archive is in Zip format")
		match = regexp.MustCompile(`\.tgz$`)
		newFile := match.ReplaceAll([]byte(archiveFile), []byte(".zip"))
		os.Rename(archiveFile, string(newFile))
		archiveFile = string(newFile)

		archive, err := zip.OpenReader(archiveFile)
		if err != nil {
			log.Printf("Could not open archive file (%s): %s", archiveFile, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer archive.Close()

		for _, f := range archive.File {
			size += f.UncompressedSize64
		}
	} else if (filetype == "application/x-gzip") || (filetype == "application/gzip") {
		_, err = file.Seek(-4, 2)
		if err != nil {
			log.Printf("Could not seek to end of archive file (%s): %s", archiveFile, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		buff = make([]byte, 4)

		_, err = file.Read(buff)
		if err != nil {
			log.Printf("Could not read last 4 bytes of archive file (%s): %s", archiveFile, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sSize := binary.LittleEndian.Uint32(buff)
		size = uint64(sSize)
	}

	log.Printf("[%s] Uncompressed size is %d bytes", shortHost2, int(size))

	if int(size) > MAX_UPLOAD_SIZE*10 {
		log.Printf("[%s] archive file is too large. Uncompressed size is %d bytes	", shortHost2, int(size))
		http.Error(w, "The uploaded file is too big. Please choose a file that's less than 10MB in size", http.StatusRequestEntityTooLarge)
		return
	}

	signatureFile = archiveFile + ".sign"

	dst, err = os.Create(signatureFile)
	if err != nil {
		log.Printf("Could not create signature file (%s): %s", fingerprint, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer dst.Close()

	if file := req.FormValue("signature_base64"); file != "" {
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(file))
		_, err = io.Copy(dst, decoder)
		if err != nil {
			log.Printf("Could not write signature file (%s): %s", fingerprint, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if _, ok := req.MultipartForm.File["signature"]; ok {
		rFile := "signature"
		file, _, err := req.FormFile(rFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		_, err = io.Copy(dst, file)
		if err != nil {
			log.Printf("Could not write archive file: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("missing file upload parameter signature (%s)", fingerprint)
		http.Error(w, "File missing", http.StatusBadRequest)
		return
	}

	dstInfo, err = dst.Stat()
	if err != nil {
		log.Printf("Could not stat signature file: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("%s recieved signature file (%d bytes)", signatureFile, dstInfo.Size())

	userAgent := req.Header.Get("User-Agent")

	// archive is signed with sha-1 if the client is windows, sha-256 otherwise
	algo := x509.SHA256WithRSA
	if strings.Contains(strings.ToLower(userAgent), "powershell") {
		algo = x509.SHA1WithRSA
	}

	archive, _ := os.ReadFile(archiveFile)
	if err != nil {
		log.Printf("Could not read archive file (%s): %s", archiveFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sign, _ := os.ReadFile(signatureFile)
	if err != nil {
		log.Printf("Could not read signature file (%s): %s", signatureFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = cert.CheckSignature(algo, archive, sign)
	if err != nil {
		log.Printf("Could not verify signature of archive (%s): %s", archiveFile, err.Error())
		http.Error(w, "", http.StatusForbidden)
		return
	}

	clientSDNCN := req.Header.Get("Cert-Client-S-DN-CN")

	metaFile = archiveFile + ".meta"
	file2, err := os.Create(metaFile)

	if err != nil {
		log.Printf("Could not create meta file (%s): %s", metaFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file2.Close()

	received := strconv.FormatInt(time.Now().Unix(), 10)

	_, err = file2.WriteString("os_hostname = " + string(osHostName) + "\ncertcn = " + clientSDNCN + "\ncertfp = " +
		fingerprint + "\nip = " + ipAddr + "\nclientversion = " + clientVersion + "\nreceived = " +
		received + "\n")

	if err != nil {
		log.Printf("Could not write to meta file (%s): %s", metaFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	file2.Sync()

	newNonce := rand.Intn(1000000)
	_, err = vars.db.Exec("UPDATE certificates SET nonce=$1 WHERE fingerprint=$2", newNonce, fingerprint)
	if err != nil {
		log.Printf("Could not update nonce for certificate (%s): %s", fingerprint, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.Rename(archiveFile, config.QueueDir+"/"+filepath.Base(archiveFile))
	if err != nil {
		log.Printf("Could not move archive file (%s): %s", archiveFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.Rename(metaFile, config.QueueDir+"/"+filepath.Base(metaFile))
	if err != nil {
		log.Printf("Could not move signature file (%s): %s", signatureFile, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "OK. nonce=%d", newNonce)

}
