package server

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateResolvConfSearch(t *testing.T) {
	fp, err := ioutil.TempFile("", "itzo-test-resolv")
	assert.NoError(t, err)
	filepath := fp.Name()
	defer os.Remove(filepath)
	_, err = fp.Write([]byte(`search something.com somethingelse.com
nameserver 192.168.0.1`))
	assert.NoError(t, err)
	updater := RealResolvConfUpdater{filepath: filepath}
	err = updater.UpdateSearch("mycluster", "mynamespace")
	assert.NoError(t, err)
	expected := []byte(`nameserver 192.168.0.1
search mynamespace.mycluster.local
`)
	actual, err := ioutil.ReadFile(filepath)
	assert.NoError(t, err)
	assert.Equal(t, string(expected), string(actual))
}
