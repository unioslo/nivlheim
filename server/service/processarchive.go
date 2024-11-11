package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"log"
	"nivlheim/utility"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	u "unicode"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func processArchive(url string, db *sql.DB) (err error) {
	log.Printf("Processing archive %s", url)

	file := config.QueueDir + "/" + url

	if !fileExists(file) {
		log.Printf("File %s does not exist", file)
		// if file doesn't exist on file system, remove from queue
		return nil
	}

	tempDir, err := os.MkdirTemp("", "archive")
	if err != nil {
		log.Println(err)
		return err
	}

	// clean up files and directories when done
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Println(err)
		}
		err = os.Remove(file)
		if err != nil {
			log.Println(err)
		}
		err = os.Remove(file + ".meta")
		if err != nil {
			log.Println(err)
		}
	}()

	if strings.HasSuffix(url, ".tgz") {
		err := unTar(tempDir, file)
		if err != nil {
			log.Println(err)
		}
	} else if strings.HasSuffix(url, ".zip") {
		err := unZip(tempDir, file)
		if err != nil {
			log.Println(err)
		}
	}

	// remove ssh private keys if they exist
	files, err := filepath.Glob(tempDir + "/files/etc/ssh/ssh_host_*_key")
	if err != nil {
		log.Println(err)
	}
	for _, f := range files {
		err = os.Remove(f)
		if err != nil {
			log.Println(err)
		}
	}

	// remove log files
	err = os.RemoveAll(tempDir + "/files/var/log")
	if err != nil {
		log.Println(err)
	}

	// read metadata
	metaData, err := readKeyValueFile(file + ".meta")
	if err != nil {
		log.Println(err)
		return err
	}
	log.Printf("Read %d key-value pairs from %s", len(metaData), file+".meta")

	/* TEMPORARY FIX:
	   / There's a bug in the Windows client, in some cases it gives the hostname without the domain.
	   / See: https://github.com/unioslo/nivlheim/issues/138 */
	if !strings.Contains(metaData["os_hostname"], ".") {
		// The file might not exist. In that case, do nothing.
		file, err := os.Open(tempDir + "/commands/DomainName")
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			// first line is the command itself
			scanner.Scan()
			// second line is the output
			scanner.Scan()
			if err := scanner.Err(); err != nil {
				return err
			}
			fqdn := metaData["hostname"] + "." + scanner.Text()
			metaData["hostname"] = fqdn
		}
	}

	curFiles := make(map[string]int64)
	var hostInfoExists int64

	rows, err := db.Query("SELECT fileid, filename FROM files WHERE certfp = $1 and current = true", metaData["certfp"])
	if err != nil {
		log.Println(err)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var fileId int64
		var filename sql.NullString
		err = rows.Scan(&fileId, &filename)
		if err != nil {
			log.Println(err)
			return err
		}
		curFiles[filename.String] = fileId
	}

	err = db.QueryRow("SELECT COUNT(*) FROM hostinfo WHERE certfp = $1", metaData["certfp"]).Scan(&hostInfoExists)
	if err != nil {
		log.Println(err)
	}

	unchangedFiles := 0

	// process each file, do this in a transaction in case of errors during processing
	err = utility.RunInTransaction(db, func(tx *sql.Tx) error {
		err = filepath.WalkDir(tempDir, func(path string, entry fs.DirEntry, err error) error {
			return processFile(&unchangedFiles, metaData, curFiles, hostInfoExists, db, path, tempDir, entry, err)
		})
		if err != nil {
			log.Printf("Error in processFile: %s", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("Error in transaction: %s", err)
		return err
	}

	// Notify the system service/daemon that a number of files
	// have been processed, so we can produce an accurate count of
	// files-per-minute.
	if unchangedFiles > 0 {
		log.Printf("Unchanged files: %d", unchangedFiles)
		pfib.Add(float64(unchangedFiles)) // pfib = parsed files interval buffer
	}

	return nil
}

func processFile(unchangedFiles *int, metadata map[string]string, curfiles map[string]int64, hostinfo int64, db *sql.DB, path string, tempdir string, de fs.DirEntry, err error) error {
	if de.IsDir() {
		return nil
	}

	// only process files under the "files" and "commands" directories
	if !(strings.Contains(path, "/files/") || strings.Contains(path, "/commands/")) {
		return nil
	}

	fileName := de.Name()

	file, err := os.Open(path)
	if err != nil {
		log.Printf("Error opening file %s: %s", path, err)
		return err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		log.Printf("Error stating file %s: %s", path, err)
		return err
	}
	modTime := fi.ModTime().Format(time.RFC3339)
	rdr := bufio.NewReader(file)
	if strings.Contains(path, "/commands/") {
		// first line of file is the actual command
		cmd, err := rdr.ReadBytes('\n')
		fileName = string(cmd)
		if err != nil {
			log.Println("Could not read until first lineshift in file", path)
			return err
		}
	} else if strings.Contains(path, "/files/") {
		fileName = strings.TrimPrefix(path, tempdir+"/files")
	}
	fileName = strings.TrimRight(fileName, "\r\n")

	// read the rest of the file
	contents, err := io.ReadAll(rdr)
	if err != nil {
		log.Println("Could not read rest of file", path)
	}

	contents2 := removeControlChars(string(contents))

	c := crc32.ChecksumIEEE([]byte(contents2))

	crc := int32(c)

	var oldCrc sql.NullInt32
	var fileId sql.NullInt64

	err = db.QueryRow("SELECT crc32, fileid FROM files WHERE "+
		" certfp = $1 AND filename = $2 ORDER BY received DESC LIMIT 1",
		metadata["certfp"], fileName).Scan(&oldCrc, &fileId)
	if err != nil && err != sql.ErrNoRows {
		log.Println("error reading crc checksum from db ", err.Error())
	}

	if oldCrc.Valid && crc == oldCrc.Int32 {
		if hostinfo > 0 {
			_, _ = db.Exec("UPDATE hostinfo SET lastseen = $1, clientversion = $2 "+
				" WHERE certfp = $3 AND lastseen < $4", metadata["iso_received"], metadata["clientversion"],
				metadata["certfp"], metadata["iso_received"])
			_, _ = db.Exec("UPDATE hostinfo SET ipaddr = $1, os_hostname= $2, dnsttl = null "+
				" WHERE (ipaddr != $3 || os_hostname != $4) AND certfp = $5", metadata["ip"],
				metadata["os_hostname"], metadata["ip"], metadata["os_hostname"], metadata["certfp"])
			*unchangedFiles++
		} else {
			/* There is NO hostinfo record.
			/ It looks like the machine was archived and just now came back.
			/ Set parsed=false so the file will be parsed again,
			/ because the hostinfo values must be re-populated. */
			_, _ = db.Exec("UPDATE files SET parsed = false WHERE fileid=$1", fileId)
		}
		_, _ = db.Exec("UPDATE files SET current=true, received=NOW() WHERE fileid = $1 "+
			"AND NOT current", fileId)
		// sql set current flag for this file
		delete(curfiles, fileName)
		return nil
	} else {
		log.Printf("This file does not exist from before %s", fileName)
	}

	// Set current to false for the previous version of this file
	if _, ok := curfiles[fileName]; ok {
		_, _ = db.Exec("UPDATE files SET current=false WHERE fileid = $1 AND current",
			curfiles[fileName])
		delete(curfiles, fileName)
	}

	// Run the database INSERT operation
	_, err = db.Exec("INSERT INTO files(ipaddr, os_hostname, certcn, certfp, filename, "+
		"received, mtime, content, crc32, is_command, clientversion, originalcertid) VALUES "+
		"($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, "+
		"(SELECT certid FROM certificates WHERE fingerprint = $12))", metadata["ip"], metadata["os_hostname"],
		metadata["certcn"], metadata["certfp"], fileName, metadata["iso_received"], modTime,
		contents2, crc, strings.Contains(path, "/commands/"), metadata["clientversion"], metadata["certfp"])
	if err != nil {
		log.Println("Error inserting file: ", err)
		return err
	}

	log.Println("Completed inserting new files into the database")

	// clear the "current" flag for files that weren't in this package
	var notCurrent []int64
	for _, fileId := range curfiles {
		_, _ = db.Exec("UPDATE files SET current=false WHERE fileid = $1 AND current", fileId)
		notCurrent = append(notCurrent, fileId)
	}

	// Notify the system service/daemon that some file(s) have had
	// their "current" flag cleared, and can be removed from the
	// in-memory search cache.
	for _, id := range notCurrent {
		removeFileFromFastSearch(id)
	}

	log.Printf("Visited: %s\n", path)
	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func removeControlChars(str string) string {
	var result strings.Builder
	space := ' '
	for _, ch := range str {
		if ch == '\r' || ch == '\n' || ch == '\t' || !u.IsControl(ch) {
			result.WriteRune(ch)
		} else {
			result.WriteRune(space)
		}
	}
	return result.String()
}

func readKeyValueFile(filename string) (map[string]string, error) {
	keyValueMap := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " = ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line: %s", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		keyValueMap[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// set the received time in RFC3339 format
	received, err := strconv.ParseInt(keyValueMap["received"], 10, 64)
	if err != nil {
		log.Printf("Unable to convert received time to int64: %s", err)
		return nil, err
	}
	t := time.Unix(received, 0)
	keyValueMap["iso_received"] = t.Format(time.RFC3339)

	return keyValueMap, nil
}

func unTar(dst string, fn string) error {

	log.Printf("Filename is %s", fn)
	fi, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer fi.Close()

	gzr, err := gzip.NewReader(fi)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()

		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			log.Printf("err: %s\n", err)
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			tdir := filepath.Dir(target)
			if _, err := os.Stat(tdir); os.IsNotExist(err) {
				if err := os.MkdirAll(tdir, 0755); err != nil {
					return err
				}
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				log.Println("Error opening file: ", err)
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				log.Println("Error copying file: ", err)
				return err
			}
			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
			err = os.Chtimes(target, header.AccessTime, header.ModTime)
			if err != nil {
				log.Printf("Error setting mod time on %s: %s", target, err)
				return err
			}
		}
	}
}

func unZip(dst string, fn string) error {
	archive, err := zip.OpenReader(fn)
	if err != nil {
		return err
	}
	defer archive.Close()

	for _, f := range archive.File {
		filePath := filepath.Join(dst, strings.ReplaceAll(f.Name, `\`, `/`))
		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		fileInArchive, err := f.Open()

		if err != nil {
			return err
		}

		if f.UncompressedSize64 < 2 {
        // file is too small to have a BOM, just copy
			_, err = io.Copy(dstFile, fileInArchive)
			if err != nil {
				return err
			}
			dstFile.Close()
		} else {
        // this file might be UTF-16, check for BOM
			var isUTF16 bytes.Buffer
			_, err = io.CopyN(&isUTF16, fileInArchive, 2)
			if err != nil {
				return err
			}
			bom := isUTF16.Bytes()

			pr := bytes.NewReader(bom)

			if len(bom) == 2 && bom[0] == 0xFF && bom[1] == 0xFE { //UTF-16LE
				scanner := bufio.NewScanner(transform.NewReader(io.MultiReader(pr, fileInArchive), unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()))
				for scanner.Scan() {
					dstFile.WriteString(scanner.Text() + "\r\n")
				}
			} else {
				if _, err := io.Copy(dstFile, io.MultiReader(pr, fileInArchive)); err != nil {
					return err
				}
			}
			dstFile.Close()
		}
		err = os.Chtimes(filePath, f.Modified, f.Modified)
		if err != nil {
			log.Printf("Error setting mod time on %s: %s", filePath, err)
			return err
		}
		fileInArchive.Close()

	}
	return nil
}
