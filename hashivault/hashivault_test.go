package hashivault

import (
	"testing"
	"log/slog"
	vault "github.com/hashicorp/vault/api"
	// requires go1.25
	"github.com/hashicorp/vault/sdk/helper/testcluster/docker"
)

/*
	Removing the tests until project is upgraded to go1.25
	https://github.com/hashicorp/vault?tab=readme-ov-file#docker-based-tests
*/

const (
	dockerImageRepo = "hashicorp/vault"
	dockerImageTag = "latest"
	kvTestMount = "secret"
	kvTestPath = "foo1"
	kvTestSecret = map[string]string{
		"username": "c##monitoring",
		"password": "ep82^RxU>iqE%ZMWr!}UmtM50?~C@P",
		"user": "monitoring",
		"pass": "kz)7E9nJm9BDpYM0=T5Me#YGwQv?pW",
	}
)

// createVaultServer Starts local Vault server for testing purposes
func createVaultServerAndClient(t *testing.T) (HashicorpVaultClient, *docker.DockerCluster) {
	t.Helper()

	opts := &docker.DockerClusterOptions{
		ImageRepo: dockerImageRepo,
		ImageTag:  dockerImageTag,
	}
	cluster := docker.NewTestDockerCluster(t, opts)
	client := cluster.Nodes()[0].APIClient()
	hvc := HashicorpVaultClient{
		client: client,
		logger: slog.Default(),
	}

	return hvc, cluster
}

func writeTestKVSecret(t *testing.T, c HashicorpVaultClient) {
	t.Helper()
	_,err := c.client.KVv2(kvTestMount).Put(context.TODO(), kvTestPath, kvTestSecret)
    if err != nil {
        t.Fatal(err)
    }
}

func TestHashiCorpVaultKVV2Secret(t *testing.T) {
	c,cluster := createVaultServerAndClient(t)
	defer cluster.Cleanup()
	writeTestKVSecret(t, c)
	_, err := c.getVaultSecret("kvv2", testMount, testPath, []string{"password","username"})
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
