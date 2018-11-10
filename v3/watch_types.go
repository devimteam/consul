package consul

import (
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml"
)

func init() {
	RegisterWellKnowType(reflect.TypeOf(String{}), watchableString)
	RegisterWellKnowType(reflect.TypeOf(Duration{}), watchableDuration)
	RegisterWellKnowType(reflect.TypeOf(Int{}), watchableInt)
	RegisterWellKnowType(reflect.TypeOf(Toml{}), tomlConfig)
}

type String struct {
	v atomic.Value
}

func (s *String) Update(raw []byte) {
	s.v.Store(string(raw))
}

func (s *String) String() string {
	return s.v.Load().(string)
}

func watchableString(_ string, raw []byte) (interface{}, error) {
	s := String{}
	s.Update(raw)
	return s, nil
}

type Duration struct {
	v atomic.Value
}

func (d *Duration) Update(raw []byte) {
	dur, err := time.ParseDuration(string(raw))
	if err != nil {
		return
	}
	d.v.Store(dur)
}

func (d Duration) Duration() time.Duration {
	return d.v.Load().(time.Duration)
}

func watchableDuration(_ string, raw []byte) (interface{}, error) {
	d := Duration{}
	d.Update(raw)
	return d, nil
}

type Int struct {
	v atomic.Value
}

func (d *Int) Update(raw []byte) {
	i, err := strconv.Atoi(string(raw))
	if err != nil {
		return
	}
	d.v.Store(i)
}

func (d Int) Int() int {
	return d.v.Load().(int)
}

func watchableInt(_ string, raw []byte) (interface{}, error) {
	d := Int{}
	d.Update(raw)
	return d, nil
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

func (t *Toml) Update(raw []byte) {
	_ = t.update(raw)
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
	return t.v.Load().(*toml.Tree)
}
