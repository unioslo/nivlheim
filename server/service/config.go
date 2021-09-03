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

func updateConfig(config *Config, key string, value string) {
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
}

// UpdateConfigFromFile reads a config file and updates a Config struct
// with values from the configuration file.
// Options in the file must have the same name as fields in the struct,
// disregarding upper/lowercase.
// Options with names that aren't recognized are ignored.
func UpdateConfigFromFile(config *Config, configFileName string) (error) {
	// Open the config file
	file, err := os.Open(configFileName)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the config file
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Parse the name=value pair
		keyAndValue := strings.SplitN(scanner.Text(), "=", 2)
		key := strings.ToLower(strings.TrimSpace(keyAndValue[0]))
		value := strings.TrimSpace(keyAndValue[1])

		updateConfig(config, key, value)
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return nil
}

// UpdateConfigFromEnvironment takes a Config struct, loops through its
// struct keys, searches the environment for "NIVLHEIM_$UPPERCASE_KEY",
// and returns a new struct with entries updated from the environment.
func UpdateConfigFromEnvironment(config *Config) {
	configValue := reflect.ValueOf(config).Elem()
	configType := configValue.Type()
	for i := 0; i < configValue.NumField(); i++ {
		name := configType.Field(i).Name
		val, ok := os.LookupEnv("NIVLHEIM_" + strings.ToUpper(name))
		if ok {
			updateConfig(config, name, val)
		}
	}
}
