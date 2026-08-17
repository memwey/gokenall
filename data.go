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

// database is the decoded store plus everything derived from it that would
// otherwise be recomputed on every address.
type database struct {
	store *binfmt.Store
	// prefectures holds the display spelling of all 47, derived once from the
	// published one. There are only 47 of them and every address carries one,
	// so deriving per address costs more than the rest of a lookup together.
	prefectures [binfmt.PrefectureCount]Name
}

var decode = sync.OnceValues(func() (*database, error) {
	store, err := binfmt.Decode(bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	db := &database{store: store}
	for i, published := range store.Prefectures() {
		db.prefectures[i] = prefectureName(published)
	}
	return db, nil
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
func mustLoad() *database {
	db, err := decode()
	if err != nil {
		panic("utfkenall: embedded database is unreadable: " + err.Error())
	}
	return db
}
