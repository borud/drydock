// Package drydock implements a way to run unit tests against a PostgreSQL
// database if you have docker installed.
//
//   import (
//       "github.com/borud/drydock"
//       "github.com/stretchr/testify/assert"
//   )
//
//   func TestSomething(t *testing.T) {
//        // This fires up a Docker container with postgres.  You can
//        // run multiple of these concurrently since this creates a
//        // new container, listening to a unique port.  New will wait
//        // until the database responds or the operation times out
//        // before responding.
//        dd, err := drydock.New("postgres:13")
//        assert.Nil(t, err)
//
//        // Ask the unit test framework to clean up once the test
//        // is done.  If the test crashes this might end up not
//        // running, so there may be docker containers still running.
//        // These will have names that start with "drydock-".
//        t.Cleanup(func() { dd.Terminate() })
//
//        // Start the container
//        err = dd.Start()
//        assert.Nil(t, err)
//
//        // This creates a new database inside the postgres instance
//        // and returns a connection to it.  Or rather, a *sqlx.DB.
//        // The idea being that every time you ask for a new DB and
//        // connection, you want to have a clean database so you can
//        // know the state.
//       db, err := dd.NewDBConn()
//       assert.Nil(t, err)
//
//       // We can then do our database things.
//       _, err = db.Exec("CREATE TABLE foo (id INTEGER NOT NULL)")
//       assert.Nil(t, err)
//
//       stmt, err := db.Preparex("INSERT INTO foo (id) VALUES ($1)")
//       assert.Nil(t, err)
//
//       for i := 0; i < 10; i++ {
//           _, err := stmt.Exec(i)
//           assert.Nil(t, err)
//       }
//
//       // We don't bother cleaning up after ourselves since
//       // the container gets nuked anyway.
//   }
package drydock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // init postgres driver
)

// Drydock represents a database drydock
type Drydock struct {
	// Image is the docker image given by repository and tag, eg. "postgres:13"
	Image string

	// DataDir is the directory where the data files live during the test
	DataDir string

	// Port is a randomly allocated free port that is used by the database instance
	Port int

	// Password of the PostgreSQL database
	Password string

	containerID string
	client      *client.Client
}

const internalPort = "5432"

// New creates a new Drydock instance.  Returns a *Drydock and a nil error if
// successful, otherwise a nil *Drydock and an error
func New(image string) (*Drydock, error) {
	// Create temporary directory
	tempDir, err := ioutil.TempDir("", "dock")
	if err != nil {
		return nil, err
	}

	// Allocate local port
	port, err := getFreePort()
	if err != nil {
		return nil, err
	}

	// Create client
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	// Create drydock
	dd := &Drydock{
		Image:    image,
		DataDir:  tempDir,
		Port:     port,
		Password: randomString(),
		client:   c,
	}

	return dd, nil
}

// Start the drydock.  Will pull the image if we do not have
// it locally before starting the container.
func (d *Drydock) Start() error {
	have, err := d.haveImage()
	if err != nil {
		return err
	}

	if !have {
		log.Printf("pulling '%s' from registry", d.Image)
		err := d.pullImage()
		if err != nil {
			return err
		}
	}
	log.Printf("starting container for PostgreSQL")
	return d.startContainer()
}

// NewDBConn creates a new database and returns a client connection to
// it.
func (d *Drydock) NewDBConn() (*sqlx.DB, error) {
	dbName, err := d.NewDB()
	if err != nil {
		return nil, err
	}

	db, err := sqlx.Connect("postgres", d.postgresConnectString(dbName))
	if err != nil {
		return nil, err
	}

	return db, err
}

// NewDB creates a new database and returns the name
func (d *Drydock) NewDB() (string, error) {
	db, err := sqlx.Connect("postgres", d.postgresConnectString(""))
	if err != nil {
		return "", err
	}

	// Create database
	dbName := "db_" + randomString()
	_, err = db.Exec("CREATE DATABASE " + dbName)
	if err != nil {
		return "", err
	}
	err = db.Close()
	if err != nil {
		return "", err
	}

	return dbName, nil
}

// Terminate removes the container, nukes the data directories and closes the
// docker client.
func (d *Drydock) Terminate() {
	err := d.client.ContainerRemove(
		context.Background(),
		d.containerID,
		types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		},
	)
	if err != nil {
		log.Printf("Error stopping container: %v", err)
	}
	os.RemoveAll(d.DataDir)
	log.Printf("shut down container for PostgreSQL")
	d.client.Close()
}

func (d *Drydock) startContainer() error {
	containerConfig := &container.Config{
		Image: d.Image,
		Env: []string{
			fmt.Sprintf("POSTGRES_PASSWORD=%s", d.Password),
			fmt.Sprintf("PGDATA=%s", d.DataDir),
		},
		Tty: false,
	}

	containerPort, err := nat.NewPort("tcp", internalPort)
	if err != nil {
		return err
	}

	containerHostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			containerPort: []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", d.Port),
				},
			},
		},
		AutoRemove: true,
	}

	container, err := d.client.ContainerCreate(
		context.Background(),
		containerConfig,
		containerHostConfig,
		nil,
		nil,
		fmt.Sprintf("drydock-%s", randomString()),
	)

	if err != nil {
		return err
	}

	d.containerID = container.ID

	err = d.client.ContainerStart(
		context.Background(),
		d.containerID,
		types.ContainerStartOptions{},
	)

	// Try to connect to database until we succeed
	timeout := time.After(10 * time.Second)
	for {
		db, err := sqlx.Connect("postgres", d.postgresConnectString(""))
		if err == nil {
			db.Close()
			break
		}

		select {
		case <-timeout:
			return errors.New("timed out while connecting to postgres")
		default:
		}
	}

	return err
}

func (d Drydock) postgresConnectString(dbName string) string {
	if dbName == "" {
		return fmt.Sprintf("host=localhost user=postgres port=%d password=%s sslmode=disable", d.Port, d.Password)
	}
	return fmt.Sprintf("host=localhost user=postgres dbname=%s port=%d password=%s sslmode=disable", dbName, d.Port, d.Password)
}

func randomString() string {
	rand.Seed(time.Now().UnixNano())
	return strconv.FormatInt(rand.Int63(), 36)
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

// haveImage returns true if we have the docker image with
// the given tag, and false if we do not have it.  Error is
// non nil if we fail to list the images.
func (d *Drydock) haveImage() (bool, error) {
	images, err := d.client.ImageList(context.Background(), types.ImageListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, image := range images {
		for _, tag := range image.RepoTags {
			if tag == d.Image {
				return true, nil
			}
		}
	}
	return false, nil
}

func (d *Drydock) pullImage() error {
	img, err := d.client.ImagePull(context.Background(), d.Image, types.ImagePullOptions{
		All: false,
	})
	if err != nil {
		return err
	}
	defer img.Close()

	_, err = io.Copy(ioutil.Discard, img)
	if err != nil {
		return err
	}
	return nil
}
