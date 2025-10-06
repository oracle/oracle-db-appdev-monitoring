package hashivault

import (
	"testing"
)

/*
	Performs some integration tests against a running Vault proxy
*/

const (
	// TODO: Mock the entire Vault response and do not depend on external Vault
	socketPath = "/var/run/vault/vault.sock"
	testMount = "dev.mt1"
	testPath = "oracle/devdbs01/monitoring"
)

func TestHashiCorpVaultKVV2Secret(t *testing.T) {
	c,err := createVaultClient(socketPath)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = c.getVaultSecret("kvv2", testMount, testPath, []string{"password","username"})
	if err != nil {
		t.Error(err)
	}
}

func TestHashiCorpVaultMissingKey(t *testing.T) {
	c,err := createVaultClient(socketPath)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = c.getVaultSecret("kvv2", testMount, testPath, []string{"password","username","keythatdoesnotexist"})
	if err == nil || err != RequiredKeyMissing {
		t.Error("Wrong error code, expected RequiredKeyMissing")
	}
}

func TestHashiCorpVaultUnsupportedSecret(t *testing.T) {
	c,err := createVaultClient(socketPath)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = c.getVaultSecret("doesnotexist", testMount, testPath, []string{"password","username"})
	if err == nil || err != UnsupportedMountType {
		t.Error("Wrong error code, expected UnsupportedMountType")
	}
}

