package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"github.com/topscore/sup/Godeps/_workspace/src/golang.org/x/crypto/pbkdf2"
	"io"

	"encoding/json"
	"fmt"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/sfreiberg/gotwilio"
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

func pkcs5pad(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func pkcs5unpad(src []byte) []byte {
	length := len(src)
	unpadding := int(src[length-1])
	return src[:(length - unpadding)]
}

func encrypt(key, plaintext []byte) []byte {
	kdfSalt := make([]byte, 32)
	if _, err := rand.Read(kdfSalt); err != nil {
		panic(err)
	}

	derivedKey := pbkdf2.Key([]byte(key), kdfSalt, 4096, 32, sha256.New)

	plaintext = pkcs5pad(plaintext, aes.BlockSize)
	if len(plaintext)%aes.BlockSize != 0 {
		panic("plaintext is not a multiple of the block size")
	}

	block, err := aes.NewCipher(derivedKey)
	check(err)

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)

	return append(kdfSalt, append(iv, ciphertext...)...)
}

func decrypt(key, ciphertext []byte) []byte {
	if len(ciphertext) < 32+aes.BlockSize {
		panic("ciphertext too short")
	}

	kdfSalt := ciphertext[:32]
	ciphertext = ciphertext[32:]

	derivedKey := pbkdf2.Key([]byte(key), kdfSalt, 4096, 32, sha256.New)

	block, err := aes.NewCipher(derivedKey)
	check(err)

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		panic("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)
	plaintext = pkcs5unpad(plaintext)

	// Its critical to note that ciphertexts must be authenticated (i.e. by
	// using crypto/hmac) before being decrypted in order to avoid creating
	// a padding oracle.

	return plaintext
}

type ConfigType struct {
	TwilioSID       string
	TwilioAuthToken string
	URL             string
	CallFrom        string
	Phones          []string
}

type StatusType struct {
	Disabled   bool
	LastStatus int
	LastRunAt  string
	NumErrors  int
}

func loadConfig(configFile, key string) ConfigType {
	file, err := ioutil.ReadFile(configFile)
	check(err)

	if len(key) > 0 {
		file = decrypt([]byte(key), file)
	}

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
	twilio := gotwilio.NewTwilioClient(config.TwilioSID, config.TwilioAuthToken)
	messageUrl := "http://twimlets.com/message?Message%5B0%5D=SITE%20IS%20DOWN!"
	callbackParams := gotwilio.NewCallbackParameters(messageUrl)

	for _, num := range config.Phones {
		fmt.Printf("!!! Calling %s\n", num)

		_, tException, err := twilio.CallWithUrlCallbacks(config.CallFrom, num, callbackParams)
		if tException != nil {
			fmt.Printf("%+v\n", tException)
			panic(err)
		}
		check(err)
	}
}

func pingSite(c *cli.Context) {
	key := c.GlobalString("key")
	configFile := c.GlobalString("config")
	statusFile := c.GlobalString("status")
	simulateDown := c.GlobalBool("down")

	config := loadConfig(configFile, key)
	status := loadStatus(statusFile)

	if status.Disabled {
		log.Println("Disabled")
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
		fmt.Printf("%+v\n", err)
		fmt.Printf("%+v\n", resp)
		panic(err)
	}
	defer resp.Body.Close()

	if simulateDown || resp.StatusCode != http.StatusOK {
		log.Println("Site is down. Status is ", resp.StatusCode)
		status.NumErrors += 1
		if status.NumErrors >= 5 {
			callDevTeam(config)
		}
	} else {
		log.Println("All is well")
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

				err = ioutil.WriteFile(c.GlobalString("config"), encrypt([]byte(key), plaintext), 0644)
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

				plaintext := decrypt([]byte(key), ciphertext)

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

		if c.GlobalBool("down") {
			log.Println("We're going to pretend the site is down, even if it's not")
		}

		if c.GlobalBool("forever") {
			fmt.Printf("Pinging every %d seconds\n", c.GlobalInt("pingFreq"))
			for {
				pingSite(c)
				time.Sleep(time.Duration(c.GlobalInt("pingFreq")) * time.Second)
			}
		} else {
			fmt.Println("Pinging site")
			pingSite(c)
		}
	}

	app.Run(os.Args)

}
