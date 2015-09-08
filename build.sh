#!/bin/bash

#GO15VENDOREXPERIMENT=1
#go-bindata -pkg webserver -prefix webserver/ -o webserver/static.go webserver/templates/
go generate $(go list ./... | grep -v /vendor/)
#CGO_ENABLED=0 godep go build -a -v -tags netgo sup.go
CGO_ENABLED=0 godep go build -v -tags netgo sup.go
