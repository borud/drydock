package drydock

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamedDatabase(t *testing.T) {
	password := "verySekrit"
	dd, err := NewDrydockBuilder().SetImage("postgres:latest").SetPassword(password).SetPort(32950).Build()
	assert.Nil(t, err)
	t.Cleanup(func() { dd.Terminate() })

	dbName := "db_foobarjalla"
	db, err := dd.NewDBConnToNamedDb(dbName)
	assert.Nil(t, err)
	defer db.Close()

	fmt.Println("JDBC connect string = ", dd.JdbcConnectString(dbName))
	createAndSelectSomeValues(t, err, db)
}

func TestDrydock(t *testing.T) {
	dd, err := New("postgres:latest")
	assert.Nil(t, err)
	t.Cleanup(func() { dd.Terminate() })

	db, err := dd.NewDBConn()
	assert.Nil(t, err)
	defer db.Close()

	createAndSelectSomeValues(t, err, db)
}

func createAndSelectSomeValues(t *testing.T, err error, db *sqlx.DB) {
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
