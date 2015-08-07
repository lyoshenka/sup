package main

import (
	"encoding/json"
	"fmt"
	// "github.com/sfreiberg/gotwilio"
	"github.com/codegangsta/cli"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type ConfigType struct {
	TwilioSID       string
	TwilioAuthToken string
	URL             string
	Phones          []string
}

type StatusType struct {
	Disabled   bool
	LastStatus int
	LastRunAt  string
	NumErrors  int
}

func loadConfig(configFile string) ConfigType {
	file, err := ioutil.ReadFile(configFile)
	check(err)

	var config ConfigType
	json.Unmarshal(file, &config)
	return config
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

func callDevTeam(config ConfigType) {
	// twilio := gotwilio.NewTwilioClient(config.TwilioSID, config.TwilioAuthToken)
	// messageUrl := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	// callbackParams := gotwilio.NewCallbackParameters(messageUrl)

	for _, num := range config.Phones {
		fmt.Printf("!!! Calling %s\n", num)

		// _, tException, tErr := twilio.CallWithUrlCallbacks("+14804184746", num, callbackParams)
		// if tException != nil {
		// 	log.Fatalln(tException)
		// }
		// if tErr != nil {
		// 	log.Fatalln(tErr)
		// }
	}
}

func pingSite(c *cli.Context) {
	configFile := c.String("config")
	statusFile := c.String("status")
	simulateDown := c.Bool("down")

	if simulateDown {
		fmt.Println("We're going to pretend the site is down, even if it's not")
	}

	config := loadConfig(configFile)
	status := loadStatus(statusFile)

	if status.Disabled {
		fmt.Println("Disabled")
		return
	}

	client := &http.Client{}

	req, err := http.NewRequest("GET", config.URL, nil)
	if err != nil {
		log.Fatalln(err)
	}

	req.Header.Set("User-Agent", "TS Simple Uptime Checker")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("error")
		fmt.Printf("%+v\n", err)
		fmt.Printf("%+v\n", resp)
	}

	if simulateDown || resp.StatusCode != http.StatusOK {
		fmt.Println("Site is down. Status is ", resp.StatusCode)
		status.NumErrors += 1
		if status.NumErrors >= 5 {
			callDevTeam(config)
		}
	} else {
		fmt.Println("All is well")
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
		cli.BoolFlag{
			Name:  "down",
			Usage: "simulate the site being down",
		},
		cli.BoolFlag{
			Name:  "forever",
			Usage: "run ping on repeate",
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
	}

	app.Action = func(c *cli.Context) {
		if c.Bool("forever") {
			for {
				pingSite(c)
				time.Sleep(10 * time.Second)
			}
		} else {
			pingSite(c)
		}
	}

	app.Run(os.Args)

}
