package testutil

import (
	"github.com/devimteam/consul"
	consulapi "github.com/hashicorp/consul/api"
)

func defaultServerConfig() *consulapi.Config {
	return consulapi.DefaultConfig()
}

func NewClient() (consul.Client, error) {
	c, err := consulapi.NewClient(defaultServerConfig())
	if err != nil {
		return nil, err
	}
	return consul.NewClientWithConsulClient(c), nil
}
