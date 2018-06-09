package test

import (
	crand "crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/devimteam/consul"
	"github.com/devimteam/consul/testutil"
	"github.com/devimteam/gounit"
)

type Nested struct {
	Name  string
	Delay float32
}

type testStruct struct {
	Name   string
	Email  string
	Offset int
	Time   time.Time
	Nested Nested
	Keys map[string]string
}

func makeTestClient() (consul.Client, error) {
	return testutil.NewClient()
}

func testKey() string {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		panic(fmt.Errorf("failed to read random bytes: %v", err))
	}

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16])
}

func TestWatchPut(t *testing.T) {
	u := gounit.New(t)

	key := testKey()

	client, err := makeTestClient()
	u.AssertNotError(err, "")

	value := "hello"

	_, err = client.Put(key, value)
	u.AssertNotError(err, "")

	kv, _, err := client.Get(key)
	u.AssertNotError(err, "")
	u.AssertNotNil(kv, "")
}

func TestLoadStruct(t *testing.T) {
	u := gounit.New(t)

	client, err := makeTestClient()
	u.AssertNotError(err, "")

	var s testStruct

	err = client.LoadStruct("service", &s)
	u.AssertNotError(err, "")

	u.AssertEquals("test", s.Name, gounit.EmptyMessage)
	u.AssertEquals("email", s.Email, gounit.EmptyMessage)
	u.AssertEquals("name", s.Nested.Name, gounit.EmptyMessage)
	u.AssertEquals(2, s.Offset, gounit.EmptyMessage)
	u.AssertEquals(float32(2.33), s.Nested.Delay, gounit.EmptyMessage)
	u.AssertEquals(map[string]string{"A":"A","B":"B"}, s.Keys, gounit.EmptyMessage)
}

func TestLoadStructDefaultValue(t *testing.T) {
	u := gounit.New(t)

	client, err := makeTestClient()
	u.AssertNotError(err, "")

	var s struct {
		Name string `consul:"default:Rob Pike"`
		Size int    `consul:"default:100"`
	}

	err = client.LoadStruct("service", &s)
	u.AssertNotError(err, "Err")
	u.AssertEquals("Rob Pike", s.Name, "Equals Name")
	u.AssertEquals(100, s.Size, "Equals Size")
}

func TestWatchGet(t *testing.T) {
	u := gounit.New(t)

	key := testKey()

	client, err := makeTestClient()
	u.AssertNotError(err, "")

	ch := client.WatchGet(key)

	value := "test"

	go func() {
		time.Sleep(100 * time.Millisecond)

		_, err := client.Put(key, value)
		u.AssertNotError(err, "put error")
	}()

	kv := <-ch

	u.AssertNotNil(kv, "key/value")
	u.AssertEquals(value, string(kv.Value), "")
}
