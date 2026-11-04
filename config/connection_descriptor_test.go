// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"strings"
	"testing"
)

func TestVerifiedTCPSDescriptorRejectsMalformedOrInsecureInput(t *testing.T) {
	for _, value := range []string{
		"adb.example.com:1521/orders_low",
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=adb.example.com)(PORT=1521)))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521)))trailing`,
		`(DESCRIPTION=(ADDRESS=(HOST="(PROTOCOL=TCPS)")(PORT=1521)))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=other)(PORT=1521)))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))(SECURITY=(SSL_SERVER_DN_MATCH=NO)))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))(SECURITY=invalid))`,
		`(DESCRIPTION_LIST=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521)))`,
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=0)))`,
	} {
		if _, err := verifiedTCPSDescriptor(value); err == nil {
			t.Fatalf("unsafe descriptor accepted: %s", value)
		}
	}
}

func TestVerifiedTCPSDescriptorPreservesQuotedCertificateDN(t *testing.T) {
	value := `(description=(address=(protocol=tcps)(host=adb.example.com)(port=1521))(connect_data=(service_name=orders_low))(security=(ssl_server_cert_dn="CN=adb.example.com,OU=Oracle (Cloud),O=Oracle Corporation")))`
	got, err := verifiedTCPSDescriptor(value)
	if err != nil || !strings.Contains(got, `"CN=adb.example.com,OU=Oracle (Cloud),O=Oracle Corporation"`) || !strings.Contains(got, "SSL_SERVER_DN_MATCH=YES") {
		t.Fatalf("certificate verification or quoted DN changed: %s, %v", got, err)
	}
}
