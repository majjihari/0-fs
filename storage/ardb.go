package storage

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/garyburd/redigo/redis"
)

func NewARDBStorage(urls ...*url.URL) (Storage, error) {
	var pools []*redis.Pool
	for _, u := range urls {
		pool := &redis.Pool{
			Dial: func() (redis.Conn, error) {
				var opts []redis.DialOption
				if u.User != nil {
					//assume ardb://password@host.com:port/
					opts = append(opts, redis.DialPassword(u.User.Username()))
				}

				return redis.Dial("tcp", u.Host, opts...)
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
			MaxActive:   10,
			IdleTimeout: 1 * time.Minute,
			Wait:        true,
		}

		//TODO: Add check back
		// c := pool.Get()
		// defer c.Close()
		// if _, err := c.Do("PING"); err != nil {
		// 	return nil, err
		// }
		pools = append(pools, pool)
	}

	return &ardbStor{
		pools: pools,
	}, nil
}

type ardbStor struct {
	pools []*redis.Pool
}

func (s *ardbStor) get(pool *redis.Pool, key string) (io.ReadCloser, error) {
	cl := pool.Get()
	defer cl.Close()

	data, err := redis.Bytes(cl.Do("GET", key))
	if err != nil {
		return nil, err
	}
	//TODO CRC check
	if len(data) <= 16 {
		return nil, fmt.Errorf("wrong data size")
	}

	return ioutil.NopCloser(bytes.NewBuffer(data[16:])), nil
}

func (s *ardbStor) Get(key string) (io.ReadCloser, error) {
	for _, pool := range s.pools {
		data, err := s.get(pool, key)
		if err == redis.ErrNil {
			continue
		} else if err != nil {
			return nil, err
		}

		return data, nil
	}
	return nil, fmt.Errorf("block '%s' not found in all pools", key)
}
