package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/andybons/hipchat"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/garyburd/redigo/redis"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/sfreiberg/gotwilio"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var redisURL string
var config configType
var verbose bool

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type configType struct {
	TwilioSID        string
	TwilioAuthToken  string
	URL              string
	CallFrom         string
	Phones           []string
	HipchatAuthToken string
	HipchatRoom      string
}

type statusType struct {
	Disabled   bool
	LastStatus int
	LastRunAt  string
	NumErrors  int
}

func getConfigKey(c *cli.Context) ([]byte, error) {
	key := c.GlobalString("key")
	if key != "" {
		return []byte(key), nil
	}

	keyFile := c.GlobalString("keyfile")
	if keyFile != "" {
		key, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		return bytes.TrimSpace(key), nil
	}

	return nil, nil
}

func hipchatMessage(message string) {
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

func loadConfig(configFile string, key []byte) configType {
	file, err := ioutil.ReadFile(configFile)
	check(err)

	if key != nil && len(key) > 0 {
		file, err = decrypt(key, file)
		check(err)
	}

	var conf configType
	json.Unmarshal(file, &conf)
	return conf
}

func getRedis() (redis.Conn, error) {
	urlParts, err := url.Parse(redisURL)
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

func loadStatus() statusType {
	var status statusType

	c, err := getRedis()
	check(err)
	defer c.Close()

	statusData, err := redis.Bytes(c.Do("GET", "sup:status"))
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

func saveStatus(status statusType) {
	json, err := json.Marshal(status)
	check(err)

	c, err := getRedis()
	check(err)
	defer c.Close()

	_, err = c.Do("SET", "sup:status", json)
	check(err)
}

func callDevTeam() {
	twilio := gotwilio.NewTwilioClient(config.TwilioSID, config.TwilioAuthToken)
	messageURL := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	callbackParams := gotwilio.NewCallbackParameters(messageURL)

	for _, num := range config.Phones {
		fmt.Printf("!!! Calling %s\n", num)

		_, tException, err := twilio.CallWithUrlCallbacks(config.CallFrom, num, callbackParams)
		if tException != nil {
			panic(fmt.Sprintf("Twilio error: %+v\n", tException))
		}
		check(err)
	}
}

func pingSite(c *cli.Context) {
	simulateDown := c.GlobalBool("down")

	status := loadStatus()

	if status.Disabled {
		log.Println("Disabled")
		return
	}

	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case error:
				hipchatMessage(x.Error())
			default:
				hipchatMessage(fmt.Sprintf("%v", x))
			}
			panic(e)
		}
	}()

	client := &http.Client{
		Timeout: time.Duration(20 * time.Second),
	}

	req, err := http.NewRequest("GET", config.URL, nil)
	if err != nil {
		log.Fatalln(err)
	}

	req.Close = true
	req.Header.Set("User-Agent", "TS Simple Uptime Checker")

	isError := false
	statusCode := 0

	resp, err := client.Do(req)
	if err != nil && err != io.EOF {
		fmt.Printf("err: %+v\n", err)
		fmt.Printf("resp: %+v\n", resp)
		isError = true
	}
	if resp != nil {
		defer resp.Body.Close()
		statusCode = resp.StatusCode
	}

	if simulateDown || isError || statusCode != http.StatusOK {
		log.Println("Site is down. Status is ", statusCode)
		status.NumErrors++
		if status.NumErrors >= 5 {
			callDevTeam()
		}
	} else {
		if verbose {
			log.Println("All is well")
		}
		status.NumErrors = 0
	}

	status.LastStatus = statusCode
	status.LastRunAt = time.Now().Format(time.RFC3339)
	saveStatus(status)
}

func main() {

	app := cli.NewApp()
	app.Name = "sup"
	app.Usage = "check if site is up"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: "./config.json",
			Usage: "file where config values are read from",
		},
		cli.StringFlag{
			Name:  "logfile",
			Value: "",
			Usage: "log messages go here. if not present, log to stdout",
		},
		cli.StringFlag{
			Name:   "key",
			Value:  "",
			Usage:  "key to lock/unlock config file",
			EnvVar: "CONFIG_KEY",
		},
		cli.StringFlag{
			Name:   "redis_url",
			Value:  "redis://localhost:6379",
			Usage:  "redis url",
			EnvVar: "REDIS_URL",
		},
		cli.StringFlag{
			Name:  "keyfile",
			Value: "",
			Usage: "file that contains key to lock/unlock config file",
		},
		cli.BoolFlag{
			Name:  "down",
			Usage: "simulate the site being down",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "verbose output",
		},
		cli.BoolFlag{
			Name:  "forever",
			Usage: "run ping on repeat",
		},
		cli.IntFlag{
			Name:  "pingFreq",
			Usage: "with --forever, how often to ping the site (in seconds)",
			Value: 60,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "reset",
			Usage: "reset status file",
			Action: func(c *cli.Context) {
				saveStatus(*new(statusType))
			},
		},
		{
			Name:  "enable",
			Usage: "undisable pinger",
			Action: func(c *cli.Context) {
				status := loadStatus()
				status.Disabled = false
				saveStatus(status)
			},
		},
		{
			Name:  "disable",
			Usage: "disable pinger",
			Action: func(c *cli.Context) {
				status := loadStatus()
				status.Disabled = true
				saveStatus(status)
			},
		},
		{
			Name:  "encrypt",
			Usage: "encrypt config file",
			Action: func(c *cli.Context) {
				key, err := getConfigKey(c)
				check(err)
				if key == nil {
					log.Println("key required to encrypt config")
					return
				}

				plaintext, err := ioutil.ReadFile(c.GlobalString("config"))
				check(err)

				ciphertext, err := encrypt([]byte(key), plaintext)
				check(err)

				err = ioutil.WriteFile(c.GlobalString("config"), ciphertext, 0644)
				check(err)
			},
		},
		{
			Name:  "decrypt",
			Usage: "decrypt config file",
			Action: func(c *cli.Context) {
				key, err := getConfigKey(c)
				check(err)
				if key == nil {
					log.Println("key required to decrypt config")
					return
				}

				ciphertext, err := ioutil.ReadFile(c.GlobalString("config"))
				check(err)

				plaintext, err := decrypt([]byte(key), ciphertext)
				check(err)

				err = ioutil.WriteFile(c.GlobalString("config"), plaintext, 0644)
				check(err)
			},
		},
	}

	app.Action = func(c *cli.Context) {
		if c.GlobalString("logfile") != "" {
			logfile, err := os.OpenFile(c.GlobalString("logfile"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			check(err)
			defer logfile.Close()
			log.SetOutput(logfile)
		}

		redisURL = c.GlobalString("redis_url")
		verbose = c.GlobalBool("verbose")

		key, err := getConfigKey(c)
		check(err)

		config = loadConfig(c.GlobalString("config"), key)

		if config.URL == "" {
			fmt.Println("No URL in config file. Either you forgot to set one, or you need to provide a decryption key.")
			os.Exit(1)
		}

		if c.GlobalBool("down") {
			log.Println("We're going to pretend the site is down, even if it's not")
		}

		if c.GlobalBool("forever") {
			fmt.Printf("Pinging %s every %d seconds\n", config.URL, c.GlobalInt("pingFreq"))
			for {
				pingSite(c)
				time.Sleep(time.Duration(c.GlobalInt("pingFreq")) * time.Second)
			}
		} else {
			// config.URL = "http://httpstat.us/504"
			fmt.Printf("Pinging %s\n", config.URL)
			pingSite(c)
		}
	}

	app.Run(os.Args)

}
