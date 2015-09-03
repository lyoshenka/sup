package webserver

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/topscore/sup/common"

	"github.com/topscore/sup/Godeps/_workspace/src/github.com/zenazn/goji"
	"github.com/topscore/sup/Godeps/_workspace/src/github.com/zenazn/goji/web"
)

func homeRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	if status.Disabled {
		fmt.Fprintln(w, "<h1>sup is disabled</h1>")
	} else {
		fmt.Fprintln(w, "<h1>sup is running</h1>")
	}

	fmt.Fprintln(w, "<h3>last ping at "+status.LastRunAt+"</h3>")
	fmt.Fprintln(w, "<h3>last status "+strconv.Itoa(status.LastStatus)+"</h3>")
}

func statusRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	status := common.GetStatus()
	encoder := json.NewEncoder(w)
	encoder.Encode(status)
}

func robotsRoute(c web.C, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "User-agent: *\nDisallow: /")
}

func StartWebServer(bind string) error {
	goji.Get("/", homeRoute)
	goji.Get("/status", statusRoute)
	goji.Get("/robots.txt", robotsRoute)

	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	goji.ServeListener(listener)
	return nil
}
