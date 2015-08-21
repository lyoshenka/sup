package main

import (
	"encoding/json"
	"fmt"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/andybons/hipchat"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/sfreiberg/gotwilio"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var config ConfigType
var verbose bool

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
	LastRunAt  string
	NumErrors  int
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

func loadConfig(configFile, key string) ConfigType {
	file, err := ioutil.ReadFile(configFile)
	check(err)

	if len(key) > 0 {
		file, err = decrypt([]byte(key), file)
		check(err)
	}

	var conf ConfigType
	json.Unmarshal(file, &conf)
	return conf
}

func loadStatus(statusFile string) StatusType {
	var status StatusType

	if _, err := os.Stat(statusFile); err == nil {
		file, err := ioutil.ReadFile(statusFile)
		check(err)
		json.Unmarshal(file, &status)
	}

	return status
}

func saveStatus(statusFile string, status StatusType) {
	json, err := json.Marshal(status)
	check(err)
	err = ioutil.WriteFile(statusFile, json, 0644)
	check(err)
}

func callDevTeam() {
	twilio := gotwilio.NewTwilioClient(config.TwilioSID, config.TwilioAuthToken)
	messageUrl := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	callbackParams := gotwilio.NewCallbackParameters(messageUrl)

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
	statusFile := c.GlobalString("status")
	simulateDown := c.GlobalBool("down")

	status := loadStatus(statusFile)

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

	client := &http.Client{}

	req, err := http.NewRequest("GET", config.URL, nil)
	if err != nil {
		log.Fatalln(err)
	}

	req.Close = true
	req.Header.Set("User-Agent", "TS Simple Uptime Checker")

	resp, err := client.Do(req)
	if err != nil && err != io.EOF {
		fmt.Printf("%+v\n", err)
		fmt.Printf("%+v\n", resp)
		panic(err)
	}
	defer resp.Body.Close()

	if simulateDown || resp.StatusCode != http.StatusOK {
		log.Println("Site is down. Status is ", resp.StatusCode)
		status.NumErrors += 1
		if status.NumErrors >= 5 {
			callDevTeam()
		}
	} else {
		if verbose {
			log.Println("All is well")
		}
		status.NumErrors = 0
	}

	status.LastStatus = resp.StatusCode
	status.LastRunAt = time.Now().Format(time.RFC3339)
	saveStatus(statusFile, status)
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
			Name:   "status",
			Value:  "./status.json",
			Usage:  "file where status information is written",
			EnvVar: "STATUS_FILE",
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
				saveStatus(c.GlobalString("status"), *new(StatusType))
			},
		},
		{
			Name:  "enable",
			Usage: "undisable pinger",
			Action: func(c *cli.Context) {
				statusFile := c.GlobalString("status")
				status := loadStatus(statusFile)
				status.Disabled = false
				saveStatus(statusFile, status)
			},
		},
		{
			Name:  "disable",
			Usage: "disable pinger",
			Action: func(c *cli.Context) {
				statusFile := c.GlobalString("status")
				status := loadStatus(statusFile)
				status.Disabled = true
				saveStatus(statusFile, status)
			},
		},
		{
			Name:  "encrypt",
			Usage: "encrypt config file",
			Action: func(c *cli.Context) {
				key := c.GlobalString("key")
				if key == "" {
					log.Println("key required to encrypt config")
					return
				}

				plaintext, err := ioutil.ReadFile(c.GlobalString("config"))
				check(err)

				// It's important to remember that ciphertexts must be authenticated
				// (i.e. by using crypto/hmac) as well as being encrypted in order to
				// be secure.

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
				key := c.GlobalString("key")
				if key == "" {
					log.Println("key required to encrypt config")
					return
				}

				ciphertext, err := ioutil.ReadFile(c.GlobalString("config"))
				check(err)

				plaintext, err := decrypt([]byte(key), ciphertext)
				check(err)

				// Its critical to note that ciphertexts must be authenticated (i.e. by
				// using crypto/hmac) before being decrypted in order to avoid creating
				// a padding oracle.

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

		verbose = c.GlobalBool("verbose")

		config = loadConfig(c.GlobalString("config"), c.GlobalString("key"))

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
