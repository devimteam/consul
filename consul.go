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

const (
	tagOptionRegexpString = "^([\\w]+):(.+)$"
)

var tagOptionRegexp = regexp.MustCompile(tagOptionRegexpString)

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
	GetStr(key string) (string, error)
	GetInt(key string) (int, error)
	GetBool(key string) (bool, error)
	// Put put KVPair
	Put(key string, value string) (*consulapi.WriteMeta, error)
	// Load struct
	LoadStruct(parent string, i interface{}) error
	// Replace struct values
	ReplaceFromStruct(parent string, i interface{}) error
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

func (c *client) GetBool(key string) (bool, error) {
	v, err := c.GetStr(key)
	if err != nil {
		return false, err
	}
	res, err := strconv.ParseBool(string(v))
	if err != nil {
		return false, err
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
	return c.recursiveLoadStruct(c.getGroupName(parent), reflect.ValueOf(i).Elem())
}

func (c *client) getGroupName(parent string) string {
	groupName := os.Getenv(groupEnvName)
	if groupName != "" {
		parent = fmt.Sprintf("%s/%s", strings.Trim(groupName, "/"), parent)
	}
	return parent
}

func (c *client) getKeyPath(parent string, field reflect.StructField) (string, error) {
	var tagOptions map[string]string
	var err error
	tagOptions, err = c.getTagOptions(field.Tag.Get("consul"))
	if err != nil {
		return "", err
	}

	kvName  := c.normalizeKeyName(field.Name)
	if name, ok := tagOptions["name"]; ok {
		kvName = name
	}
	return fmt.Sprintf("%s/%s", parent, kvName), nil
}

func (c *client) recursiveLoadStruct(parent string, val reflect.Value) error {
	for i := 0; i < val.NumField(); i++ {
		value := val.Field(i)
		field := val.Type().Field(i)

		var tagOptions map[string]string
		var err error

		tagOptions, err = c.getTagOptions(field.Tag.Get("consul"))
		if err != nil {
			return err
		}

		// err always nil
		path, _ := c.getKeyPath(parent, field)
		if _, ok := value.Interface().(time.Time); ok {
			// break ifs
		} else if field.Type.Kind() == reflect.Map {
			if field.Type.Key().Kind() != reflect.String {
				return fmt.Errorf("%s is unsupported map's key type", field.Type.Key().String())
			}
			if field.Type.Elem().Kind() != reflect.String {
				return fmt.Errorf("%s is unsupported map's value type", field.Type.Elem().String())
			}
			_, _, err := c.Get(path)
			if err != nil {
				if _, ok := err.(ErrKVNotFound); !ok {
					return err
				} else {
					_, err = c.Put(path+"/", "")
					if err != nil {
						return err
					}
				}
			}
			m, err := c.loadMapStringString(path, value)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(m))
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

			v, err := c.typifyValue(field.Type, fieldValue)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(v))
		}
	}
	return nil
}

func (c *client) loadMapStringString(parent string, val reflect.Value) (map[string]string, error ){
	pairs, _, err := c.kv.List(parent, nil)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		key := strings.TrimLeft(strings.TrimPrefix(p.Key, parent), "/")
		if key == "" {
			continue
		}
		m[key] = string(p.Value)
	}
	return m, nil
}

func (c *client) typifyValue(reflectType reflect.Type, value []byte) (interface{}, error) {
	switch reflectType.Kind() {
	case reflect.String:
		return string(value), nil
	case reflect.Float32:
		if len(value) == 0 {
			return float32(0.0), nil
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 32)
		if err != nil {
			return nil, err
		}
		return float32(n), nil
	case reflect.Float64:
		if len(value) == 0 {
			return float64(0.0), nil
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case reflect.Int:
		if len(value) == 0 {
			return 0, nil
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case reflect.Bool:
		b, err := strconv.ParseBool(string(value))
		if err != nil {
			return nil, err
		}
		return b, nil
	case reflect.Map:

	}

	if reflectType == reflect.TypeOf(time.Duration(5)) {
		n, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
		if err != nil {
			return nil, err
		}
		return time.Duration(n), nil
	}
	return nil, errors.New(fmt.Sprintf("unsupported type \"%s\"", reflectType.Kind().String()))
}

func (c *client) ReplaceFromStruct(parent string, i interface{}) error {
	return c.recursiveReplaceStruct(c.getGroupName(parent), reflect.ValueOf(i).Elem())
}

func (c *client) recursiveReplaceStruct(parent string, val reflect.Value) error {
	for i := 0; i < val.NumField(); i++ {
		value := val.Field(i)
		field := val.Type().Field(i)
		var err error

		path, err := c.getKeyPath(parent, field)

		if _, ok := value.Interface().(time.Time); ok {
		} else if field.Type.Kind() == reflect.Struct {
			err = c.recursiveReplaceStruct(path, value)
			if err != nil {
				return err
			}
		} else {
			fieldValue, err := c.stringifyValue(value)
			if err != nil {
				return err
			}
			_, err = c.Put(path, fieldValue)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *client) stringifyValue(value reflect.Value) (string, error) {
	switch value.Type().Kind() {
	case reflect.String:
		return value.String(), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'f', -1, 64), nil
	case reflect.Int:
		return strconv.FormatInt(value.Int(), 10), nil
	case reflect.Bool:
		return strconv.FormatBool(value.Bool()), nil
	}

	if _, ok := value.Interface().(time.Duration); ok {
		return strconv.FormatInt(value.Int(), 10), nil
	}

	return "", errors.New(fmt.Sprintf("unsupported type \"%s\"", value.Type().Kind().String()))
}

func (c *client) normalizeKeyName(name string) string {
	s := regexp.MustCompile("([A-Z]+[^A-Z]*)").FindAllString(name, -1)
	ss := strings.Join(s[:], ".")
	return strings.ToLower(ss)
}

func (c *client) getTagOptions(v string) (map[string]string, error) {
	res := make(map[string]string)

	if v == "" {
		return res, nil
	}

	options := strings.Split(v, ";")
	for _, option := range options {
		parts := tagOptionRegexp.FindAllStringSubmatch(option, 1)
		if len(parts) == 1 && len(parts[0]) == 3 {
			optionName := parts[0][1]
			optionValue := parts[0][2]
			if !c.allowOption(optionName) {
				continue
			}
			res[optionName] = optionValue
		}
	}
	return res, nil
}

func (c *client) allowOption(name string) bool {
	_, ok := allowOptions[name]
	return ok
}
