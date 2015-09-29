package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/topscore/sup/common"
	"github.com/topscore/sup/webserver"

	"github.com/codegangsta/cli"
	"github.com/sfreiberg/gotwilio"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func callDevTeam() {
	config := common.GetConfig()
	twilio := gotwilio.NewTwilioClient(config.TwilioSID, config.TwilioAuthToken)
	messageURL := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	callbackParams := gotwilio.NewCallbackParameters(messageURL)

	for _, num := range config.Phones {
		fmt.Printf("!!! Calling %s\n", num)

		_, tException, err := twilio.CallWithUrlCallbacks(config.TwilioCallFrom, num, callbackParams)
		if tException != nil {
			panic(fmt.Sprintf("Twilio error: %+v\n", tException))
		}
		check(err)
	}
}

func pingSite(c *cli.Context) {
	simulateDown := c.GlobalBool("down")

	config := common.GetConfig()
	status := common.GetStatus()

	if config.URL == "" {
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
		if status.NumErrors >= 5 && !status.Disabled {
			callDevTeam()
		}
	} else {
		status.NumErrors = 0
	}

	status.LastStatus = statusCode
	status.LastRunAt = time.Now()
	common.SetStatus(status)
}

func main() {

	app := cli.NewApp()
	app.Name = "sup"
	app.Usage = "check if site is up"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "logfile",
			Value: "",
			Usage: "log messages go here. if not present, log to stdout",
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

	app.Action = func(c *cli.Context) {
		if c.GlobalString("logfile") != "" {
			logfile, err := os.OpenFile(c.GlobalString("logfile"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			check(err)
			defer logfile.Close()
			log.SetOutput(logfile)
		}

		common.RedisURL = c.GlobalString("redis_url")

		config := common.GetConfig()

		if c.GlobalBool("down") {
			log.Println("We're going to pretend the site is down, even if it's not")
		}

		if c.GlobalBool("web") {
			err := webserver.StartWebServer(":"+strconv.Itoa(c.GlobalInt("port")), c.GlobalString("web_auth"))
			if err != nil {
				log.Fatal(err)
			}
		} else if c.GlobalBool("forever") {
			fmt.Printf("Pinging %s every %d seconds\n", config.URL, c.GlobalInt("pingFreq"))
			for {
				pingSite(c)
				pingFreq := config.PingFreq
				if pingFreq < 60 {
					log.Println("pingFreq is to low, bumping it up to 60")
					pingFreq = 60
				}
				time.Sleep(time.Duration(pingFreq) * time.Second)
			}
		} else {
			fmt.Printf("Pinging %s\n", config.URL)
			pingSite(c)
		}
	}

	app.Run(os.Args)

}
