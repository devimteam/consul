package consul

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

const groupEnvName = "GROUP_NAME"

type ErrKVNotFound struct {
	Key string
}

func (e ErrKVNotFound) Error() string {
	return fmt.Sprintf("kv \"%s\" not found", e.Key)
}

var (
	ErrInvalidServiceAddr = errors.New("invalid service address")
	ErrInvalidPort        = errors.New("invalid port")
	ErrInvalidTagOptions  = errors.New("invalid tag options")
)

var allowOptions = map[string]string{"name": "", "default": ""}

//Client provides an interface for getting data out of Consul
type Client interface {
	// GetServices get a services from consul
	GetServices(service string, tag string) ([]*consulapi.ServiceEntry, *consulapi.QueryMeta, error)
	// GetFirstService get a first service from consul
	GetFirstService(service string, tag string) (*consulapi.ServiceEntry, *consulapi.QueryMeta, error)
	// RegisterService register a service with local agent
	RegisterService(name string, addr string, tags ...string) error
	// DeRegisterService deregister a service with local agent
	DeRegisterService(string) error
	// Get get KVPair
	Get(key string) (*consulapi.KVPair, *consulapi.QueryMeta, error)
	// WatchGet
	WatchGet(key string) chan *consulapi.KVPair
	// GetStr get string value
	GetStr(key string) (string, error)
	// GetInt get string value
	GetInt(key string) (int, error)
	// Put put KVPair
	Put(key string, value string) (*consulapi.WriteMeta, error)
	// Load struct
	LoadStruct(parent string, i interface{}) error
}

type client struct {
	kv     *consulapi.KV
	health *consulapi.Health
	meta   map[string]*consulapi.QueryMeta
	agent  *consulapi.Agent
}

// NewClient returns a Client interface for given consul address
func NewClientWithConsulClient(c *consulapi.Client) Client {
	return &client{
		kv:     c.KV(),
		health: c.Health(),
		agent:  c.Agent(),
		meta:   make(map[string]*consulapi.QueryMeta),
	}
}

// NewClient returns a Client interface for given consul address
func NewClientWithDefaultConfig() (Client, error) {
	return NewClient(consulapi.DefaultConfig())
}

// NewClient returns a Client interface for given consul address
func NewClient(config *consulapi.Config) (Client, error) {
	c, err := consulapi.NewClient(config)
	if err != nil {
		return nil, err
	}
	return NewClientWithConsulClient(c), nil
}

// Get KVPair
func (c *client) Get(key string) (*consulapi.KVPair, *consulapi.QueryMeta, error) {
	kv, meta, err := c.kv.Get(key, nil)
	if err != nil {
		return nil, nil, err
	}
	if kv == nil {
		return nil, nil, ErrKVNotFound{Key: key}
	}

	c.meta[key] = meta

	return kv, meta, nil
}

func (c *client) WatchGet(key string) chan *consulapi.KVPair {
	doneCh := make(chan *consulapi.KVPair)
	go func(k string, ch chan *consulapi.KVPair) {
		for {
			var lastIndex uint64 = 1
			if meta, ok := c.meta[key]; ok {
				lastIndex = meta.LastIndex
			}
			kv, meta, err := c.kv.Get(k, &consulapi.QueryOptions{WaitIndex: lastIndex})

			if lastIndex == 1 && kv == nil {
				continue
			}

			if err != nil {
				close(ch)
			}
			c.meta[key] = meta
			ch <- kv
		}
	}(key, doneCh)
	return doneCh
}

// GetStr string
func (c *client) GetStr(key string) (string, error) {
	kv, _, err := c.Get(key)
	if err != nil {
		return "", err
	}
	return string(kv.Value), nil
}

func (c *client) GetInt(key string) (int, error) {
	v, err := c.GetStr(key)
	if err != nil {
		return 0, err
	}
	res, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return res, nil
}

// Put KVPair
func (c *client) Put(key string, value string) (*consulapi.WriteMeta, error) {
	p := &consulapi.KVPair{Key: key, Value: []byte(value)}
	return c.kv.Put(p, nil)
}

// RegisterService a service with consul local agent
func (c *client) RegisterService(name string, addr string, tags ...string) error {
	host, strPort, err := net.SplitHostPort(addr)
	if err != nil {
		return ErrInvalidServiceAddr
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return ErrInvalidPort
	}

	reg := &consulapi.AgentServiceRegistration{
		ID:      name,
		Name:    name,
		Address: host,
		Port:    port,
		Tags:    tags,
		Check: &consulapi.AgentServiceCheck{
			Script:   fmt.Sprintf("curl localhost:%d > /dev/null 2>&1", port),
			Interval: "10s",
		},
	}
	return c.agent.ServiceRegister(reg)
}

// DeRegisterService a service with consul local agent
func (c *client) DeRegisterService(id string) error {
	return c.agent.ServiceDeregister(id)
}

// GetFirstService get first service
func (c *client) GetFirstService(service string, tag string) (*consulapi.ServiceEntry, *consulapi.QueryMeta, error) {
	addrs, meta, err := c.GetServices(service, tag)
	if err != nil {
		return nil, nil, err
	}
	if len(addrs) == 0 {
		return nil, nil, errors.New(fmt.Sprintf("service \"%s\" not found", service))
	}
	return addrs[0], meta, nil
}

// GetServices return a services
func (c *client) GetServices(service string, tag string) ([]*consulapi.ServiceEntry, *consulapi.QueryMeta, error) {
	passingOnly := true
	addrs, meta, err := c.health.Service(service, tag, passingOnly, nil)
	if err != nil {
		return nil, nil, err
	}
	if len(addrs) == 0 {
		return nil, nil, errors.New(fmt.Sprintf("service \"%s\" not found", service))
	}
	return addrs, meta, nil
}

func (c *client) LoadStruct(parent string, i interface{}) error {
	groupName := os.Getenv(groupEnvName)
	if groupName != "" {
		parent = fmt.Sprintf("%s/%s", strings.Trim(groupName, "/"), parent)
	}
	return c.recursiveLoadStruct(parent, reflect.ValueOf(i).Elem())
}

func (c *client) recursiveLoadStruct(parent string, val reflect.Value) error {
	for i := 0; i < val.NumField(); i++ {
		value := val.Field(i)
		field := val.Type().Field(i)

		var tagOptions map[string]string
		var err error

		tag := field.Tag.Get("consul")
		if tag != "" {
			tagOptions, err = c.getTagOptions(tag)
			if err != nil {
				return err
			}
		}

		var kvName string
		if name, ok := tagOptions["name"]; ok {
			kvName = name
		} else {
			kvName = c.normalizeKeyName(field.Name)
		}

		path := fmt.Sprintf("%s/%s", parent, kvName)

		if _, ok := value.Interface().(time.Time); ok {
		} else if field.Type.Kind() == reflect.Struct {
			err = c.recursiveLoadStruct(path, value)
			if err != nil {
				return err
			}
		} else {
			var fieldValue []byte

			if defaultValue, ok := tagOptions["default"]; ok {
				fieldValue = []byte(defaultValue)
			}

			kv, _, err := c.Get(path)

			if err != nil {
				if _, ok := err.(ErrKVNotFound); !ok {
					return err
				} else {
					_, err = c.Put(path, string(fieldValue))
					if err != nil {
						return err
					}
				}
			}

			if kv != nil {
				fieldValue = kv.Value
			}

			v, err := c.normalizeValue(field.Type.Kind(), fieldValue)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(v))
		}
	}
	return nil
}

func (c *client) normalizeValue(kind reflect.Kind, value []byte) (interface{}, error) {
	switch kind {
	case reflect.String:
		return string(value), nil
	case reflect.Float32:
		n, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 32)
		if err != nil {
			return nil, err
		}
		return float32(n), nil
	case reflect.Float64:
		n, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case reflect.Int:
		n, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
		if err != nil {
			return nil, err
		}
		return int(n), nil
	default:
		return nil, errors.New(fmt.Sprintf("unsupported type \"%s\"", kind.String()))
	}
}

func (c *client) normalizeKeyName(name string) string {
	s := regexp.MustCompile("([A-Z]+[^A-Z]*)").FindAllString(name, -1)
	ss := strings.Join(s[:], ".")
	return strings.ToLower(ss)
}

func (c *client) getTagOptions(v string) (map[string]string, error) {
	parts := strings.Split(v, ":")

	size := len(parts)
	if size%2 != 0 {
		return nil, ErrInvalidTagOptions
	}

	res := make(map[string]string)
	for i := 0; i < len(parts); i += 2 {
		name := parts[i]
		value := parts[i+1]

		if !c.allowOption(name) {
			continue
		}

		res[name] = value
	}

	return res, nil
}

func (c *client) allowOption(name string) bool {
	_, ok := allowOptions[name]
	return ok
}
