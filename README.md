# Drydock

This code is just an experiment for now.  A sketch to investigate an
idea.  If it proves useful I will perhaps make it a bit cleaner and
turn it into a library.

## Idea

The basic idea behind this code is this

  - You want to run unit tests against a PostgreSQL database
  - You want to do all the setup and teardown in the unit test
  - You have docker installed, you might as well use it

Let's have an example first and then talk later.  With any luck you
don't need to read on.

## Example

```go
func TestSomething(t *testing.T) {
     // This fires up a Docker container with postgres.  You can
     // run multiple of these concurrently since this creates a
     // new container, listening to a unique port.  New will wait
     // until the database responds or the operation times out
     // before responding.
     dd, err := drydock.New("postgres")
     assert.Nil(t, err)

     // Ask the unit test framework to clean up once the test
     // is done.  If the test crashes this might end up not
     // running, so there may be docker containers still running.
     // These will have names that start with "drydock-".
     t.Cleanup(func() { dd.Terminate() })

     // This creates a new database inside the postgres instance
     // and returns a connection to it.  Or rather, a *sqlx.DB.
     // The idea being that every time you ask for a new DB and
     // connection, you want to have a clean database so you can
     // know the state.
    db, err := dd.NewDBConn()
    assert.Nil(t, err)

    // We can then do our database things. 
    _, err = db.Exec("CREATE TABLE foo (id INTEGER NOT NULL)")
    assert.Nil(t, err)

    stmt, err := db.Preparex("INSERT INTO foo (id) VALUES ($1)")
    assert.Nil(t, err)

    for i := 0; i < 10; i++ {
        _, err := stmt.Exec(i)
        assert.Nil(t, err)
    }

    // We don't bother cleaning up after ourselves since
    // the container gets nuked anyway.
}
```
