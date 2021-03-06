package consul

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/vetcher/go-case"
)

type KV interface {
	Get(path string) ([]byte, error)
	Put(path string, value []byte) error
}

type Updatable interface {
	Update([]byte) error
}

type options struct {
	onlyPull      bool
	disableListen bool
	refreshPeriod time.Duration
	kv            KV
	normalizer    func(string) string
	logger        Logger
}

type Client struct {
	kv   KV
	stop func()
	ctx  context.Context
	opts options

	watch struct {
		list []watchItem
		lock sync.Mutex
	}
}

func NewClient(opts ...Option) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cl := &Client{
		stop: cancel,
		ctx:  ctx,
		opts: options{
			refreshPeriod: time.Minute,
			normalizer:    go_case.ToDotSnakeCase,
		},
	}
	for _, opt := range opts {
		opt(&cl.opts)
	}
	if cl.opts.kv == nil {
		c, err := consulapi.NewClient(consulapi.DefaultConfig())
		if err != nil {
			return nil, err
		}
		cl.kv = consulKV{kv: c.KV()}
	} else {
		cl.kv = cl.opts.kv
	}
	if !cl.opts.disableListen {
		go cl.runWatch()
	}
	return cl, nil
}

func Must(client *Client, err error) *Client {
	if err != nil {
		panic(err)
	}
	return client
}

func (c *Client) PullOrPush(path string, out interface{}) error {
	v := reflect.ValueOf(out)
	if !v.Elem().CanSet() {
		return errors.New("out is not a pointer")
	}
	if err := c.pullOrPush(path, v.Elem(), nil); err != nil {
		return err
	}
	c.updateWatch()
	return nil
}

func (c *Client) Watch(path string, out Updatable) {
	c.registerWatch(path, reflect.ValueOf(out))
}

type CustomParser func(path string, content []byte) (interface{}, error)

var wellKnowTypeParsers = map[reflect.Type]CustomParser{}

func RegisterWellKnownType(t reflect.Type, fn CustomParser) {
	wellKnowTypeParsers[t] = fn
}

var reflectUpdatableInterface = reflect.TypeOf((*Updatable)(nil)).Elem()

func (c *Client) pullOrPush(consulPath string, dst reflect.Value, structTag *reflect.StructField) error {
	if !dst.CanSet() {
		return nil
	}
	content, err := c.kv.Get(consulPath)
	if err != nil {
		return errors.Wrapf(err, "get from '%s'", consulPath)
	}
	if !c.opts.onlyPull && len(content) == 0 {
		if _, ok := wellKnowTypeParsers[dst.Type()]; ok || dst.Kind() != reflect.Struct {
			if structTag != nil {
				opts := makeTagOpts(structTag.Tag.Get("consul"))
				if opts.Default != nil {
					content = []byte(*opts.Default)
				}
			}
			err := c.kv.Put(consulPath, content)
			if err != nil {
				return errors.Wrapf(err, "put to '%s'", consulPath)
			}
		}
	}
	if !c.opts.disableListen {
		c.registerWatch(consulPath, dst)
	}
	if fn, ok := wellKnowTypeParsers[dst.Type()]; ok {
		val, err := fn(consulPath, content)
		if err != nil {
			return errors.Wrapf(err, "custom parser to %s value from path '%s'", dst.Type(), consulPath)
		}
		dst.Set(reflect.ValueOf(val))
		return nil
	}
	switch dst.Kind() {
	case reflect.Struct:
		for i, n := 0, dst.NumField(); i < n; i++ {
			field := dst.Field(i)
			if !field.CanSet() {
				continue
			}
			fieldType := dst.Type().Field(i)
			err := c.pullOrPush(c.makeConsulPath(consulPath, fieldType), field, &fieldType)
			if err != nil {
				return err
			}
		}
	default:
		val, err := c.defaultParser(dst, content)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(val))
		return nil
	}
	return nil
}

func (c *Client) registerWatch(consulPath string, dst reflect.Value) {
	if dst.CanInterface() && dst.Type().Implements(reflectUpdatableInterface) {
		c.watch.lock.Lock()
		c.watch.list = append(c.watch.list, watchItem{path: consulPath, target: dst.Interface().(Updatable)})
		c.watch.lock.Unlock()
	} else if dst.CanAddr() && dst.Addr().Type().Implements(reflectUpdatableInterface) {
		c.watch.lock.Lock()
		c.watch.list = append(c.watch.list, watchItem{path: consulPath, target: dst.Addr().Interface().(Updatable)})
		c.watch.lock.Unlock()
	}
}

func (c *Client) makeConsulPath(pref string, fieldType reflect.StructField) string {
	tagOpts := makeTagOpts(fieldType.Tag.Get("consul"))
	var kName string
	if tagOpts.Name == nil {
		kName = c.opts.normalizer(fieldType.Name)
	} else {
		kName = *tagOpts.Name
	}
	return path.Join(pref, kName)
}

type tagOpts struct {
	Name    *string
	Default *string
}

func makeTagOpts(scope string) tagOpts {
	var tOpts tagOpts
	opts := strings.Split(scope, ";")
	for i := range opts {
		kv := strings.SplitN(opts[i], ":", 2)
		if len(kv) == 0 {
			continue
		}
		switch strings.ToLower(kv[0]) {
		case "default":
			if len(kv) == 1 {
				continue
			}
			s := kv[1]
			tOpts.Default = &s
		case "name":
			if len(kv) == 1 {
				continue
			}
			s := kv[1]
			tOpts.Name = &s
		}
	}
	return tOpts
}

func (c *Client) defaultParser(t reflect.Value, value []byte) (interface{}, error) {
	value = bytes.TrimSpace(value)
	switch t.Kind() {
	case reflect.String:
		return string(value), nil
	case reflect.Float32:
		if len(value) == 0 {
			return float32(0.0), nil
		}
		n, err := strconv.ParseFloat(string(value), 32)
		return float32(n), err
	case reflect.Float64:
		if len(value) == 0 {
			return float64(0.0), nil
		}
		return strconv.ParseFloat(string(value), 64)
	case reflect.Int:
		if len(value) == 0 {
			return int(0), nil
		}
		n, err := strconv.ParseInt(string(value), 10, 64)
		return int(n), err
	case reflect.Int16:
		if len(value) == 0 {
			return int16(0), nil
		}
		n, err := strconv.ParseInt(string(value), 10, 16)
		return int16(n), err
	case reflect.Int32:
		if len(value) == 0 {
			return int32(0), nil
		}
		n, err := strconv.ParseInt(string(value), 10, 32)
		return int32(n), err
	case reflect.Int64:
		if len(value) == 0 {
			return int64(0), nil
		}
		n, err := strconv.ParseInt(string(value), 10, 64)
		return int64(n), err
	case reflect.Uint:
		if len(value) == 0 {
			return uint(0), nil
		}
		n, err := strconv.ParseUint(string(value), 10, 64)
		return uint(n), err
	case reflect.Uint32:
		if len(value) == 0 {
			return uint32(0), nil
		}
		n, err := strconv.ParseUint(string(value), 10, 32)
		return uint32(n), err
	case reflect.Uint64:
		if len(value) == 0 {
			return 0, nil
		}
		return strconv.ParseUint(string(value), 10, 64)
	case reflect.Bool:
		return strconv.ParseBool(string(value))
	case reflect.Slice:
		if t.Elem().Kind() != reflect.Uint8 {
			return nil, fmt.Errorf("[]%s is not supported", t.Elem().Kind())
		}
		return []byte(value), nil
	default:
		return nil, errors.Errorf("can not find parser for %s", t.Type())
	}
}

func (c *Client) Stop() {
	c.stop()
}

func (c *Client) runWatch() {
	timer := time.NewTimer(c.opts.refreshPeriod)
	timer.Stop()
	defer timer.Stop()
	for {
		timer.Reset(c.opts.refreshPeriod)
		select {
		case <-timer.C:
			c.updateWatch()
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Client) updateWatch() {
	c.watch.lock.Lock()
	for _, item := range c.watch.list {
		raw, err := c.kv.Get(item.path)
		if err != nil {
			_ = c.opts.logger.Log("path", item.path, "error", err)
			continue
		}
		if err := item.target.Update(raw); err != nil {
			_ = c.opts.logger.Log("path", item.path, "error", err)
		}
	}
	c.watch.lock.Unlock()
}

type watchItem struct {
	path   string
	target Updatable
}
