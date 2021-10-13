package main

import (
	"fmt"
	"regexp"

	//ldap "gopkg.in/ldap.v3"
	ldap "github.com/go-ldap/ldap/v3"
)

type LDAPUser struct {
	Username           string
	Groups             []string
	PrimaryAffiliation string
}

func LDAPLookupUser(username string) (*LDAPUser, error) {
	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%s",
		config.LDAPServer, ldap.DefaultLdapPort))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Search for the given username
	searchRequest := ldap.NewSearchRequest(
		config.LDAPUserTree,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(uid=%s)", username),
		[]string{},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) < 1 {
		return nil, nil
	}

	user := &LDAPUser{Username: username}

	// Get a list of groups.
	// Parse values on the form:  cn=groupname,cn=foo,dc=bar,dc=baz
	groupNameRE := regexp.MustCompile("^cn=([\\w\\-]+),")
	groups := make([]string, 0, 10)
	for _, value := range sr.Entries[0].GetAttributeValues(config.LDAPMemberAttr) {
		m := groupNameRE.FindStringSubmatch(value)
		if m != nil {
			groups = append(groups, m[1])
		}
	}
	user.Groups = groups

	// Get the primary affiliation
	a := sr.Entries[0].GetAttributeValues(config.LDAPPrimaryAttr)
	if len(a) > 0 {
		user.PrimaryAffiliation = a[0]
	}

	return user, nil
}
