package main

import (
	"bufio"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Config options (set in /etc/nivlheim/server.conf)
type Config struct {
	Oauth2ClientID              string
	Oauth2ClientSecret          string
	Oauth2Scopes                []string
	Oauth2AuthorizationEndpoint string
	Oauth2TokenEndpoint         string
	Oauth2UserInfoEndpoint      string
	Oauth2LogoutEndpoint        string
	AuthRequired                bool
	ArchiveDayLimit             int
	DeleteDayLimit              int
	HideUnknownHosts            bool
	LDAPServer                  string
	LDAPUserTree                string
	LDAPMemberAttr              string
	LDAPPrimaryAttr             string
	LDAPAdminGroup              string
	AllAccessGroups             []string
	HostOwnerPluginURL          string
	PGhost, PGdatabase, PGuser  string
	PGpassword, PGsslmode       string
	PGport                      int
}

func updateConfig(config *Config, key string, value string) (*Config) {
	// Use reflection to set values in the Config struct and
	// cast values to the expected type.
	structValue := reflect.ValueOf(config).Elem()
	structFieldValue := structValue.FieldByNameFunc(func(s string) bool {
		return strings.ToLower(s) == strings.ToLower(key) // compare names in a case-insensitive way
	})
	if structFieldValue.IsValid() && structFieldValue.CanSet() {
		switch structFieldValue.Kind() {
		case reflect.String:
			structFieldValue.SetString(value)
		case reflect.Int:
			i, err := strconv.Atoi(value)
			if err == nil {
				structFieldValue.Set(reflect.ValueOf(i))
			}
		case reflect.Bool:
			structFieldValue.Set(reflect.ValueOf(isTrueish(value)))
		case reflect.Slice:
			if structFieldValue.Type().Elem().Kind() == reflect.String {
				// Lists of values are expected to be comma-separated.
				structFieldValue.Set(reflect.ValueOf(strings.Split(value, ",")))
			}
		}
	}
	return config
}

// ReadConfigFile reads a config file and returns a Config struct
// where the values are filled in.
// Options in the file must have the same name as fields in the struct,
// disregarding upper/lowercase.
// Options with names that aren't recognized are ignored.
func ReadConfigFile(configFileName string) (*Config, error) {
	// Open the config file
	file, err := os.Open(configFileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the config file
	config := &Config{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Parse the name=value pair
		keyAndValue := strings.SplitN(scanner.Text(), "=", 2)
		key := strings.ToLower(strings.TrimSpace(keyAndValue[0]))
		value := strings.TrimSpace(keyAndValue[1])

		config = updateConfig(config, key, value)
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return config, nil
}
