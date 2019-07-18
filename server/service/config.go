package main

import (
	"bufio"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Configuration options (set in /etc/nivlheim/server.conf)
type Config struct {
	Oauth2ClientID              string
	Oauth2ClientSecret          string
	Oauth2Scopes                []string
	Oauth2AuthorizationEndpoint string
	Oauth2TokenEndpoint         string
	Oauth2UserInfoEndpoint      string
	Oauth2LogoutEndpoint        string
	AuthRequired                bool
	AuthPluginURL               string
	ArchiveDayLimit             int
	DeleteDayLimit              int
	LDAPServer                  string
	LDAPUserTree                string
	LDAPMemberAttr              string
	LDAPPrimaryAttr             string
	LDAPAdminGroup              string
}

func ReadConfigFile(configFileName string) (*Config, error) {
	// Open the config file
	file, err := os.Open(configFileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the config file
	config := &Config{}
	structValue := reflect.ValueOf(config).Elem()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Parse the name=value pair
		keyAndValue := strings.SplitN(scanner.Text(), "=", 2)
		key := strings.ToLower(strings.TrimSpace(keyAndValue[0]))
		value := strings.TrimSpace(keyAndValue[1])

		// Use reflection to set values in the Config struct
		structFieldValue := structValue.FieldByNameFunc(func(s string) bool {
			return strings.ToLower(s) == key // compare names in a case-insensitive way
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
					structFieldValue.Set(reflect.ValueOf(strings.Split(value, ",")))
				}

			}
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return config, nil
}
