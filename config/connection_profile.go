// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"strings"

	adb "github.com/oracle/oci-go-sdk/v65/database"
)

func connectionProfileForService(connectionStrings *adb.AutonomousDatabaseConnectionStrings, service string, mutual bool) (string, error) {
	service = strings.ToUpper(strings.TrimSpace(service))
	switch service {
	case "LOW", "MEDIUM", "HIGH", "TP", "TPURGENT":
	default:
		return "", fmt.Errorf("unsupported connection service %q; use LOW, MEDIUM, HIGH, TP, or TPURGENT", service)
	}
	if connectionStrings == nil {
		return "", fmt.Errorf("Autonomous Database has no connection profiles")
	}
	tlsAuth := adb.DatabaseConnectionStringProfileTlsAuthenticationServer
	if mutual {
		tlsAuth = adb.DatabaseConnectionStringProfileTlsAuthenticationMutual
	}
	var best string
	var bestRank int
	for _, profile := range connectionStrings.Profiles {
		if !strings.EqualFold(string(profile.ConsumerGroup), service) || profile.Protocol != adb.DatabaseConnectionStringProfileProtocolTcps || profile.TlsAuthentication != tlsAuth {
			continue
		}
		// A database's FQDN profile is preferred over IP addresses, and direct
		// profiles over redirects. A lexical tie-breaker avoids polling changes
		// merely because OCI reordered its profiles.
		rank := 0
		if profile.HostFormat != adb.DatabaseConnectionStringProfileHostFormatFqdn {
			rank += 2
		}
		if profile.SessionMode != adb.DatabaseConnectionStringProfileSessionModeDirect {
			rank++
		}
		value, err := verifiedTCPSDescriptor(stringValue(profile.Value))
		if err != nil {
			continue
		}
		if best == "" || rank < bestRank || (rank == bestRank && value < best) {
			best, bestRank = value, rank
		}
	}
	if best == "" {
		return "", fmt.Errorf("Autonomous Database has no verified TCPS %s profile for service %s", tlsAuth, service)
	}
	return best, nil
}
