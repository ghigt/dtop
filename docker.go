package main

import (
	"log"
	"os"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

func initClient() *docker.Client {
	var client *docker.Client
	var e error

	certs := os.Getenv(*fCerts)
	host := os.Getenv(*fHost)
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	if strings.HasPrefix(host, "unix") {
		client, e = docker.NewClient(host)
		if e != nil {
			log.Fatal(e)
		}
	} else {
		client, e = docker.NewTLSClient(host, certs+"/cert.pem", certs+"/key.pem", "")
		if e != nil {
			log.Fatal(e)
		}
	}

	return client
}
