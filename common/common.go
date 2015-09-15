package common

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/andybons/hipchat"
	"github.com/garyburd/redigo/redis"
)

var RedisURL string

var redisConfigKey = "sup:config"
var redisStatusKey = "sup:status"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type ConfigType struct {
	TwilioSID        string
	TwilioAuthToken  string
	URL              string
	CallFrom         string
	Phones           []string
	HipchatAuthToken string
	HipchatRoom      string
}

type StatusType struct {
	Disabled   bool
	LastStatus int
	LastRunAt  time.Time
	NumErrors  int
}

func HipchatMessage(message string) {
	config := GetConfig()
	if config.HipchatAuthToken == "" || config.HipchatRoom == "" {
		return
	}

	client := hipchat.NewClient(config.HipchatAuthToken)

	err := client.PostMessage(hipchat.MessageRequest{
		RoomId:        config.HipchatRoom,
		From:          "SUP",
		Message:       message,
		Color:         hipchat.ColorRed,
		MessageFormat: hipchat.FormatText,
		Notify:        true,
	})

	check(err)
}

func GetConfig() ConfigType {
	var conf ConfigType

	c, err := getRedis()
	check(err)
	defer c.Close()

	confData, err := redis.Bytes(c.Do("GET", redisConfigKey))
	if err != nil {
		if err.Error() == "redigo: nil returned" {
			confData = []byte("")
		} else {
			check(err)
		}
	}

	json.Unmarshal(confData, &conf)
	return conf
}

func SetConfig(config ConfigType) {
	json, err := json.Marshal(config)
	check(err)

	c, err := getRedis()
	check(err)
	defer c.Close()

	_, err = c.Do("SET", redisConfigKey, json)
	check(err)
}

func GetStatus() StatusType {
	var status StatusType

	c, err := getRedis()
	check(err)
	defer c.Close()

	statusData, err := redis.Bytes(c.Do("GET", redisStatusKey))
	if err != nil {
		if err.Error() == "redigo: nil returned" {
			statusData = []byte("")
		} else {
			check(err)
		}
	}

	json.Unmarshal(statusData, &status)

	return status
}

func SetStatus(status StatusType) {
	json, err := json.Marshal(status)
	check(err)

	c, err := getRedis()
	check(err)
	defer c.Close()

	_, err = c.Do("SET", redisStatusKey, json)
	check(err)
}

func getRedis() (redis.Conn, error) {
	urlParts, err := url.Parse(RedisURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Redis url: %s", err)
	}

	auth := ""
	if urlParts.User != nil {
		if password, ok := urlParts.User.Password(); ok {
			auth = password
		}
	}

	c, err := redis.Dial("tcp", urlParts.Host)
	if err != nil {
		return nil, fmt.Errorf("error connecting to Redis: %s", err)
	}

	if len(auth) > 0 {
		_, err = c.Do("AUTH", auth)
		if err != nil {
			return nil, fmt.Errorf("error authenticating with Redis: %s", err)
		}
	}

	if len(urlParts.Path) > 1 {
		db := strings.TrimPrefix(urlParts.Path, "/")
		_, err = c.Do("SELECT", db)
		if err != nil {
			return nil, fmt.Errorf("error selecting Redis db: %s", err)
		}
	}

	return c, nil
}
