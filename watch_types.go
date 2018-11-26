package consul

import (
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml"
)

func init() {
	RegisterWellKnownType(reflect.TypeOf(String{}), watchableString)
	RegisterWellKnownType(reflect.TypeOf(Duration{}), watchableDuration)
	RegisterWellKnownType(reflect.TypeOf(Int{}), watchableInt)
	RegisterWellKnownType(reflect.TypeOf(Toml{}), tomlConfig)
}

type String struct {
	v atomic.Value
}

func (s *String) Update(raw []byte) error {
	s.v.Store(string(raw))
	return nil
}

func (s *String) String() string {
	str, _ := s.v.Load().(string)
	return str
}

func watchableString(_ string, raw []byte) (interface{}, error) {
	s := String{}
	return s, s.Update(raw)
}

type Duration struct {
	v atomic.Value
}

func (d *Duration) Update(raw []byte) error {
	dur, err := time.ParseDuration(string(raw))
	if err != nil {
		return err
	}
	d.v.Store(dur)
	return nil
}

func (d Duration) Duration() time.Duration {
	dur, _ := d.v.Load().(time.Duration)
	return dur
}

func watchableDuration(_ string, raw []byte) (interface{}, error) {
	d := Duration{}
	return d, d.Update(raw)
}

type Int struct {
	v atomic.Value
}

func (d *Int) Update(raw []byte) error {
	i, err := strconv.Atoi(string(raw))
	if err != nil {
		return err
	}
	d.v.Store(i)
	return nil
}

func (d Int) Int() int {
	i, _ := d.v.Load().(int)
	return i
}

func watchableInt(_ string, raw []byte) (interface{}, error) {
	d := Int{}
	return d, d.Update(raw)
}

type Toml struct {
	v atomic.Value
}

func tomlConfig(_ string, raw []byte) (interface{}, error) {
	t := Toml{}
	if err := t.update(raw); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Toml) Update(raw []byte) error {
	return t.update(raw)
}

func (t *Toml) update(raw []byte) error {
	tree, err := toml.LoadBytes(raw)
	if err != nil {
		return err
	}
	t.v.Store(tree)
	return nil
}

func (t Toml) Tree() *toml.Tree {
	tree, _ := t.v.Load().(*toml.Tree)
	return tree
}
