package drydock

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/url"
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
	Image       string
	DataDir     string
	Port        int
	Password    string
	containerID string
	client      *client.Client
}

const internalPort = "5432"

type DrydockBuilder struct {
	Image    string
	Port     int
	Password string
}

func NewDrydockBuilder() *DrydockBuilder {

	// Get a random string for password
	password := randomString()

	builder := DrydockBuilder{
		Image:    "",
		Port:     -1,
		Password: password,
	}

	return &builder
}

func (builder *DrydockBuilder) Build() (*Drydock, error) {
	// Create temporary directory
	tempDir, err := ioutil.TempDir("", "dock")
	if err != nil {
		return nil, err
	}

	// If a valid portnumber isn't set, then find
	// a free port.
	if builder.Port < 0 {
		// Find  an unused local port
		port, err := getFreePort()
		if err != nil {
			return nil, err
		}
		builder.Port = port
	}

	if builder.Image == "" {
		return nil, fmt.Errorf("Docker image name can't be empty.")
	}

	// Create client
	c, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	// Create drydock
	dd := &Drydock{
		Image:    builder.Image,
		DataDir:  tempDir,
		Port:     builder.Port,
		Password: builder.Password,
		client:   c,
	}

	return dd, dd.startContainer()
}

func (builder *DrydockBuilder) SetImage(image string) *DrydockBuilder {
	builder.Image = image
	return builder
}

func (builder *DrydockBuilder) SetPassword(password string) *DrydockBuilder {
	builder.Password = password
	return builder
}

func (builder *DrydockBuilder) SetPort(port int) *DrydockBuilder {
	builder.Port = port
	return builder
}

// New creates a new Drydock instance with a random password and a free port
func New(image string) (*Drydock, error) {
	dd, err := NewDrydockBuilder().SetImage(image).Build()
	return dd, err
}

// NewDBConn creates a new database and returns a client connection to
// it.
func (d *Drydock) NewDBConn() (*sqlx.DB, error) {
	dbName := "db_" + randomString()
	db, err := d.NewDBConnToNamedDb(dbName)
	return db, err
}

func (d *Drydock) NewDBConnToNamedDb(dbName string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", d.postgresConnectString(""))
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("CREATE DATABASE " + dbName)
	if err != nil {
		return nil, err
	}
	err = db.Close()
	if err != nil {
		return nil, err
	}

	db, err = sqlx.Connect("postgres", d.postgresConnectString(dbName))
	if err != nil {
		return nil, err
	}

	return db, err
}

// Terminate ...
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
				nat.PortBinding{
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
		case <-time.After(100 * time.Millisecond):
			break
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

func (d Drydock) JdbcConnectString(dbName string) string {
	return fmt.Sprintf("jdbc:postgresql://%s:%d/%s?user=%s&password=%s", "localhost", d.Port, url.QueryEscape(dbName), "postgres", url.QueryEscape(d.Password))
}

func randomString() string {
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
