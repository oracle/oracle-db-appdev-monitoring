// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora && !oracledb

package db

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

func descriptorWithWallet(descriptor, directory string) (string, error) {
	if strings.TrimSpace(directory) == "" || strings.ContainsAny(directory, "\x00\r\n\t") {
		return "", errors.New("a wallet directory with a valid Oracle Net path is required")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(directory)
	wallet := `(MY_WALLET_DIRECTORY="` + escaped + `")`
	const matchDN = "(SSL_SERVER_DN_MATCH=YES)"

	root, err := descriptorFields(descriptor)
	if err != nil || len(root) != 1 || root[0].name != "DESCRIPTION" {
		return "", errors.New("expected a long Oracle Net DESCRIPTION")
	}
	description := root[0]
	fields, err := descriptorFields(descriptor[description.valueStart:description.end])
	if err != nil {
		return "", err
	}
	var security *descriptorField
	for i := range fields {
		if fields[i].name != "SECURITY" {
			continue
		}
		if security != nil {
			return "", errors.New("Oracle Net description has multiple SECURITY sections")
		}
		security = &fields[i]
	}
	if security == nil {
		return descriptor[:description.end] + "(SECURITY=" + wallet + matchDN + ")" + descriptor[description.end:], nil
	}
	start := description.valueStart + security.valueStart
	end := description.valueStart + security.end
	securityFields, err := descriptorFields(descriptor[start:end])
	if err != nil {
		return "", err
	}
	matchConfigured := false
	for _, field := range securityFields {
		switch field.name {
		case "MY_WALLET_DIRECTORY", "WALLET_LOCATION":
			return "", errors.New("Oracle Net profile already specifies a wallet location")
		case "SSL_SERVER_DN_MATCH", "TLS_SERVER_DN_MATCH":
			matchConfigured = true
		}
	}
	if !matchConfigured {
		wallet += matchDN
	}
	return descriptor[:end] + wallet + descriptor[end:], nil
}

type descriptorField struct {
	name            string
	valueStart, end int
}

// descriptorFields finds the immediate fields of a descriptor section without
// reserializing it. Quoted values such as certificate DNs can contain parentheses.
func descriptorFields(section string) ([]descriptorField, error) {
	var fields []descriptorField
	var depth, start int
	var quote rune
	var escaped bool
	for i, ch := range section {
		if depth == 0 {
			if unicode.IsSpace(ch) {
				continue
			}
			if ch != '(' {
				return nil, errors.New("invalid Oracle Net descriptor section")
			}
			start, depth = i, 1
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth != 0 {
				continue
			}
			name, _, ok := strings.Cut(section[start+1:i], "=")
			key := strings.Trim(strings.TrimSpace(name), `"'`)
			if !ok || key == "" || strings.ContainsAny(key, "()") {
				return nil, errors.New("invalid Oracle Net descriptor field")
			}
			fields = append(fields, descriptorField{strings.ToUpper(key), start + len(name) + 2, i})
		}
	}
	if depth != 0 {
		return nil, errors.New("unclosed Oracle Net descriptor field")
	}
	return fields, nil
}
