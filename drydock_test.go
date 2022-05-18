package drydock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDrydock(t *testing.T) {
	dd, err := New("postgres:14")
	assert.Nil(t, err)
	t.Cleanup(func() { dd.Terminate() })

	err = dd.Start()
	assert.Nil(t, err)

	db, err := dd.NewDBConn()
	assert.Nil(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE foo (id INTEGER NOT NULL)")
	assert.Nil(t, err)

	stmt, err := db.Preparex("INSERT INTO foo (id) VALUES ($1)")
	assert.Nil(t, err)

	for i := 0; i < 100; i++ {
		_, err := stmt.Exec(i)
		assert.Nil(t, err)
	}

	var ids []int
	err = db.Select(&ids, "SELECT id FROM foo")
	assert.Nil(t, err)
	assert.Equal(t, 100, len(ids))
}
