package utfkenall

import (
	"bytes"
	_ "embed"
	"sync"

	"github.com/memwey/utfkenall/internal/binfmt"
)

//go:generate go run ./tools/gendata -out data/kenall.bin

//go:embed data/kenall.bin
var packed []byte

var decode = sync.OnceValues(func() (*binfmt.Store, error) {
	return binfmt.Decode(bytes.NewReader(packed))
})

// Load decodes the embedded database and reports whether it is usable.
//
// Calling it is optional — the first lookup decodes the database anyway — but
// it lets a program pay the cost at a chosen moment, such as during start-up
// rather than while serving the first request.
func Load() error {
	_, err := decode()
	return err
}

// mustLoad is for the accessors that have no error to return. A failure here
// means the embedded blob does not match the format this build expects, which
// is a broken binary rather than anything a caller can recover from.
func mustLoad() *binfmt.Store {
	s, err := decode()
	if err != nil {
		panic("utfkenall: embedded database is unreadable: " + err.Error())
	}
	return s
}
