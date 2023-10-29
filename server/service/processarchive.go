package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"bufio"
	"bytes"
	"database/sql"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"io/fs"
	"fmt"
	u "unicode"
	"hash/crc32"
	"time"
	"strconv"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func processArchive(url string, db *sql.DB) (err error) {
	log.Printf("Processing archive %s", url)

	file := config.QueueDir + "/" + url
	log.Println(file)

	if !fileExists(file) {
		log.Printf("File %s does not exist", file)
		// if file doesn't exist, remove from queue
		return nil
	}

	tempDir, err := os.MkdirTemp("", "archive")
	if err != nil {
		log.Println(err)
		return err
	}

	defer func() {
		log.Println("Removing tempdir")
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Println(err)
		}
		log.Println("Removing archive")
		err = os.Remove(file)
		if err != nil {
			log.Println(err)
		}
		log.Println("Removing meta file")
		err = os.Remove(file + ".meta")
		if err != nil {
			log.Println(err)
		}
	}()

	if strings.HasSuffix(url, ".tgz") {
        log.Println("tar-file, untaring")
        err := unTar(tempDir, file)
		if err != nil {
	        log.Println(err)
		}
    } else if strings.HasSuffix(url, ".zip") {
        log.Println("zip-file, unziping")
        err := unZip(tempDir, file)
		if err != nil {
	        log.Println(err)
		}
    }

	// remove ssh private keys is they exist
	files, err := filepath.Glob(tempDir + "/files/etc/ssh/ssh_host_*_key")
	if err != nil {
		log.Println(err)
	}
	for _, f := range files {
		log.Println("Removing ", f)
		err = os.Remove(f)
		if err != nil {
			log.Println(err)
		}
	}

	err = os.RemoveAll(tempDir + "/files/var/log")
	if err != nil {
		log.Println(err)
	}

	metaData, err := readKeyValueFile(file + ".meta")
	if err != nil {
		log.Println(err)
		return err
	}
	log.Printf("Read %d key-value pairs from %s", len(metaData), file + ".meta")
	for key, value := range metaData {
		log.Printf("%s = %s", key, value)
	}

    /* TEMPORARY FIX:
    / There's a bug in the Windows client, in some cases it gives the hostname without the domain.
    / See: https://github.com/unioslo/nivlheim/issues/138 */
	if !strings.Contains(metaData["hostname"], ".") {
		file, err := os.Open(tempDir + "/commands/DomainName")
		if err != nil {
			log.Printf("Could not open file %s: %s", tempDir + "/commands/DomainName", err)
			return nil
		}
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

	var curFiles map[string]int64

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

	err = filepath.WalkDir(tempDir, func(path string, entry fs.DirEntry, err error) error {
		return processFile(metaData, curFiles, path, entry, err)
	})
	if err != nil {
		log.Printf("Error in processFile: %s", err)
		return err
	}
	fmt.Printf("filepath.WalkDir returned %v\n", err)

	return nil
}

func processFile(metadata map[string]string, curfiles map[string]int64, path string, de fs.DirEntry, err error) error {
	log.Println("processFile: ", path)
	log.Println("processFile: ", de.Name())
	log.Println(curfiles)
	if de.IsDir() {
		log.Printf("Skipping directory %s", path)
        return nil
    }

	if !(strings.Contains(path, "/files/") || strings.Contains(path, "/commands/")) {
		log.Printf("Skipping file %s", path)
		return nil
	}

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
	log.Printf("mod time: %s", fi.ModTime())

	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	log.Printf("%T\n", contents)
	log.Printf("contents: %s", contents)
	// latin1 to utf-8
	buf := make([]rune, len(contents))
	for i, b := range contents {
		buf[i] = rune(b)
	}
	contents2 := removeControlChars(string(buf))
	log.Printf("contents2: %s", contents2)

	crc := crc32.ChecksumIEEE([]byte(contents2))
	log.Printf("crc: %d %x", crc, crc)

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

	log.Println("før gzip.NewReader")
	gzr, err := gzip.NewReader(fi)
	if err != nil {
		return err
	}
	defer gzr.Close()
	log.Println("etter gzip.NewReader, før tar.NewReader")
	tr := tar.NewReader(gzr)
	for {
		log.Println("før header")
		header, err := tr.Next()

		log.Println("etter header")
		switch {
		// if no more files are found return
		case err == io.EOF:
			log.Println("EOF")
			return nil

		// return any other error
		case err != nil:
			log.Printf("err: %s\n", err)
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			log.Println("header is nil")
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		log.Println("target: ", target)
		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			log.Printf("Dir %s\n", target)
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			log.Printf("File %s\n", target)
			tdir := filepath.Dir(target)
			if _, err := os.Stat(tdir); os.IsNotExist(err) {
				if err := os.MkdirAll(tdir, 0755); err != nil {
					return err
				}
			}
			//fmt.Printf("tdir: %s\n", tdir)
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
		log.Println("etter for loop")
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
        log.Println("unzipping file ", filePath)
        if f.FileInfo().IsDir() {
            log.Println("Creating directory")
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

        var isUTF16 bytes.Buffer
        _, err = io.CopyN(&isUTF16, fileInArchive, 2)
		if err != nil {
			return err
		}
        bom := isUTF16.Bytes()

        pr := bytes.NewReader(bom)

        if len(bom) == 2 && bom[0] == 0xFF && bom[1] == 0xFE {//UTF-16LE
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
		err = os.Chtimes(filePath, f.Modified, f.Modified)
		if err != nil {
			log.Printf("Error setting mod time on %s: %s", filePath, err)
			return err
		}
        fileInArchive.Close()

    }
    return nil
}

