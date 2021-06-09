# Drydock

This code is an experimental library for running PostgreSQL in Docker during unit tests.

[![PkgGoDev](https://pkg.go.dev/badge/github.com/borud/drydock)](<https://pkg.go.dev/github.com/borud/drydock>) [![Go Report Card](https://goreportcard.com/badge/github.com/borud/drydock)](https://goreportcard.com/report/github.com/borud/drydock)[![Actions Status](https://github.com/borud/drydock/workflows/build/badge.svg)](https://github.com/borud/<repo>/drydock)

## Idea

The basic idea behind this code is this

- You want to run unit tests against a PostgreSQL database
- You want to do all the setup and teardown in the unit test
- You have docker installed, you might as well use it

## Example

```go
import (
    "github.com/borud/drydock"
    "github.com/stretchr/testify/assert"
)

func TestSomething(t *testing.T) {
     // This fires up a Docker container with postgres.  You can
     // run multiple of these concurrently since this creates a
     // new container, listening to a unique port.  New will wait
     // until the database responds or the operation times out
     // before responding.
     dd, err := drydock.New("postgres:13")
     assert.Nil(t, err)

     // Ask the unit test framework to clean up once the test
     // is done.  If the test crashes this might end up not
     // running, so there may be docker containers still running.
     // These will have names that start with "drydock-".
     t.Cleanup(func() { dd.Terminate() })

     // Start the container
     err = dd.Start()
     assert.Nil(t, err)

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

## Where to take this?

This seems to work pretty well.  The container tends to come up in
about 500ms on my machine, and PostgreSQL then needs another 2500ms or
so before it is ready to serve requests.

I should be doable to make this somewhat generic, so that it can work
for a wider range of databases, and perhaps too things that are not
databases.  What other scenarios would it be nice to make use of
ephemeral docker containers.

I haven't really figured out the cleanup phase yet.  If we kill the
test before it shuts down the docker container will remain and needs
to be removed manually.  There are several ways we could try to solve
that.

There is also the question of "do we need this?".  It might be cleaner
to manage docker containers for unit testing in the build system.  But
being able to do this directly from the tests is somewhat enticing in
its immediacy and simplicity.

If you find this idea interesting, please do not hesitate to grab the
code and play with it.  Let me know if you do something interesting
with it.
