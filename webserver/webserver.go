package webserver

//go:generate go-bindata -pkg $GOPACKAGE -o static.go templates/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/topscore/sup/common"

	"github.com/lyoshenka/go-bindata-html-template"

	"github.com/goji/httpauth"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var templates *template.Template

func loadTemplates() error {
	t, err := template.New("mytmpl", Asset).ParseFiles(AssetNames()...)
	if err == nil {
		templates = t
	}
	return err
}

func getTemplate(name string, data interface{}) string {
	var doc bytes.Buffer
	templates.ExecuteTemplate(&doc, name, data)
	return doc.String()
}

func homeRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	templateArgs := map[string]interface{}{
		"url":          common.Config.URL,
		"enabled":      !status.Disabled,
		"lastPingTime": status.LastRunAt.Format("2006-01-02 15:04:05 MST"),
		"lastStatus":   status.LastStatus,
	}
	fmt.Fprintln(w, getTemplate("home", templateArgs))
}

func statusRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	encoder := json.NewEncoder(w)
	encoder.Encode(status)
}

func robotsRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "User-agent: *\nDisallow: /")
}

func toggleEnabledRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	status.Disabled = !status.Disabled
	log.Printf("setting disabled to %t\n", status.Disabled)
	common.SetStatus(status)
	http.Redirect(w, r, "/", http.StatusFound)
}

func StartWebServer(bind, auth string) error {
	err := loadTemplates()
	if err != nil {
		return err
	}

	if auth != "" {
		authParts := strings.Split(auth, ":")
		goji.Use(httpauth.SimpleBasicAuth(authParts[0], authParts[1]))
	}

	goji.Get("/", homeRoute)
	goji.Get("/status", statusRoute)
	goji.Get("/robots.txt", robotsRoute)
	goji.Get("/toggleEnabled", toggleEnabledRoute)

	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	goji.ServeListener(listener)
	return nil
}
