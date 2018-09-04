package consul

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/vetcher/go-case"
)

const groupEnvName = "GROUP_NAME"

const tagOptionRegexpString = "^([\\w]+):(.+)$"

var tagOptionRegexp = regexp.MustCompile(tagOptionRegexpString)

type ErrKVNotFound struct{ Key string }

func (e ErrKVNotFound) Error() string { return fmt.Sprintf("kv \"%s\" not found", e.Key) }

var allowOptions = map[string]struct{}{
	"name":    {},
	"default": {},
}

// Client provides an interface for getting data out of Consul
type Client interface {
	// Primitive, that gets value by key
	Get(key string) (*consulapi.KVPair, *consulapi.QueryMeta, error)
	// Primitive, that puts value by key
	Put(key string, value string) (*consulapi.WriteMeta, error)
	// LoadStruct loads kv pairs to structure.
	// Allowed types for struct fields:
	// - string, []byte
	// - int, int64, uint, uint64, float32, float64
	// - time.Duration
	// - map[string]string
	LoadStruct(parent string, i interface{}) error
	// ReplaceFromStruct pushes structure's value to server
	// Same supported types with LoadStruct
	ReplaceFromStruct(parent string, i interface{}) error
}

type client struct {
	kv *consulapi.KV
}

// NewClient returns a Client interface for given consul address
func NewClientWithConsulClient(c *consulapi.Client) Client {
	return &client{
		kv: c.KV(),
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
	return kv, meta, nil
}

// Put KVPair
func (c *client) Put(key string, value string) (*consulapi.WriteMeta, error) {
	p := &consulapi.KVPair{Key: key, Value: []byte(value)}
	return c.kv.Put(p, nil)
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

func (c *client) getKeyPath(parent string, field reflect.StructField) string {
	tagOptions := c.getTagOptions(field.Tag.Get("consul"))
	kvName := c.normalizeKeyName(field.Name)
	if name, ok := tagOptions["name"]; ok {
		kvName = name
	}
	return fmt.Sprintf("%s/%s", parent, kvName)
}

func (c *client) recursiveLoadStruct(parent string, val reflect.Value) error {
	for i := 0; i < val.NumField(); i++ {
		value := val.Field(i)
		field := val.Type().Field(i)
		tagOptions := c.getTagOptions(field.Tag.Get("consul"))
		path := c.getKeyPath(parent, field)

		switch field.Type.Kind() {
		case reflect.Map:
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
				}
				_, err = c.Put(path+"/", "")
				if err != nil {
					return err
				}
			}
			m, err := c.loadMapStringString(path, value)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(m))
		case reflect.Struct:
			err := c.recursiveLoadStruct(path, value)
			if err != nil {
				return err
			}
		default:
			var fieldValue string
			if defaultValue, ok := tagOptions["default"]; ok {
				fieldValue = defaultValue
			}

			kv, _, err := c.Get(path)
			if err != nil {
				if _, ok := err.(ErrKVNotFound); !ok {
					return err
				}
				_, err = c.Put(path, fieldValue)
				if err != nil {
					return err
				}
			}

			if kv != nil {
				fieldValue = string(kv.Value)
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

func (c *client) loadMapStringString(parent string, val reflect.Value) (map[string]string, error) {
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

func (c *client) typifyValue(reflectType reflect.Type, value string) (interface{}, error) {
	value = strings.TrimSpace(value)
	switch reflectType.Kind() {
	case reflect.String:
		return value, nil
	case reflect.Float32:
		if len(value) == 0 {
			return float32(0.0), nil
		}
		n, err := strconv.ParseFloat(value, 32)
		return float32(n), err
	case reflect.Float64:
		if len(value) == 0 {
			return float64(0.0), nil
		}
		return strconv.ParseFloat(value, 64)
	case reflect.Int:
		if len(value) == 0 {
			return 0, nil
		}
		n, err := strconv.ParseInt(value, 10, 64)
		return int(n), err
	case reflect.Uint:
		if len(value) == 0 {
			return uint(0), nil
		}
		n, err := strconv.ParseUint(value, 10, 64)
		return uint(n), err
	case reflect.Uint64:
		if len(value) == 0 {
			return 0, nil
		}
		return strconv.ParseUint(value, 10, 64)
	case reflect.Bool:
		return strconv.ParseBool(value)
	case reflect.Slice:
		if reflectType.Elem().Kind() != reflect.Uint8 {
			return nil, fmt.Errorf("slice of %s is not supported", reflectType.Elem().Kind().String())
		}
		return []byte(value), nil
	}

	if reflectType == reflect.TypeOf(time.Duration(5)) {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.ParseDuration(value)
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
		path := c.getKeyPath(parent, field)
		switch field.Type.Kind() {
		case reflect.Struct:
			err := c.recursiveReplaceStruct(path, value)
			if err != nil {
				return err
			}
		case reflect.Map:
			if field.Type.Key().Kind() != reflect.String {
				return fmt.Errorf("%s is unsupported map's key type", field.Type.Key().String())
			}
			if field.Type.Elem().Kind() != reflect.String {
				return fmt.Errorf("%s is unsupported map's value type", field.Type.Elem().String())
			}
			_, err := c.Put(path+"/", "")
			if err != nil {
				return err
			}
			for _, k := range value.MapKeys() {
				_, err = c.Put(k.String(), value.MapIndex(k).String())
				if err != nil {
					return err
				}
			}
		default:
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
	return go_case.ToDotSnakeCase(name)
}

func (c *client) getTagOptions(v string) map[string]string {
	res := make(map[string]string)
	if v == "" {
		return res
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
	return res
}

func (c *client) allowOption(name string) bool {
	_, ok := allowOptions[name]
	return ok
}
