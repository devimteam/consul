package consul

import (
	"reflect"
	"time"
)

func init() {
	RegisterWellKnowType(reflect.TypeOf(time.Duration(0)), timeDuration)
	RegisterWellKnowType(reflect.TypeOf(time.Time{}), timeTime)
}

func timeTime(_ string, raw []byte) (interface{}, error) {
	return time.Parse(time.RFC3339, string(raw))
}

func timeDuration(_ string, raw []byte) (interface{}, error) {
	return time.ParseDuration(string(raw))
}
