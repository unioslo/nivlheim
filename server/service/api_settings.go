package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
)

type apiMethodSettings struct {
	db *sql.DB
}

type SystemSetting int

const (
	DaysIfNotSeenThenArchive SystemSetting = iota + 1
	DaysIfNotSeenThenDelete
)

func (s SystemSetting) String() string {
	return [...]string{"",
		"DaysIfNotSeenThenArchive",
		"DaysIfNotSeenThenDelete",
	}[s]
}

func (s SystemSetting) DefaultValue() string {
	return [...]string{"",
		"30",
		"180",
	}[s]
}

var AllSettings = []SystemSetting{
	DaysIfNotSeenThenArchive,
	DaysIfNotSeenThenDelete}

func getSystemSetting(db *sql.DB, key SystemSetting) string {
	var value sql.NullString
	err := db.QueryRow("SELECT value FROM settings WHERE key = $1",
		key.String()).Scan(&value)
	if err != nil && err != sql.ErrNoRows {
		log.Panic(err)
	}
	// Default values
	if err == sql.ErrNoRows {
		return key.DefaultValue()
	}
	return value.String
}

func getSystemSettingAsInt(db *sql.DB, key SystemSetting) int {
	s := getSystemSetting(db, key)
	value, err := strconv.Atoi(s)
	if err != nil {
		value, _ = strconv.Atoi(key.DefaultValue())
	}
	return value
}

func setSystemSetting(db *sql.DB, key SystemSetting, value string) error {
	// Verify the value
	switch key {
	case DaysIfNotSeenThenArchive:
		i, err := strconv.Atoi(value)
		if i < 1 {
			return errors.New(key.String() + " must be at least 1")
		}
		if err != nil {
			return errors.New("Invalid value for " + key.String())
		}
	case DaysIfNotSeenThenDelete:
		i, err := strconv.Atoi(value)
		if i < 1 {
			return errors.New(key.String() + " must be at least 1")
		}
		if err != nil {
			return errors.New("Invalid value for " + key.String())
		}
	default:
		log.Panicf("No system setting with that id constant: %d", key)
	}
	// Store the value
	res, err := db.Exec("UPDATE settings SET value=$2 WHERE key=$1", key.String(), value)
	if err != nil {
		return err
	}
	i, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if i == 0 {
		_, err = db.Exec("INSERT INTO settings(key,value) VALUES($1,$2)",
			key.String(), value)
	}
	return err
}

func (vars *apiMethodSettings) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		settings := make(map[string]string, 0)
		for _, s := range AllSettings {
			settings[s.String()] = getSystemSetting(vars.db, s)
		}
		returnJSON(w, req, settings)

	case "PUT":
		match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing key in URL path", http.StatusUnprocessableEntity)
			return
		}
		name := match[1]
		value := req.FormValue("value")
		var key SystemSetting
		for _, s := range AllSettings {
			if s.String() == name {
				key = s
				break
			}
		}
		if key == 0 {
			http.Error(w, "No such setting", http.StatusNotFound)
			return
		}
		err := setSystemSetting(vars.db, key, value)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "", http.StatusNoContent) // 204 No Content

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
