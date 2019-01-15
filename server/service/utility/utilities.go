package utility

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// IsEqualJSON returns true if the 2 supplied strings contain JSON data
// that is semantically equal.
func IsEqualJSON(s1, s2 string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false, fmt.Errorf("Error unmarshalling string 1 :: %s", err.Error())
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false, fmt.Errorf("Error unmarshalling string 2 :: %s", err.Error())
	}
	o1 = deepConvertRFC3339(o1)
	o2 = deepConvertRFC3339(o2)

	return reflect.DeepEqual(o1, o2), nil
}

func deepConvertRFC3339(o interface{}) interface{} {
	// detect strings that are timestamps
	s, ok := o.(string)
	if ok {
		t, err := time.Parse(time.RFC3339, s)
		if err == nil {
			// convert all timestamps to UTC so different timezones won't mess up the comparison
			return t.UTC()
		}
		return s
	}
	// traverse maps
	m, ok := o.(map[string]interface{})
	if ok {
		for key, value := range m {
			m[key] = deepConvertRFC3339(value)
		}
		return m
	}
	// traverse slices
	a, ok := o.([]interface{})
	if ok {
		for i, value := range a {
			a[i] = deepConvertRFC3339(value)
		}
		return a
	}
	return o
}

// GetString lets you specify a path to the value that you want
// (e.g. "aaa.bbb.ccc") and have it extracted from the data structure.
func GetString(v interface{}, path string) string {
	for _, key := range strings.Split(path, ".") {
		iKey, err := strconv.ParseInt(key, 10, 32)
		if err == nil {
			// If the key is a number, we assume the structure is an array
			arr, ok := v.([]interface{})
			if !ok {
				return ""
			}
			v = arr[iKey]
		} else {
			// If the key isn't a number, we assume the structure is a map
			m, ok := v.(map[string]interface{})
			if !ok {
				return ""
			}
			v = m[key]
		}
	}
	return fmt.Sprintf("%v", v)
}

// RunInTransaction runs the given function inside a database transaction.
// Does rollback if the function returns an error or panics.
// Otherwise, does commit.
func RunInTransaction(db *sql.DB, txFunc func(*sql.Tx) error) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // re-throw panic after Rollback
		} else if err != nil {
			tx.Rollback() // err is non-nil; don't change it
		} else {
			err = tx.Commit() // err is nil; if Commit returns error update err
		}
	}()
	err = txFunc(tx)
	return err
}

// RunStatementsInTransaction runs all the given statements inside a database transaction.
// If a statement fails, the transaction is rolled back.
// That way either all of them take effect or none of them do.
func RunStatementsInTransaction(db *sql.DB, statements []string, args ...interface{}) error {
	return RunInTransaction(db, func(tx *sql.Tx) error {
		for _, st := range statements {
			if _, err := tx.Exec(st, args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// RandomStringID returns a string of 32 characters,
// Each character is from the set [A-Za-z0-9].
func RandomStringID() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < 32; i++ {
		j := rand.Intn(len(charset))
		b.WriteString(charset[j : j+1])
	}
	return b.String()
}
