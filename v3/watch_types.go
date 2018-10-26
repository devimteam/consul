package consul

import (
	"reflect"
	"strconv"
	"sync/atomic"
	"time"
)

func init() {
	RegisterWellKnowType(reflect.TypeOf(String{}), watchableString)
	RegisterWellKnowType(reflect.TypeOf(Duration{}), watchableDuration)
	RegisterWellKnowType(reflect.TypeOf(Int{}), watchableInt)
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
