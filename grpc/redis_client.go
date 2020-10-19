package grpc

import (
	context "context"
	redis "github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func (c *RedisClient) IsConnEstablished() bool {
	return c.client != nil
}

// Connects to a Redis server at |addr| (e.g. "127.0.0.1:6379").
func (c *RedisClient) EstablishConnection(addr string, pass string, db int) error {
	c.client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
	})
	c.ctx = context.Background()
	return nil
}

// Closes the connection and resets the client to nil.
func (c *RedisClient) CloseConnection() error {
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}
