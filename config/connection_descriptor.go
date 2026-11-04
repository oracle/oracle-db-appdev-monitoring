// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type netDescriptorNode struct {
	name     string
	scalar   string
	children []netDescriptorNode
}

func (n netDescriptorNode) String() string {
	var value strings.Builder
	value.WriteByte('(')
	value.WriteString(n.name)
	value.WriteByte('=')
	if len(n.children) == 0 {
		value.WriteString(n.scalar)
	} else {
		for _, child := range n.children {
			value.WriteString(child.String())
		}
	}
	value.WriteByte(')')
	return value.String()
}

type netDescriptorParser struct {
	input    string
	position int
}

func (p *netDescriptorParser) space() {
	for p.position < len(p.input) && unicode.IsSpace(rune(p.input[p.position])) {
		p.position++
	}
}
func (p *netDescriptorParser) node(depth int) (netDescriptorNode, error) {
	n := netDescriptorNode{}
	if depth > 32 {
		return n, fmt.Errorf("connection descriptor exceeds maximum nesting")
	}
	p.space()
	if p.position >= len(p.input) || p.input[p.position] != '(' {
		return n, fmt.Errorf("expected a parenthesized connection descriptor")
	}
	p.position++
	start := p.position
	for p.position < len(p.input) && p.input[p.position] != '=' {
		c := p.input[p.position]
		if c == '(' || c == ')' {
			return n, fmt.Errorf("invalid connection descriptor field")
		}
		p.position++
	}
	if p.position >= len(p.input) {
		return n, fmt.Errorf("unterminated connection descriptor field")
	}
	n.name = strings.ToUpper(strings.TrimSpace(p.input[start:p.position]))
	if n.name == "" {
		return n, fmt.Errorf("empty connection descriptor field")
	}
	for _, c := range n.name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return n, fmt.Errorf("invalid connection descriptor field name")
		}
	}
	p.position++
	p.space()
	if p.position < len(p.input) && p.input[p.position] == '(' {
		for {
			child, err := p.node(depth + 1)
			if err != nil {
				return n, err
			}
			n.children = append(n.children, child)
			p.space()
			if p.position >= len(p.input) {
				return n, fmt.Errorf("unterminated connection descriptor")
			}
			if p.input[p.position] != '(' {
				break
			}
		}
	} else {
		start = p.position
		quoted := false
		for p.position < len(p.input) {
			c := p.input[p.position]
			if c == '"' {
				quoted = !quoted
			} else if c == '\\' && quoted && p.position+1 < len(p.input) {
				p.position++
			} else if c == ')' && !quoted {
				break
			} else if c == '(' && !quoted {
				return n, fmt.Errorf("invalid parenthesis in connection descriptor value")
			}
			p.position++
		}
		if quoted {
			return n, fmt.Errorf("unterminated quoted connection descriptor value")
		}
		n.scalar = strings.TrimSpace(p.input[start:p.position])
	}
	if p.position >= len(p.input) || p.input[p.position] != ')' {
		return n, fmt.Errorf("unterminated connection descriptor")
	}
	p.position++
	return n, nil
}

// verifiedTCPSDescriptor validates the structure rather than matching strings
// inside quoted values. All actual network addresses must use TCPS, and every
// description receives certificate identity verification unless already enabled.
func verifiedTCPSDescriptor(value string) (string, error) {
	if len(value) > 64*1024 {
		return "", fmt.Errorf("connection descriptor exceeds maximum size")
	}
	parser := netDescriptorParser{input: strings.TrimSpace(value)}
	root, err := parser.node(0)
	if err != nil {
		return "", err
	}
	parser.space()
	if parser.position != len(parser.input) || (root.name != "DESCRIPTION" && root.name != "DESCRIPTION_LIST") {
		return "", fmt.Errorf("a full TCPS connection descriptor is required")
	}
	if root.name == "DESCRIPTION_LIST" {
		for _, child := range root.children {
			if child.name != "DESCRIPTION" {
				return "", fmt.Errorf("DESCRIPTION_LIST must contain descriptions")
			}
		}
	}
	addresses := 0
	var secure func(*netDescriptorNode) error
	secure = func(n *netDescriptorNode) error {
		if (n.name == "DESCRIPTION" || n.name == "ADDRESS" || n.name == "SECURITY") && len(n.children) == 0 {
			return fmt.Errorf("connection descriptor %s must contain fields", n.name)
		}
		if n.name == "ADDRESS" {
			addresses++
			protocols, hosts, ports := 0, 0, 0
			for _, child := range n.children {
				switch child.name {
				case "PROTOCOL":
					protocols++
					if !strings.EqualFold(strings.Trim(child.scalar, "\""), "TCPS") {
						return fmt.Errorf("connection descriptor contains a non-TCPS address")
					}
				case "HOST":
					if strings.Trim(child.scalar, "\"") != "" {
						hosts++
					}
				case "PORT":
					port, err := strconv.Atoi(strings.Trim(child.scalar, "\""))
					if err == nil && port > 0 && port <= 65535 {
						ports++
					}
				}
			}
			if protocols != 1 || hosts != 1 || ports != 1 {
				return fmt.Errorf("TCPS address requires one protocol, host, and valid port")
			}
		}
		if n.name == "SSL_SERVER_DN_MATCH" {
			v := strings.Trim(n.scalar, "\"")
			if !strings.EqualFold(v, "yes") && !strings.EqualFold(v, "true") && v != "1" {
				return fmt.Errorf("server certificate verification must be enabled")
			}
		}
		for i := range n.children {
			if err := secure(&n.children[i]); err != nil {
				return err
			}
		}
		if n.name == "DESCRIPTION" {
			security := -1
			for i, child := range n.children {
				if child.name == "SECURITY" {
					if security != -1 {
						return fmt.Errorf("duplicate connection descriptor SECURITY sections")
					}
					security = i
				}
			}
			if security == -1 {
				n.children = append(n.children, netDescriptorNode{name: "SECURITY"})
				security = len(n.children) - 1
			}
			match := false
			for _, child := range n.children[security].children {
				if child.name == "SSL_SERVER_DN_MATCH" {
					match = true
				}
			}
			if !match {
				n.children[security].children = append(n.children[security].children, netDescriptorNode{name: "SSL_SERVER_DN_MATCH", scalar: "YES"})
			}
		}
		return nil
	}
	if err := secure(&root); err != nil {
		return "", err
	}
	if addresses == 0 {
		return "", fmt.Errorf("connection descriptor has no TCPS address")
	}
	return root.String(), nil
}
