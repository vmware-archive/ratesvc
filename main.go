/*
Copyright (c) 2017 Bitnami

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/heptiolabs/healthcheck"
	"github.com/kubeapps/common/datastore"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/negroni"
)

var dbSession datastore.Session

func main() {
	dbHost := flag.String("mongo-host", "localhost:27017", "MongoDB host")
	dbName := flag.String("mongo-database", "ratesvc", "MongoDB database")
	flag.Parse()

	mongoConfig := datastore.Config{Host: *dbHost, Database: *dbName}
	var err error
	dbSession, err = datastore.NewSession(mongoConfig)
	if err != nil {
		log.WithFields(log.Fields{"host": *dbHost}).Fatal(err)
	}

	r := mux.NewRouter()

	// Healthcheck
	health := healthcheck.NewHandler()
	r.Handle("/live", health)
	r.Handle("/ready", health)

	// Routes
	apiv1 := r.PathPrefix("/v1").Subrouter()
	apiv1.Methods("GET").Path("/stars").HandlerFunc(GetStars)
	apiv1.Methods("PUT").Path("/stars").HandlerFunc(UpdateStar)
	apiv1.Methods("GET").Path("/comments/{itemID}").HandlerFunc(GetComments)
	apiv1.Methods("POST").Path("/comments/{repo}/{chartName}").HandlerFunc(CreateComment)

	n := negroni.Classic()
	n.UseHandler(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.WithFields(log.Fields{"addr": addr}).Info("Started RateSvc")
	http.ListenAndServe(addr, n)
}
