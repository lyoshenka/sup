package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/topscore/sup/common"
	"github.com/topscore/sup/crypt"
	"github.com/topscore/sup/webserver"

	"github.com/topscore/sup/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/sfreiberg/gotwilio"
)

var verbose bool

func check(e error) {
	if e != nil {
		panic(e)
	}
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

func callDevTeam() {
	twilio := gotwilio.NewTwilioClient(common.Config.TwilioSID, common.Config.TwilioAuthToken)
	messageURL := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	callbackParams := gotwilio.NewCallbackParameters(messageURL)

	for _, num := range common.Config.Phones {
		fmt.Printf("!!! Calling %s\n", num)

		_, tException, err := twilio.CallWithUrlCallbacks(common.Config.CallFrom, num, callbackParams)
		if tException != nil {
			panic(fmt.Sprintf("Twilio error: %+v\n", tException))
		}
		check(err)
	}
}

func pingSite(c *cli.Context) {
	simulateDown := c.GlobalBool("down")

	status := common.GetStatus()

	if status.Disabled {
		log.Println("Disabled")
		return
	}

	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case error:
				common.HipchatMessage(x.Error())
			default:
				common.HipchatMessage(fmt.Sprintf("%v", x))
			}
			panic(e)
		}
	}()

	client := &http.Client{
		Timeout: time.Duration(20 * time.Second),
	}

	req, err := http.NewRequest("GET", common.Config.URL, nil)
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
	common.SetStatus(status)
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
			Name:  "keyfile",
			Value: "",
			Usage: "file that contains key to lock/unlock config file",
		},
		cli.StringFlag{
			Name:   "redis_url",
			Value:  "redis://localhost:6379",
			Usage:  "redis url",
			EnvVar: "REDIS_URL",
		},

		cli.BoolFlag{
			Name:  "down",
			Usage: "simulate the site being down",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "verbose output",
		},
		cli.IntFlag{
			Name:  "pingFreq",
			Usage: "with --forever, how often to ping the site (in seconds)",
			Value: 60,
		},

		cli.IntFlag{
			Name:   "port",
			Usage:  "web server will listen on this port",
			EnvVar: "PORT",
			Value:  8000,
		},
		cli.StringFlag{
			Name:   "web_auth",
			Value:  "",
			Usage:  "basic http auth for web server. format: username:password",
			EnvVar: "WEB_AUTH",
		},

		cli.BoolFlag{
			Name:  "forever",
			Usage: "run ping on repeat",
		},
		cli.BoolFlag{
			Name:  "web",
			Usage: "run web server",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "reset",
			Usage: "reset status file",
			Action: func(c *cli.Context) {
				common.SetStatus(*new(common.StatusType))
			},
		},
		{
			Name:  "enable",
			Usage: "undisable pinger",
			Action: func(c *cli.Context) {
				status := common.GetStatus()
				status.Disabled = false
				common.SetStatus(status)
			},
		},
		{
			Name:  "disable",
			Usage: "disable pinger",
			Action: func(c *cli.Context) {
				status := common.GetStatus()
				status.Disabled = true
				common.SetStatus(status)
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

				ciphertext, err := crypt.Encrypt([]byte(key), plaintext)
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

				plaintext, err := crypt.Decrypt([]byte(key), ciphertext)
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

		common.RedisURL = c.GlobalString("redis_url")
		verbose = c.GlobalBool("verbose")

		key, err := getConfigKey(c)
		check(err)

		common.Config = common.LoadConfig(c.GlobalString("config"), key)

		if common.Config.URL == "" {
			fmt.Println("No URL in config file. Either you forgot to set one, or you need to provide a decryption key.")
			os.Exit(1)
		}

		if c.GlobalBool("down") {
			log.Println("We're going to pretend the site is down, even if it's not")
		}

		if c.GlobalBool("web") {
			err = webserver.StartWebServer(":"+strconv.Itoa(c.GlobalInt("port")), c.GlobalString("web_auth"))
			if err != nil {
				log.Fatal(err)
			}
		} else if c.GlobalBool("forever") {
			fmt.Printf("Pinging %s every %d seconds\n", common.Config.URL, c.GlobalInt("pingFreq"))
			for {
				pingSite(c)
				time.Sleep(time.Duration(c.GlobalInt("pingFreq")) * time.Second)
			}
		} else {
			// common.Config.URL = "http://httpstat.us/504"
			fmt.Printf("Pinging %s\n", common.Config.URL)
			pingSite(c)
		}
	}

	app.Run(os.Args)

}
