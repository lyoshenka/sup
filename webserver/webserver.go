package webserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/topscore/sup/common"

	"github.com/topscore/sup/Godeps/_workspace/src/github.com/goji/httpauth"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/zenazn/goji"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/zenazn/goji/web"
)

var templates *template.Template

func loadTemplates() error {
	var tmpl []string

	_, filename, _, _ := runtime.Caller(1)

	fn := func(path string, f os.FileInfo, err error) error {
		if f.IsDir() != true && strings.HasSuffix(f.Name(), ".html") {
			tmpl = append(tmpl, path)
		}
		return nil
	}

	err := filepath.Walk(filepath.Dir(filename)+"/templates", fn)

	if err != nil {
		return err
	}

	templates = template.Must(template.ParseFiles(tmpl...))
	return nil
}

func getTemplate(name string, data interface{}) string {
	var doc bytes.Buffer
	templates.ExecuteTemplate(&doc, name, data)
	return doc.String()
}

func homeRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	templateArgs := map[string]interface{}{
		"enabled":      !status.Disabled,
		"lastPingTime": status.LastRunAt,
		"lastStatus":   strconv.Itoa(status.LastStatus),
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
