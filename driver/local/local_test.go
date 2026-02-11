package local_test

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/nuln/sbox/driver/local"
	"github.com/nuln/sbox/sboxtest"
)

func TestLocalEngine(t *testing.T) {
	engine := local.NewWithFs(afero.NewMemMapFs())
	sboxtest.StorageTestSuite(t, engine)
}
