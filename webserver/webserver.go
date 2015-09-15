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
	baseTemplate "html/template"

	"github.com/goji/httpauth"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var templates *template.Template

func loadTemplates() error {
	t, err := template.New("mytmpl", Asset).Funcs(template.FuncMap{
		"safehtml": func(value interface{}) baseTemplate.HTML {
			return baseTemplate.HTML(fmt.Sprint(value))
		},
	}).ParseFiles(AssetNames()...)

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
	config := common.GetConfig()
	templateArgs := map[string]interface{}{
		"url":          config.URL,
		"enabled":      !status.Disabled,
		"lastPingTime": status.LastRunAt.Format("2006-01-02 15:04:05 MST"),
		"lastStatus":   status.LastStatus,
		"numContacts":  len(config.Phones),
	}
	fmt.Fprintln(w, getTemplate("home", templateArgs))
}

func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil

}

func configRoute(c web.C, w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		r.ParseForm()
		confData := r.FormValue("configData")

		if !isJSON(confData) {
			http.Error(w, "Invalid json", 500)
			return
		}

		var newConf common.ConfigType
		json.Unmarshal([]byte(confData), &newConf)
		common.SetConfig(newConf)

		http.Redirect(w, r, "/config?success=Saved", http.StatusFound)
	}

	conf := common.GetConfig()
	json, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	templateArgs := map[string]interface{}{
		"configData": strings.TrimSpace(string(json)),
		"success":    r.URL.Query().Get("success"),
	}
	fmt.Fprintln(w, getTemplate("config", templateArgs))
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
	goji.Handle("/config", configRoute)

	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	goji.ServeListener(listener)
	return nil
}
