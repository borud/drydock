package drydock

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDrydock(t *testing.T) {
	dd, err := New("postgres:13")
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

	for i := 0; i < 1000; i++ {
		_, err := stmt.Exec(i)
		assert.Nil(t, err)
	}

	var ids []int
	err = db.Select(&ids, "SELECT id FROM foo")
	assert.Nil(t, err)
	assert.Equal(t, 1000, len(ids))

}
