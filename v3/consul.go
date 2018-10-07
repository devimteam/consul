package consul

import consulapi "github.com/hashicorp/consul/api"

type consulKV struct {
	kv *consulapi.KV
}

func (kv consulKV) Get(path string) ([]byte, error) {
	pair, _, err := kv.kv.Get(path, nil)
	if err != nil {
		return nil, err
	}
	if pair == nil {
		return nil, nil
	}
	return pair.Value, nil
}

func (kv consulKV) Put(path string, value []byte) error {
	_, err := kv.kv.Put(&consulapi.KVPair{Key: path, Value: value}, nil)
	return err
}
