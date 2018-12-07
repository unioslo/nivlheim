package utility

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
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

	return reflect.DeepEqual(o1, o2), nil
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

func RunInTransaction(db *sql.DB, statements []string, args ...interface{}) error {
	var tx *sql.Tx
	var hasCommitted bool
	var err error
	defer func() {
		if r := recover(); r != nil {
			if tx != nil {
				tx.Rollback()
			}
			panic(r)
		} else if err != nil {
			if tx != nil {
				tx.Rollback()
			}
		} else if !hasCommitted {
			if tx != nil {
				tx.Rollback()
			}
		}
	}()
	tx, err = db.Begin()
	if err != nil {
		return err
	}
	for _, st := range statements {
		_, err = tx.Exec(st, args...)
		if err != nil {
			return err
		}
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	hasCommitted = true
	return err
}
