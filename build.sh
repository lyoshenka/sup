#!/bin/bash

go-bindata -pkg webserver -prefix webserver/ -o webserver/static.go webserver/templates/
CGO_ENABLED=0 godep go build sup.go -v -a -tags netgo
