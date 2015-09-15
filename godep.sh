#!/bin/bash

go get
GO15VENDOREXPERIMENT=1 godep save ./...
#GO15VENDOREXPERIMENT=1 godep update PACKAGE
