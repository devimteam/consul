package consul

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
)

func TestMain(t *testing.M) {
	t.Run()
}

func ExampleNewClient() {
	type testStruct struct {
		Name    string        `consul:"default:name"`
		Email   string        `consul:"default:email"`
		Offset  int           `consul:"default:1"`
		Int64   int64         `consul:"default:164"`
		Uint64  uint64        `consul:"default:1644"`
		Time    time.Time     `consul:"default:2006-01-02T15:04:05Z"`
		Timeout time.Duration `consul:"default:25h"`
	}
	c, err := NewClient(SetLogger(log.NewJSONLogger(os.Stdout)))
	if err != nil {
		panic(err)
	}
	config := testStruct{}
	if err := c.PullOrPush("some", &config); err != nil {
		panic(err)
	}
	fmt.Println(config.Name)
	fmt.Println(config.Email)
	fmt.Println(config.Offset)
	fmt.Println(config.Int64)
	fmt.Println(config.Uint64)
	fmt.Println(config.Time)
	fmt.Println(config.Timeout)
	// Output: name
	// email
	// 1
	// 164
	// 1644
	// 2006-01-02 15:04:05 +0000 UTC
	// 25h0m0s
}

func ExampleNewClient_watch() {
	type testStruct struct {
		Name             string        `consul:"default:name"`
		Timeout          time.Duration `consul:"default:25h"`
		Int              int           `consul:"default:1"`
		WatchableName    String        `consul:"default:some-name"`
		WatchableTimeout Duration      `consul:"default:5s"`
		WatchableInt     Int           `consul:"default:5"`
	}
	c, err := NewClient(Period(time.Second))
	if err != nil {
		panic(err)
	}
	config := testStruct{}
	if err := c.PullOrPush("some", &config); err != nil {
		panic(err)
	}
	fmt.Println(config.Name)
	fmt.Println(config.Timeout)
	fmt.Println(config.Int)
	fmt.Println(config.WatchableName.String())
	fmt.Println(config.WatchableTimeout.Duration())
	fmt.Println(config.WatchableInt.Int())
	time.Sleep(time.Second * 20) // here you can change your variables in consul
	fmt.Println(config.WatchableName.String())
	fmt.Println(config.WatchableTimeout.Duration())
	fmt.Println(config.WatchableInt.Int())

	// Output: name
	// 25h0m0s
	// 1
	// some-name
	// 5s
	// 5
	// some-name
	// 5s
	// 5
}
